package service

import (
	"fmt"
	"math"
	"path/filepath"
	"unicode/utf8"

	"openapi/common"
	"openapi/dto"
	relaycommon "openapi/relay/common"
	relayconstant "openapi/relay/constant"
	"openapi/types"

	gin "github.com/king54346/gin-tiny"
)

// EstimateRequestToken 在转发前估算请求的 prompt token 数。
// 上游返回 usage 时以上游为准，该值只在上游未返回 usage 时用于记录用量。
func EstimateRequestToken(c gin.Context, meta *types.TokenCountMeta, info *relaycommon.RelayInfo) (int, error) {
	if meta == nil || info.RelayFormat == types.RelayFormatOpenAIRealtime {
		return 0, nil
	}
	if info.RelayMode == relayconstant.RelayModeAudioTranscription || info.RelayMode == relayconstant.RelayModeAudioTranslation {
		return estimateAudioFileToken(c)
	}

	tkm := 0
	if meta.TokenType == types.TokenTypeTextNumber {
		tkm += utf8.RuneCountInString(meta.CombineText)
	} else {
		tkm += CountTextToken(meta.CombineText, info.OriginModelName)
	}

	if info.RelayFormat == types.RelayFormatOpenAI {
		tkm += meta.ToolsCount * 8
		tkm += meta.MessagesCount * 3 // 每条消息的格式化 token
		tkm += meta.NameCount * 3
		tkm += 3
	}
	return tkm, nil
}

// AudioDurationToTokens 音频时长（秒）换算为 token：一分钟 1000 token，与按分钟计价对齐
func AudioDurationToTokens(seconds float64) int {
	return int(math.Round(math.Ceil(seconds) / 60.0 * 1000))
}

// estimateAudioFileToken 按上传音频文件的总时长估算 token
func estimateAudioFileToken(c gin.Context) (int, error) {
	form, err := common.ParseMultipartFormReusable(c)
	if err != nil {
		return 0, fmt.Errorf("error parsing multipart form: %w", err)
	}
	total := 0
	for _, fileHeader := range form.File["file"] {
		file, err := fileHeader.Open()
		if err != nil {
			return 0, fmt.Errorf("error opening audio file: %w", err)
		}
		duration, err := common.GetAudioDuration(c.Request().Context(), file, filepath.Ext(fileHeader.Filename))
		file.Close()
		if err != nil {
			return 0, fmt.Errorf("error getting audio duration: %w", err)
		}
		total += AudioDurationToTokens(duration)
	}
	return total, nil
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
