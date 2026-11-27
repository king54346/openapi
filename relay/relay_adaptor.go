package relay

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"openapi/constant"
	"openapi/relay/channel"
	"openapi/relay/channel/ali"
	"openapi/relay/channel/claude"
	"openapi/relay/channel/deepseek"
	"openapi/relay/channel/gemini"
	"openapi/relay/channel/minimax"
	"openapi/relay/channel/moonshot"
	"openapi/relay/channel/openai"
	"openapi/relay/channel/siliconflow"
	taskali "openapi/relay/channel/task/ali"
	taskdoubao "openapi/relay/channel/task/doubao"
	"openapi/relay/channel/task/hailuo"
	tasksora "openapi/relay/channel/task/sora"
	"openapi/relay/channel/volcengine"
	"openapi/relay/channel/zhipu"
	"openapi/relay/channel/zhipu_4v"
	relaycommon "openapi/relay/common"
	"openapi/types"

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
	case constant.APITypeAnthropic:
		return &claude.Adaptor{}
	case constant.APITypeGemini:
		return &gemini.Adaptor{}
	case constant.APITypeZhipu:
		return &zhipu.Adaptor{}
	case constant.APITypeZhipuV4:
		return &zhipu_4v.Adaptor{}
	case constant.APITypeSiliconFlow:
		return &siliconflow.Adaptor{}
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
	case constant.ChannelTypeSora, constant.ChannelTypeOpenAI, constant.ChannelTypeCustom:
		return &tasksora.TaskAdaptor{}
	case constant.ChannelTypeMiniMax:
		return &hailuo.TaskAdaptor{}
	}
	return nil
}

// convertRequestError 请求格式转换失败：渠道不支持该接口时返回 400，其余为本地转换错误。
// 两种情况都不重试、不计入渠道失败（问题在请求或渠道能力，换同类渠道也无济于事）。
func convertRequestError(c gin.Context, info *relaycommon.RelayInfo, err error) *types.StarAPIError {
	if errors.Is(err, channel.ErrNotImplemented) {
		return types.NewErrorWithStatusCode(
			fmt.Errorf("channel type %s does not support %s", constant.GetChannelTypeName(info.ChannelType), c.Request().URL.Path),
			types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
}
