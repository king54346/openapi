package dto

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"openapi/common"
	"openapi/types"

	gin "github.com/king54346/gin-tiny"
)

// Google Gemini GenerateContent / EmbedContent 协议结构及与 OpenAI 协议的双向转换。

type GeminiInlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

type GeminiFileData struct {
	MimeType string `json:"mimeType,omitempty"`
	FileURI  string `json:"fileUri"`
}

type GeminiFunctionCall struct {
	ID   string         `json:"id,omitempty"`
	Name string         `json:"name"`
	Args map[string]any `json:"args,omitempty"`
}

type GeminiFunctionResponse struct {
	ID       string         `json:"id,omitempty"`
	Name     string         `json:"name"`
	Response map[string]any `json:"response"`
}

type GeminiPart struct {
	Text             string                  `json:"text,omitempty"`
	Thought          bool                    `json:"thought,omitempty"`
	InlineData       *GeminiInlineData       `json:"inlineData,omitempty"`
	FileData         *GeminiFileData         `json:"fileData,omitempty"`
	FunctionCall     *GeminiFunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *GeminiFunctionResponse `json:"functionResponse,omitempty"`
}

type GeminiChatContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []GeminiPart `json:"parts"`
}

type GeminiFunctionDeclaration struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters,omitempty"`
}

type GeminiChatTool struct {
	FunctionDeclarations []GeminiFunctionDeclaration `json:"functionDeclarations,omitempty"`
	GoogleSearch         any                         `json:"googleSearch,omitempty"`
	CodeExecution        any                         `json:"codeExecution,omitempty"`
}

type GeminiFunctionCallingConfig struct {
	Mode                 string   `json:"mode,omitempty"`
	AllowedFunctionNames []string `json:"allowedFunctionNames,omitempty"`
}

type GeminiToolConfig struct {
	FunctionCallingConfig *GeminiFunctionCallingConfig `json:"functionCallingConfig,omitempty"`
}

type GeminiThinkingConfig struct {
	IncludeThoughts bool `json:"includeThoughts,omitempty"`
	ThinkingBudget  *int `json:"thinkingBudget,omitempty"`
}

type GeminiGenerationConfig struct {
	Temperature      *float64              `json:"temperature,omitempty"`
	TopP             *float64              `json:"topP,omitempty"`
	TopK             *int                  `json:"topK,omitempty"`
	MaxOutputTokens  int                   `json:"maxOutputTokens,omitempty"`
	CandidateCount   int                   `json:"candidateCount,omitempty"`
	StopSequences    []string              `json:"stopSequences,omitempty"`
	ResponseMimeType string                `json:"responseMimeType,omitempty"`
	ResponseSchema   any                   `json:"responseSchema,omitempty"`
	Seed             *int                  `json:"seed,omitempty"`
	ThinkingConfig   *GeminiThinkingConfig `json:"thinkingConfig,omitempty"`
}

type GeminiSafetySetting struct {
	Category  string `json:"category"`
	Threshold string `json:"threshold"`
}

// GeminiChatRequest Gemini generateContent / streamGenerateContent / embedContent 请求体。
type GeminiChatRequest struct {
	Contents          []GeminiChatContent     `json:"contents,omitempty"`
	SystemInstruction *GeminiChatContent      `json:"systemInstruction,omitempty"`
	Tools             []GeminiChatTool        `json:"tools,omitempty"`
	ToolConfig        *GeminiToolConfig       `json:"toolConfig,omitempty"`
	SafetySettings    []GeminiSafetySetting   `json:"safetySettings,omitempty"`
	GenerationConfig  *GeminiGenerationConfig `json:"generationConfig,omitempty"`
	// EmbedContent 请求兼容字段
	Content              *GeminiChatContent `json:"content,omitempty"`
	TaskType             string             `json:"taskType,omitempty"`
	Title                string             `json:"title,omitempty"`
	OutputDimensionality *int               `json:"outputDimensionality,omitempty"`

	// 进程内元数据，不序列化到上游 JSON
	Model  string `json:"-"`
	Action string `json:"-"`
	Stream bool   `json:"-"`
}

func (r *GeminiChatRequest) GetTokenCountMeta() *types.TokenCountMeta {
	var texts []string
	if r.SystemInstruction != nil {
		for _, p := range r.SystemInstruction.Parts {
			if p.Text != "" {
				texts = append(texts, p.Text)
			}
		}
	}
	for _, c := range r.Contents {
		for _, p := range c.Parts {
			if p.Text != "" {
				texts = append(texts, p.Text)
			}
		}
	}
	if r.Content != nil {
		for _, p := range r.Content.Parts {
			if p.Text != "" {
				texts = append(texts, p.Text)
			}
		}
	}
	maxTokens := 0
	if r.GenerationConfig != nil {
		maxTokens = r.GenerationConfig.MaxOutputTokens
	}
	return &types.TokenCountMeta{
		CombineText: strings.Join(texts, "\n"),
		MaxTokens:   maxTokens,
	}
}

