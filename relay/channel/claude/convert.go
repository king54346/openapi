package claude

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"openapi/common"
	"openapi/dto"
	relaycommon "openapi/relay/common"
	"openapi/types"
	"strings"

	gin "github.com/king54346/gin-tiny"
)

// OpenAIRequestToClaudeRequest 把 tokens.OpenAI Chat Completions 请求转换成 tokens.Claude Messages 请求。
func OpenAIRequestToClaudeRequest(request *dto.GeneralOpenAIRequest) (*dto.ClaudeRequest, error) {
	if request == nil {
		return nil, fmt.Errorf("request is nil")
	}
	out := &dto.ClaudeRequest{
		Model:       request.Model,
		Temperature: request.Temperature,
		TopP:        request.TopP,
		Stream:      request.Stream != nil && *request.Stream,
	}
	if request.GetMaxTokens() > 0 {
		out.MaxTokens = int(request.GetMaxTokens())
	}
	if stops := parseStopSequences(request.Stop); len(stops) > 0 {
		out.StopSequences = stops
	}

	system := make([]string, 0)
	messages := make([]dto.ClaudeMessage, 0, len(request.Messages))
	for _, message := range request.Messages {
		switch message.Role {
		case "system", "developer":
			if text := message.StringContent(); text != "" {
				system = append(system, text)
			}
			continue
		}

		if message.Role == "tool" {
			block := dto.ClaudeContentBlock{
				Type:      "tool_result",
				ToolUseId: message.ToolCallId,
				Content:   mustMarshal(message.StringContent()),
			}
			messages = append(messages, dto.ClaudeMessage{Role: "user", Content: mustMarshal([]dto.ClaudeContentBlock{block})})
			continue
		}

		if message.Role == "assistant" {
			blocks := make([]dto.ClaudeContentBlock, 0, 2)
			if text := message.StringContent(); strings.TrimSpace(text) != "" {
				blocks = append(blocks, dto.ClaudeContentBlock{Type: dto.ContentTypeText, Text: text})
			}
			for _, toolCall := range message.ParseToolCalls() {
				var input any
				if toolCall.Function.Arguments != "" {
					_ = common.Unmarshal([]byte(toolCall.Function.Arguments), &input)
				}
				blocks = append(blocks, dto.ClaudeContentBlock{
					Type:  "tool_use",
					Id:    toolCall.ID,
					Name:  toolCall.Function.Name,
					Input: input,
				})
			}
			if len(blocks) == 0 {
				continue
			}
			messages = append(messages, dto.ClaudeMessage{Role: "assistant", Content: mustMarshal(blocks)})
			continue
		}

		blocks := make([]dto.ClaudeContentBlock, 0, 2)
		for _, part := range message.ParseContent() {
			switch part.Type {
			case dto.ContentTypeText:
				if part.Text != "" {
					blocks = append(blocks, dto.ClaudeContentBlock{Type: dto.ContentTypeText, Text: part.Text})
				}
			case dto.ContentTypeImageURL:
				image := part.GetImageMedia()
				if image == nil || image.Url == "" {
					continue
				}
				blocks = append(blocks, dto.ClaudeContentBlock{Type: "image", Source: openAIImageToClaudeSource(image.Url)})
			}
		}
		if len(blocks) == 0 {
			continue
		}
		messages = append(messages, dto.ClaudeMessage{Role: "user", Content: mustMarshal(blocks)})
	}

	if len(system) > 0 {
		out.System = mustMarshal(strings.Join(system, "\n"))
	}
	out.Messages = messages

	for _, tool := range request.Tools {
		if tool.Function.Name == "" {
			continue
		}
		out.Tools = append(out.Tools, dto.ClaudeTool{
			Name:        tool.Function.Name,
			Description: tool.Function.Description,
			InputSchema: tool.Function.Parameters,
		})
	}
	return out, nil
}

