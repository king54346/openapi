package controller

import (
	"net/http"
	"openapi/common"
	"openapi/constant"
	"openapi/model"
	"openapi/types"

	gin "github.com/king54346/gin-tiny"
)

// Playground 体验中心：与 /v1 接口功能等价，但使用登录用户（session）身份而不是 API 令牌，
// 因此不需要用户先创建令牌。渠道已由 Distribute 中间件选好（分组已移除，不再按分组过滤）。

// playgroundTokenName 体验中心请求在日志里的令牌名
const playgroundTokenName = "playground"

// preparePlayground 补齐转发链路依赖的用户上下文。返回 false 表示已经写过错误响应。
func preparePlayground(c gin.Context) bool {
	userID := c.GetInt("id")
	if userID <= 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{"message": "login is required", "type": "invalid_request_error"}})
		return false
	}

	common.SetContextKey(c, constant.ContextKeyUserId, userID)
	// 体验中心不经过 TokenAuth，这里补上日志与转发需要的身份信息
	c.Set("token_name", playgroundTokenName)
	if _, ok := c.Get("username"); !ok {
		if cache, err := model.GetUserCache(userID); err == nil {
			c.Set("username", cache.Username)
		}
	}
	return true
}

// Playground 对话体验：POST /pg/chat/completions。
func Playground(c gin.Context) {
	if !preparePlayground(c) {
		return
	}
	Relay(c, types.RelayFormatOpenAI)
}

// PlaygroundImage 绘画体验：POST /pg/images/generations。
func PlaygroundImage(c gin.Context) {
	if !preparePlayground(c) {
		return
	}
	Relay(c, types.RelayFormatOpenAIImage)
}

// PlaygroundEmbedding 嵌入体验：POST /pg/embeddings。
func PlaygroundEmbedding(c gin.Context) {
	if !preparePlayground(c) {
		return
	}
	Relay(c, types.RelayFormatEmbedding)
}

// PlaygroundRerank 重排序体验：POST /pg/rerank。
func PlaygroundRerank(c gin.Context) {
	if !preparePlayground(c) {
		return
	}
	Relay(c, types.RelayFormatRerank)
}

// PlaygroundAudio 语音体验：POST /pg/audio/speech 与 /pg/audio/transcriptions。
func PlaygroundAudio(c gin.Context) {
	if !preparePlayground(c) {
		return
	}
	Relay(c, types.RelayFormatOpenAIAudio)
}

// PlaygroundTask 视频体验：POST /pg/video/generations 提交、GET /pg/video/tasks/:task_id 查询。
func PlaygroundTask(c gin.Context) {
	if !preparePlayground(c) {
		return
	}
	RelayTask(c)
}
