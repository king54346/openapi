package service

import "testing"

func TestCountTextTokenUsesTiktokenForOpenAI(t *testing.T) {
	// o200k_base / cl100k_base 下 "hello world" 均为 2 个 token
	for _, model := range []string{"gpt-4o", "gpt-4", "gpt-5-unknown"} {
		if got := CountTextToken("hello world", model); got != 2 {
			t.Fatalf("%s: CountTextToken = %d, want 2", model, got)
		}
	}
	if !isOpenAITextModel("GPT-4o-mini") || isOpenAITextModel("deepseek-chat") {
		t.Fatal("isOpenAITextModel classification wrong")
	}
}
