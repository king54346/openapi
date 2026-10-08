package service

import (
	"fmt"
	"strings"

	"openapi/common"
	"openapi/constant"
	"openapi/dto"

	gin "github.com/king54346/gin-tiny"
)

// ResponseText2Usage 上游未返回 usage 时，根据输出文本估算 completion token
func ResponseText2Usage(c gin.Context, responseText string, modelName string, promptTokens int) *dto.Usage {
	common.SetContextKey(c, constant.ContextKeyLocalCountTokens, true)
	usage := &dto.Usage{}
	usage.PromptTokens = promptTokens
	usage.CompletionTokens = EstimateTokenByModel(modelName, responseText)
	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	return usage
}

func ValidUsage(usage *dto.Usage) bool {
	return usage != nil && (usage.PromptTokens != 0 || usage.CompletionTokens != 0)
}

// CountTextToken 估算文本的 token 数（启发式，不依赖分词器）
func CountTextToken(text string, model string) int {
	if text == "" {
		return 0
	}
	return EstimateTokenByModel(model, text)
}

func CountTokenInput(input any, model string) int {
	switch v := input.(type) {
	case string:
		return CountTextToken(v, model)
	case []string:
		return CountTextToken(strings.Join(v, ""), model)
	case []any:
		var sb strings.Builder
		for _, item := range v {
			sb.WriteString(fmt.Sprintf("%v", item))
		}
		return CountTextToken(sb.String(), model)
	}
	return CountTextToken(fmt.Sprintf("%v", input), model)
}

// CountTokenRealtime 估算 realtime 事件中的文本 token。
// 音频 token 需要解码音频才能计算，这里不做估算，以上游 response.done 中的 usage 为准。
func CountTokenRealtime(isFirstRequest bool, realtimeTools []dto.RealTimeTool, request dto.RealtimeEvent, model string) (textToken int, audioToken int) {
	switch request.Type {
	case dto.RealtimeEventTypeSessionUpdate:
		if request.Session != nil {
			textToken += CountTextToken(request.Session.Instructions, model)
		}
	case dto.RealtimeEventResponseAudioTranscriptionDelta, dto.RealtimeEventResponseFunctionCallArgumentsDelta:
		textToken += CountTextToken(request.Delta, model)
	case dto.RealtimeEventConversationItemCreated:
		if request.Item != nil && request.Item.Type == "message" {
			for _, content := range request.Item.Content {
				if content.Type == "input_text" {
					textToken += CountTextToken(content.Text, model)
				}
			}
		}
	case dto.RealtimeEventTypeResponseDone:
		if !isFirstRequest {
			for _, tool := range realtimeTools {
				textToken += 8 + CountTokenInput(tool, model)
			}
		}
	}
	return textToken, audioToken
}