func (r *GeminiChatRequest) IsStream(c gin.Context) bool {
	if r.Stream {
		return true
	}
	if c != nil && c.Request() != nil {
		if strings.Contains(c.Request().URL.Path, ":streamGenerateContent") {
			return true
		}
	}
	return false
}

func (r *GeminiChatRequest) SetModelName(modelName string) {
	if modelName != "" {
		r.Model = modelName
	}
}

type GeminiUsageMetadata struct {
	PromptTokenCount        int `json:"promptTokenCount"`
	CandidatesTokenCount    int `json:"candidatesTokenCount"`
	TotalTokenCount         int `json:"totalTokenCount"`
	CachedContentTokenCount int `json:"cachedContentTokenCount,omitempty"`
	ThoughtsTokenCount      int `json:"thoughtsTokenCount,omitempty"`
}

type GeminiChatCandidate struct {
	Content       GeminiChatContent `json:"content"`
	FinishReason  string            `json:"finishReason,omitempty"`
	Index         int               `json:"index"`
	SafetyRatings any               `json:"safetyRatings,omitempty"`
}

type GeminiChatResponse struct {
	Candidates    []GeminiChatCandidate `json:"candidates,omitempty"`
	UsageMetadata *GeminiUsageMetadata  `json:"usageMetadata,omitempty"`
	ModelVersion  string                `json:"modelVersion,omitempty"`
	ResponseID    string                `json:"responseId,omitempty"`
}

type GeminiEmbeddingValues struct {
	Values []float64 `json:"values"`
}

type GeminiEmbeddingRequest struct {
	Model                string            `json:"model,omitempty"`
	Content              GeminiChatContent `json:"content"`
	TaskType             string            `json:"taskType,omitempty"`
	OutputDimensionality *int              `json:"outputDimensionality,omitempty"`
}

type GeminiBatchEmbeddingRequest struct {
	Requests []GeminiEmbeddingRequest `json:"requests"`
}

type GeminiEmbeddingResponse struct {
	Embedding  *GeminiEmbeddingValues  `json:"embedding,omitempty"`
	Embeddings []GeminiEmbeddingValues `json:"embeddings,omitempty"`
}

type GeminiModelInfo struct {
	Name                       string   `json:"name"`
	Version                    string   `json:"version,omitempty"`
	DisplayName                string   `json:"displayName,omitempty"`
	Description                string   `json:"description,omitempty"`
	SupportedGenerationMethods []string `json:"supportedGenerationMethods,omitempty"`
}

type GeminiModelListResponse struct {
	Models []GeminiModelInfo `json:"models"`
}

// GeminiUsageToUsage 将 Gemini UsageMetadata 转为通用 Usage。
func GeminiUsageToUsage(meta *GeminiUsageMetadata) *Usage {
	if meta == nil {
		return &Usage{}
	}
	total := meta.TotalTokenCount
	completion := meta.CandidatesTokenCount + meta.ThoughtsTokenCount
	if total == 0 {
		total = meta.PromptTokenCount + completion
	}
	u := &Usage{
		PromptTokens:     meta.PromptTokenCount,
		CompletionTokens: completion,
		TotalTokens:      total,
	}
	if meta.CachedContentTokenCount > 0 {
		u.PromptTokensDetails.CachedTokens = meta.CachedContentTokenCount
		u.PromptCacheHitTokens = meta.CachedContentTokenCount
	}
	if meta.ThoughtsTokenCount > 0 {
		u.CompletionTokenDetails.ReasoningTokens = meta.ThoughtsTokenCount
	}
	return u
}

