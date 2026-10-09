package service

import (
	"openapi/common"
	"openapi/constant"
	"openapi/dto"

	gin "github.com/king54346/gin-tiny"
)

// NewUsage 按 prompt/completion token 构造 Usage，TotalTokens 取两者之和
func NewUsage(promptTokens, completionTokens int) *dto.Usage {
	return &dto.Usage{
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      promptTokens + completionTokens,
	}
}

// MarkUsageEstimated 标记本次请求的 usage 含本地估算值，记录用量时会据此标注
func MarkUsageEstimated(c gin.Context) {
	common.SetContextKey(c, constant.ContextKeyUsageEstimated, true)
}

// EstimateUsageFromText 上游未返回 usage 时，根据输出文本估算 completion token
func EstimateUsageFromText(c gin.Context, responseText string, modelName string, promptTokens int) *dto.Usage {
	MarkUsageEstimated(c)
	return NewUsage(promptTokens, CountTextToken(responseText, modelName))
}

func ValidUsage(usage *dto.Usage) bool {
	return usage != nil && (usage.PromptTokens != 0 || usage.CompletionTokens != 0)
}
