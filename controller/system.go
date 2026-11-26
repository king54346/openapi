package controller

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"openapi/common"
	"openapi/model"
	httpclient "openapi/service/http"
	"openapi/setting"
	"strings"
	"sync"
	"time"

	gin "github.com/king54346/gin-tiny"
)

// 系统公开状态与初始化接口。
// 计费、订阅、双因素认证等能力未在本项目实现，相关开关固定返回 false。

const (
	// uptimeKumaTimeout Uptime Kuma 状态拉取超时
	uptimeKumaTimeout = 10 * time.Second
	// uptimeKumaCacheTTL 状态缓存时间，避免频繁打上游
	uptimeKumaCacheTTL = 30 * time.Second
	// maxUptimeKumaResponseBytes 响应体读取上限
	maxUptimeKumaResponseBytes = 2 << 20
)

// GetSetup 查询系统是否已完成初始化（前端据此决定是否跳转到初始化页）。
func GetSetup(c gin.Context) {
	initialized, err := model.IsSetupInitialized()
	if err != nil {
		apiError(c, err)
		return
	}
	rootExists, err := model.HasRootUser()
	if err != nil {
		apiError(c, err)
		return
	}
	apiSuccess(c, gin.H{
		"initialized": initialized,
		"root_user":   rootExists,
		"version":     common.Version,
	})
}

