package relay

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"openapi/common"
	"openapi/constant"
	"openapi/dto"
	"openapi/logger"
	"openapi/relay/channel"
	"openapi/relay/channel/claude"
	relaycommon "openapi/relay/common"
	"openapi/relay/helper"
	relayerr "openapi/service/relayerr"
	"openapi/types"
	"strings"

	gin "github.com/king54346/gin-tiny"
)

// channelAdaptor 渠道适配器接口别名，避免与 relay 包内其它标识重名。
type channelAdaptor = channel.Adaptor

// tokens.Claude 协议转发：/v1/messages 直连 Anthropic 渠道，或跨协议转发到任意 tokens.OpenAI 兼容渠道。

// ClaudeHelper 处理 RelayFormatClaude 的请求。
func ClaudeHelper(c gin.Context, info *relaycommon.RelayInfo) (apiErr *types.StarAPIError) {
	info.InitChannelMeta(c)

	request, ok := info.Request.(*dto.ClaudeRequest)
	if !ok {
		return types.NewErrorWithStatusCode(fmt.Errorf("invalid request type, expected dto.ClaudeRequest, got %T", info.Request),
			types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}

	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return types.NewError(fmt.Errorf("invalid api type: %d", info.ApiType), types.ErrorCodeInvalidApiType, types.ErrOptionWithSkipRetry())
	}
	adaptor.Init(info)

	if info.ApiType == constant.APITypeAnthropic {
		return claudeNativeRelay(c, info, adaptor, request)
	}
	return claudeToOpenAIRelay(c, info, adaptor, request)
}

// claudeNativeRelay tokens.Claude 请求发往 Anthropic 渠道：请求体保持 tokens.Claude 协议。
func claudeNativeRelay(c gin.Context, info *relaycommon.RelayInfo, adaptor channelAdaptor, request *dto.ClaudeRequest) *types.StarAPIError {
	if err := helper.ModelMappedHelper(c, info, request); err != nil {
		return types.NewError(err, types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())
	}

	jsonData, err := common.Marshal(request)
	if err != nil {
		return types.NewError(err, types.ErrorCodeJsonMarshalFailed, types.ErrOptionWithSkipRetry())
	}
	if len(info.ParamOverride) > 0 {
		jsonData, err = relaycommon.ApplyParamOverride(jsonData, info.ParamOverride, relaycommon.BuildParamOverrideContext(info))
		if err != nil {
			return types.NewError(err, types.ErrorCodeChannelParamOverrideInvalid, types.ErrOptionWithSkipRetry())
		}
	}
	logger.LogDebug(c, "claude request body: "+string(jsonData))

	resp, err := adaptor.DoRequest(c, info, bytes.NewBuffer(jsonData))
	if err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	}
	httpResp, ok := resp.(*http.Response)
	if !ok || httpResp == nil {
		return types.NewError(fmt.Errorf("upstream response is nil"), types.ErrorCodeBadResponse)
	}
	if httpResp.StatusCode != http.StatusOK {
		apiErr := relayerr.RelayErrorHandler(c.Request().Context(), httpResp, false)
		relayerr.ResetStatusCode(apiErr, c.GetString("status_code_mapping"))
		return apiErr
	}

	usage, apiErr := adaptor.DoResponse(c, httpResp, info)
	if apiErr != nil {
		relayerr.ResetStatusCode(apiErr, c.GetString("status_code_mapping"))
		return apiErr
	}
	recordClaudeUsage(c, info, usage)
	return nil
}

// claudeToOpenAIRelay tokens.Claude 请求转成 tokens.OpenAI 请求发给 tokens.OpenAI 兼容渠道，再把响应转回 tokens.Claude 格式。
func claudeToOpenAIRelay(c gin.Context, info *relaycommon.RelayInfo, adaptor channelAdaptor, request *dto.ClaudeRequest) *types.StarAPIError {
	openaiRequest, err := dto.ClaudeRequestToOpenAIRequest(request)
	if err != nil {
		return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}
	// 模型映射在 tokens.Claude 请求上完成，再同步到 tokens.OpenAI 请求
	if err := helper.ModelMappedHelper(c, info, request); err != nil {
		return types.NewError(err, types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())
	}
	openaiRequest.Model = request.Model

	converted, err := adaptor.ConvertOpenAIRequest(c, info, openaiRequest)
	if err != nil {
		return convertRequestError(c, info, err)
	}
	jsonData, err := common.Marshal(converted)
	if err != nil {
		return types.NewError(err, types.ErrorCodeJsonMarshalFailed, types.ErrOptionWithSkipRetry())
	}
	if len(info.ParamOverride) > 0 {
		jsonData, err = relaycommon.ApplyParamOverride(jsonData, info.ParamOverride, relaycommon.BuildParamOverrideContext(info))
		if err != nil {
			return types.NewError(err, types.ErrorCodeChannelParamOverrideInvalid, types.ErrOptionWithSkipRetry())
		}
	}

	resp, err := adaptor.DoRequest(c, info, bytes.NewBuffer(jsonData))
	if err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	}
	httpResp, ok := resp.(*http.Response)
	if !ok || httpResp == nil {
		return types.NewError(fmt.Errorf("upstream response is nil"), types.ErrorCodeBadResponse)
	}
	if httpResp.StatusCode != http.StatusOK {
		apiErr := relayerr.RelayErrorHandler(c.Request().Context(), httpResp, false)
		relayerr.ResetStatusCode(apiErr, c.GetString("status_code_mapping"))
		return apiErr
	}

	usage, apiErr := writeClaudeFromOpenAI(c, httpResp, info)
	if apiErr != nil {
		relayerr.ResetStatusCode(apiErr, c.GetString("status_code_mapping"))
		return apiErr
	}
	recordClaudeUsage(c, info, usage)
	return nil
}

