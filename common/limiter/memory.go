package limiter

import (
	"sync"
	"time"
)

// InMemoryRateLimiter 进程内滑动窗口限流器（Redis 未启用时的降级方案）。
type InMemoryRateLimiter struct {
	store              map[string]*[]int64
	mutex              sync.Mutex
	expirationDuration time.Duration
}

// Init 初始化存储并启动过期清理协程，可重复调用。
func (l *InMemoryRateLimiter) Init(expirationDuration time.Duration) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	if l.store != nil {
		return
	}
	l.store = make(map[string]*[]int64)
	l.expirationDuration = expirationDuration
	if expirationDuration > 0 {
		go l.clearExpiredItems()
	}
}

func (l *InMemoryRateLimiter) clearExpiredItems() {
	for {
		time.Sleep(l.expirationDuration)
		l.mutex.Lock()
		now := time.Now().Unix()
		for key, queue := range l.store {
			size := len(*queue)
			if size == 0 || now-(*queue)[size-1] > int64(l.expirationDuration.Seconds()) {
				delete(l.store, key)
			}
		}
		l.mutex.Unlock()
	}
}

// Request 记录一次请求并返回是否放行；duration 单位为秒，maxRequestNum<=0 视为不限流。
func (l *InMemoryRateLimiter) Request(key string, maxRequestNum int, duration int64) bool {
	if maxRequestNum <= 0 {
		return true
	}
	l.mutex.Lock()
	defer l.mutex.Unlock()
	// [old <-- new]
	now := time.Now().Unix()
	queue, ok := l.store[key]
	if !ok {
		s := make([]int64, 0, maxRequestNum)
		queue = &s
		l.store[key] = queue
	}
	if len(*queue) < maxRequestNum {
		*queue = append(*queue, now)
		return true
	}
	if now-(*queue)[0] >= duration {
		*queue = append((*queue)[1:], now)
		return true
	}
	return false
}
