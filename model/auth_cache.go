package model

import (
	"sync"
	"time"

	"openapi/common"
)

// ttlCache 带过期时间的简单内存缓存，存值拷贝，调用方拿到的副本可随意修改。
type ttlCache[K comparable, V any] struct {
	mu      sync.RWMutex
	entries map[K]ttlEntry[V]
}

type ttlEntry[V any] struct {
	value  V
	expire time.Time
}

// maxAuthCacheEntries 超过后写入时顺带清理过期项，避免无界增长
const maxAuthCacheEntries = 10000

func newTTLCache[K comparable, V any]() *ttlCache[K, V] {
	return &ttlCache[K, V]{entries: make(map[K]ttlEntry[V])}
}

func (c *ttlCache[K, V]) get(key K) (V, bool) {
	c.mu.RLock()
	e, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok || time.Now().After(e.expire) {
		var zero V
		return zero, false
	}
	return e.value, true
}

func (c *ttlCache[K, V]) set(key K, value V) {
	ttl := authCacheTTL()
	if ttl <= 0 {
		return
	}
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) >= maxAuthCacheEntries {
		for k, e := range c.entries {
			if now.After(e.expire) {
				delete(c.entries, k)
			}
		}
	}
	c.entries[key] = ttlEntry[V]{value: value, expire: now.Add(ttl)}
}

func (c *ttlCache[K, V]) delete(key K) {
	c.mu.Lock()
	delete(c.entries, key)
	c.mu.Unlock()
}

func (c *ttlCache[K, V]) clear() {
	c.mu.Lock()
	c.entries = make(map[K]ttlEntry[V])
	c.mu.Unlock()
}

// authCacheTTL 令牌/用户缓存有效期，与渠道缓存同步周期一致（SYNC_FREQUENCY）。
// 通过本服务的管理接口修改会立即失效；直接改库的变更最多延迟一个周期生效。
func authCacheTTL() time.Duration {
	return time.Duration(common.SyncFrequency) * time.Second
}

var (
	tokenCache = newTTLCache[string, Token]() // key -> token
	userCache  = newTTLCache[int, User]()     // id -> user
)

// InvalidateTokenCache 令牌被修改或删除后调用。
func InvalidateTokenCache(key string) {
	tokenCache.delete(key)
}

// InvalidateUserCache 用户被修改或删除后调用。
func InvalidateUserCache(userId int) {
	userCache.delete(userId)
}

// ClearAuthCache 清空令牌与用户缓存。
func ClearAuthCache() {
	tokenCache.clear()
	userCache.clear()
}
