package compat

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"openapi/common"
	"openapi/dto"
)

// ResponsesRequestToChatCompletionsRequest 将 OpenAI /v1/responses 请求降级转换为 Chat Completions 请求，
// 用于上游未原生提供 Responses 接口的渠道（如 DeepSeek、Moonshot、MiniMax、Claude、Gemini）。
func ResponsesRequestToChatCompletionsRequest(req *dto.OpenAIResponsesRequest) (*dto.GeneralOpenAIRequest, error) {
	if req == nil {
		return nil, fmt.Errorf("responses request is nil")
	}
	out := &dto.GeneralOpenAIRequest{
		Model:       req.Model,
		Stream:      req.Stream,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		MaxTokens:   req.MaxOutputTokens,
	}
	if req.Reasoning != nil && req.Reasoning.Effort != "" {
		out.ReasoningEffort = req.Reasoning.Effort
	}

	if len(req.Instructions) > 0 {
		if sysText, ok := common.DecodeJSONString(req.Instructions); ok && strings.TrimSpace(sysText) != "" {
			out.Messages = append(out.Messages, dto.Message{
				Role:    "system",
				Content: sysText,
			})
		}
	}

	switch common.JSONKindOf(req.Input) {
	case common.JSONString:
		if text, ok := common.DecodeJSONString(req.Input); ok {
			out.Messages = append(out.Messages, dto.Message{
				Role:    "user",
				Content: text,
			})
		}
	case common.JSONArray:
		var rawItems []json.RawMessage
		if common.Unmarshal(req.Input, &rawItems) == nil {
			for _, raw := range rawItems {
				var item struct {
					Type      string          `json:"type"`
					Role      string          `json:"role"`
					Content   json.RawMessage `json:"content"`
					CallID    string          `json:"call_id"`
					Name      string          `json:"name"`
					Arguments json.RawMessage `json:"arguments"`
					Output    json.RawMessage `json:"output"`
				}
				if common.Unmarshal(raw, &item) != nil {
					continue
				}
				switch item.Type {
				case "function_call":
					args := dto.ResponsesArgumentsString(item.Arguments)
					msg := dto.Message{Role: "assistant"}
					msg.SetToolCalls([]dto.ToolCallResponse{{
						ID:   item.CallID,
						Type: "function",
						Function: dto.FunctionResponse{
							Name:      item.Name,
							Arguments: args,
						},
					}})
					out.Messages = append(out.Messages, msg)
				case "function_call_output":
					outStr := dto.ResponsesArgumentsString(item.Output)
					out.Messages = append(out.Messages, dto.Message{
						Role:       "tool",
						ToolCallId: item.CallID,
						Content:    outStr,
					})
				default:
					role := item.Role
					if role == "" {
						role = "user"
					}
					if text, ok := common.DecodeJSONString(item.Content); ok {
						out.Messages = append(out.Messages, dto.Message{Role: role, Content: text})
						continue
					}
					var parts []struct {
						Type     string          `json:"type"`
						Text     string          `json:"text"`
						ImageURL json.RawMessage `json:"image_url"`
					}
					if common.Unmarshal(item.Content, &parts) == nil && len(parts) > 0 {
						var media []dto.MediaContent
						for _, p := range parts {
							switch p.Type {
							case "input_text", "output_text", "text":
								media = append(media, dto.MediaContent{Type: dto.ContentTypeText, Text: p.Text})
							case "input_image":
								if imgURL, ok := common.DecodeJSONString(p.ImageURL); ok && imgURL != "" {
									media = append(media, dto.MediaContent{
										Type:     dto.ContentTypeImageURL,
										ImageUrl: map[string]any{"url": imgURL},
									})
								}
							}
						}
						if len(media) == 1 && media[0].Type == dto.ContentTypeText {
							out.Messages = append(out.Messages, dto.Message{Role: role, Content: media[0].Text})
						} else if len(media) > 0 {
							out.Messages = append(out.Messages, dto.Message{Role: role, Content: media})
						}
					}
				}
			}
		}
	}

	if len(req.Tools) > 0 {
		for _, t := range req.GetToolsMap() {
			if common.Interface2String(t["type"]) != "function" {
				continue
			}
			name := common.Interface2String(t["name"])
			desc := common.Interface2String(t["description"])
			params := t["parameters"]
			if fn, ok := t["function"].(map[string]any); ok {
				if name == "" {
					name = common.Interface2String(fn["name"])
				}
				if desc == "" {
					desc = common.Interface2String(fn["description"])
				}
				if params == nil {
					params = fn["parameters"]
				}
			}
			if name != "" {
				out.Tools = append(out.Tools, dto.ToolCallRequest{
					Type: "function",
					Function: dto.FunctionRequest{
						Name:        name,
						Description: desc,
						Parameters:  params,
					},
				})
			}
		}
	}

	return out, nil
}

// ChatCompletionsResponseToResponsesResponse 将 Chat Completions 响应包装为 OpenAI Responses 响应体。
func ChatCompletionsResponseToResponsesResponse(chatResp *dto.OpenAITextResponse, modelName string) *dto.OpenAIResponsesResponse {
	respID := "resp_" + common.GetRandomString(20)
	msgID := "msg_" + common.GetRandomString(20)
	now := int(time.Now().Unix())
	statusCompleted := json.RawMessage(`"completed"`)

	var outputs []dto.ResponsesOutput
	if chatResp != nil {
		for _, choice := range chatResp.Choices {
			if text := choice.Message.StringContent(); text != "" || len(choice.Message.ParseToolCalls()) == 0 {
				outputs = append(outputs, dto.ResponsesOutput{
					Type:   "message",
					ID:     msgID,
					Status: "completed",
					Role:   "assistant",
					Content: []dto.ResponsesOutputContent{{
						Type:        "output_text",
						Text:        text,
						Annotations: []any{},
					}},
				})
			}
			for _, tc := range choice.Message.ParseToolCalls() {
				argBytes, _ := common.Marshal(tc.Function.Arguments)
				outputs = append(outputs, dto.ResponsesOutput{
					Type:      "function_call",
					ID:        "fc_" + common.GetRandomString(16),
					Status:    "completed",
					CallId:    tc.ID,
					Name:      tc.Function.Name,
					Arguments: argBytes,
				})
			}
		}
	}

	var usage *dto.Usage
	if chatResp != nil {
		u := chatResp.Usage
		u.InputTokens = u.PromptTokens
		u.OutputTokens = u.CompletionTokens
		usage = &u
		if modelName == "" {
			modelName = chatResp.Model
		}
	}

	return &dto.OpenAIResponsesResponse{
		ID:        respID,
		Object:    "response",
		CreatedAt: now,
		Status:    statusCompleted,
		Model:     modelName,
		Output:    outputs,
		Usage:     usage,
	}
}
