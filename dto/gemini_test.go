package dto

import (
	"testing"
)

func TestOpenAIAndGeminiRoundTrip(t *testing.T) {
	temp := 0.5
	maxTok := uint(256)
	assistantMsg := Message{Role: "assistant"}
	assistantMsg.SetToolCalls([]ToolCallResponse{{
		ID:   "call_1",
		Type: "function",
		Function: FunctionResponse{
			Name:      "get_weather",
			Arguments: `{"city":"Shanghai"}`,
		},
	}})

	oaiReq := &GeneralOpenAIRequest{
		Model:       "gemini-2.5-flash",
		Temperature: &temp,
		MaxTokens:   &maxTok,
		Stop:        "END",
		ResponseFormat: &ResponseFormat{
			Type: "json_object",
		},
		Messages: []Message{
			{Role: "system", Content: "You are helpful."},
			{
				Role: "user",
				Content: []MediaContent{
					{Type: ContentTypeText, Text: "Look at this"},
					{Type: ContentTypeImageURL, ImageUrl: map[string]any{"url": "data:image/png;base64,QUJDRA=="}},
				},
			},
			assistantMsg,
			{Role: "tool", ToolCallId: "call_1", Content: `{"temp":24}`},
		},
		Tools: []ToolCallRequest{{
			Type: "function",
			Function: FunctionRequest{
				Name:        "get_weather",
				Description: "Get weather",
				Parameters:  map[string]any{"type": "object"},
			},
		}},
	}

	gemReq, err := OpenAIRequestToGeminiRequest(oaiReq)
	if err != nil {
		t.Fatalf("OpenAIRequestToGeminiRequest: %v", err)
	}
	if gemReq.SystemInstruction == nil || len(gemReq.SystemInstruction.Parts) != 1 || gemReq.SystemInstruction.Parts[0].Text != "You are helpful." {
		t.Fatalf("unexpected systemInstruction: %+v", gemReq.SystemInstruction)
	}
	if len(gemReq.Contents) != 3 {
		t.Fatalf("expected 3 contents, got %d", len(gemReq.Contents))
	}
	if gemReq.Contents[0].Parts[1].InlineData == nil || gemReq.Contents[0].Parts[1].InlineData.MimeType != "image/png" {
		t.Fatalf("inlineData not parsed: %+v", gemReq.Contents[0].Parts)
	}
	if gemReq.Contents[1].Role != "model" || gemReq.Contents[1].Parts[0].FunctionCall == nil {
		t.Fatalf("assistant tool call not mapped: %+v", gemReq.Contents[1])
	}
	if gemReq.Contents[2].Parts[0].FunctionResponse == nil || gemReq.Contents[2].Parts[0].FunctionResponse.Name != "get_weather" {
		t.Fatalf("tool response function name not inferred: %+v", gemReq.Contents[2])
	}
	if gemReq.GenerationConfig == nil || gemReq.GenerationConfig.MaxOutputTokens != 256 || gemReq.GenerationConfig.ResponseMimeType != "application/json" {
		t.Fatalf("unexpected generationConfig: %+v", gemReq.GenerationConfig)
	}

	// Reverse convert back to OpenAI request
	backOAI := GeminiRequestToOpenAIRequest(gemReq, "gpt-4o", false)
	if len(backOAI.Messages) != 4 || backOAI.Messages[0].Role != "system" || backOAI.Messages[3].Role != "tool" {
		t.Fatalf("reverse OpenAI messages mismatch: %+v", backOAI.Messages)
	}
	if len(backOAI.Tools) != 1 || backOAI.Tools[0].Function.Name != "get_weather" {
		t.Fatalf("reverse OpenAI tools mismatch: %+v", backOAI.Tools)
	}
}

func TestGeminiResponseConversions(t *testing.T) {
	gemResp := &GeminiChatResponse{
		ResponseID:   "resp_gem_1",
		ModelVersion: "gemini-2.5-pro",
		Candidates: []GeminiChatCandidate{{
			Index:        0,
			FinishReason: "STOP",
			Content: GeminiChatContent{
				Role: "model",
				Parts: []GeminiPart{
					{Text: "thinking step", Thought: true},
					{Text: "final answer"},
				},
			},
		}},
		UsageMetadata: &GeminiUsageMetadata{
			PromptTokenCount:        12,
			CandidatesTokenCount:    8,
			ThoughtsTokenCount:      5,
			TotalTokenCount:         25,
			CachedContentTokenCount: 4,
		},
	}

	oaiResp, usage := GeminiResponseToOpenAIResponse(gemResp, "gemini-2.5-pro")
	if usage.PromptTokens != 12 || usage.CompletionTokens != 13 || usage.TotalTokens != 25 || usage.CompletionTokenDetails.ReasoningTokens != 5 {
		t.Fatalf("unexpected usage: %+v", usage)
	}
	if len(oaiResp.Choices) != 1 || oaiResp.Choices[0].Message.StringContent() != "final answer" || oaiResp.Choices[0].Message.ReasoningContent != "thinking step" {
		t.Fatalf("unexpected OpenAI response: %+v", oaiResp.Choices)
	}

	backGem := OpenAIResponseToGeminiResponse(oaiResp)
	if len(backGem.Candidates) != 1 || len(backGem.Candidates[0].Content.Parts) != 2 || !backGem.Candidates[0].Content.Parts[0].Thought {
		t.Fatalf("unexpected reverse Gemini response: %+v", backGem)
	}
}
