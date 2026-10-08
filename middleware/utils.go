package middleware

import (
	"openapi/types"

	gin "github.com/king54346/gin-tiny"
)

// abortWithOpenAiMessage 以 OpenAI 兼容的错误格式中止请求。
func abortWithOpenAiMessage(c gin.Context, statusCode int, message string, code ...types.ErrorCode) {
	openAICode := types.ErrorCodeInvalidRequest
	if len(code) > 0 && code[0] != "" {
		openAICode = code[0]
	}
	c.JSON(statusCode, gin.H{
		"error": gin.H{
			"message": message,
			"type":    "invalid_request_error",
			"code":    string(openAICode),
		},
	})
	c.Abort()
}
