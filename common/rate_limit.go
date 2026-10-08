package common

import "time"

// 限流开关与配额（默认全部关闭）。限流实现见 common/limiter 包。
var (
	GlobalWebRateLimitEnable         = false
	GlobalWebRateLimitNum            = 0
	GlobalWebRateLimitDuration int64 = 60
	GlobalApiRateLimitEnable         = false
	GlobalApiRateLimitNum            = 0
	GlobalApiRateLimitDuration int64 = 60
	CriticalRateLimitEnable          = false
	CriticalRateLimitNum             = 0
	CriticalRateLimitDuration  int64 = 60
	DownloadRateLimitNum             = 0
	DownloadRateLimitDuration  int64 = 60
	UploadRateLimitNum               = 0
	UploadRateLimitDuration    int64 = 60

	// RateLimitKeyExpirationDuration 限流 key 过期时间。
	RateLimitKeyExpirationDuration = time.Minute
)
