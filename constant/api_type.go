package constant

// APIType 决定使用哪个 Adaptor，只在进程内使用，不持久化
const (
	APITypeOpenAI = iota
	APITypeAnthropic
	APITypePaLM
	APITypeBaidu
	APITypeZhipu
	APITypeAli
	APITypeXunfei
	APITypeAIProxyLibrary
	APITypeTencent
	APITypeGemini
	APITypeZhipuV4
	APITypeOllama
	APITypePerplexity
	APITypeAws
	APITypeCohere
	APITypeDify
	APITypeJina
	APITypeCloudflare
	APITypeSiliconFlow
	APITypeVertexAi
	APITypeMistral
	APITypeDeepSeek
	APITypeMokaAI
	APITypeVolcEngine
	APITypeBaiduV2
	APITypeOpenRouter
	APITypeXinference
	APITypeXai
	APITypeCoze
	APITypeJimeng
	APITypeMoonshot
	APITypeSubmodel
	APITypeMiniMax
	APITypeReplicate
	APITypeCodex
	APITypeDummy
)

// ChannelType2APIType 渠道类型 -> APIType，第二个返回值表示是否受支持
func ChannelType2APIType(channelType int) (int, bool) {
	switch channelType {
	case ChannelTypeOpenAI, ChannelTypeCustom:
		return APITypeOpenAI, true
	case ChannelTypeAli:
		return APITypeAli, true
	case ChannelTypeOpenRouter:
		return APITypeOpenRouter, true
	case ChannelTypeMoonshot:
		return APITypeMoonshot, true
	case ChannelTypeMiniMax:
		return APITypeMiniMax, true
	case ChannelTypeDeepSeek:
		return APITypeDeepSeek, true
	case ChannelTypeVolcEngine:
		return APITypeVolcEngine, true
	case ChannelTypeAnthropic:
		return APITypeAnthropic, true
	case ChannelTypeGemini:
		return APITypeGemini, true
	case ChannelTypeZhipu:
		return APITypeZhipu, true
	case ChannelTypeZhipu_v4:
		return APITypeZhipuV4, true
	case ChannelTypeSiliconFlow:
		return APITypeSiliconFlow, true
	}
	return -1, false
}

// IsRelaySupportedChannelType 渠道类型是否有对应的转发实现（普通接口或视频任务）。
func IsRelaySupportedChannelType(channelType int) bool {
	if _, ok := ChannelType2APIType(channelType); ok {
		return true
	}
	switch channelType {
	case ChannelTypeDoubaoVideo, ChannelTypeSora:
		return true
	}
	return false
}
