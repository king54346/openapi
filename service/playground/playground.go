package playground

import (
	"openapi/common"
	"openapi/constant"
	"openapi/model"

	gin "github.com/king54346/gin-tiny"
)

const PlaygroundTokenName = "playground"

// PrepareContext 为体验中心请求注入登录用户身份与日志令牌名；未登录时返回 false。
func PrepareContext(c gin.Context) bool {
	userID := c.GetInt("id")
	if userID <= 0 {
		return false
	}
	common.SetContextKey(c, constant.ContextKeyUserId, userID)
	c.Set("token_name", PlaygroundTokenName)
	if _, ok := c.Get("username"); !ok {
		if cache, err := model.GetUserCache(userID); err == nil {
			c.Set("username", cache.Username)
		}
	}
	return true
}
