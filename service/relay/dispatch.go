package relay

import (
	"errors"
	"openapi/dto"
	"openapi/model"
	"openapi/setting"
	"strings"

	gin "github.com/king54346/gin-tiny"
)

// distributor 相关依赖：模型名归一化与渠道选择。分组概念已移除，所有启用渠道的模型对所有请求可见。

// RetryParam 渠道重试选型参数。

type RetryParam struct {
	Ctx       gin.Context
	ModelName string
	Retry     *int
}

// MjRequestError Midjourney 请求解析错误。

type MjRequestError struct {
	Description string
}

// thinkingSuffixes 模型名里可以剥离的推理相关后缀（由易到难依次匹配）。

var thinkingSuffixes = []string{
	"-thinking",
	"-nothinking",
	"-online",
	"-search",
	"-minimal",
	"-xhigh",
	"-medium",
	"-high",
	"-low",
}

// compactModelSuffix responses compact 请求的模型后缀（与 helper.CompactModelSuffix 一致）

const compactModelSuffix = "-openai-compact"

// FormatMatchingModelName 归一化模型名用于黑白名单匹配：
// 兼容 g-* / gpt-4-gizmo-*、thinking- 前缀，并剥离 -openai-compact 与推理/联网后缀。
// 配置为「保留 thinking 后缀」的模型原样返回。

func FormatMatchingModelName(modelName string) string {
	matched := strings.TrimSpace(modelName)
	if matched == "" || setting.ShouldPreserveThinkingSuffix(matched) {
		return modelName
	}
	if strings.HasPrefix(matched, "gpt-4-gizmo-") || strings.HasPrefix(matched, "g-") {
		return "gpt-4-gizmo-*"
	}
	matched = strings.TrimPrefix(matched, "thinking-")
	for range 4 { // 最多剥 4 层，避免异常模型名导致的循环
		trimmed := matched
		if strings.HasSuffix(matched, compactModelSuffix) {
			trimmed = strings.TrimSuffix(matched, compactModelSuffix)
		} else {
			for _, suffix := range thinkingSuffixes {
				if strings.HasSuffix(matched, suffix) {
					trimmed = strings.TrimSuffix(matched, suffix)
					break
				}
			}
		}
		if trimmed == "" || trimmed == matched {
			break
		}
		matched = trimmed
	}
	return matched
}

// WithCompactModelSuffix 补 responses compact 模型后缀（已存在时不重复追加）。

func WithCompactModelSuffix(modelName string) string {
	if strings.HasSuffix(modelName, compactModelSuffix) {
		return modelName
	}
	return modelName + compactModelSuffix
}

// GetMjRequestModel 解析 Midjourney 请求模型：本项目未对接 Midjourney 上游，
// 因此统一返回错误，由分发中间件拒绝该请求。

func GetMjRequestModel(relayMode int, req *dto.MidjourneyRequest) (string, *MjRequestError, bool) {
	return "", &MjRequestError{Description: "midjourney upstream is not supported by this deployment"}, false
}

// CacheGetRandomSatisfiedChannel 从渠道缓存中为模型选一个渠道。

func CacheGetRandomSatisfiedChannel(param *RetryParam) (*model.Channel, error) {
	if param == nil {
		return nil, errors.New("retry param is nil")
	}
	retry := 0
	if param.Retry != nil {
		retry = *param.Retry
	}
	return model.GetRandomSatisfiedChannel("", param.ModelName, retry)
}
