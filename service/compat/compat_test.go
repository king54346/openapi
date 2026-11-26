package compat

import (
	"openapi/dto"
	"openapi/service/tokens"
	"openapi/setting"
	"testing"
)

func newDisabledPolicy() setting.ChatCompletionsToResponsesPolicy {
	return setting.ChatCompletionsToResponsesPolicy{Enabled: false, AllChannels: true}
}

func newMatchingPolicy() setting.ChatCompletionsToResponsesPolicy {
	return setting.ChatCompletionsToResponsesPolicy{Enabled: true, AllChannels: true, ModelPatterns: []string{`^gpt-4o`}}
}

func newInvalidRegexPolicy() setting.ChatCompletionsToResponsesPolicy {
	return setting.ChatCompletionsToResponsesPolicy{Enabled: true, AllChannels: true, ModelPatterns: []string{`(`}}
}

func TestEstimateTokenBasics(t *testing.T) {
	if got := tokens.EstimateTokenByModel("gpt-4o", ""); got != 0 {
		t.Fatalf("empty text should be 0, got %d", got)
	}
	if got := tokens.EstimateTokenByModel("gpt-4o", "hello world"); got <= 0 {
		t.Fatalf("expected positive tokens, got %d", got)
	}
	if got := tokens.EstimateTokenByModel("claude-3-5-sonnet", "hello world"); got <= 0 {
		t.Fatalf("expected positive tokens, got %d", got)
	}
	if got := tokens.EstimateTokenByModel("gemini-1.5-pro", "hello world"); got <= 0 {
		t.Fatalf("expected positive tokens, got %d", got)
	}
	if got := tokens.EstimateTokenByModel("some-unknown-model", "hello"); got <= 0 {
		t.Fatalf("unknown model should fall back to tokens.OpenAI weights, got %d", got)
	}
}

func TestEstimateTokenCJKAndEmoji(t *testing.T) {
	cjk := tokens.EstimateTokenByModel("gpt-4o", "你好世界")
	latin := tokens.EstimateTokenByModel("gpt-4o", "hi")
	if cjk <= latin {
		t.Fatalf("expected CJK to cost more than short latin: cjk=%d latin=%d", cjk, latin)
	}
	if got := tokens.EstimateTokenByModel("gpt-4o", "hi 🙂"); got <= latin {
		t.Fatalf("emoji should add cost: base=%d got=%d", latin, got)
	}
}

func TestShouldChatCompletionsUseResponsesPolicy(t *testing.T) {
	if ShouldChatCompletionsUseResponsesPolicy(newDisabledPolicy(), 1, 1, "gpt-4o") {
		t.Fatal("disabled policy should not match")
	}
	if !ShouldChatCompletionsUseResponsesPolicy(newMatchingPolicy(), 1, 1, "gpt-4o-mini") {
		t.Fatal("expected matching policy to return true")
	}
	if ShouldChatCompletionsUseResponsesPolicy(newMatchingPolicy(), 1, 1, "claude-3") {
		t.Fatal("non-matching model should return false")
	}
	// Invalid regex must not panic or match.
	if ShouldChatCompletionsUseResponsesPolicy(newInvalidRegexPolicy(), 1, 1, "gpt-4o") {
		t.Fatal("invalid regex should not match")
	}
}

func TestResponsesResponseToChatCompletionsUsageFallback(t *testing.T) {
	resp := &dto.OpenAIResponsesResponse{
		Model: "gpt-4o",
		Usage: &dto.Usage{
			InputTokens:  5,
			OutputTokens: 7,
		},
		Output: []dto.ResponsesOutput{
			{Type: "message", Role: "assistant", Content: []dto.ResponsesOutputContent{{Type: "output_text", Text: "hi"}}},
		},
	}
	out, usage, err := ResponsesResponseToChatCompletionsResponse(resp, "chatcmpl-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if usage.PromptTokens != 5 || usage.CompletionTokens != 7 || usage.TotalTokens != 12 {
		t.Fatalf("unexpected usage: %+v", usage)
	}
	if len(out.Choices) != 1 || out.Choices[0].Message.Content != "hi" {
		t.Fatalf("unexpected message: %+v", out.Choices)
	}
	// PromptTokens-only legacy payload should populate both PromptTokens and InputTokens.
	resp2 := &dto.OpenAIResponsesResponse{
		Model: "gpt-4o",
		Usage: &dto.Usage{PromptTokens: 3, CompletionTokens: 4},
		Output: []dto.ResponsesOutput{
			{Type: "message", Role: "assistant", Content: []dto.ResponsesOutputContent{{Type: "output_text", Text: "ok"}}},
		},
	}
	_, usage2, err := ResponsesResponseToChatCompletionsResponse(resp2, "chatcmpl-2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if usage2.InputTokens != 3 || usage2.OutputTokens != 4 || usage2.TotalTokens != 7 {
		t.Fatalf("legacy usage not normalized: %+v", usage2)
	}
}

func TestResponsesResponseToChatCompletionsToolCallsKeepText(t *testing.T) {
	resp := &dto.OpenAIResponsesResponse{
		Model: "gpt-4o",
		Output: []dto.ResponsesOutput{
			{Type: "message", Role: "assistant", Content: []dto.ResponsesOutputContent{{Type: "output_text", Text: "thinking out loud"}}},
			{Type: "function_call", ID: "call_1", Name: "get_weather", Arguments: []byte(`{"city":"Paris"}`)},
		},
	}
	out, _, err := ResponsesResponseToChatCompletionsResponse(resp, "chatcmpl-3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Choices[0].FinishReason != "tool_calls" {
		t.Fatalf("expected tool_calls finish reason, got %q", out.Choices[0].FinishReason)
	}
	if len(out.Choices[0].Message.ParseToolCalls()) != 1 {
		t.Fatalf("expected 1 tool call, got %+v", out.Choices[0].Message.ToolCalls)
	}
	if out.Choices[0].Message.StringContent() == "" {
		t.Fatal("assistant text should be preserved alongside tool calls")
	}
}

func TestExtractOutputTextPrefersAssistant(t *testing.T) {
	resp := &dto.OpenAIResponsesResponse{
		Output: []dto.ResponsesOutput{
			{Type: "message", Role: "user", Content: []dto.ResponsesOutputContent{{Type: "output_text", Text: "user-text"}}},
			{Type: "message", Role: "assistant", Content: []dto.ResponsesOutputContent{{Type: "output_text", Text: "assistant-text"}}},
		},
	}
	if got := ExtractOutputTextFromResponses(resp); got != "assistant-text" {
		t.Fatalf("expected assistant text, got %q", got)
	}
	if got := ExtractOutputTextFromResponses(nil); got != "" {
		t.Fatalf("nil response should return empty, got %q", got)
	}
}

func TestCountTokenHelpers(t *testing.T) {
	if tokens.CountTextToken("", "gpt-4o") != 0 {
		t.Fatal("empty text should be 0")
	}
	if tokens.CountTextToken("hello", "gpt-4o") <= 0 {
		t.Fatal("expected positive count")
	}
	if tokens.CountTokenInput([]string{"hello", " ", "world"}, "gpt-4o") <= 0 {
		t.Fatal("expected positive count for string slice")
	}
	if !tokens.ValidUsage(&dto.Usage{PromptTokens: 1}) {
		t.Fatal("usage with prompt tokens should be valid")
	}
	if tokens.ValidUsage(&dto.Usage{}) {
		t.Fatal("empty usage should be invalid")
	}
	if tokens.ValidUsage(nil) {
		t.Fatal("nil usage should be invalid")
	}
}
