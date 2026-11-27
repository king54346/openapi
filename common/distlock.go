package common

import (
	"context"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

/**
REDIS_KEY_PREFIX（默认 openapi:）以及基于 Lua 脚本校验持有者 Token 的分布式锁（支持单次抢锁 AcquireLock、自动续租 AcquireLockWithRenew、启动等待锁 AcquireLockWait
*/

// RedisKeyPrefix 统一 Redis Key 命名空间前缀，支持通过 REDIS_KEY_PREFIX 覆盖（多环境共用同一 Redis DB 时隔离）。
func RedisKeyPrefix() string {
	prefix := strings.TrimSpace(os.Getenv("REDIS_KEY_PREFIX"))
	if prefix == "" {
		return "openapi:"
	}
	if !strings.HasSuffix(prefix, ":") {
		prefix += ":"
	}
	return prefix
}

// RedisKey 拼接带统一命名空间前缀的 Redis Key。
func RedisKey(parts ...string) string {
	return RedisKeyPrefix() + strings.Join(parts, ":")
}

var releaseLockScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("DEL", KEYS[1])
end
return 0
`)

var renewLockScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("PEXPIRE", KEYS[1], ARGV[2])
end
return 0
`)

// AcquireLock 尝试非阻塞获取 Redis 分布式锁（SET key token NX PX ttl）。
// 未启用 Redis 时直接返回 (noop, true)；若 Redis 网络异常则返回 (_, false) 避免脑裂并发执行。
func AcquireLock(ctx context.Context, name string, ttl time.Duration) (release func(), acquired bool) {
	return AcquireLockOnClient(ctx, RDB, RedisEnabled, name, ttl)
}

// AcquireLockOnClient 在指定 Redis 客户端上获取分布式锁（便于多节点单元测试注入独立客户端）。
func AcquireLockOnClient(ctx context.Context, client *redis.Client, enabled bool, name string, ttl time.Duration) (release func(), acquired bool) {
	if !enabled || client == nil {
		return func() {}, true
	}
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	fullKey := RedisKey("lock", name)
	token := uuid.NewString()

	ok, err := client.SetNX(ctx, fullKey, token, ttl).Result()
	if err != nil || !ok {
		if err != nil {
			SysError("redis acquire lock " + fullKey + " failed: " + err.Error())
		}
		return func() {}, false
	}

	var once sync.Once
	releaseFn := func() {
		once.Do(func() {
			rctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = releaseLockScript.Run(rctx, client, []string{fullKey}, token).Err()
		})
	}
	return releaseFn, true
}

// AcquireLockWithRenew 获取分布式锁并启动后台看门狗按 ttl/3 自动续期，直至调用返回的 release()。
func AcquireLockWithRenew(ctx context.Context, name string, ttl time.Duration) (release func(), acquired bool) {
	return AcquireLockWithRenewOnClient(ctx, RDB, RedisEnabled, name, ttl)
}

// AcquireLockWithRenewOnClient 在指定客户端上获取带自动续期的分布式锁。
func AcquireLockWithRenewOnClient(ctx context.Context, client *redis.Client, enabled bool, name string, ttl time.Duration) (release func(), acquired bool) {
	if !enabled || client == nil {
		return func() {}, true
	}
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	fullKey := RedisKey("lock", name)
	token := uuid.NewString()

	ok, err := client.SetNX(ctx, fullKey, token, ttl).Result()
	if err != nil || !ok {
		if err != nil {
			SysError("redis acquire renewable lock " + fullKey + " failed: " + err.Error())
		}
		return func() {}, false
	}

	stopRenew := make(chan struct{})
	doneRenew := make(chan struct{})
	interval := max(ttl/3, 500*time.Millisecond)
	ms := ttl.Milliseconds()

	go func() {
		defer close(doneRenew)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-stopRenew:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				rctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				res, err := renewLockScript.Run(rctx, client, []string{fullKey}, token, ms).Int64()
				cancel()
				if err != nil || res == 0 {
					return
				}
			}
		}
	}()

	var once sync.Once
	releaseFn := func() {
		once.Do(func() {
			close(stopRenew)
			<-doneRenew
			rctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = releaseLockScript.Run(rctx, client, []string{fullKey}, token).Err()
		})
	}
	return releaseFn, true
}

// AcquireLockWait 阻塞等待获取分布式锁（如启动期 AutoMigrate 串行化），超时未抢到锁时返回 (_, false)。
func AcquireLockWait(ctx context.Context, name string, ttl, maxWait time.Duration) (release func(), acquired bool) {
	if !RedisEnabled || RDB == nil {
		return func() {}, true
	}
	deadline := time.Now().Add(maxWait)
	for {
		if rel, ok := AcquireLockWithRenew(ctx, name, ttl); ok {
			return rel, true
		}
		if time.Now().After(deadline) {
			return func() {}, false
		}
		select {
		case <-ctx.Done():
			return func() {}, false
		case <-time.After(100 * time.Millisecond):
		}
	}
}
