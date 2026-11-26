package dto

import (
	"encoding/json"
	"fmt"
	"openapi/common"
	"openapi/types"
	"strings"

	gin "github.com/king54346/gin-tiny"
)

// Anthropic Claude Messages API 协议（/v1/messages）以及与 OpenAI Chat Completions 的双向转换。

// ClaudeRequest Claude Messages 请求体。
type ClaudeRequest struct {
	Model         string          `json:"model"`
	System        json.RawMessage `json:"system,omitempty"` // 字符串或内容块数组
	Messages      []ClaudeMessage `json:"messages"`
	MaxTokens     int             `json:"max_tokens,omitempty"`
	Temperature   *float64        `json:"temperature,omitempty"`
	TopP          *float64        `json:"top_p,omitempty"`
	TopK          *int            `json:"top_k,omitempty"`
	Stream        bool            `json:"stream,omitempty"`
	StopSequences []string        `json:"stop_sequences,omitempty"`
	Tools         []ClaudeTool    `json:"tools,omitempty"`
	ToolChoice    json.RawMessage `json:"tool_choice,omitempty"`
	Thinking      json.RawMessage `json:"thinking,omitempty"`
	Metadata      json.RawMessage `json:"metadata,omitempty"`
}

// ClaudeMessage 请求中的一条消息；content 可以是字符串或内容块数组。
type ClaudeMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// ClaudeTool Claude 工具定义（input_schema 兼容 JSON Schema）。
type ClaudeTool struct {
	Type        string `json:"type,omitempty"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	InputSchema any    `json:"input_schema,omitempty"`
}

// ClaudeContentBlock 内容块，请求与响应共用。
type ClaudeContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	// tool_use
	Id    string `json:"id,omitempty"`
	Name  string `json:"name,omitempty"`
	Input any    `json:"input,omitempty"`
	// tool_result
	ToolUseId string          `json:"tool_use_id,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
	// image
	Source *ClaudeImageSource `json:"source,omitempty"`
}

// ClaudeImageSource 图片块的数据来源（base64 或 url）。
type ClaudeImageSource struct {
	Type      string `json:"type"` // base64 / url
	MediaType string `json:"media_type,omitempty"`
	Data      string `json:"data,omitempty"`
	Url       string `json:"url,omitempty"`
}

// ClaudeUsage Claude 用量（含缓存读写）。
type ClaudeUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
}

// ClaudeResponse Claude Messages 响应体。
type ClaudeResponse struct {
	Id           string               `json:"id"`
	Type         string               `json:"type"`
	Role         string               `json:"role"`
	Model        string               `json:"model"`
	Content      []ClaudeContentBlock `json:"content"`
	StopReason   string               `json:"stop_reason,omitempty"`
	StopSequence *string              `json:"stop_sequence"`
	Usage        ClaudeUsage          `json:"usage"`
}

// ClaudeError Claude 错误响应体。
type ClaudeError struct {
	Type  string            `json:"type"`
	Error ClaudeErrorDetail `json:"error"`
}

// ClaudeErrorDetail 错误详情。
type ClaudeErrorDetail struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// ClaudeStreamEvent Claude 流式事件。
type ClaudeStreamEvent struct {
	Type         string              `json:"type"`
	Index        *int                `json:"index,omitempty"`
	Message      *ClaudeResponse     `json:"message,omitempty"`
	ContentBlock *ClaudeContentBlock `json:"content_block,omitempty"`
	Delta        *ClaudeStreamDelta  `json:"delta,omitempty"`
	Usage        *ClaudeUsage        `json:"usage,omitempty"`
	Error        *ClaudeErrorDetail  `json:"error,omitempty"`
}

// ClaudeStreamDelta 流式增量。
type ClaudeStreamDelta struct {
	Type        string `json:"type,omitempty"`
	Text        string `json:"text,omitempty"`
	PartialJson string `json:"partial_json,omitempty"`
	StopReason  string `json:"stop_reason,omitempty"`
	Thinking    string `json:"thinking,omitempty"`
}

