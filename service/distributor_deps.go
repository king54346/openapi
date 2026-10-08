package service

import (
	"errors"

	"openapi/common"
	"openapi/constant"
	"openapi/dto"
	"openapi/model"

	gin "github.com/king54346/gin-tiny"
)

// distributor 相关依赖桩：待接入真实分组/渠道/亲和性实现后逐个替换。

// RetryParam 渠道重试选型参数。
type RetryParam struct {
	Ctx        gin.Context
	ModelName  string
	TokenGroup string
	Retry      *int
}

// MjRequestError Midjourney 请求解析错误。
type MjRequestError struct {
	Description string
}

// FormatMatchingModelName 归一化模型名用于匹配（桩实现：原样返回）。
func FormatMatchingModelName(modelName string) string {
	return modelName
}

// WithCompactModelSuffix 补 responses compact 模型后缀（桩实现）。
func WithCompactModelSuffix(modelName string) string {
	const suffix = "-openai-compact"
	if len(modelName) >= len(suffix) && modelName[len(modelName)-len(suffix):] == suffix {
		return modelName
	}
	return modelName + suffix
}

// IsUserGroupAllowedForExtraVisibleModel 用户分组是否可调用额外可见模型（桩实现：放行）。
func IsUserGroupAllowedForExtraVisibleModel(userGroup, modelName string) bool {
	return true
}

// GroupInUserUsableGroups 分组是否在用户可用分组内（桩实现：相等即通过）。
func GroupInUserUsableGroups(usingGroup, group string) bool {
	return group == "" || group == usingGroup
}

// GetMjRequestModel 解析 Midjourney 请求模型（桩实现：未实现）。
func GetMjRequestModel(relayMode int, req *dto.MidjourneyRequest) (string, *MjRequestError, bool) {
	return "", &MjRequestError{Description: "midjourney request parsing is not implemented"}, false
}

// GetPreferredChannelByAffinity 按亲和性取偏好渠道（桩实现：无偏好）。
func GetPreferredChannelByAffinity(c gin.Context, modelName, usingGroup string) (int, bool) {
	return 0, false
}

// MarkChannelAffinityUsed 标记亲和性已使用（桩实现：无操作）。
func MarkChannelAffinityUsed(c gin.Context, group string, channelID int) {}

// GetUserAutoGroup 取用户 auto 分组展开列表：暂无 auto 分组配置，只展开为用户自身分组。
func GetUserAutoGroup(userGroup string) []string {
	if userGroup == "" {
		return nil
	}
	return []string{userGroup}
}

// CacheGetRandomSatisfiedChannel 从渠道缓存中为分组和模型选一个渠道。
// 令牌分组为 auto 时依次尝试用户的 auto 分组，并把命中的分组写入上下文。
// 返回的 string 为实际选中的分组；没有可用渠道时 channel 为 nil。
func CacheGetRandomSatisfiedChannel(param *RetryParam) (*model.Channel, string, error) {
	if param == nil {
		return nil, "", errors.New("retry param is nil")
	}
	retry := 0
	if param.Retry != nil {
		retry = *param.Retry
	}
	if param.TokenGroup != "auto" {
		channel, err := model.GetRandomSatisfiedChannel(param.TokenGroup, param.ModelName, retry)
		return channel, param.TokenGroup, err
	}

	var userGroup string
	if param.Ctx != nil {
		userGroup = common.GetContextKeyString(param.Ctx, constant.ContextKeyUserGroup)
	}
	for _, group := range GetUserAutoGroup(userGroup) {
		channel, err := model.GetRandomSatisfiedChannel(group, param.ModelName, retry)
		if err != nil {
			return nil, group, err
		}
		if channel != nil {
			if param.Ctx != nil {
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroup, group)
			}
			return channel, group, nil
		}
	}
	return nil, userGroup, nil
}

// RecordChannelAffinity 记录渠道亲和性（桩实现：无操作）。
func RecordChannelAffinity(c gin.Context, channelID int) {}
