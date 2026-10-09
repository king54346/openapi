package service

import (
	"fmt"
	"strings"
	"sync"

	"github.com/tiktoken-go/tokenizer"
)

var (
	// defaultTokenEncoder 模型不在 tiktoken 列表中时使用（新版 GPT 系列均为 o200k_base）
	defaultTokenEncoder     tokenizer.Codec
	defaultTokenEncoderOnce sync.Once

	// tokenEncoderMap 按模型名缓存分词器，模型数量有限，不会无限增长
	tokenEncoderMap   = make(map[string]tokenizer.Codec)
	tokenEncoderMutex sync.RWMutex
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
func CountTextToken(text string, model string) int {
	if text == "" {
		return 0
	}
	if isOpenAITextModel(model) {
		return getTokenNum(getTokenEncoder(model), text)
	}
	return EstimateTokenByModel(model, text)
}

// CountTokenInput 统计任意输入（字符串、字符串数组或其他结构）的 token 数
func CountTokenInput(input any, model string) int {
	switch v := input.(type) {
	case string:
		return CountTextToken(v, model)
	case []string:
		return CountTextToken(strings.Join(v, ""), model)
	case []any:
		var sb strings.Builder
		for _, item := range v {
			sb.WriteString(fmt.Sprintf("%v", item))
		}
		return CountTextToken(sb.String(), model)
	}
	return CountTextToken(fmt.Sprintf("%v", input), model)
}
