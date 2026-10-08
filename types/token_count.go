package types

type TokenType string

const (
	TokenTypeTextNumber TokenType = "text_number" // 按字符数估算
	TokenTypeTokenizer  TokenType = "tokenizer"   // 按分词估算
)

// TokenCountMeta 用于在上游未返回 usage 时估算 prompt token 数，只统计文本
type TokenCountMeta struct {
	TokenType     TokenType `json:"token_type,omitempty"`
	CombineText   string    `json:"combine_text,omitempty"`   // 所有消息拼接后的文本
	ToolsCount    int       `json:"tools_count,omitempty"`    // 工具数量
	NameCount     int       `json:"name_count,omitempty"`     // 带 name 的消息数量
	MessagesCount int       `json:"messages_count,omitempty"` // 消息数量
	MaxTokens     int       `json:"max_tokens,omitempty"`     // 请求允许的最大输出 token
}
