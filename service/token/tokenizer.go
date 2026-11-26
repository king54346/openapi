package token

import (
	"strings"
	"sync"

	"github.com/tiktoken-go/tokenizer"
)

func getDefaultTokenEncoder() tokenizer.Codec {
	defaultTokenEncoderOnce.Do(func() {
		defaultTokenEncoder, _ = tokenizer.Get(tokenizer.O200kBase)
	})
	return defaultTokenEncoder
}

func getTokenEncoder(model string) tokenizer.Codec {
	tokenEncoderMutex.RLock()
	encoder, ok := tokenEncoderMap[model]
	tokenEncoderMutex.RUnlock()
	if ok {
		return encoder
	}

	tokenEncoderMutex.Lock()
	defer tokenEncoderMutex.Unlock()
	if encoder, ok := tokenEncoderMap[model]; ok {
		return encoder
	}
	encoder, err := tokenizer.ForModel(tokenizer.Model(model))
	if err != nil {
		// 不支持的模型也缓存默认分词器，避免重复查找
		encoder = getDefaultTokenEncoder()
	}
	tokenEncoderMap[model] = encoder
	return encoder
}

func getTokenNum(encoder tokenizer.Codec, text string) int {
	if text == "" {
		return 0
	}
	n, _ := encoder.Count(text)
	return n
}

// isOpenAITextModel 判断是否为可用 tiktoken 精确分词的 OpenAI 文本模型

func isOpenAITextModel(model string) bool {
	model = strings.ToLower(model)
	for _, prefix := range []string{"gpt-", "chatgpt-", "o1", "o3", "o4", "text-embedding-", "davinci", "babbage"} {
		if strings.HasPrefix(model, prefix) {
			return true
		}
	}
	return false
}

// CountTextToken 统计文本的 token 数：OpenAI 模型用 tiktoken 精确分词，其余模型启发式估算

type Provider string

const (
	OpenAI  Provider = "openai"  // 代表 GPT-3.5, GPT-4, GPT-4o
	Gemini  Provider = "gemini"  // 代表 Gemini 1.0, 1.5 Pro/Flash
	Claude  Provider = "claude"  // 代表 Claude 3, 3.5 Sonnet
	Unknown Provider = "unknown" // 兜底默认
)

// multipliers 定义不同厂商的计费权重

var (
	// defaultTokenEncoder 模型不在 tiktoken 列表中时使用（新版 GPT 系列均为 o200k_base）
	defaultTokenEncoder     tokenizer.Codec
	defaultTokenEncoderOnce sync.Once

	// tokenEncoderMap 按模型名缓存分词器，模型数量有限，不会无限增长
	tokenEncoderMap   = make(map[string]tokenizer.Codec)
	tokenEncoderMutex sync.RWMutex
)
