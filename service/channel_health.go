package service

import (
	"fmt"
	"net/http"
	"strings"
	"sync"

	"openapi/common"
	"openapi/model"
	"openapi/types"
)

// 渠道健康：按渠道统计连续失败次数，达到阈值或遇到致命错误（密钥失效、欠费等）时
// 自动禁用渠道（状态改为 ChannelStatusAutoDisabled，需管理员手动启用）。
// 只统计渠道侧问题，请求本身有误（400 等）不计数；渠道关闭 auto_ban 时不处理。

var channelFailures = struct {
	sync.Mutex
	counts map[int]int
}{counts: make(map[int]int)}

// fatalChannelErrorMarkers 出现即说明渠道密钥或账户不可用，立即禁用
var fatalChannelErrorMarkers = []string{
	"invalid_api_key",
	"incorrect api key",
	"invalid api key",
	"account_deactivated",
	"insufficient_quota",
	"billing_not_active",
	"credit balance is too low",
	"organization has been disabled",
	"arrearage",      // 阿里云欠费
	"accountoverdue", // 火山引擎欠费
}

// isChannelFault 错误是否由渠道（上游）导致。
func isChannelFault(apiErr *types.StarAPIError) bool {
	if types.IsChannelError(apiErr) {
		return true
	}
	if types.IsSkipRetryError(apiErr) {
		// 本地请求处理错误（参数校验、格式转换等）
		return false
	}
	code := apiErr.StatusCode
	return code == http.StatusUnauthorized || code == http.StatusForbidden ||
		code == http.StatusTooManyRequests || code >= http.StatusInternalServerError
}

// isFatalChannelError 是否为密钥失效、欠费等不会自行恢复的错误。
func isFatalChannelError(apiErr *types.StarAPIError) bool {
	if apiErr.StatusCode == http.StatusUnauthorized {
		return true
	}
	text := strings.ToLower(apiErr.Error() + " " + string(apiErr.GetErrorCode()))
	for _, marker := range fatalChannelErrorMarkers {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

// RecordChannelSuccess 转发成功后清零渠道的连续失败计数。
func RecordChannelSuccess(channelID int) {
	channelFailures.Lock()
	delete(channelFailures.counts, channelID)
	channelFailures.Unlock()
}

// RecordChannelFailure 记录一次渠道失败，必要时自动禁用，返回本次是否禁用了渠道（或多 key 渠道中的某个 key）。
// usingKey 为本次使用的 key，多 key 渠道只禁用该 key。
func RecordChannelFailure(channelID int, usingKey string, autoBan bool, apiErr *types.StarAPIError) bool {
	if !common.ChannelAutoDisableEnabled || !autoBan || apiErr == nil || channelID <= 0 {
		return false
	}
	if !isChannelFault(apiErr) {
		return false
	}
	fatal := isFatalChannelError(apiErr)

	channelFailures.Lock()
	channelFailures.counts[channelID]++
	count := channelFailures.counts[channelID]
	channelFailures.Unlock()

	threshold := max(common.ChannelAutoDisableThreshold, 1)
	if !fatal && count < threshold {
		return false
	}

	reason := fmt.Sprintf("auto disabled after %d consecutive failures, last error (status %d): %s",
		count, apiErr.StatusCode, apiErr.MaskSensitiveError())
	if fatal {
		reason = fmt.Sprintf("auto disabled due to fatal error (status %d): %s", apiErr.StatusCode, apiErr.MaskSensitiveError())
	}
	if !model.UpdateChannelStatus(channelID, usingKey, common.ChannelStatusAutoDisabled, reason) {
		return false
	}
	RecordChannelSuccess(channelID) // 计数清零，管理员重新启用后从头统计
	common.SysError(fmt.Sprintf("channel #%d %s", channelID, reason))
	model.RefreshChannelCache()
	return true
}
