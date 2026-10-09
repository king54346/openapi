package controller

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"openapi/common"
	"openapi/model"
	"openapi/service"

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
func (r *tokenRequest) validate(userGroup string) error {
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
	if r.Group != "" {
		if _, ok := service.GetUserUsableGroups(userGroup)[r.Group]; !ok {
			return fmt.Errorf("no access to group %s", r.Group)
		}
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
	t.Group = r.Group
	t.CrossGroupRetry = r.CrossGroupRetry
}

// currentUserGroup 当前登录用户的分组（session 中取不到时回源数据库）。
func currentUserGroup(c gin.Context) string {
	if g := c.GetString("user_group"); g != "" {
		return g
	}
	if u, err := model.GetUserCache(c.GetInt("id")); err == nil {
		return u.Group
	}
	return ""
}

// AddToken 新建令牌，返回含完整 key 的令牌。
func AddToken(c gin.Context) {
	var req tokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiErrorMsg(c, "invalid request: "+err.Error())
		return
	}
	if err := req.validate(currentUserGroup(c)); err != nil {
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

	if err := req.validate(currentUserGroup(c)); err != nil {
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
