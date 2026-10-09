package relay

import (
	"fmt"
	"strings"
	"time"

	"openapi/common"
	"openapi/constant"
	"openapi/dto"
	"openapi/logger"
	"openapi/model"
	relaycommon "openapi/relay/common"
	"openapi/service"

	gin "github.com/king54346/gin-tiny"
)

// UsageRecord 一次请求结束后的用量信息
type UsageRecord struct {
	ModelName        string
	PromptTokens     int
	CompletionTokens int
	CachedTokens     int
	// Usage 上游返回的完整 usage（含音频、图片等细分）；上游未返回时为本地估算值
	Usage *dto.Usage
	// Estimated 为 true 表示上游没有返回完整 usage，部分或全部 token 来自本地估算
	Estimated      bool
	UseTimeSeconds int
	// Extra 附加信息，如内置工具调用次数、任务 action 等
	Extra map[string]any
}

// UsageRecorder 在请求成功结束后接收用量信息。
// 默认实现只写入用量日志；需要计费、扣额度时用 SetUsageRecorder 替换。
type UsageRecorder func(c gin.Context, info *relaycommon.RelayInfo, record UsageRecord)

var usageRecorder UsageRecorder = recordUsageLog

// SetUsageRecorder 替换默认的用量处理逻辑，传入 nil 时忽略
func SetUsageRecorder(r UsageRecorder) {
	if r != nil {
		usageRecorder = r
	}
}

// recordUsage 汇总 usage 并交给 usageRecorder，usage 为 nil 时按估算的 prompt token 记录
func recordUsage(c gin.Context, info *relaycommon.RelayInfo, usage *dto.Usage, extra map[string]any) {
	record := UsageRecord{
		ModelName:      info.OriginModelName,
		UseTimeSeconds: int(time.Since(info.StartTime).Seconds()),
		Extra:          extra,
		Estimated:      common.GetContextKeyBool(c, constant.ContextKeyUsageEstimated),
	}
	if usage == nil {
		record.Estimated = true
		usage = service.NewUsage(info.GetEstimatePromptTokens(), 0)
	}
	record.Usage = usage
	record.PromptTokens = usage.PromptTokens
	record.CompletionTokens = usage.CompletionTokens
	record.CachedTokens = cachedTokensOf(usage)

	if record.Extra == nil {
		record.Extra = map[string]any{}
	}
	if info.ResponsesUsageInfo != nil {
		for name, tool := range info.ResponsesUsageInfo.BuiltInTools {
			if tool != nil && tool.CallCount > 0 {
				record.Extra[name+"_call_count"] = tool.CallCount
			}
		}
	}
	if reason := common.GetContextKeyString(c, constant.ContextKeyAdminRejectReason); reason != "" {
		record.Extra["reject_reason"] = reason
	}

	usageRecorder(c, info, record)
}

// cachedTokensOf 统一缓存命中 token：优先 prompt_tokens_details.cached_tokens，
// 其次顶层 prompt_cache_hit_tokens（DeepSeek 等）
func cachedTokensOf(usage *dto.Usage) int {
	if usage.PromptTokensDetails.CachedTokens > 0 {
		return usage.PromptTokensDetails.CachedTokens
	}
	return usage.PromptCacheHitTokens
}

// realtimeUsageToUsage 把 realtime 的 input/output 用量转换为通用 Usage
func realtimeUsageToUsage(u *dto.RealtimeUsage) *dto.Usage {
	if u == nil {
		return nil
	}
	usage := &dto.Usage{
		PromptTokens:     u.InputTokens,
		CompletionTokens: u.OutputTokens,
		TotalTokens:      u.TotalTokens,
	}
	usage.PromptTokensDetails.CachedTokens = u.InputTokenDetails.CachedTokens
	usage.PromptTokensDetails.TextTokens = u.InputTokenDetails.TextTokens
	usage.PromptTokensDetails.AudioTokens = u.InputTokenDetails.AudioTokens
	usage.CompletionTokenDetails.TextTokens = u.OutputTokenDetails.TextTokens
	usage.CompletionTokenDetails.AudioTokens = u.OutputTokenDetails.AudioTokens
	return usage
}

// recordUsageLog 默认的用量处理：写入 logs 表，未初始化数据库时只输出到日志
func recordUsageLog(c gin.Context, info *relaycommon.RelayInfo, record UsageRecord) {
	var content []string
	if record.Estimated {
		content = append(content, "上游未返回完整 usage，token 含本地估算值")
	}
	if record.PromptTokens+record.CompletionTokens == 0 && record.Extra["action"] == nil {
		logger.LogWarn(c, fmt.Sprintf("total tokens is 0, userId %d, channelId %d, model %s", info.UserId, info.ChannelId, record.ModelName))
	}

	if model.LOG_DB == nil {
		logger.LogInfo(c, fmt.Sprintf("usage: model=%s, channel=%d, prompt=%d, completion=%d, cached=%d, use_time=%ds",
			record.ModelName, info.ChannelId, record.PromptTokens, record.CompletionTokens, record.CachedTokens, record.UseTimeSeconds))
		return
	}

	other := record.Extra
	if record.CachedTokens > 0 {
		other["cache_tokens"] = record.CachedTokens
	}
	if record.Usage != nil {
		if record.Usage.PromptTokensDetails.ImageTokens > 0 {
			other["image_tokens"] = record.Usage.PromptTokensDetails.ImageTokens
		}
		if record.Usage.PromptTokensDetails.AudioTokens > 0 {
			other["audio_input_tokens"] = record.Usage.PromptTokensDetails.AudioTokens
		}
		if record.Usage.CompletionTokenDetails.AudioTokens > 0 {
			other["audio_output_tokens"] = record.Usage.CompletionTokenDetails.AudioTokens
		}
	}
	model.RecordConsumeLog(c, info.UserId, model.RecordConsumeLogParams{
		ChannelId:        info.ChannelId,
		PromptTokens:     record.PromptTokens,
		CompletionTokens: record.CompletionTokens,
		ModelName:        record.ModelName,
		TokenName:        c.GetString("token_name"),
		Content:          strings.Join(content, ", "),
		TokenId:          info.TokenId,
		UseTimeSeconds:   record.UseTimeSeconds,
		IsStream:         info.IsStream,
		Group:            info.UsingGroup,
		Other:            other,
	})
}