// openAIImageToClaudeSource data URI 转 base64 块，其余按 url 块处理。
func openAIImageToClaudeSource(url string) *dto.ClaudeImageSource {
	if strings.HasPrefix(url, "data:") {
		header, data, found := strings.Cut(strings.TrimPrefix(url, "data:"), ";base64,")
		if found {
			mediaType := header
			if mediaType == "" {
				mediaType = "image/png"
			}
			return &dto.ClaudeImageSource{Type: "base64", MediaType: mediaType, Data: data}
		}
	}
	return &dto.ClaudeImageSource{Type: "url", Url: url}
}

// parseStopSequences stop 字段兼容字符串与字符串数组。
func parseStopSequences(stop any) []string {
	switch value := stop.(type) {
	case string:
		if value != "" {
			return []string{value}
		}
	case []string:
		return value
	case []any:
		out := make([]string, 0, len(value))
		for _, item := range value {
			if str, ok := item.(string); ok && str != "" {
				out = append(out, str)
			}
		}
		return out
	}
	return nil
}

// mustMarshal 序列化失败时返回 nil（这些结构都是可序列化的普通值）。
func mustMarshal(value any) []byte {
	encoded, err := common.Marshal(value)
	if err != nil {
		return nil
	}
	return encoded
}

// convertClaudeResponseForOpenAI 把 tokens.Claude 渠道的响应转换成 tokens.OpenAI 格式返回给客户端。
func convertClaudeResponseForOpenAI(c gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (any, *types.StarAPIError) {
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		return convertClaudeStreamToOpenAI(c, resp, info)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeReadResponseBodyFailed)
	}
	var claudeResp dto.ClaudeResponse
	if err := common.Unmarshal(body, &claudeResp); err != nil {
		logUpstreamError(c, body)
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	openaiResp := dto.OpenAIResponseFromClaude(&claudeResp, info.RequestId, info.OriginModelName)
	c.JSON(http.StatusOK, openaiResp)
	return claudeUsageToUsage(claudeResp.Usage), nil
}

// convertClaudeStreamToOpenAI 把 tokens.Claude SSE 事件转换成 tokens.OpenAI chat.completion.chunk。
func convertClaudeStreamToOpenAI(c gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (any, *types.StarAPIError) {
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64<<10), 64<<20)

	id := dto.NewClaudeMessageID(info.RequestId)
	created := common.GetTimestamp()
	var usage *dto.Usage
	toolIndex := -1

	writeChunk := func(delta dto.ChatCompletionsStreamResponseChoiceDelta, finishReason *string) error {
		chunk := dto.ChatCompletionsStreamResponse{
			Id:      id,
			Object:  "chat.completion.chunk",
			Created: created,
			Model:   info.OriginModelName,
			Choices: []dto.ChatCompletionsStreamResponseChoice{{
				Delta:        delta,
				FinishReason: finishReason,
			}},
		}
		return writeSSEData(c, &chunk)
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "event:") {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		var event dto.ClaudeStreamEvent
		if err := common.Unmarshal([]byte(payload), &event); err != nil {
			continue
		}
		switch event.Type {
		case "message_start":
			role := "assistant"
			_ = writeChunk(dto.ChatCompletionsStreamResponseChoiceDelta{Role: role, Content: common.GetPointer("")}, nil)
			if event.Message != nil {
				converted := claudeUsageToUsage(event.Message.Usage)
				usage = converted
			}
		case "content_block_start":
			if event.ContentBlock != nil && event.ContentBlock.Type == "tool_use" {
				toolIndex++
				index := toolIndex
				call := dto.ToolCallResponse{
					ID:    event.ContentBlock.Id,
					Type:  "function",
					Index: &index,
					Function: dto.FunctionResponse{
						Name: event.ContentBlock.Name,
					},
				}
				_ = writeChunk(dto.ChatCompletionsStreamResponseChoiceDelta{ToolCalls: []dto.ToolCallResponse{call}}, nil)
			}
		case "content_block_delta":
			if event.Delta == nil {
				continue
			}
			switch event.Delta.Type {
			case "text_delta":
				_ = writeChunk(dto.ChatCompletionsStreamResponseChoiceDelta{
					Content: common.GetPointer(event.Delta.Text),
				}, nil)
			case "input_json_delta":
				index := toolIndex
				if index < 0 {
					index = 0
				}
				call := dto.ToolCallResponse{Index: &index, Function: dto.FunctionResponse{Arguments: event.Delta.PartialJson}}
				_ = writeChunk(dto.ChatCompletionsStreamResponseChoiceDelta{ToolCalls: []dto.ToolCallResponse{call}}, nil)
			case "thinking_delta":
				_ = writeChunk(dto.ChatCompletionsStreamResponseChoiceDelta{
					ReasoningContent: common.GetPointer(event.Delta.Thinking),
				}, nil)
			}
		case "message_delta":
			finishReason := "stop"
			if event.Delta != nil && event.Delta.StopReason != "" {
				finishReason = dto.ClaudeStopReasonToOpenAI(event.Delta.StopReason)
			}
			_ = writeChunk(dto.ChatCompletionsStreamResponseChoiceDelta{}, &finishReason)
			if event.Usage != nil {
				merged := claudeUsageToUsage(*event.Usage)
				if usage != nil {
					merged.PromptTokens = usage.PromptTokens
					merged.TotalTokens = merged.PromptTokens + merged.CompletionTokens
					if usage.PromptTokensDetails.CachedTokens > 0 {
						merged.PromptTokensDetails.CachedTokens = usage.PromptTokensDetails.CachedTokens
					}
				}
				usage = merged
			}
		case "message_stop":
			_ = writeSSEDone(c)
			return usage, nil
		}
	}
	_ = writeSSEDone(c)
	return usage, nil
}

