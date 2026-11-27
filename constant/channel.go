package constant

// 渠道类型的数值会持久化到数据库（channelsvc.type），不要修改已有取值
const (
	ChannelTypeUnknown     = 0
	ChannelTypeOpenAI      = 1
	ChannelTypeAnthropic   = 14
	ChannelTypeZhipu       = 16
	ChannelTypeAli         = 17
	ChannelTypeOpenRouter  = 20
	ChannelTypeGemini      = 24
	ChannelTypeMoonshot    = 25
	ChannelTypeZhipu_v4    = 26
	ChannelTypeMiniMax     = 35
	ChannelTypeSiliconFlow = 40
	ChannelTypeDeepSeek    = 43
	ChannelTypeVolcEngine  = 45
	ChannelTypeDoubaoVideo = 54 // 仅用于视频任务
	ChannelTypeSora        = 55 // 仅用于视频任务

	// 以下渠道类型仅 distributor 引用，用于按渠道设置上下文；
	// 具体上游对接尚未移植，数值保留待定（勿与已有值冲突）。
	ChannelTypeAzure    = 3
	ChannelTypeVertexAi = 8
	ChannelTypeXunfei   = 9
	ChannelCloudflare   = 11
	ChannelTypeMokaAI   = 12
	ChannelTypeCoze     = 13
)

var ChannelBaseURLs = map[int]string{
	ChannelTypeOpenAI:      "https://api.openai.com",
	ChannelTypeAnthropic:   "https://api.anthropic.com",
	ChannelTypeZhipu:       "https://open.bigmodel.cn",
	ChannelTypeAli:         "https://dashscope.aliyuncs.com",
	ChannelTypeOpenRouter:  "https://openrouter.ai/api",
	ChannelTypeGemini:      "https://generativelanguage.googleapis.com",
	ChannelTypeMoonshot:    "https://api.moonshot.cn",
	ChannelTypeZhipu_v4:    "https://open.bigmodel.cn",
	ChannelTypeMiniMax:     "https://api.minimax.chat",
	ChannelTypeSiliconFlow: "https://api.siliconflow.cn",
	ChannelTypeDeepSeek:    "https://api.deepseek.com",
	ChannelTypeVolcEngine:  "https://ark.cn-beijing.volces.com",
	ChannelTypeDoubaoVideo: "https://ark.cn-beijing.volces.com",
	ChannelTypeSora:        "https://api.openai.com",
}

var ChannelTypeNames = map[int]string{
	ChannelTypeUnknown:     "Unknown",
	ChannelTypeOpenAI:      "OpenAI",
	ChannelTypeAnthropic:   "Anthropic",
	ChannelTypeZhipu:       "Zhipu",
	ChannelTypeAli:         "Ali",
	ChannelTypeOpenRouter:  "OpenRouter",
	ChannelTypeGemini:      "Gemini",
	ChannelTypeMoonshot:    "Moonshot",
	ChannelTypeZhipu_v4:    "ZhipuV4",
	ChannelTypeMiniMax:     "MiniMax",
	ChannelTypeSiliconFlow: "SiliconFlow",
	ChannelTypeDeepSeek:    "DeepSeek",
	ChannelTypeVolcEngine:  "VolcEngine",
	ChannelTypeDoubaoVideo: "DoubaoVideo",
	ChannelTypeSora:        "Sora",
}

func GetChannelTypeName(channelType int) string {
	if name, ok := ChannelTypeNames[channelType]; ok {
		return name
	}
	return "Unknown"
}

// ChannelSpecialBase 渠道 BaseURL 填写为特殊套餐名时，改用套餐对应的兼容地址
type ChannelSpecialBase struct {
	ClaudeBaseURL string
	OpenAIBaseURL string
}

var ChannelSpecialBases = map[string]ChannelSpecialBase{
	"glm-coding-plan": {
		ClaudeBaseURL: "https://open.bigmodel.cn/api/anthropic",
		OpenAIBaseURL: "https://open.bigmodel.cn/api/coding/paas/v4",
	},
	"glm-coding-plan-international": {
		ClaudeBaseURL: "https://api.z.ai/api/anthropic",
		OpenAIBaseURL: "https://api.z.ai/api/coding/paas/v4",
	},
	"kimi-coding-plan": {
		ClaudeBaseURL: "https://api.kimi.com/coding",
		OpenAIBaseURL: "https://api.kimi.com/coding/v1",
	},
	"doubao-coding-plan": {
		ClaudeBaseURL: "https://ark.cn-beijing.volces.com/api/coding",
		OpenAIBaseURL: "https://ark.cn-beijing.volces.com/api/coding/v3",
	},
}
