package controller

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"openapi/common"
	"openapi/model"

	gin "github.com/king54346/gin-tiny"
)

// 令牌管理接口（需登录）：用户只能操作自己的令牌。

const maxTokenNameLength = 50

// GetAllTokens 分页列出当前用户的令牌。
func GetAllTokens(c gin.Context) {
	page := common.GetPageQuery(c)
	tokens, total, err := model.GetUserTokens(c.GetInt("id"), page.GetStartIdx(), page.GetPageSize())
	if err != nil {
		apiError(c, err)
		return
	}
	apiSuccess(c, pageResult(page, tokens, total))
}

// SearchTokens 按名称（keyword）或 key（token）分页搜索当前用户的令牌。
func SearchTokens(c gin.Context) {
	page := common.GetPageQuery(c)
	tokens, total, err := model.SearchUserTokens(c.GetInt("id"), c.Query("keyword"), c.Query("token"), page.GetStartIdx(), page.GetPageSize())
	if err != nil {
		apiError(c, err)
		return
	}
	apiSuccess(c, pageResult(page, tokens, total))
}

// GetToken 取当前用户的单个令牌。
func GetToken(c gin.Context) {
	id, ok := paramID(c)
	if !ok {
		return
	}
	token, err := model.GetUserTokenById(id, c.GetInt("id"))
	if err != nil {
		tokenNotFoundOrError(c, err)
		return
	}
	apiSuccess(c, token)
}

func tokenNotFoundOrError(c gin.Context, err error) {
	if errors.Is(err, model.ErrTokenNotFound) {
		apiErrorMsg(c, "token not found")
		return
	}
	apiError(c, err)
}

// tokenRequest 新增/修改令牌的请求体。
type tokenRequest struct {
	Name               string `json:"name"`
	ModelLimits        string `json:"model_limits"`
	AllowIps           string `json:"allow_ips"`
	Group              string `json:"group"`
	Id                 int    `json:"id"`
	Status             int    `json:"status"`
	ExpiredTime        int64  `json:"expired_time"`
	RemainQuota        int    `json:"remain_quota"`
	ModelLimitsType    int    `json:"model_limits_type"`
	UnlimitedQuota     bool   `json:"unlimited_quota"`
	ModelLimitsEnabled bool   `json:"model_limits_enabled"`
	CrossGroupRetry    bool   `json:"cross_group_retry"`
}

// validate 校验通用字段；userGroup 为当前用户所在分组。
func (r *tokenRequest) validate() error {
	r.Name = strings.TrimSpace(r.Name)
	if utf8.RuneCountInString(r.Name) > maxTokenNameLength {
		return fmt.Errorf("token name must be at most %d characters", maxTokenNameLength)
	}
	if r.ExpiredTime != -1 && r.ExpiredTime != 0 && r.ExpiredTime < time.Now().Unix() {
		return errors.New("expired time must be in the future, or -1 for never")
	}
	if r.ExpiredTime == 0 {
		r.ExpiredTime = -1
	}
	if r.RemainQuota < 0 {
		return errors.New("remain quota must not be negative")
	}
	if r.ModelLimitsType != 0 && r.ModelLimitsType != 1 {
		return errors.New("model_limits_type must be 0 (whitelist) or 1 (blacklist)")
	}
	return nil
}

func (r *tokenRequest) applyTo(t *model.Token) {
	t.Name = r.Name
	t.ExpiredTime = r.ExpiredTime
	t.RemainQuota = r.RemainQuota
	t.UnlimitedQuota = r.UnlimitedQuota
	t.ModelLimitsEnabled = r.ModelLimitsEnabled
	t.ModelLimits = r.ModelLimits
	t.ModelLimitsType = r.ModelLimitsType
	t.IpLimits = r.AllowIps
	t.CrossGroupRetry = r.CrossGroupRetry
}

// AddToken 新建令牌，返回含完整 key 的令牌。
func AddToken(c gin.Context) {
	var req tokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiErrorMsg(c, "invalid request: "+err.Error())
		return
	}
	if err := req.validate(); err != nil {
		apiErrorMsg(c, err.Error())
		return
	}
	token := &model.Token{UserId: c.GetInt("id")}
	req.applyTo(token)
	if err := token.Insert(); err != nil {
		apiError(c, err)
		return
	}
	apiSuccess(c, token)
}

