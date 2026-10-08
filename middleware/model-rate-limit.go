package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"openapi/common"
	"openapi/common/limiter"
	"openapi/constant"
	"openapi/setting"

	gin "github.com/king54346/gin-tiny"
)

const (
	ModelRequestRateLimitCountMark        = "MRRL"
	ModelRequestRateLimitSuccessCountMark = "MRRLS"
)

// modelInMemoryLimiter 模型限流专用的内存限流器实例，
// 与通用限流（rate-limit.go 的 inMemoryRateLimiter）隔离，避免 key 冲突。
var modelInMemoryLimiter limiter.InMemoryRateLimiter

func modelRateLimitWindow() time.Duration {
	minutes := setting.GetGlobalSettings().ModelRequestRateLimitDurationMinutes
	if minutes <= 0 {
		minutes = 1
	}
	return time.Duration(minutes) * time.Minute
}

// modelRateLimitExceededMessage 超限提示文案。
func modelRateLimitExceededMessage(template string, maxCount int) string {
	minutes := setting.GetGlobalSettings().ModelRequestRateLimitDurationMinutes
	return fmt.Sprintf(template, minutes, maxCount)
}

// redisRateLimitHandler Redis 限流处理器：先查成功数窗口，再原子地检查并计入总数窗口。
func redisRateLimitHandler(duration int64, totalMaxCount, successMaxCount int) gin.HandlerFunc {
	return func(c gin.Context) {
		userID := strconv.Itoa(c.GetInt("id"))
		ctx := context.Background()
		window := modelRateLimitWindow()

		successKey := fmt.Sprintf("rateLimit:%s:%s", ModelRequestRateLimitSuccessCountMark, userID)
		allowed, err := limiter.WindowAllow(ctx, successKey, successMaxCount, duration, window)
		if err != nil {
			fmt.Println("check success rate limit failed:", err.Error())
			abortWithOpenAiMessage(c, http.StatusInternalServerError, "rate_limit_check_failed")
			return
		}
		if !allowed {
			abortWithOpenAiMessage(c, http.StatusTooManyRequests,
				modelRateLimitExceededMessage("request limit reached: at most %d requests per %d minutes", successMaxCount))
			return
		}

		if totalMaxCount > 0 {
			totalKey := fmt.Sprintf("rateLimit:%s:%s", ModelRequestRateLimitCountMark, userID)
			allowed, err := limiter.WindowTake(ctx, totalKey, totalMaxCount, duration, window)
			if err != nil {
				fmt.Println("check total rate limit failed:", err.Error())
				abortWithOpenAiMessage(c, http.StatusInternalServerError, "rate_limit_check_failed")
				return
			}
			if !allowed {
				abortWithOpenAiMessage(c, http.StatusTooManyRequests,
					modelRateLimitExceededMessage("total request limit reached: at most %d requests per %d minutes (including failures), please check your requests", totalMaxCount))
				return
			}
		}

		c.Next()

		if c.Response().Status() < 400 {
			_ = limiter.WindowRecord(ctx, successKey, successMaxCount, window)
		}
	}
}

// memoryRateLimitHandler 内存限流处理器（Redis 未启用时的降级方案）。
func memoryRateLimitHandler(duration int64, totalMaxCount, successMaxCount int) gin.HandlerFunc {
	modelInMemoryLimiter.Init(modelRateLimitWindow())

	return func(c gin.Context) {
		userID := strconv.Itoa(c.GetInt("id"))
		totalKey := ModelRequestRateLimitCountMark + userID
		successKey := ModelRequestRateLimitSuccessCountMark + userID

		if totalMaxCount > 0 && !modelInMemoryLimiter.Request(totalKey, totalMaxCount, duration) {
			c.Status(http.StatusTooManyRequests)
			c.Abort()
			return
		}

		if !modelInMemoryLimiter.Request(successKey+"_check", successMaxCount, duration) {
			c.Status(http.StatusTooManyRequests)
			c.Abort()
			return
		}

		c.Next()

		if c.Response().Status() < 400 {
			modelInMemoryLimiter.Request(successKey, successMaxCount, duration)
		}
	}
}

// ModelRequestRateLimit 模型请求限流中间件。
func ModelRequestRateLimit() gin.HandlerFunc {
	return func(c gin.Context) {
		cfg := setting.GetGlobalSettings()
		if !cfg.ModelRequestRateLimitEnabled {
			c.Next()
			return
		}

		duration := int64(cfg.ModelRequestRateLimitDurationMinutes * 60)
		totalMaxCount := cfg.ModelRequestRateLimitCount
		successMaxCount := cfg.ModelRequestRateLimitSuccessCount

		group := common.GetContextKeyString(c, constant.ContextKeyTokenGroup)
		if group == "" {
			group = common.GetContextKeyString(c, constant.ContextKeyUserGroup)
		}
		if groupTotalCount, groupSuccessCount, found := setting.GetGroupRateLimit(group); found {
			totalMaxCount = groupTotalCount
			successMaxCount = groupSuccessCount
		}

		if common.RedisEnabled {
			redisRateLimitHandler(duration, totalMaxCount, successMaxCount)(c)
		} else {
			memoryRateLimitHandler(duration, totalMaxCount, successMaxCount)(c)
		}
	}
}
