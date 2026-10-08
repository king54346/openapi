package middleware

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"

	"openapi/common"
	"openapi/constant"
	"openapi/logger"
	"openapi/model"
	"openapi/service"
	"openapi/types"

	gin "github.com/king54346/gin-tiny"
	"github.com/king54346/gin-tiny/middleware/sessions"
)

func validUserInfo(username string, role int) bool {
	if strings.TrimSpace(username) == "" {
		return false
	}
	return common.IsValidateRole(role)
}

func authHelper(c gin.Context, minRole int) {
	session := sessions.Default(c)
	username := session.Get("username")
	role := session.Get("role")
	id := session.Get("id")
	status := session.Get("status")
	useAccessToken := false

	if username == nil {
		accessToken := c.Request().Header.Get("Authorization")
		if accessToken == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"message": "no auth: not logged in and no access token provided",
			})
			c.Abort()
			return
		}
		user := model.ValidateAccessToken(accessToken)
		if user != nil && user.Username != "" {
			if !validUserInfo(user.Username, user.Role) {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": "no auth: invalid user info",
				})
				c.Abort()
				return
			}
			username = user.Username
			role = user.Role
			id = user.Id
			status = user.Status
			useAccessToken = true
		} else {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "no auth: invalid access token",
			})
			c.Abort()
			return
		}
	}

	apiUserIdStr := c.Request().Header.Get("Open-Api-User")
	if apiUserIdStr == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "no auth: missing Open-Api-User header",
		})
		c.Abort()
		return
	}
	apiUserId, err := strconv.Atoi(apiUserIdStr)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "no auth: malformed Open-Api-User header",
		})
		c.Abort()
		return
	}
	loginId, ok := toInt(id)
	if !ok || loginId != apiUserId {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "no auth: Open-Api-User does not match the logged-in user",
		})
		c.Abort()
		return
	}
	// session 里的状态/角色/分组是登录时的快照，以库中最新值为准，
	// 这样禁用、降级、改分组对已登录的 session 立即生效
	fresh, err := model.GetUserCache(loginId)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "no auth: user not found",
		})
		c.Abort()
		return
	}
	status, role = fresh.Status, fresh.Role
	userStatus, ok := toInt(status)
	if !ok {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "no auth: invalid user status",
		})
		c.Abort()
		return
	}
	if userStatus == common.UserStatusDisabled {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "user is disabled",
		})
		c.Abort()
		return
	}
	userRole, ok := toInt(role)
	if !ok || userRole < minRole {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "no auth: insufficient permission",
		})
		c.Abort()
		return
	}
	usernameStr, ok := username.(string)
	if !ok || !validUserInfo(usernameStr, userRole) {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "no auth: invalid user info",
		})
		c.Abort()
		return
	}
	c.Set("username", username)
	c.Set("role", userRole)
	c.Set("id", loginId)
	c.Set("group", fresh.Group)
	c.Set("user_group", fresh.Group)
	c.Set("use_access_token", useAccessToken)

	c.Next()
}

// toInt 安全地把 session 中的数值转成 int，避免直接断言 panic。
func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int8:
		return int(n), true
	case int16:
		return int(n), true
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case uint:
		return int(n), true
	case uint32:
		return int(n), true
	case uint64:
		return int(n), true
	case float64:
		return int(n), true
	case string:
		i, err := strconv.Atoi(strings.TrimSpace(n))
		if err != nil {
			return 0, false
		}
		return i, true
	}
	return 0, false
}

func TryUserAuth() gin.HandlerFunc {
	return func(c gin.Context) {
		session := sessions.Default(c)
		if id := session.Get("id"); id != nil {
			c.Set("id", id)
		}
		c.Next()
	}
}

func UserAuth() gin.HandlerFunc {
	return func(c gin.Context) {
		authHelper(c, common.RoleCommonUser)
	}
}

func AdminAuth() gin.HandlerFunc {
	return func(c gin.Context) {
		authHelper(c, common.RoleAdminUser)
	}
}

func RootAuth() gin.HandlerFunc {
	return func(c gin.Context) {
		authHelper(c, common.RoleRootUser)
	}
}

func WssAuth(c gin.Context) {
	c.Next()
}

