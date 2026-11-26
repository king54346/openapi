package compat

import (
	"encoding/json"
	"testing"

	"openapi/dto"
)

func TestResponsesRequestToChatCompletions(t *testing.T) {
	maxOut := uint(128)
	req := &dto.OpenAIResponsesRequest{
		Model:           "deepseek-chat",
		Instructions:    json.RawMessage(`"Be concise"`),
		MaxOutputTokens: &maxOut,
		Reasoning:       &dto.Reasoning{Effort: "high"},
		Input: json.RawMessage(`[
			{"role":"user","content":[{"type":"input_text","text":"What is the weather?"}]},
			{"type":"function_call","call_id":"call_abc","name":"lookup","arguments":"{\"q\":\"BJ\"}"},
			{"type":"function_call_output","call_id":"call_abc","output":"{\"sunny\":true}"}
		]`),
		Tools: json.RawMessage(`[{"type":"function","name":"lookup","description":"search","parameters":{"type":"object"}}]`),
	}

	chatReq, err := ResponsesRequestToChatCompletionsRequest(req)
	if err != nil {
		t.Fatalf("ResponsesRequestToChatCompletionsRequest: %v", err)
	}
	if len(chatReq.Messages) != 4 {
		t.Fatalf("expected 4 messages (system, user, assistant tool_call, tool), got %d: %+v", len(chatReq.Messages), chatReq.Messages)
	}
	if chatReq.Messages[0].Role != "system" || chatReq.Messages[0].StringContent() != "Be concise" {
		t.Fatalf("system message mismatch: %+v", chatReq.Messages[0])
	}
	if len(chatReq.Messages[2].ParseToolCalls()) != 1 || chatReq.Messages[2].ParseToolCalls()[0].ID != "call_abc" {
		t.Fatalf("assistant tool call mismatch: %+v", chatReq.Messages[2])
	}
	if chatReq.Messages[3].Role != "tool" || chatReq.Messages[3].ToolCallId != "call_abc" {
		t.Fatalf("tool output mismatch: %+v", chatReq.Messages[3])
	}
	if len(chatReq.Tools) != 1 || chatReq.Tools[0].Function.Name != "lookup" {
		t.Fatalf("tools mismatch: %+v", chatReq.Tools)
	}
	if chatReq.ReasoningEffort != "high" {
		t.Fatalf("reasoning effort mismatch: %q", chatReq.ReasoningEffort)
	}
}

func TestChatCompletionsResponseToResponsesResponse(t *testing.T) {
	msg := dto.Message{Role: "assistant", Content: "Sunny today"}
	msg.SetToolCalls([]dto.ToolCallResponse{{
		ID:   "call_9",
		Type: "function",
		Function: dto.FunctionResponse{
			Name:      "lookup",
			Arguments: `{"q":"BJ"}`,
		},
	}})
	chatResp := &dto.OpenAITextResponse{
		Id:    "chatcmpl-1",
		Model: "deepseek-chat",
		Choices: []dto.OpenAITextResponseChoice{{
			Index:        0,
			Message:      msg,
			FinishReason: "tool_calls",
		}},
		Usage: dto.Usage{PromptTokens: 10, CompletionTokens: 6, TotalTokens: 16},
	}

	resp := ChatCompletionsResponseToResponsesResponse(chatResp, "deepseek-chat")
	if resp.Object != "response" || len(resp.Output) != 2 {
		t.Fatalf("unexpected responses output: %+v", resp)
	}
	if resp.Output[0].Type != "message" || resp.Output[0].Content[0].Text != "Sunny today" {
		t.Fatalf("unexpected message output: %+v", resp.Output[0])
	}
	if resp.Output[1].Type != "function_call" || resp.Output[1].CallId != "call_9" || resp.Output[1].Name != "lookup" {
		t.Fatalf("unexpected function_call output: %+v", resp.Output[1])
	}
	if resp.Usage == nil || resp.Usage.InputTokens != 10 || resp.Usage.OutputTokens != 6 {
		t.Fatalf("unexpected usage: %+v", resp.Usage)
	}
}