// copyStreamRaw 原样转发上游 SSE（用于 tokens.Claude 原生透传）。
func copyStreamRaw(c gin.Context, resp *http.Response, info *relaycommon.RelayInfo) {
	c.Response().Header().Set("Content-Type", "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().Header().Set("Connection", "keep-alive")
	c.Response().Header().Set("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	reader := bufio.NewReader(resp.Body)
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			_, _ = c.Response().Write(line)
			_ = flushWriter(c)
			info.SetFirstResponseTime()
		}
		if err != nil {
			return
		}
	}
}

// writeSSEData 输出一条 tokens.OpenAI 风格的 SSE 数据行。
func writeSSEData(c gin.Context, data any) error {
	encoded, err := common.Marshal(data)
	if err != nil {
		return err
	}
	c.Render(-1, common.CustomEvent{Data: "data: " + string(encoded)})
	return flushWriter(c)
}

// writeSSEDone 输出 tokens.OpenAI 流的结束标记。
func writeSSEDone(c gin.Context) error {
	c.Render(-1, common.CustomEvent{Data: "data: [DONE]"})
	return flushWriter(c)
}

// writeClaudeSSE 输出一条 Anthropic 风格事件（event + data 两行）。
func writeClaudeSSE(c gin.Context, eventType string, payload any) error {
	encoded, err := common.Marshal(payload)
	if err != nil {
		return err
	}
	c.Render(-1, common.CustomEvent{Data: "event: " + eventType + "\n"})
	c.Render(-1, common.CustomEvent{Data: "data: " + string(encoded)})
	return flushWriter(c)
}

// WriteClaudeSSE 输出一条 Anthropic SSE 事件，供其它包复用的导出包装。
func WriteClaudeSSE(c gin.Context, eventType string, payload any) error {
	return writeClaudeSSE(c, eventType, payload)
}

// flushWriter 刷新响应缓冲。
func flushWriter(c gin.Context) error {
	if flusher, ok := c.Response().(http.Flusher); ok {
		flusher.Flush()
	}
	return nil
}
