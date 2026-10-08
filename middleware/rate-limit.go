package middleware

import (
	"context"
	"fmt"
	"net/http"

	"openapi/common"
	"openapi/common/limiter"

	gin "github.com/king54346/gin-tiny"
)

var inMemoryRateLimiter limiter.InMemoryRateLimiter

var defNext = func(c gin.Context) {
	c.Next()
}

func redisRateLimiter(c gin.Context, maxRequestNum int, duration int64, mark string) {
	ctx := context.Background()
	key := "rateLimit:" + mark + c.ClientIP()
	allowed, err := limiter.WindowTake(ctx, key, maxRequestNum, duration, common.RateLimitKeyExpirationDuration)
	if err != nil {
		fmt.Println(err.Error())
		c.Status(http.StatusInternalServerError)
		c.Abort()
		return
	}
	if !allowed {
		c.Status(http.StatusTooManyRequests)
		c.Abort()
		return
	}
}

func memoryRateLimiter(c gin.Context, maxRequestNum int, duration int64, mark string) {
	key := mark + c.ClientIP()
	if !inMemoryRateLimiter.Request(key, maxRequestNum, duration) {
		c.Status(http.StatusTooManyRequests)
		c.Abort()
		return
	}
}

func rateLimitFactory(maxRequestNum int, duration int64, mark string) func(c gin.Context) {
	if common.RedisEnabled {
		return func(c gin.Context) {
			redisRateLimiter(c, maxRequestNum, duration, mark)
		}
	} else {
		// It's safe to call multi times.
		inMemoryRateLimiter.Init(common.RateLimitKeyExpirationDuration)
		return func(c gin.Context) {
			memoryRateLimiter(c, maxRequestNum, duration, mark)
		}
	}
}

func GlobalWebRateLimit() func(c gin.Context) {
	if common.GlobalWebRateLimitEnable {
		return rateLimitFactory(common.GlobalWebRateLimitNum, common.GlobalWebRateLimitDuration, "GW")
	}
	return defNext
}

func GlobalAPIRateLimit() func(c gin.Context) {
	if common.GlobalApiRateLimitEnable {
		return rateLimitFactory(common.GlobalApiRateLimitNum, common.GlobalApiRateLimitDuration, "GA")
	}
	return defNext
}

func CriticalRateLimit() func(c gin.Context) {
	if common.CriticalRateLimitEnable {
		return rateLimitFactory(common.CriticalRateLimitNum, common.CriticalRateLimitDuration, "CT")
	}
	return defNext
}

func DownloadRateLimit() func(c gin.Context) {
	return rateLimitFactory(common.DownloadRateLimitNum, common.DownloadRateLimitDuration, "DW")
}

func UploadRateLimit() func(c gin.Context) {
	return rateLimitFactory(common.UploadRateLimitNum, common.UploadRateLimitDuration, "UP")
}
