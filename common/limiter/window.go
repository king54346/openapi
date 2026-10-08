package limiter

import (
	"context"
	_ "embed"
	"time"

	"openapi/common"

	"github.com/redis/go-redis/v9"
)

//go:embed lua/sliding_window.lua
var slidingWindowLua string

// slidingWindowScript 首次执行走 EVALSHA，脚本未加载时自动回退 EVAL。
var slidingWindowScript = redis.NewScript(slidingWindowLua)

const (
	modeCheck  = 0 // 只检查
	modeTake   = 1 // 检查并记录
	modeRecord = 2 // 只记录
)

// runWindow 执行滑动窗口脚本。maxCount<=0 或 Redis 未连接时直接放行。
func runWindow(ctx context.Context, key string, maxCount int, duration int64, expiration time.Duration, mode int) (bool, error) {
	rdb := common.RDB
	if maxCount <= 0 || rdb == nil {
		return true, nil
	}
	n, err := slidingWindowScript.Run(ctx, rdb, []string{key},
		maxCount, duration*1000, expiration.Milliseconds(), mode).Int()
	if err != nil {
		return false, err
	}
	return n == 1, nil
}

// WindowAllow 只检查窗口是否允许新请求，不记录（配合 WindowRecord 实现"成功后才计数"）。
// duration 单位为秒；超限时刷新 key 过期时间为 expiration。
func WindowAllow(ctx context.Context, key string, maxCount int, duration int64, expiration time.Duration) (bool, error) {
	return runWindow(ctx, key, maxCount, duration, expiration, modeCheck)
}

// WindowTake 原子地检查并记录一次请求，放行时计数，拒绝时不计数。duration 单位为秒。
func WindowTake(ctx context.Context, key string, maxCount int, duration int64, expiration time.Duration) (bool, error) {
	return runWindow(ctx, key, maxCount, duration, expiration, modeTake)
}

// WindowRecord 无条件记录一次请求，裁剪到 maxCount 条并刷新过期时间。
func WindowRecord(ctx context.Context, key string, maxCount int, expiration time.Duration) error {
	_, err := runWindow(ctx, key, maxCount, 0, expiration, modeRecord)
	return err
}
