package service

import (
	"errors"
	"strings"

	"openapi/dto"
)

func ResponsesResponseToChatCompletionsResponse(resp *dto.OpenAIResponsesResponse, id string) (*dto.OpenAITextResponse, *dto.Usage, error) {
	if resp == nil {
		return nil, nil, errors.New("response is nil")
	}

	text := ExtractOutputTextFromResponses(resp)

	usage := normalizeResponsesUsage(resp.Usage)

	created := resp.CreatedAt

	toolCalls := extractResponsesToolCalls(resp.Output)

	finishReason := "stop"
	if len(toolCalls) > 0 {
		finishReason = "tool_calls"
	}

	msg := dto.Message{
		Role:    "assistant",
		Content: text,
	}
	if len(toolCalls) > 0 {
		msg.SetToolCalls(toolCalls)
	}

	out := &dto.OpenAITextResponse{
		Id:      id,
		Object:  "chat.completion",
		Created: created,
		Model:   resp.Model,
		Choices: []dto.OpenAITextResponseChoice{
			{
				Index:        0,
				Message:      msg,
				FinishReason: finishReason,
			},
		},
		Usage: *usage,
	}

	return out, usage, nil
}

func normalizeResponsesUsage(src *dto.Usage) *dto.Usage {
	usage := &dto.Usage{}
	if src == nil {
		return usage
	}
	usage.PromptTokens = src.PromptTokens
	if usage.PromptTokens == 0 {
		usage.PromptTokens = src.InputTokens
	}
	usage.InputTokens = src.InputTokens
	if usage.InputTokens == 0 {
		usage.InputTokens = usage.PromptTokens
	}
	usage.CompletionTokens = src.CompletionTokens
	if usage.CompletionTokens == 0 {
		usage.CompletionTokens = src.OutputTokens
	}
	usage.OutputTokens = src.OutputTokens
	if usage.OutputTokens == 0 {
		usage.OutputTokens = usage.CompletionTokens
	}
	usage.TotalTokens = src.TotalTokens
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	if src.InputTokensDetails != nil {
		usage.PromptTokensDetails.CachedTokens = src.InputTokensDetails.CachedTokens
		usage.PromptTokensDetails.ImageTokens = src.InputTokensDetails.ImageTokens
		usage.PromptTokensDetails.AudioTokens = src.InputTokensDetails.AudioTokens
	}
	if src.CompletionTokenDetails.ReasoningTokens != 0 {
		usage.CompletionTokenDetails.ReasoningTokens = src.CompletionTokenDetails.ReasoningTokens
	}
	return usage
}

func extractResponsesToolCalls(outputs []dto.ResponsesOutput) []dto.ToolCallResponse {
	var toolCalls []dto.ToolCallResponse
	for _, out := range outputs {
		if out.Type != "function_call" {
			continue
		}
		name := strings.TrimSpace(out.Name)
		if name == "" {
			continue
		}
		callID := strings.TrimSpace(out.CallId)
		if callID == "" {
			callID = strings.TrimSpace(out.ID)
		}
		toolCalls = append(toolCalls, dto.ToolCallResponse{
			ID:   callID,
			Type: "function",
			Function: dto.FunctionResponse{
				Name:      name,
				Arguments: out.ArgumentsString(),
			},
		})
	}
	return toolCalls
}

func ExtractOutputTextFromResponses(resp *dto.OpenAIResponsesResponse) string {
	if resp == nil || len(resp.Output) == 0 {
		return ""
	}

	var sb strings.Builder

	// Prefer assistant message outputs.
	for _, out := range resp.Output {
		if out.Type != "message" {
			continue
		}
		if out.Role != "" && out.Role != "assistant" {
			continue
		}
		for _, c := range out.Content {
			if c.Type == "output_text" && c.Text != "" {
				sb.WriteString(c.Text)
			}
		}
	}
	if sb.Len() > 0 {
		return sb.String()
	}
	for _, out := range resp.Output {
		for _, c := range out.Content {
			if c.Text != "" {
				sb.WriteString(c.Text)
			}
		}
	}
	return sb.String()
}