func TokenAuth() gin.HandlerFunc {
	return func(c gin.Context) {
		if protocol := c.Request().Header.Get("Sec-WebSocket-Protocol"); protocol != "" {
			key := protocol
			for _, part := range strings.Split(key, ",") {
				part = strings.TrimSpace(part)
				if strings.HasPrefix(part, "openai-insecure-api-key") {
					key = strings.TrimPrefix(part, "openai-insecure-api-key.")
					break
				}
			}
			c.Request().Header.Set("Authorization", "Bearer "+strings.TrimSpace(key))
		}
		if strings.Contains(c.Request().URL.Path, "/v1/messages") || strings.Contains(c.Request().URL.Path, "/v1/models") {
			if anthropicKey := c.Request().Header.Get("x-api-key"); anthropicKey != "" {
				c.Request().Header.Set("Authorization", "Bearer "+anthropicKey)
			}
		}
		if strings.HasPrefix(c.Request().URL.Path, "/v1beta/models") ||
			strings.HasPrefix(c.Request().URL.Path, "/v1beta/openai/models") ||
			strings.HasPrefix(c.Request().URL.Path, "/v1/models/") {
			if skKey := c.Query("key"); skKey != "" {
				c.Request().Header.Set("Authorization", "Bearer "+skKey)
			}
			if xGoogKey := c.Request().Header.Get("x-goog-api-key"); xGoogKey != "" {
				c.Request().Header.Set("Authorization", "Bearer "+xGoogKey)
			}
		}
		key := normalizeBearerKey(c.Request().Header.Get("Authorization"))
		parts := []string{}
		if key == "" || key == "midjourney-proxy" {
			key = normalizeBearerKey(c.Request().Header.Get("mj-api-secret"))
			key = strings.TrimPrefix(key, "sk-")
			parts = strings.Split(key, "-")
			key = parts[0]
		} else {
			key = strings.TrimPrefix(key, "sk-")
			parts = strings.Split(key, "-")
			key = parts[0]
		}
		token, err := model.ValidateUserToken(key)
		if err != nil {
			abortWithOpenAiMessage(c, http.StatusUnauthorized, err.Error())
			return
		}
		if token == nil {
			abortWithOpenAiMessage(c, http.StatusUnauthorized, "invalid token")
			return
		}
		if id := c.GetInt("id"); id == 0 {
			c.Set("id", token.UserId)
		}

		if allowIps := token.GetIpLimits(); len(allowIps) > 0 {
			clientIP := c.ClientIP()
			ip := net.ParseIP(clientIP)
			if ip == nil {
				abortWithOpenAiMessage(c, http.StatusForbidden, "cannot parse client IP address")
				return
			}
			if !common.IsIpInCIDRList(ip, allowIps) {
				logger.LogDebug(c, "client IP %s is not in the token allowlist", clientIP)
				abortWithOpenAiMessage(c, http.StatusForbidden, "your IP is not in the token allowlist", types.ErrorCodeAccessDenied)
				return
			}
			logger.LogDebug(c, "client IP %s passed the token IP allowlist check", clientIP)
		}

		userCache, err := model.GetUserCache(token.UserId)
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, model.ErrUserNotFound) {
				// 令牌所属用户已删除
				status = http.StatusUnauthorized
			}
			abortWithOpenAiMessage(c, status, err.Error())
			return
		}
		if userCache.Status != common.UserStatusEnabled {
			abortWithOpenAiMessage(c, http.StatusForbidden, "user is disabled")
			return
		}

		userCache.WriteContext(c)

		userGroup := userCache.Group
		if tokenGroup := token.Group; tokenGroup != "" {
			if _, ok := service.GetUserUsableGroups(userGroup)[tokenGroup]; !ok {
				abortWithOpenAiMessage(c, http.StatusForbidden, fmt.Sprintf("no access to group %s", tokenGroup))
				return
			}
			if !service.ContainsGroupRatio(tokenGroup) && tokenGroup != "auto" {
				abortWithOpenAiMessage(c, http.StatusForbidden, fmt.Sprintf("group %s is deprecated", tokenGroup))
				return
			}
			userGroup = tokenGroup
		}
		common.SetContextKey(c, constant.ContextKeyUsingGroup, userGroup)

		if err := SetupContextForToken(c, token, parts...); err != nil {
			return
		}
		c.Next()
	}
}

// normalizeBearerKey 去掉 Authorization 头的 Bearer 前缀并 trim 空格。
func normalizeBearerKey(key string) string {
	key = strings.TrimSpace(key)
	if len(key) > 7 && strings.EqualFold(key[:7], "Bearer ") {
		return strings.TrimSpace(key[7:])
	}
	return key
}

func SetupContextForToken(c gin.Context, token *model.Token, parts ...string) error {
	if token == nil {
		return fmt.Errorf("token is nil")
	}
	c.Set("id", token.UserId)
	c.Set("token_id", token.Id)
	c.Set("token_key", token.Key)
	c.Set("token_name", token.Name)
	c.Set("token_unlimited_quota", token.UnlimitedQuota)
	if !token.UnlimitedQuota {
		c.Set("token_quota", token.RemainQuota)
	}
	if token.ModelLimitsEnabled {
		c.Set("token_model_limit_enabled", true)
		c.Set("token_model_limit", token.GetModelLimitsMap())
		c.Set("token_model_limit_type", token.ModelLimitsType)
	} else {
		c.Set("token_model_limit_enabled", false)
	}
	common.SetContextKey(c, constant.ContextKeyTokenGroup, token.Group)
	common.SetContextKey(c, constant.ContextKeyTokenCrossGroupRetry, token.CrossGroupRetry)
	if len(parts) > 1 {
		if !model.IsAdmin(token.UserId) {
			abortWithOpenAiMessage(c, http.StatusForbidden, "regular users cannot specify a channel")
			return fmt.Errorf("regular users cannot specify a channel")
		}
		c.Set("specific_channel_id", parts[1])
	}
	return nil
}