func (r *ClaudeRequest) GetTokenCountMeta() *types.TokenCountMeta {
	meta := &types.TokenCountMeta{TokenType: types.TokenTypeTokenizer, MaxTokens: r.MaxTokens}
	texts := make([]string, 0, len(r.Messages)+1)
	if system := r.SystemText(); system != "" {
		texts = append(texts, system)
	}
	for _, message := range r.Messages {
		if message.Role != "" {
			texts = append(texts, message.Role)
		}
		for _, block := range message.ParseContent() {
			if block.Type == ContentTypeText && block.Text != "" {
				texts = append(texts, block.Text)
			}
		}
	}
	meta.MessagesCount = len(r.Messages)
	meta.ToolsCount = len(r.Tools)
	meta.CombineText = strings.Join(texts, "\n")
	return meta
}

func (r *ClaudeRequest) IsStream(c gin.Context) bool { return r.Stream }

func (r *ClaudeRequest) SetModelName(modelName string) {
	if modelName != "" {
		r.Model = modelName
	}
}

// SystemText 把 system 字段（字符串或内容块数组）拼成纯文本。
func (r *ClaudeRequest) SystemText() string {
	if len(r.System) == 0 {
		return ""
	}
	var text string
	if err := common.Unmarshal(r.System, &text); err == nil {
		return strings.TrimSpace(text)
	}
	var blocks []ClaudeContentBlock
	if err := common.Unmarshal(r.System, &blocks); err != nil {
		return ""
	}
	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if block.Text != "" {
			parts = append(parts, block.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// ParseContent 解析消息内容为内容块列表（字符串内容转成单个 text 块）。
func (m *ClaudeMessage) ParseContent() []ClaudeContentBlock {
	if len(m.Content) == 0 {
		return nil
	}
	var text string
	if err := common.Unmarshal(m.Content, &text); err == nil {
		if strings.TrimSpace(text) == "" {
			return nil
		}
		return []ClaudeContentBlock{{Type: ContentTypeText, Text: text}}
	}
	var blocks []ClaudeContentBlock
	if err := common.Unmarshal(m.Content, &blocks); err != nil {
		return nil
	}
	return blocks
}

// clonePtr 复制指针字段，避免直接改动调用方数据。
func clonePtr[T any](value *T) *T {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

// ClaudeImageToOpenAIPart 把 Claude 图片块转成 OpenAI 的 image_url 内容块。
func ClaudeImageToOpenAIPart(source *ClaudeImageSource) (MediaContent, error) {
	if source == nil {
		return MediaContent{}, fmt.Errorf("image block without source")
	}
	switch source.Type {
	case "base64":
		if source.Data == "" {
			return MediaContent{}, fmt.Errorf("base64 image without data")
		}
		mediaType := source.MediaType
		if mediaType == "" {
			mediaType = "image/png"
		}
		return MediaContent{
			Type:     ContentTypeImageURL,
			ImageUrl: &MessageImageUrl{Url: "data:" + mediaType + ";base64," + source.Data},
		}, nil
	case "url":
		if source.Url == "" {
			return MediaContent{}, fmt.Errorf("url image without url")
		}
		return MediaContent{Type: ContentTypeImageURL, ImageUrl: &MessageImageUrl{Url: source.Url}}, nil
	}
	return MediaContent{}, fmt.Errorf("unsupported image source type %q", source.Type)
}

// ClaudeStopReasonToOpenAI 结束原因映射。
func ClaudeStopReasonToOpenAI(stopReason string) string {
	switch stopReason {
	case "max_tokens", "model_context_window_exceeded":
		return "length"
	case "tool_use":
		return "tool_calls"
	default:
		return "stop"
	}
}

// OpenAIStopReasonToClaude 结束原因反向映射。
func OpenAIStopReasonToClaude(finishReason string) string {
	switch finishReason {
	case "length":
		return "max_tokens"
	case "tool_calls", "function_call":
		return "tool_use"
	default:
		return "end_turn"
	}
}

// ClaudeRequestToOpenAIRequest 把 Claude Messages 请求转换成 OpenAI Chat Completions 请求。
func ClaudeRequestToOpenAIRequest(request *ClaudeRequest) (*GeneralOpenAIRequest, error) {
	if request == nil {
		return nil, fmt.Errorf("claude request is nil")
	}
	out := &GeneralOpenAIRequest{
		Model:       request.Model,
		Temperature: clonePtr(request.Temperature),
		TopP:        clonePtr(request.TopP),
		TopK:        clonePtr(request.TopK),
	}
	if request.MaxTokens > 0 {
		maxTokens := uint(request.MaxTokens)
		out.MaxTokens = &maxTokens
	}
	if request.Stream {
		stream := true
		out.Stream = &stream
	}
	if len(request.StopSequences) > 0 {
		out.Stop = request.StopSequences
	}

	if system := request.SystemText(); system != "" {
		out.Messages = append(out.Messages, Message{Role: "system", Content: system})
	}
	for _, message := range request.Messages {
		role := message.Role
		if role != "assistant" {
			role = "user"
		}
		blocks := message.ParseContent()
		if role == "assistant" {
			content, toolCalls, err := claudeAssistantBlocksToOpenAI(blocks)
			if err != nil {
				return nil, err
			}
			assistant := Message{Role: "assistant", Content: content}
			if len(toolCalls) > 0 {
				assistant.SetToolCalls(toolCalls)
			}
			out.Messages = append(out.Messages, assistant)
			continue
		}

		parts := make([]MediaContent, 0, len(blocks))
		for _, block := range blocks {
			switch block.Type {
			case ContentTypeText:
				if block.Text != "" {
					parts = append(parts, MediaContent{Type: ContentTypeText, Text: block.Text})
				}
			case "image":
				part, err := ClaudeImageToOpenAIPart(block.Source)
				if err != nil {
					return nil, err
				}
				parts = append(parts, part)
			case "tool_result":
				// tool_result 对应 OpenAI 的 tool 消息，需要单独成条
				result := Message{Role: "tool", ToolCallId: block.ToolUseId, Content: block.ContentText()}
				out.Messages = append(out.Messages, result)
			}
		}
		if len(parts) == 0 {
			continue
		}
		user := Message{Role: "user"}
		if len(parts) == 1 && parts[0].Type == ContentTypeText {
			user.Content = parts[0].Text
		} else {
			user.SetMediaContent(parts)
		}
		out.Messages = append(out.Messages, user)
	}

	for _, tool := range request.Tools {
		out.Tools = append(out.Tools, ToolCallRequest{
			Type: "function",
			Function: FunctionRequest{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.InputSchema,
			},
		})
	}
	if len(request.ToolChoice) > 0 {
		out.ToolChoice = claudeToolChoiceToOpenAI(request.ToolChoice)
	}
	return out, nil
}

// ContentText tool_result 的内容（字符串或文本块数组）拼成纯文本。
func (b ClaudeContentBlock) ContentText() string {
	if len(b.Content) == 0 {
		return ""
	}
	var text string
	if err := common.Unmarshal(b.Content, &text); err == nil {
		return text
	}
	var blocks []ClaudeContentBlock
	if err := common.Unmarshal(b.Content, &blocks); err != nil {
		return ""
	}
	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if block.Text != "" {
			parts = append(parts, block.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// claudeAssistantBlocksToOpenAI 处理 assistant 消息：文本拼成 content，tool_use 转成 tool_calls。
func claudeAssistantBlocksToOpenAI(blocks []ClaudeContentBlock) (any, []ToolCallRequest, error) {
	texts := make([]string, 0, len(blocks))
	toolCalls := make([]ToolCallRequest, 0)
	for _, block := range blocks {
		switch block.Type {
		case ContentTypeText:
			if block.Text != "" {
				texts = append(texts, block.Text)
			}
		case "tool_use":
			arguments := "{}"
			if block.Input != nil {
				encoded, err := common.Marshal(block.Input)
				if err != nil {
					return nil, nil, err
				}
				arguments = string(encoded)
			}
			toolCalls = append(toolCalls, ToolCallRequest{
				ID:       block.Id,
				Type:     "function",
				Function: FunctionRequest{Name: block.Name, Arguments: arguments},
			})
		}
	}
	if len(texts) == 0 && len(toolCalls) > 0 {
		return nil, toolCalls, nil
	}
	return strings.Join(texts, "\n"), toolCalls, nil
}

// claudeToolChoiceToOpenAI tool_choice 映射。
func claudeToolChoiceToOpenAI(raw json.RawMessage) any {
	var choice struct {
		Type string `json:"type"`
		Name string `json:"name"`
	}
	if err := common.Unmarshal(raw, &choice); err != nil {
		return nil
	}
	switch choice.Type {
	case "auto":
		return "auto"
	case "any":
		return "required"
	case "none":
		return "none"
	case "tool":
		return map[string]any{
			"type":     "function",
			"function": map[string]any{"name": choice.Name},
		}
	}
	return nil
}

// OpenAIResponseToClaudeResponse 把 OpenAI 非流式响应转换成 Claude 响应。
func OpenAIResponseToClaudeResponse(response *OpenAITextResponse, id string) *ClaudeResponse {
	out := &ClaudeResponse{
		Id:      id,
		Type:    "message",
		Role:    "assistant",
		Model:   response.Model,
		Content: make([]ClaudeContentBlock, 0, 2),
		Usage: ClaudeUsage{
			InputTokens:  response.Usage.PromptTokens,
			OutputTokens: response.Usage.CompletionTokens,
		},
	}
	if response.Usage.PromptTokensDetails.CachedTokens > 0 {
		out.Usage.CacheReadInputTokens = response.Usage.PromptTokensDetails.CachedTokens
	}
	if len(response.Choices) == 0 {
		out.StopReason = "end_turn"
		return out
	}

	choice := response.Choices[0]
	if text := choice.Message.StringContent(); strings.TrimSpace(text) != "" {
		out.Content = append(out.Content, ClaudeContentBlock{Type: ContentTypeText, Text: text})
	}
	for _, toolCall := range choice.Message.ParseToolCalls() {
		var input any
		if toolCall.Function.Arguments != "" {
			_ = common.Unmarshal([]byte(toolCall.Function.Arguments), &input)
		}
		out.Content = append(out.Content, ClaudeContentBlock{
			Type:  "tool_use",
			Id:    toolCall.ID,
			Name:  toolCall.Function.Name,
			Input: input,
		})
	}
	if len(out.Content) == 0 {
		out.Content = append(out.Content, ClaudeContentBlock{Type: ContentTypeText, Text: ""})
	}
	out.StopReason = OpenAIStopReasonToClaude(choice.FinishReason)
	return out
}

// NewClaudeMessageID 生成 Claude 消息 id（形如 msg_xxx）。
func NewClaudeMessageID(requestId string) string {
	if requestId == "" {
		return "msg_" + common.GetUUID()
	}
	return "msg_" + requestId
}

// OpenAIResponseFromClaude 把 Claude 响应转换成 OpenAI Chat Completions 响应。
func OpenAIResponseFromClaude(response *ClaudeResponse, requestId, modelName string) *OpenAITextResponse {
	if modelName == "" {
		modelName = response.Model
	}
	out := &OpenAITextResponse{
		Id:      NewClaudeMessageID(requestId),
		Object:  "chat.completion",
		Created: common.GetTimestamp(),
		Model:   modelName,
		Usage: Usage{
			PromptTokens:     response.Usage.InputTokens,
			CompletionTokens: response.Usage.OutputTokens,
			TotalTokens:      response.Usage.InputTokens + response.Usage.OutputTokens,
		},
	}
	if response.Usage.CacheReadInputTokens > 0 {
		out.Usage.PromptTokensDetails.CachedTokens = response.Usage.CacheReadInputTokens
		out.Usage.PromptCacheHitTokens = response.Usage.CacheReadInputTokens
	}

	message := Message{Role: "assistant", Content: ""}
	texts := make([]string, 0, 1)
	toolCalls := make([]ToolCallRequest, 0)
	for _, block := range response.Content {
		switch block.Type {
		case ContentTypeText:
			if block.Text != "" {
				texts = append(texts, block.Text)
			}
		case "tool_use":
			arguments := "{}"
			if block.Input != nil {
				if encoded, err := common.Marshal(block.Input); err == nil {
					arguments = string(encoded)
				}
			}
			toolCalls = append(toolCalls, ToolCallRequest{
				ID:       block.Id,
				Type:     "function",
				Function: FunctionRequest{Name: block.Name, Arguments: arguments},
			})
		}
	}
	if len(texts) > 0 {
		message.Content = strings.Join(texts, "\n")
	}
	if len(toolCalls) > 0 {
		message.SetToolCalls(toolCalls)
	}

	out.Choices = []OpenAITextResponseChoice{{
		Index:        0,
		Message:      message,
		FinishReason: ClaudeStopReasonToOpenAI(response.StopReason),
	}}
	return out
}
