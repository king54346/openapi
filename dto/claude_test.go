package dto

import (
	"openapi/common"
	"strings"
	"testing"
)

// TestClaudeRequestToOpenAI 覆盖 system、图片、tool_use/tool_result 与 tools 的转换。
func TestClaudeRequestToOpenAI(t *testing.T) {
	raw := `{
		"model": "claude-3-7-sonnet-20250219",
		"system": "be nice",
		"max_tokens": 128,
		"stream": true,
		"tools": [{"name":"get_weather","description":"weather","input_schema":{"type":"object"}}],
		"messages": [
			{"role":"user","content":[{"type":"text","text":"hi"},{"type":"image","source":{"type":"url","url":"https://example.com/a.png"}}]},
			{"role":"assistant","content":[{"type":"text","text":"calling"},{"type":"tool_use","id":"toolu_1","name":"get_weather","input":{"city":"sf"}}]},
			{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":[{"type":"text","text":"sunny"}]}]}
		]
	}`
	var request ClaudeRequest
	if err := common.Unmarshal([]byte(raw), &request); err != nil {
		t.Fatal(err)
	}

	converted, err := ClaudeRequestToOpenAIRequest(&request)
	if err != nil {
		t.Fatal(err)
	}
	if converted.Model != request.Model || converted.MaxTokens == nil || *converted.MaxTokens != 128 {
		t.Fatalf("model/max_tokens: %+v", converted)
	}
	if converted.Stream == nil || !*converted.Stream {
		t.Fatal("stream should be true")
	}
	if len(converted.Tools) != 1 || converted.Tools[0].Function.Name != "get_weather" {
		t.Fatalf("tools: %+v", converted.Tools)
	}
	// system + user + assistant + tool（只含 tool_result 的消息转成 tool 消息，不再额外产生 user 消息）
	if len(converted.Messages) != 4 {
		t.Fatalf("expected 4 messages, got %d: %+v", len(converted.Messages), converted.Messages)
	}
	if converted.Messages[0].Role != "system" || converted.Messages[0].StringContent() != "be nice" {
		t.Fatalf("system message: %+v", converted.Messages[0])
	}
	assistant := converted.Messages[2]
	if assistant.Role != "assistant" || len(assistant.ParseToolCalls()) != 1 {
		t.Fatalf("assistant tool_use: %+v", assistant)
	}
	if !strings.Contains(assistant.ParseToolCalls()[0].Function.Arguments, "sf") {
		t.Fatalf("tool arguments: %+v", assistant.ParseToolCalls()[0])
	}
	toolMessage := converted.Messages[3]
	if toolMessage.Role != "tool" || toolMessage.ToolCallId != "toolu_1" || toolMessage.StringContent() != "sunny" {
		t.Fatalf("tool_result message: %+v", toolMessage)
	}
}

// TestOpenAIResponseToClaude 校验结束原因与用量的反向映射。
func TestOpenAIResponseToClaude(t *testing.T) {
	response := &OpenAITextResponse{
		Model: "gpt-4o",
		Usage: Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
	}
	response.Choices = []OpenAITextResponseChoice{{
		Index:        0,
		Message:      Message{Role: "assistant", Content: "hello"},
		FinishReason: "length",
	}}

	claude := OpenAIResponseToClaudeResponse(response, "msg_1")
	if claude.StopReason != "max_tokens" {
		t.Fatalf("stop reason: %q", claude.StopReason)
	}
	if len(claude.Content) != 1 || claude.Content[0].Text != "hello" {
		t.Fatalf("content: %+v", claude.Content)
	}
	if claude.Usage.InputTokens != 10 || claude.Usage.OutputTokens != 5 {
		t.Fatalf("usage: %+v", claude.Usage)
	}

	// 反向：tokens.Claude 响应转 tokens.OpenAI
	back := OpenAIResponseFromClaude(claude, "req_1", "")
	if back.Object != "chat.completion" || len(back.Choices) != 1 {
		t.Fatalf("openai response: %+v", back)
	}
	if back.Choices[0].FinishReason != "length" || back.Usage.TotalTokens != 15 {
		t.Fatalf("openai choice/usage: %+v", back)
	}
}
