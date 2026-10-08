package relay

import (
	"strconv"

	"openapi/constant"
	"openapi/relay/channel"
	"openapi/relay/channel/ali"
	"openapi/relay/channel/deepseek"
	"openapi/relay/channel/minimax"
	"openapi/relay/channel/moonshot"
	"openapi/relay/channel/openai"
	taskali "openapi/relay/channel/task/ali"
	taskdoubao "openapi/relay/channel/task/doubao"
	"openapi/relay/channel/task/hailuo"
	tasksora "openapi/relay/channel/task/sora"
	"openapi/relay/channel/volcengine"

	gin "github.com/king54346/gin-tiny"
)

// GetAdaptor 根据 APIType 返回对应的 Adaptor，不支持的类型返回 nil
func GetAdaptor(apiType int) channel.Adaptor {
	switch apiType {
	case constant.APITypeOpenAI, constant.APITypeOpenRouter:
		// OpenRouter 与 OpenAI 协议兼容，差异在 openai.Adaptor 内部按渠道类型处理
		return &openai.Adaptor{}
	case constant.APITypeAli:
		return &ali.Adaptor{}
	case constant.APITypeDeepSeek:
		return &deepseek.Adaptor{}
	case constant.APITypeMiniMax:
		return &minimax.Adaptor{}
	case constant.APITypeMoonshot:
		return &moonshot.Adaptor{}
	case constant.APITypeVolcEngine:
		return &volcengine.Adaptor{}
	}
	return nil
}

func GetTaskPlatform(c gin.Context) constant.TaskPlatform {
	channelType := c.GetInt("channel_type")
	if channelType > 0 {
		return constant.TaskPlatform(strconv.Itoa(channelType))
	}
	return constant.TaskPlatform(c.GetString("platform"))
}

// GetTaskAdaptor 根据任务平台（渠道类型）返回对应的 TaskAdaptor，不支持的平台返回 nil
func GetTaskAdaptor(platform constant.TaskPlatform) channel.TaskAdaptor {
	channelType, err := strconv.ParseInt(string(platform), 10, 64)
	if err != nil {
		return nil
	}
	switch channelType {
	case constant.ChannelTypeAli:
		return &taskali.TaskAdaptor{}
	case constant.ChannelTypeDoubaoVideo, constant.ChannelTypeVolcEngine:
		return &taskdoubao.TaskAdaptor{}
	case constant.ChannelTypeSora, constant.ChannelTypeOpenAI:
		return &tasksora.TaskAdaptor{}
	case constant.ChannelTypeMiniMax:
		return &hailuo.TaskAdaptor{}
	}
	return nil
}