// UpdateToken 修改令牌。?status_only=true 时只修改状态；
// 启用已过期或额度耗尽的令牌会被拒绝，需先调整过期时间或额度。
func UpdateToken(c gin.Context) {
	var req tokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiErrorMsg(c, "invalid request: "+err.Error())
		return
	}
	token, err := model.GetUserTokenById(req.Id, c.GetInt("id"))
	if err != nil {
		tokenNotFoundOrError(c, err)
		return
	}

	if c.Query("status_only") == "true" {
		if req.Status == common.TokenStatusEnabled {
			if token.ExpiredTime != -1 && token.ExpiredTime < time.Now().Unix() {
				apiErrorMsg(c, "token has expired, update its expired time first")
				return
			}
			if !token.UnlimitedQuota && token.RemainQuota <= 0 {
				apiErrorMsg(c, "token quota is exhausted, update its quota first")
				return
			}
		} else if req.Status != common.TokenStatusDisabled {
			apiErrorMsg(c, "status must be 1 (enabled) or 2 (disabled)")
			return
		}
		if err := token.UpdateStatus(req.Status); err != nil {
			apiError(c, err)
			return
		}
		apiSuccess(c, token)
		return
	}

	if err := req.validate(); err != nil {
		apiErrorMsg(c, err.Error())
		return
	}
	req.applyTo(token)
	// 修改了过期时间或额度后，过期/耗尽状态的令牌自动恢复为启用
	if token.Status == common.TokenStatusExpired && (token.ExpiredTime == -1 || token.ExpiredTime > time.Now().Unix()) {
		token.Status = common.TokenStatusEnabled
	}
	if token.Status == common.TokenStatusExhausted && (token.UnlimitedQuota || token.RemainQuota > 0) {
		token.Status = common.TokenStatusEnabled
	}
	if err := token.Update(); err != nil {
		apiError(c, err)
		return
	}
	apiSuccess(c, token)
}

// DeleteToken 删除当前用户的单个令牌。
func DeleteToken(c gin.Context) {
	id, ok := paramID(c)
	if !ok {
		return
	}
	rows, err := model.DeleteUserTokens(c.GetInt("id"), []int{id})
	if err != nil {
		apiError(c, err)
		return
	}
	if rows == 0 {
		apiErrorMsg(c, "token not found")
		return
	}
	apiOK(c)
}

// DeleteTokenBatch 批量删除当前用户的令牌，请求体 {"ids": [...]}，返回实际删除条数。
func DeleteTokenBatch(c gin.Context) {
	var req idsRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Ids) == 0 {
		apiErrorMsg(c, "ids is required")
		return
	}
	rows, err := model.DeleteUserTokens(c.GetInt("id"), req.Ids)
	if err != nil {
		apiError(c, err)
		return
	}
	apiSuccess(c, rows)
}

// 令牌「访问限制」模板：按用户隔离，供创建/编辑令牌时快速套用访问限制。

const maxTokenTemplateNameLength = 64

// GetUserTokenAccessTemplates 分页列出当前用户的模板。
func GetUserTokenAccessTemplates(c gin.Context) {
	page := common.GetPageQuery(c)
	templates, total, err := model.ListUserTokenAccessTemplates(c.GetInt("id"), page.GetStartIdx(), page.GetPageSize())
	if err != nil {
		apiError(c, err)
		return
	}
	apiSuccess(c, pageResult(page, templates, total))
}

// tokenTemplateRequest 新建/修改模板的请求体。
type tokenTemplateRequest struct {
	Id              int    `json:"id"`
	Name            string `json:"name"`
	ModelLimitsType int    `json:"model_limits_type"`
	ModelLimits     string `json:"model_limits"`
	AllowIps        string `json:"allow_ips"`
}

// validate 校验并规整字段。
func (r *tokenTemplateRequest) validate() error {
	r.Name = strings.TrimSpace(r.Name)
	if r.Name == "" {
		return errors.New("name is required")
	}
	if len([]rune(r.Name)) > maxTokenTemplateNameLength {
		return errors.New("name must be at most 64 characters")
	}
	if r.ModelLimitsType != 0 && r.ModelLimitsType != 1 {
		return errors.New("model_limits_type must be 0 (whitelist) or 1 (blacklist)")
	}
	if len(r.ModelLimits) > 2048 {
		return errors.New("model_limits is too long")
	}
	return nil
}

// AddTokenAccessTemplate 新建模板。
func AddTokenAccessTemplate(c gin.Context) {
	var req tokenTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiErrorMsg(c, "invalid request: "+err.Error())
		return
	}
	if err := req.validate(); err != nil {
		apiErrorMsg(c, err.Error())
		return
	}
	template := &model.TokenAccessTemplate{
		UserId:          c.GetInt("id"),
		Name:            req.Name,
		ModelLimitsType: req.ModelLimitsType,
		ModelLimits:     req.ModelLimits,
		AllowIps:        req.AllowIps,
	}
	if err := template.Insert(); err != nil {
		apiError(c, err)
		return
	}
	apiSuccess(c, template)
}