// PostSetup 首次初始化：创建 root 账号。已初始化时拒绝。
func PostSetup(c gin.Context) {
	var req struct {
		Username        string `json:"username"`
		Password        string `json:"password"`
		ConfirmPassword string `json:"confirm_password"`
		DisplayName     string `json:"display_name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		apiErrorMsg(c, "invalid request: "+err.Error())
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if strings.TrimSpace(req.Username) == "" {
		apiErrorMsg(c, "username is required")
		return
	}
	if err := validateUsername(req.Username); err != nil {
		apiErrorMsg(c, err.Error())
		return
	}
	if err := validatePassword(req.Password); err != nil {
		apiErrorMsg(c, err.Error())
		return
	}
	if req.ConfirmPassword != "" && req.ConfirmPassword != req.Password {
		apiErrorMsg(c, "passwords do not match")
		return
	}
	root, err := model.CreateRootUserAndMarkSetup(req.Username, req.Password, strings.TrimSpace(req.DisplayName))
	if err != nil {
		if err == model.ErrAlreadyInitialized || err == model.ErrUsernameTaken {
			apiErrorMsg(c, err.Error())
			return
		}
		apiError(c, err)
		return
	}
	apiSuccess(c, gin.H{"id": root.Id, "username": root.Username, "role": root.Role})
}

// GetStatus 公开的系统状态：版本、启动时间、可选的站点密钥等。
func GetStatus(c gin.Context) {
	settings := setting.GetGlobalSettings()
	initialized, err := model.IsSetupInitialized()
	if err != nil {
		common.SysError("failed to load setup state: " + err.Error())
	}

	// 公开字段里不能出现任何密钥（Turnstile 密钥、支付密钥等），只暴露 site key
	apiSuccess(c, gin.H{
		"version":               common.Version,
		"start_time":            common.StartTime,
		"setup":                 initialized,
		"server_address":        settings.ServerAddress,
		"docs_link":             settings.DocsLink,
		"turnstile_check":       settings.TurnstileCheckEnabled,
		"turnstile_site_key":    turnstileSiteKeyIfEnabled(settings),
		"channel_affinity":      settings.ChannelAffinityEnabled,
		"email_verification":    false,
		"github_oauth":          false,
		"wechat_login":          false,
		"telegram_oauth":        false,
		"passkey_login":         false,
		"display_2fa":           false,
		"self_use_mode_enabled": false,
		"demo_site_enabled":     false,
		"enable_batch_update":   common.BatchUpdateEnabled,
		"uptime_kuma_enabled":   settings.UptimeKumaStatusEnabled,
	})
}

// turnstileSiteKeyIfEnabled 未开启人机验证时不暴露 site key。
func turnstileSiteKeyIfEnabled(settings *setting.GlobalSettings) string {
	if !settings.TurnstileCheckEnabled {
		return ""
	}
	return settings.TurnstileSiteKey
}

// statusCheckItem 一项自检结果
type statusCheckItem struct {
	Name    string `json:"name"`
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

// TestStatus 管理员连通性自检：数据库、Redis、渠道缓存。
func TestStatus(c gin.Context) {
	items := make([]statusCheckItem, 0, 3)

	items = append(items, checkDatabase())
	if common.RedisEnabled {
		items = append(items, checkRedis(c.Request().Context()))
	}
	items = append(items, checkChannelCache())

	success := true
	for _, item := range items {
		if !item.Success {
			success = false
			break
		}
	}
	apiSuccess(c, gin.H{"success": success, "items": items})
}

// checkDatabase 数据库连通性
func checkDatabase() statusCheckItem {
	item := statusCheckItem{Name: "database"}
	if model.DB == nil {
		item.Message = "database is not initialized"
		return item
	}
	sqlDB, err := model.DB.DB()
	if err != nil {
		item.Message = err.Error()
		return item
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		item.Message = err.Error()
		return item
	}
	item.Success = true
	return item
}

// checkRedis Redis 连通性
func checkRedis(parent context.Context) statusCheckItem {
	item := statusCheckItem{Name: "redis"}
	if common.RDB == nil {
		item.Message = "redis client is not initialized"
		return item
	}
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()
	if err := common.RDB.Ping(ctx).Err(); err != nil {
		item.Message = err.Error()
		return item
	}
	item.Success = true
	return item
}

// checkChannelCache 渠道缓存是否可用
func checkChannelCache() statusCheckItem {
	item := statusCheckItem{Name: "channel_cache"}
	models := model.GetEnabledModels()
	if models == nil {
		// 缓存未加载或被清空，重建一次再判断
		if err := model.InitChannelCache(); err != nil {
			item.Message = err.Error()
			return item
		}
		models = model.GetEnabledModels()
	}
	if models == nil {
		item.Message = "channel cache is not initialized"
		return item
	}
	item.Success = true
	item.Message = fmt.Sprintf("%d models available", len(models))
	return item
}

// uptimeKumaCache Uptime Kuma 状态的短期缓存
var uptimeKumaCache struct {
	mu        sync.Mutex
	body      []byte
	fetchedAt time.Time
}

// GetUptimeKumaStatus 代理 Uptime Kuma 状态页数据（前端直连会有跨域问题）。
// 结果缓存 30 秒；未配置时返回空对象，前端据此隐藏入口。
func GetUptimeKumaStatus(c gin.Context) {
	settings := setting.GetGlobalSettings()
	if !settings.UptimeKumaStatusEnabled || strings.TrimSpace(settings.UptimeKumaUrl) == "" {
		apiSuccess(c, gin.H{})
		return
	}

	body, err := loadUptimeKumaStatus(settings)
	if err != nil {
		apiErrorMsg(c, "failed to load uptime kuma status: "+err.Error())
		return
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", body)
}

// loadUptimeKumaStatus 拉取并缓存状态页数据。
func loadUptimeKumaStatus(settings *setting.GlobalSettings) ([]byte, error) {
	uptimeKumaCache.mu.Lock()
	defer uptimeKumaCache.mu.Unlock()
	if uptimeKumaCache.body != nil && time.Since(uptimeKumaCache.fetchedAt) < uptimeKumaCacheTTL {
		return uptimeKumaCache.body, nil
	}

	baseURL := strings.TrimSuffix(strings.TrimSpace(settings.UptimeKumaUrl), "/")
	slug := strings.TrimSpace(settings.UptimeKumaSlug)
	// Uptime Kuma 状态页接口：/api/status-page/{slug}；未配置 slug 时用默认页
	endpoint := baseURL + "/api/status-page"
	if slug != "" {
		endpoint += "/" + slug
	}

	ctx, cancel := context.WithTimeout(context.Background(), uptimeKumaTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := httpclient.GetHttpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("request uptime kuma: %s", common.MaskSensitiveInfo(err.Error()))
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxUptimeKumaResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("read uptime kuma response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("uptime kuma returned status %d", resp.StatusCode)
	}

	uptimeKumaCache.body = body
	uptimeKumaCache.fetchedAt = time.Now()
	return body, nil
}

// 系统配置（options 表）与公开内容接口。
// 读写都需要 root 权限；公开内容接口供前端未登录时展示公告、协议等。

// GetOptions 读取全部系统配置。敏感项（以 Secret / Token / Key 结尾）以占位符返回。
func GetOptions(c gin.Context) {
	options, err := model.ListOptionsFromDB()
	if err != nil {
		apiError(c, err)
		return
	}
	values := make(map[string]string, len(options))
	for _, option := range options {
		values[option.Key] = model.MaskOptionValue(option.Key, option.Value)
	}
	apiSuccess(c, values)
}

// optionUpdateRequest 修改配置的请求体。
type optionUpdateRequest struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// UpdateOption 修改单个配置项并立即生效（内存配置同步刷新）。
func UpdateOption(c gin.Context) {
	var req optionUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiErrorMsg(c, "invalid request: "+err.Error())
		return
	}
	req.Key = strings.TrimSpace(req.Key)
	if req.Key == "" {
		apiErrorMsg(c, "key is required")
		return
	}
	// 前端拿到的是脱敏值，原样回传时不能把占位符写进数据库
	if model.IsSensitiveOptionKey(req.Key) && isMaskedOptionValue(req.Value) {
		apiSuccess(c, gin.H{"key": req.Key})
		return
	}
	if err := model.UpdateOptionValue(req.Key, req.Value); err != nil {
		if err == model.ErrOptionKeyRequired {
			apiErrorMsg(c, err.Error())
			return
		}
		apiError(c, err)
		return
	}
	apiSuccess(c, gin.H{"key": req.Key})
}

// isMaskedOptionValue 判断是否是 GetOptions 返回的脱敏占位符。
func isMaskedOptionValue(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return false
	}
	for _, r := range trimmed {
		if r != '*' {
			return false
		}
	}
	return true
}

// legacyConsoleOptions 旧版控制台设置键 -> 当前配置键。
// 旧键由早期前端写入，新版本不再读取，这里做一次性的兼容迁移。
var legacyConsoleOptions = map[string]string{
	"console_setting.api_info":       "ApiInfo",
	"console_setting.faq":            "FAQ",
	"console_setting.announcements":  "Announcements",
	"console_setting.uptime_kuma":    "UptimeKumaUrl",
	"console_setting.allow_redirect": "AllowRedirect",
}

// MigrateConsoleSetting 把旧版控制台设置迁移为当前配置项。
// 幂等：目标配置已存在时跳过，不会覆盖管理员的现有配置。
// 请求体可选 {"keys": ["console_setting.api_info"]}，为空时迁移全部已知旧键。
func MigrateConsoleSetting(c gin.Context) {
	var req struct {
		Keys []string `json:"keys"`
	}
	// 请求体为空时仍执行全量迁移
	_ = c.ShouldBindJSON(&req)

	candidates := make([]string, 0, len(legacyConsoleOptions))
	if len(req.Keys) == 0 {
		for legacy := range legacyConsoleOptions {
			candidates = append(candidates, legacy)
		}
	} else {
		candidates = append(candidates, req.Keys...)
	}

	values := model.GetAllOptionValues()
	migrated := make([]string, 0, len(candidates))
	skipped := make([]string, 0, len(candidates))
	for _, legacy := range candidates {
		legacy = strings.TrimSpace(legacy)
		target, known := legacyConsoleOptions[legacy]
		if !known {
			skipped = append(skipped, legacy)
			continue
		}
		value, exists := values[legacy]
		if !exists {
			skipped = append(skipped, legacy)
			continue
		}
		if _, targetExists := values[target]; targetExists {
			skipped = append(skipped, legacy)
			continue
		}
		if err := model.UpdateOptionValue(target, value); err != nil {
			apiError(c, err)
			return
		}
		migrated = append(migrated, legacy+" -> "+target)
	}
	apiSuccess(c, gin.H{"migrated": migrated, "skipped": skipped})
}

// writePublicContent 输出一项公开内容配置；未配置时返回空串而不是报错。
func writePublicContent(c gin.Context, content string) {
	apiSuccess(c, content)
}

// GetNotice 系统公告
func GetNotice(c gin.Context) {
	writePublicContent(c, setting.GetGlobalSettings().Notice)
}

// GetAbout 关于页内容
func GetAbout(c gin.Context) {
	writePublicContent(c, setting.GetGlobalSettings().About)
}

// GetHomePageContent 首页自定义内容
func GetHomePageContent(c gin.Context) {
	writePublicContent(c, setting.GetGlobalSettings().HomePageContent)
}

// GetUserAgreement 用户协议
func GetUserAgreement(c gin.Context) {
	writePublicContent(c, setting.GetGlobalSettings().UserAgreement)
}

// GetPrivacyPolicy 隐私政策
func GetPrivacyPolicy(c gin.Context) {
	writePublicContent(c, setting.GetGlobalSettings().PrivacyPolicy)
}
