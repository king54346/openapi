package system

import (
	"context"
	"fmt"
	"time"

	"openapi/common"
	"openapi/model"
	"openapi/setting"
)

type StatusCheckItem struct {
	Name    string `json:"name"`
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

func GetPublicStatus() map[string]any {
	settings := setting.GetGlobalSettings()
	initialized, err := model.IsSetupInitialized()
	if err != nil {
		common.SysError("failed to load setup state: " + err.Error())
	}
	siteKey := ""
	if settings.TurnstileCheckEnabled {
		siteKey = settings.TurnstileSiteKey
	}
	return map[string]any{
		"version":               common.Version,
		"start_time":            common.StartTime,
		"setup":                 initialized,
		"server_address":        settings.ServerAddress,
		"docs_link":             settings.DocsLink,
		"turnstile_check":       settings.TurnstileCheckEnabled,
		"turnstile_site_key":    siteKey,
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
	}
}

func RunStatusChecks(parent context.Context) (bool, []StatusCheckItem) {
	items := make([]StatusCheckItem, 0, 3)
	items = append(items, checkDatabase())
	if common.RedisEnabled {
		items = append(items, checkRedis(parent))
	}
	items = append(items, checkChannelCache())

	success := true
	for _, item := range items {
		if !item.Success {
			success = false
			break
		}
	}
	return success, items
}

func checkDatabase() StatusCheckItem {
	item := StatusCheckItem{Name: "database"}
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

func checkRedis(parent context.Context) StatusCheckItem {
	item := StatusCheckItem{Name: "redis"}
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

func checkChannelCache() StatusCheckItem {
	item := StatusCheckItem{Name: "channel_cache"}
	models := model.GetEnabledModels()
	if models == nil {
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
