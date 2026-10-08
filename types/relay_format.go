package types

type RelayFormat string

const (
	RelayFormatOpenAI                    RelayFormat = "openai"
	RelayFormatOpenAIResponses           RelayFormat = "openai_responses"
	RelayFormatOpenAIResponsesCompaction RelayFormat = "openai_responses_compaction"
	RelayFormatOpenAIAudio               RelayFormat = "openai_audio"
	RelayFormatOpenAIImage               RelayFormat = "openai_image"
	RelayFormatOpenAIRealtime            RelayFormat = "openai_realtime"
	RelayFormatRerank                    RelayFormat = "rerank"
	RelayFormatEmbedding                 RelayFormat = "embedding"
	RelayFormatClaude                    RelayFormat = "claude"
	RelayFormatGemini                    RelayFormat = "gemini"
	RelayFormatTask                      RelayFormat = "task"
)