// OpenAIRequestToGeminiRequest 把 OpenAI Chat Completions 请求转换为 Gemini generateContent 请求。
func OpenAIRequestToGeminiRequest(req *GeneralOpenAIRequest) (*GeminiChatRequest, error) {
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	out := &GeminiChatRequest{
		Model:  req.Model,
		Stream: req.Stream != nil && *req.Stream,
	}

	var sysParts []GeminiPart
	callNameByID := make(map[string]string)

	for _, msg := range req.Messages {
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		if role == "system" || role == "developer" {
			if text := strings.TrimSpace(msg.StringContent()); text != "" {
				sysParts = append(sysParts, GeminiPart{Text: text})
			}
			continue
		}

		if role == "tool" || role == "function" {
			funcName := ""
			if msg.Name != nil && *msg.Name != "" {
				funcName = *msg.Name
			} else if n, ok := callNameByID[msg.ToolCallId]; ok {
				funcName = n
			}
			if funcName == "" {
				funcName = "tool_result"
			}
			var respObj map[string]any
			rawContent := msg.StringContent()
			if err := common.Unmarshal([]byte(rawContent), &respObj); err != nil || respObj == nil {
				respObj = map[string]any{"content": rawContent}
			}
			out.Contents = append(out.Contents, GeminiChatContent{
				Role: "user",
				Parts: []GeminiPart{{
					FunctionResponse: &GeminiFunctionResponse{
						ID:       msg.ToolCallId,
						Name:     funcName,
						Response: respObj,
					},
				}},
			})
			continue
		}

		geminiRole := "user"
		if role == "assistant" || role == "model" {
			geminiRole = "model"
		}

		var parts []GeminiPart
		if msg.IsStringContent() {
			if text := msg.StringContent(); text != "" {
				parts = append(parts, GeminiPart{Text: text})
			}
		} else {
			for _, part := range msg.ParseContent() {
				switch part.Type {
				case ContentTypeText:
					if part.Text != "" {
						parts = append(parts, GeminiPart{Text: part.Text})
					}
				case ContentTypeImageURL:
					img := part.GetImageMedia()
					if img != nil && img.Url != "" {
						if mime, b64, ok := parseDataURL(img.Url); ok {
							parts = append(parts, GeminiPart{
								InlineData: &GeminiInlineData{MimeType: mime, Data: b64},
							})
						} else {
							mime := img.MimeType
							if mime == "" {
								mime = "image/jpeg"
							}
							parts = append(parts, GeminiPart{
								FileData: &GeminiFileData{MimeType: mime, FileURI: img.Url},
							})
						}
					}
				}
			}
		}

		for _, tc := range msg.ParseToolCalls() {
			if tc.ID != "" && tc.Function.Name != "" {
				callNameByID[tc.ID] = tc.Function.Name
			}
			var args map[string]any
			if tc.Function.Arguments != "" {
				_ = common.Unmarshal([]byte(tc.Function.Arguments), &args)
			}
			if args == nil {
				args = map[string]any{}
			}
			parts = append(parts, GeminiPart{
				FunctionCall: &GeminiFunctionCall{
					ID:   tc.ID,
					Name: tc.Function.Name,
					Args: args,
				},
			})
		}

		if len(parts) > 0 {
			out.Contents = append(out.Contents, GeminiChatContent{
				Role:  geminiRole,
				Parts: parts,
			})
		}
	}

	if len(sysParts) > 0 {
		out.SystemInstruction = &GeminiChatContent{Parts: sysParts}
	}

	// Tools 转换
	if len(req.Tools) > 0 {
		var decls []GeminiFunctionDeclaration
		for _, t := range req.Tools {
			if t.Type == "function" && t.Function.Name != "" {
				decls = append(decls, GeminiFunctionDeclaration{
					Name:        t.Function.Name,
					Description: t.Function.Description,
					Parameters:  t.Function.Parameters,
				})
			}
		}
		if len(decls) > 0 {
			out.Tools = []GeminiChatTool{{FunctionDeclarations: decls}}
		}
	}

	// GenerationConfig 转换
	genCfg := &GeminiGenerationConfig{}
	hasGenCfg := false
	if req.Temperature != nil {
		genCfg.Temperature = req.Temperature
		hasGenCfg = true
	}
	if req.TopP != nil {
		genCfg.TopP = req.TopP
		hasGenCfg = true
	}
	if maxTok := req.GetMaxTokens(); maxTok > 0 {
		genCfg.MaxOutputTokens = int(maxTok)
		hasGenCfg = true
	}
	if req.Stop != nil {
		switch stopVal := req.Stop.(type) {
		case string:
			if stopVal != "" {
				genCfg.StopSequences = []string{stopVal}
				hasGenCfg = true
			}
		case []any:
			for _, item := range stopVal {
				if s, ok := item.(string); ok && s != "" {
					genCfg.StopSequences = append(genCfg.StopSequences, s)
				}
			}
			if len(genCfg.StopSequences) > 0 {
				hasGenCfg = true
			}
		}
	}
	if req.ResponseFormat != nil && req.ResponseFormat.Type == "json_object" {
		genCfg.ResponseMimeType = "application/json"
		hasGenCfg = true
	}
	if hasGenCfg {
		out.GenerationConfig = genCfg
	}

	return out, nil
}