// UpdateTokenAccessTemplate 修改自己的模板。
func UpdateTokenAccessTemplate(c gin.Context) {
	var req tokenTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Id <= 0 {
		apiErrorMsg(c, "invalid request")
		return
	}
	if err := req.validate(); err != nil {
		apiErrorMsg(c, err.Error())
		return
	}
	userID := c.GetInt("id")
	template, err := model.GetTokenAccessTemplateById(req.Id, userID)
	if err != nil {
		tokenTemplateNotFoundOrError(c, err)
		return
	}
	template.Name = req.Name
	template.ModelLimitsType = req.ModelLimitsType
	template.ModelLimits = req.ModelLimits
	template.AllowIps = req.AllowIps
	if err := template.Update(); err != nil {
		apiError(c, err)
		return
	}
	apiSuccess(c, template)
}

// DeleteTokenAccessTemplate 删除自己的模板（硬删除）。
func DeleteTokenAccessTemplate(c gin.Context) {
	id, ok := paramID(c)
	if !ok {
		return
	}
	userID := c.GetInt("id")
	template, err := model.GetTokenAccessTemplateById(id, userID)
	if err != nil {
		tokenTemplateNotFoundOrError(c, err)
		return
	}
	if err := template.Delete(); err != nil {
		apiError(c, err)
		return
	}
	apiOK(c)
}

// tokenTemplateNotFoundOrError 区分"不存在/越权"与内部错误。
func tokenTemplateNotFoundOrError(c gin.Context, err error) {
	if errors.Is(err, model.ErrTokenAccessTemplateNotFound) {
		apiErrorMsg(c, "template not found")
		return
	}
	apiError(c, err)
}

// 令牌使用情况与调用分析。本项目不做计费，因此不返回金额，只返回请求次数与 token 统计。

// tokenAnalyticsDefaultDays 分析接口默认统计最近多少天
const tokenAnalyticsDefaultDays = 30

// GetTokenUsage GET /api/usage/token/：调用方用自己的 API Key 查询这个令牌的用量。
func GetTokenUsage(c gin.Context) {
	tokenID := c.GetInt("token_id")
	userID := c.GetInt("id")
	token, err := model.GetUserTokenById(tokenID, userID)
	if err != nil {
		tokenNotFoundOrError(c, err)
		return
	}

	summary, err := model.GetTokenUsageSummary(token.Id)
	if err != nil {
		apiError(c, err)
		return
	}

	apiSuccess(c, gin.H{
		"object":            "token_usage",
		"id":                token.Id,
		"name":              token.Name,
		"status":            token.Status,
		"expired_time":      token.ExpiredTime,
		"remain_quota":      token.RemainQuota,
		"unlimited_quota":   token.UnlimitedQuota,
		"used_quota":        token.UsedQuota,
		"created_time":      token.CreatedTime,
		"accessed_time":     token.AccessedTime,
		"total_requests":    summary.TotalCalls,
		"prompt_tokens":     summary.TotalPromptTokens,
		"completion_tokens": summary.TotalCompletionTokens,
		"total_tokens":      summary.TotalPromptTokens + summary.TotalCompletionTokens,
	})
}

// GetTokenAnalytics GET /api/token/analytics/:id：令牌调用趋势与模型分布。
// 时间范围用 start_time / end_time（秒级时间戳）指定，默认最近 30 天。
func GetTokenAnalytics(c gin.Context) {
	id, ok := paramID(c)
	if !ok {
		return
	}
	token, err := model.GetUserTokenById(id, c.GetInt("id"))
	if err != nil {
		tokenNotFoundOrError(c, err)
		return
	}

	endTime := common.GetTimestamp()
	if value := c.Query("end_time"); value != "" {
		if parsed := common.String2Int(value); parsed > 0 {
			endTime = int64(parsed)
		}
	}
	startTime := endTime - int64(tokenAnalyticsDefaultDays)*24*3600
	if value := c.Query("start_time"); value != "" {
		if parsed := common.String2Int(value); parsed > 0 {
			startTime = int64(parsed)
		}
	}
	if days := common.String2Int(c.Query("days")); days > 0 {
		startTime = endTime - int64(days)*24*3600
	}
	if startTime > endTime {
		apiErrorMsg(c, "start_time must be earlier than end_time")
		return
	}

	analytics, err := GetTokenAnalytics(token.Id, startTime, endTime)
	if err != nil {
		apiError(c, err)
		return
	}
	apiSuccess(c, gin.H{
		"token_id":    token.Id,
		"token_name":  token.Name,
		"start_time":  startTime,
		"end_time":    endTime,
		"model_stats": analytics.ModelStats,
		"daily_stats": analytics.DailyStats,
		"summary":     analytics.Summary,
		// 便于前端直接展示时间窗口
		"range_days": (endTime - startTime) / 86400,
	})
}
