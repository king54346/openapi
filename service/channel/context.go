package channel

import (
	"errors"
	"strings"

	"openapi/common"
	"openapi/constant"
	"openapi/model"
	"openapi/types"

	gin "github.com/king54346/gin-tiny"
)

// SetupContextForSelectedChannel 将选中的渠道元数据、密钥及覆盖规则写入请求上下文。
func SetupContextForSelectedChannel(c gin.Context, ch *model.Channel, modelName string) *types.StarAPIError {
	c.Set("original_model", modelName)
	if ch == nil {
		return types.NewError(errors.New("channel is nil"), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
	}
	common.SetContextKey(c, constant.ContextKeyChannelId, ch.Id)
	common.SetContextKey(c, constant.ContextKeyChannelName, ch.Name)
	common.SetContextKey(c, constant.ContextKeyChannelType, ch.Type)
	common.SetContextKey(c, constant.ContextKeyChannelCreateTime, ch.CreatedTime)
	common.SetContextKey(c, constant.ContextKeyChannelSetting, ch.GetSetting())
	common.SetContextKey(c, constant.ContextKeyChannelOtherSetting, ch.GetOtherSettings())
	common.SetContextKey(c, constant.ContextKeyChannelParamOverride, ch.GetParamOverride())
	common.SetContextKey(c, constant.ContextKeyChannelHeaderOverride, ch.GetHeaderOverride())
	if ch.OpenAIOrganization != nil && *ch.OpenAIOrganization != "" {
		common.SetContextKey(c, constant.ContextKeyChannelOrganization, *ch.OpenAIOrganization)
	}
	common.SetContextKey(c, constant.ContextKeyChannelAutoBan, ch.GetAutoBan())
	common.SetContextKey(c, constant.ContextKeyChannelModelMapping, ch.GetModelMapping())
	common.SetContextKey(c, constant.ContextKeyChannelStatusCodeMapping, ch.GetStatusCodeMapping())

	key, index, apiErr := ch.GetNextEnabledKey()
	if apiErr != nil {
		return apiErr
	}
	if ch.ChannelInfo.IsMultiKey {
		common.SetContextKey(c, constant.ContextKeyChannelIsMultiKey, true)
		common.SetContextKey(c, constant.ContextKeyChannelMultiKeyIndex, index)
	} else {
		common.SetContextKey(c, constant.ContextKeyChannelIsMultiKey, false)
	}
	common.SetContextKey(c, constant.ContextKeyChannelKey, key)
	common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, ch.GetBaseURL())
	common.SetContextKey(c, constant.ContextKeySystemPromptOverride, false)

	SetupChannelOtherContext(c, ch)
	return nil
}

// SetupChannelOtherContext 解析渠道 Other 列（API 版本、region、plugin、bot_id）写入请求上下文。
func SetupChannelOtherContext(c gin.Context, ch *model.Channel) {
	if ch == nil {
		return
	}
	other := strings.TrimSpace(ch.Other)
	switch ch.Type {
	case constant.ChannelTypeAzure, constant.ChannelTypeXunfei,
		constant.ChannelCloudflare, constant.ChannelTypeMokaAI, constant.ChannelTypeGemini:
		version := other
		if version == "" {
			version = strings.TrimSpace(c.Query("api_version"))
		}
		if version == "" && ch.Type == constant.ChannelTypeGemini {
			version = "v1beta"
		}
		if version != "" {
			c.Set("api_version", version)
		}
	case constant.ChannelTypeVertexAi:
		if other != "" {
			c.Set("region", other)
		}
	case constant.ChannelTypeAli:
		if other != "" {
			c.Set("plugin", other)
		}
	case constant.ChannelTypeCoze:
		if other != "" {
			c.Set("bot_id", other)
		}
	}
}