// GeminiRequestToOpenAIRequest 把 Gemini 请求转换为 OpenAI Chat Completions 请求（用于跨协议转发到 OpenAI 渠道）。
func GeminiRequestToOpenAIRequest(req *GeminiChatRequest, modelName string, stream bool) *GeneralOpenAIRequest {
	out := &GeneralOpenAIRequest{
		Model:  modelName,
		Stream: &stream,
	}
	if req == nil {
		return out
	}

	if req.SystemInstruction != nil {
		var sb strings.Builder
		for _, p := range req.SystemInstruction.Parts {
			if p.Text != "" {
				if sb.Len() > 0 {
					sb.WriteByte('\n')
				}
				sb.WriteString(p.Text)
			}
		}
		if sb.Len() > 0 {
			out.Messages = append(out.Messages, Message{
				Role:    "system",
				Content: sb.String(),
			})
		}
	}

	callIdx := 0
	for _, c := range req.Contents {
		role := "user"
		if strings.EqualFold(c.Role, "model") || strings.EqualFold(c.Role, "assistant") {
			role = "assistant"
		}
		var textParts []MediaContent
		var toolCalls []ToolCallResponse
		var toolMessages []Message

		for _, p := range c.Parts {
			if p.Thought {
				continue
			}
			if p.Text != "" {
				textParts = append(textParts, MediaContent{Type: ContentTypeText, Text: p.Text})
			}
			if p.InlineData != nil && p.InlineData.Data != "" {
				mime := p.InlineData.MimeType
				if mime == "" {
					mime = "image/jpeg"
				}
				dataURL := "data:" + mime + ";base64," + p.InlineData.Data
				textParts = append(textParts, MediaContent{
					Type:     ContentTypeImageURL,
					ImageUrl: map[string]any{"url": dataURL},
				})
			}
			if p.FunctionCall != nil {
				callIdx++
				callID := p.FunctionCall.ID
				if callID == "" {
					callID = fmt.Sprintf("call_gemini_%d", callIdx)
				}
				argsBytes, _ := common.Marshal(p.FunctionCall.Args)
				toolCalls = append(toolCalls, ToolCallResponse{
					ID:   callID,
					Type: "function",
					Function: FunctionResponse{
						Name:      p.FunctionCall.Name,
						Arguments: string(argsBytes),
					},
				})
			}
			if p.FunctionResponse != nil {
				callID := p.FunctionResponse.ID
				if callID == "" {
					callID = p.FunctionResponse.Name
				}
				respBytes, _ := common.Marshal(p.FunctionResponse.Response)
				nameCopy := p.FunctionResponse.Name
				toolMessages = append(toolMessages, Message{
					Role:       "tool",
					Name:       &nameCopy,
					ToolCallId: callID,
					Content:    string(respBytes),
				})
			}
		}

		if len(textParts) > 0 || len(toolCalls) > 0 {
			msg := Message{Role: role}
			if len(textParts) == 1 && textParts[0].Type == ContentTypeText {
				msg.Content = textParts[0].Text
			} else if len(textParts) > 0 {
				msg.Content = textParts
			}
			if len(toolCalls) > 0 {
				msg.SetToolCalls(toolCalls)
			}
			out.Messages = append(out.Messages, msg)
		}
		out.Messages = append(out.Messages, toolMessages...)
	}

	for _, toolGroup := range req.Tools {
		for _, decl := range toolGroup.FunctionDeclarations {
			out.Tools = append(out.Tools, ToolCallRequest{
				Type: "function",
				Function: FunctionRequest{
					Name:        decl.Name,
					Description: decl.Description,
					Parameters:  decl.Parameters,
				},
			})
		}
	}

	if cfg := req.GenerationConfig; cfg != nil {
		out.Temperature = cfg.Temperature
		out.TopP = cfg.TopP
		if cfg.MaxOutputTokens > 0 {
			maxTok := uint(cfg.MaxOutputTokens)
			out.MaxTokens = &maxTok
		}
		if len(cfg.StopSequences) > 0 {
			out.Stop = cfg.StopSequences
		}
		if cfg.ResponseMimeType == "application/json" {
			out.ResponseFormat = &ResponseFormat{Type: "json_object"}
		}
	}

	return out
}