// recordClaudeUsage 统一上报用量。
func recordClaudeUsage(c gin.Context, info *relaycommon.RelayInfo, usage any) {
	if converted, ok := usage.(*dto.Usage); ok && converted != nil {
		recordUsage(c, info, converted, nil)
		return
	}
	recordUsage(c, info, nil, nil)
}

// writeClaudeFromOpenAI 把 tokens.OpenAI 渠道的响应转换成 tokens.Claude 协议写给客户端。
func writeClaudeFromOpenAI(c gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.StarAPIError) {
	defer resp.Body.Close()

	if strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		return streamOpenAIToClaude(c, resp, info)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeReadResponseBodyFailed)
	}
	var openaiResp dto.OpenAITextResponse
	if err := common.Unmarshal(body, &openaiResp); err != nil {
		logger.LogError(c, "failed to parse upstream response: "+common.MaskSensitiveInfo(string(body)))
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	claudeResp := dto.OpenAIResponseToClaudeResponse(&openaiResp, dto.NewClaudeMessageID(info.RequestId))
	c.JSON(http.StatusOK, claudeResp)
	return &openaiResp.Usage, nil
}

// streamOpenAIToClaude 把 tokens.OpenAI SSE 流转换成 Anthropic 事件流。
func streamOpenAIToClaude(c gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.StarAPIError) {
	helper.SetEventStreamHeaders(c)

	id := dto.NewClaudeMessageID(info.RequestId)
	modelName := info.OriginModelName
	message := &dto.ClaudeResponse{Id: id, Type: "message", Role: "assistant", Model: modelName, Content: []dto.ClaudeContentBlock{}}
	_ = claude.WriteClaudeSSE(c, "message_start", map[string]any{"type": "message_start", "message": message})

	var (
		contentStarted bool
		toolIndex      = -1
		usage          *dto.Usage
		stopReason     = "end_turn"
		textIndex      = 0
	)

	scan := bufio.NewScanner(resp.Body)
	scan.Buffer(make([]byte, 64<<10), 64<<20)
	for scan.Scan() {
		line := strings.TrimSpace(scan.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" {
			continue
		}
		if payload == "[DONE]" {
			break
		}
		var chunk dto.ChatCompletionsStreamResponse
		if err := common.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}
		info.SetFirstResponseTime()
		if chunk.Usage != nil {
			usage = chunk.Usage
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		choice := chunk.Choices[0]

		if content, ok := deltaContent(choice.Delta.Content); ok && content != "" {
			if !contentStarted {
				index := textIndex
				_ = claude.WriteClaudeSSE(c, "content_block_start", map[string]any{
					"type":          "content_block_start",
					"index":         index,
					"content_block": map[string]any{"type": "text", "text": ""},
				})
				contentStarted = true
			}
			_ = claude.WriteClaudeSSE(c, "content_block_delta", map[string]any{
				"type":  "content_block_delta",
				"index": textIndex,
				"delta": map[string]any{"type": "text_delta", "text": content},
			})
		}

		for _, toolCall := range choice.Delta.ToolCalls {
			if toolCall.ID != "" {
				toolIndex++
				_ = claude.WriteClaudeSSE(c, "content_block_start", map[string]any{
					"type":  "content_block_start",
					"index": toolIndex,
					"content_block": map[string]any{
						"type":  "tool_use",
						"id":    toolCall.ID,
						"name":  toolCall.Function.Name,
						"input": map[string]any{},
					},
				})
			}
			if toolCall.Function.Arguments != "" && toolIndex >= 0 {
				_ = claude.WriteClaudeSSE(c, "content_block_delta", map[string]any{
					"type":  "content_block_delta",
					"index": toolIndex,
					"delta": map[string]any{"type": "input_json_delta", "partial_json": toolCall.Function.Arguments},
				})
			}
		}

		if choice.FinishReason != nil && *choice.FinishReason != "" {
			stopReason = dto.OpenAIStopReasonToClaude(*choice.FinishReason)
		}
	}

	// 结束当前内容块
	if contentStarted {
		_ = claude.WriteClaudeSSE(c, "content_block_stop", map[string]any{"type": "content_block_stop", "index": textIndex})
	}
	for i := 0; i <= toolIndex; i++ {
		_ = claude.WriteClaudeSSE(c, "content_block_stop", map[string]any{"type": "content_block_stop", "index": i})
	}

	usagePayload := map[string]any{"output_tokens": 0}
	if usage != nil {
		usagePayload["output_tokens"] = usage.CompletionTokens
	}
	_ = claude.WriteClaudeSSE(c, "message_delta", map[string]any{
		"type":  "message_delta",
		"delta": map[string]any{"stop_reason": stopReason, "stop_sequence": nil},
		"usage": usagePayload,
	})
	_ = claude.WriteClaudeSSE(c, "message_stop", map[string]any{"type": "message_stop"})
	return usage, nil
}

// deltaContent 取增量文本（指针可能为 nil）。
func deltaContent(content *string) (string, bool) {
	if content == nil {
		return "", false
	}
	return *content, true
}
