package constant

import "strings"

// RelayMode 表示请求对应的上游接口类型，仅在进程内使用
const (
	RelayModeUnknown = iota
	RelayModeChatCompletions
	RelayModeCompletions
	RelayModeEmbeddings
	RelayModeModerations
	RelayModeImagesGenerations
	RelayModeImagesEdits
	RelayModeEdits

	RelayModeAudioSpeech        // tts
	RelayModeAudioTranscription // whisper 语音转文字（保留原语言）
	RelayModeAudioTranslation   // whisper 语音转文字并翻译成英文

	RelayModeVideoFetchByID
	RelayModeVideoSubmit

	RelayModeRerank

	RelayModeResponses

	RelayModeRealtime

	RelayModeResponsesCompact
)

func Path2RelayMode(path string) int {
	relayMode := RelayModeUnknown
	if strings.HasPrefix(path, "/v1/chat/completions") || strings.HasPrefix(path, "/pg/chat/completions") {
		relayMode = RelayModeChatCompletions
	} else if strings.HasPrefix(path, "/v1/completions") {
		relayMode = RelayModeCompletions
	} else if strings.HasPrefix(path, "/v1/embeddings") || strings.HasPrefix(path, "/pg/embeddings") {
		relayMode = RelayModeEmbeddings
	} else if strings.HasSuffix(path, "embeddings") {
		relayMode = RelayModeEmbeddings
	} else if strings.HasPrefix(path, "/v1/moderations") {
		relayMode = RelayModeModerations
	} else if strings.HasPrefix(path, "/v1/images/generations") || strings.HasPrefix(path, "/doubao/v3/images/generations") ||
		strings.HasPrefix(path, "/pg/images/generations") {
		relayMode = RelayModeImagesGenerations
	} else if strings.HasPrefix(path, "/v1/images/edits") {
		relayMode = RelayModeImagesEdits
	} else if strings.HasPrefix(path, "/v1/edits") {
		relayMode = RelayModeEdits
	} else if strings.HasPrefix(path, "/v1/responses/compact") {
		relayMode = RelayModeResponsesCompact
	} else if strings.HasPrefix(path, "/v1/responses") {
		relayMode = RelayModeResponses
	} else if strings.HasPrefix(path, "/v1/audio/speech") || strings.HasPrefix(path, "/pg/audio/speech") {
		relayMode = RelayModeAudioSpeech
	} else if strings.HasPrefix(path, "/v1/audio/transcriptions") || strings.HasPrefix(path, "/pg/audio/transcriptions") {
		relayMode = RelayModeAudioTranscription
	} else if strings.HasPrefix(path, "/v1/audio/translations") || strings.HasPrefix(path, "/pg/audio/translations") {
		relayMode = RelayModeAudioTranslation
	} else if strings.HasPrefix(path, "/v1/rerank") || strings.HasPrefix(path, "/pg/rerank") {
		relayMode = RelayModeRerank
	} else if strings.HasPrefix(path, "/v1/realtime") {
		relayMode = RelayModeRealtime
	}
	return relayMode
}