// GeminiResponseToOpenAIResponse 把 Gemini 非流式响应转换为 OpenAI Chat Completions 响应。
func GeminiResponseToOpenAIResponse(resp *GeminiChatResponse, modelName string) (*OpenAITextResponse, *Usage) {
	usage := GeminiUsageToUsage(resp.UsageMetadata)
	id := resp.ResponseID
	if id == "" {
		id = "chatcmpl-" + common.GetRandomString(16)
	}
	model := modelName
	if model == "" && resp.ModelVersion != "" {
		model = resp.ModelVersion
	}

	var choices []OpenAITextResponseChoice
	for i, cand := range resp.Candidates {
		var textBuilder, thoughtBuilder strings.Builder
		var toolCalls []ToolCallResponse
		for j, part := range cand.Content.Parts {
			if part.Thought {
				thoughtBuilder.WriteString(part.Text)
				continue
			}
			if part.Text != "" {
				textBuilder.WriteString(part.Text)
			}
			if part.FunctionCall != nil {
				callID := part.FunctionCall.ID
				if callID == "" {
					callID = fmt.Sprintf("call_%d_%d", i, j)
				}
				argsBytes, _ := common.Marshal(part.FunctionCall.Args)
				toolCalls = append(toolCalls, ToolCallResponse{
					ID:   callID,
					Type: "function",
					Function: FunctionResponse{
						Name:      part.FunctionCall.Name,
						Arguments: string(argsBytes),
					},
				})
			}
		}

		msg := Message{
			Role:             "assistant",
			Content:          textBuilder.String(),
			ReasoningContent: thoughtBuilder.String(),
		}
		finishReason := mapGeminiFinishReason(cand.FinishReason)
		if len(toolCalls) > 0 {
			msg.SetToolCalls(toolCalls)
			finishReason = "tool_calls"
		}
		choices = append(choices, OpenAITextResponseChoice{
			Index:        i,
			Message:      msg,
			FinishReason: finishReason,
		})
	}

	return &OpenAITextResponse{
		Id:      id,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: choices,
		Usage:   *usage,
	}, usage
}

// OpenAIResponseToGeminiResponse 把 OpenAI 非流式响应转换为 Gemini 响应。
func OpenAIResponseToGeminiResponse(resp *OpenAITextResponse) *GeminiChatResponse {
	if resp == nil {
		return &GeminiChatResponse{}
	}
	out := &GeminiChatResponse{
		ResponseID:   resp.Id,
		ModelVersion: resp.Model,
		UsageMetadata: &GeminiUsageMetadata{
			PromptTokenCount:     resp.Usage.PromptTokens,
			CandidatesTokenCount: resp.Usage.CompletionTokens,
			TotalTokenCount:      resp.Usage.TotalTokens,
		},
	}
	for _, choice := range resp.Choices {
		var parts []GeminiPart
		if choice.Message.ReasoningContent != "" {
			parts = append(parts, GeminiPart{Text: choice.Message.ReasoningContent, Thought: true})
		}
		if text := choice.Message.StringContent(); text != "" {
			parts = append(parts, GeminiPart{Text: text})
		}
		for _, tc := range choice.Message.ParseToolCalls() {
			var args map[string]any
			if tc.Function.Arguments != "" {
				_ = common.Unmarshal([]byte(tc.Function.Arguments), &args)
			}
			parts = append(parts, GeminiPart{
				FunctionCall: &GeminiFunctionCall{
					ID:   tc.ID,
					Name: tc.Function.Name,
					Args: args,
				},
			})
		}
		out.Candidates = append(out.Candidates, GeminiChatCandidate{
			Index:        choice.Index,
			Content:      GeminiChatContent{Role: "model", Parts: parts},
			FinishReason: mapOpenAIFinishReasonToGemini(choice.FinishReason),
		})
	}
	return out
}

func mapGeminiFinishReason(reason string) string {
	switch strings.ToUpper(reason) {
	case "MAX_TOKENS":
		return "length"
	case "SAFETY", "RECITATION", "PROHIBITED_CONTENT", "BLOCKLIST":
		return "content_filter"
	case "STOP", "":
		return "stop"
	default:
		return "stop"
	}
}

func mapOpenAIFinishReasonToGemini(reason string) string {
	switch reason {
	case "length":
		return "MAX_TOKENS"
	case "content_filter":
		return "SAFETY"
	default:
		return "STOP"
	}
}

func parseDataURL(raw string) (mimeType string, b64 string, ok bool) {
	if !strings.HasPrefix(raw, "data:") {
		return "", "", false
	}
	rest := strings.TrimPrefix(raw, "data:")
	semi := strings.IndexByte(rest, ';')
	comma := strings.IndexByte(rest, ',')
	if semi <= 0 || comma <= semi {
		return "", "", false
	}
	if !strings.EqualFold(rest[semi+1:comma], "base64") {
		return "", "", false
	}
	return rest[:semi], rest[comma+1:], true
}

// Ensure json.RawMessage import is referenced if needed
var _ json.RawMessage
