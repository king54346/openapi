package channel

import (
	"context"
	"fmt"
	"net/http"
	"openapi/common"
	"openapi/model"
	"openapi/types"
	"strconv"
	"strings"
	"sync"
	"time"
)

/**
渠道健康检查与自动禁用机制：
1. 记录渠道连续失败次数（内存 + Redis），超过阈值自动禁用,集群级连续失败自动封禁。
2. fatalChannelErrorMarkers：出现即说明渠道密钥或账户不可用，立即禁用。
3. isChannelFault：判断错误是否由渠道（上游）导致。
4. isFatalChannelError：判断是否为密钥失效、欠费等不会自行恢复的错误。
5. RecordChannelSuccess：转发成功后清零渠道的连续失败计数（启用 Redis 时同步清零集群计数）。
6. RecordChannelFailure：记录一次渠道失败，必要时自动禁用并通知全集群下线该渠道。
*/

const healthFailureKeyTTL = 15 * time.Minute

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

func channelFailureRedisKey(channelID int) string {
	return common.RedisKey("health", "fail", strconv.Itoa(channelID))
}

// isChannelFault 错误是否由渠道（上游）导致。
func isChannelFault(apiErr *types.StarAPIError) bool {
	if types.IsChannelError(apiErr) {
		return true
	}
	if types.IsSkipRetryError(apiErr) {
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

// RecordChannelSuccess 转发成功后清零渠道的连续失败计数（启用 Redis 时同步清零集群计数）。
func RecordChannelSuccess(channelID int) {
	if channelID <= 0 {
		return
	}
	if common.RedisEnabled && common.RDB != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = common.RDB.Del(ctx, channelFailureRedisKey(channelID)).Err()
	}
	channelFailures.Lock()
	delete(channelFailures.counts, channelID)
	channelFailures.Unlock()
}

func incrementChannelFailure(channelID int) int {
	if common.RedisEnabled && common.RDB != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		key := channelFailureRedisKey(channelID)
		pipe := common.RDB.Pipeline()
		incrCmd := pipe.Incr(ctx, key)
		pipe.Expire(ctx, key, healthFailureKeyTTL)
		if _, err := pipe.Exec(ctx); err == nil {
			return int(incrCmd.Val())
		} else {
			common.SysError("redis channel failure incr failed, falling back to memory: " + err.Error())
		}
	}
	channelFailures.Lock()
	channelFailures.counts[channelID]++
	count := channelFailures.counts[channelID]
	channelFailures.Unlock()
	return count
}

// RecordChannelFailure 记录一次渠道失败，必要时自动禁用并通知全集群下线该渠道。
func RecordChannelFailure(channelID int, usingKey string, autoBan bool, apiErr *types.StarAPIError) bool {
	if !common.ChannelAutoDisableEnabled || !autoBan || apiErr == nil || channelID <= 0 {
		return false
	}
	if !isChannelFault(apiErr) {
		return false
	}
	fatal := isFatalChannelError(apiErr)
	count := incrementChannelFailure(channelID)

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
