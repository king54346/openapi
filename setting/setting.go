// Package setting 保存转发相关的运行时配置，默认值可在启动时直接修改。
package setting

import (
	"slices"
	"strings"
)

// ChatCompletionsToResponsesPolicy 控制是否把 /v1/chat/completions 请求转成 Responses API 发给上游
type ChatCompletionsToResponsesPolicy struct {
	Enabled       bool     `json:"enabled"`
	AllChannels   bool     `json:"all_channels"`
	ChannelIDs    []int    `json:"channel_ids,omitempty"`
	ChannelTypes  []int    `json:"channel_types,omitempty"`
	ModelPatterns []string `json:"model_patterns,omitempty"`
}

func (p ChatCompletionsToResponsesPolicy) IsChannelEnabled(channelID int, channelType int) bool {
	if !p.Enabled {
		return false
	}
	if p.AllChannels {
		return true
	}
	if channelID > 0 && slices.Contains(p.ChannelIDs, channelID) {
		return true
	}
	if channelType > 0 && slices.Contains(p.ChannelTypes, channelType) {
		return true
	}
	return false
}

type GlobalSettings struct {
	// PassThroughRequestEnabled 为 true 时请求体原样转发给上游，不做任何转换
	PassThroughRequestEnabled bool `json:"pass_through_request_enabled"`
	// ThinkingModelBlacklist 中的模型保留 -thinking/-nothinking 等后缀，不做推理参数转换
	ThinkingModelBlacklist           []string                         `json:"thinking_model_blacklist"`
	ChatCompletionsToResponsesPolicy ChatCompletionsToResponsesPolicy `json:"chat_completions_to_responses_policy"`

	// 流式响应期间向下游定时发送 ping，防止连接被中间代理断开
	PingIntervalEnabled bool `json:"ping_interval_enabled"`
	PingIntervalSeconds int  `json:"ping_interval_seconds"`

	// ServerAddress 本服务对外地址，用于拼接视频内容等回调 URL
	ServerAddress string `json:"server_address"`

	// QwenSyncImageModels 阿里通义中走同步接口的图片模型（按包含关系匹配）
	QwenSyncImageModels []string `json:"qwen_sync_image_models"`

	// 模型请求限流配置（按用户 ID 限流，支持按分组覆盖）
	ModelRequestRateLimitEnabled         bool `json:"model_request_rate_limit_enabled"`
	ModelRequestRateLimitDurationMinutes int  `json:"model_request_rate_limit_duration_minutes"`
	ModelRequestRateLimitCount           int  `json:"model_request_rate_limit_count"`
	ModelRequestRateLimitSuccessCount    int  `json:"model_request_rate_limit_success_count"`
}

// GroupRateLimit 分组限流覆盖（total/success 为 0 表示不限制该项）。
type GroupRateLimit struct {
	TotalCount   int `json:"total_count"`
	SuccessCount int `json:"success_count"`
}

var globalSettings = GlobalSettings{
	ThinkingModelBlacklist: []string{
		"moonshotai/kimi-k2-thinking",
		"kimi-k2-thinking",
	},
	ChatCompletionsToResponsesPolicy: ChatCompletionsToResponsesPolicy{
		Enabled:     false,
		AllChannels: true,
	},
	PingIntervalEnabled: false,
	PingIntervalSeconds: 60,
	ServerAddress:       "http://localhost:3000",
	QwenSyncImageModels: []string{
		"z-image",
		"qwen-image",
		"wan2.6",
		"wan2.7",
		"qwen-image-edit",
		"qwen-image-edit-max",
		"qwen-image-edit-max-2026-01-16",
		"qwen-image-edit-plus",
		"qwen-image-edit-plus-2025-12-15",
		"qwen-image-edit-plus-2025-10-30",
	},
}

func GetGlobalSettings() *GlobalSettings {
	return &globalSettings
}

// groupRateLimits 分组限流覆盖表（group -> GroupRateLimit），待接入真实配置中心后持久化。
var groupRateLimits = map[string]GroupRateLimit{}

// GetGroupRateLimit 获取分组限流覆盖，found 为 false 表示该分组无覆盖。
func GetGroupRateLimit(group string) (totalCount int, successCount int, found bool) {
	limit, ok := groupRateLimits[group]
	if !ok {
		return 0, 0, false
	}
	return limit.TotalCount, limit.SuccessCount, true
}

// SetGroupRateLimit 设置分组限流覆盖。
func SetGroupRateLimit(group string, totalCount int, successCount int) {
	if group == "" {
		return
	}
	groupRateLimits[group] = GroupRateLimit{TotalCount: totalCount, SuccessCount: successCount}
}

// ShouldPreserveThinkingSuffix 判断模型是否配置为保留 thinking/-nothinking/-low/-high/-medium 后缀
func ShouldPreserveThinkingSuffix(modelName string) bool {
	target := strings.TrimSpace(modelName)
	if target == "" {
		return false
	}
	for _, entry := range globalSettings.ThinkingModelBlacklist {
		if strings.TrimSpace(entry) == target {
			return true
		}
	}
	return false
}

// IsSyncImageModel 判断阿里图片模型是否走同步接口
func IsSyncImageModel(model string) bool {
	for _, m := range globalSettings.QwenSyncImageModels {
		if strings.Contains(model, m) {
			return true
		}
	}
	return false
}
