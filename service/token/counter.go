package token

import (
	"fmt"
	"math"
	"openapi/common"
	"openapi/dto"
	relaycommon "openapi/relay/common"
	relayconstant "openapi/relay/constant"
	"openapi/types"
	"path/filepath"
	"strings"
	"unicode/utf8"

	gin "github.com/king54346/gin-tiny"
)

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

func CountTextToken(text string, model string) int {
	if text == "" {
		return 0
	}
	if isOpenAITextModel(model) {
		return getTokenNum(getTokenEncoder(model), text)
	}
	return EstimateTokenByModel(model, text)
}

// CountTokenInput 统计任意输入（字符串、字符串数组或其他结构）的 token 数

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

// Provider 定义模型厂商大类
