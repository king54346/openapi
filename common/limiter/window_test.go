package limiter

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"openapi/common"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// setupRedis 启动 miniredis 并替换 common.RDB，测试结束后恢复。
func setupRedis(t *testing.T) *miniredis.Miniredis {
	t.Helper()
	m := miniredis.RunT(t)
	m.SetTime(time.Unix(1_700_000_000, 0))
	client := redis.NewClient(&redis.Options{Addr: m.Addr()})
	old := common.RDB
	common.RDB = client
	t.Cleanup(func() {
		common.RDB = old
		_ = client.Close()
	})
	return m
}

func mustTake(t *testing.T, key string, max int, duration int64) bool {
	t.Helper()
	ok, err := WindowTake(t.Context(), key, max, duration, time.Minute)
	if err != nil {
		t.Fatalf("WindowTake: %v", err)
	}
	return ok
}

func TestWindowTakeLimitsAndSlides(t *testing.T) {
	m := setupRedis(t)

	for i := 0; i < 3; i++ {
		if !mustTake(t, "k", 3, 10) {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
	if mustTake(t, "k", 3, 10) {
		t.Fatal("4th request should be denied")
	}
	// 被拒绝的请求不计数
	if n, _ := m.List("k"); len(n) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(n))
	}

	m.SetTime(time.Unix(1_700_000_009, 0))
	if mustTake(t, "k", 3, 10) {
		t.Fatal("still inside window, should be denied")
	}

	m.SetTime(time.Unix(1_700_000_010, 0))
	if !mustTake(t, "k", 3, 10) {
		t.Fatal("window slid, should be allowed")
	}
	if n, _ := m.List("k"); len(n) != 3 {
		t.Fatalf("list should be trimmed to 3, got %d", len(n))
	}
}

func TestWindowTakeSetsExpiration(t *testing.T) {
	m := setupRedis(t)

	mustTake(t, "k", 3, 10)
	if ttl := m.TTL("k"); ttl != time.Minute {
		t.Fatalf("expected ttl 1m, got %v", ttl)
	}
	m.FastForward(time.Minute)
	if m.Exists("k") {
		t.Fatal("key should expire")
	}
}

func TestWindowAllowDoesNotRecord(t *testing.T) {
	m := setupRedis(t)
	ctx := t.Context()

	for i := 0; i < 5; i++ {
		ok, err := WindowAllow(ctx, "k", 2, 10, time.Minute)
		if err != nil || !ok {
			t.Fatalf("check %d: ok=%v err=%v", i, ok, err)
		}
	}
	if m.Exists("k") {
		t.Fatal("WindowAllow must not write")
	}

	for i := 0; i < 2; i++ {
		if err := WindowRecord(ctx, "k", 2, time.Minute); err != nil {
			t.Fatalf("WindowRecord: %v", err)
		}
	}
	ok, err := WindowAllow(ctx, "k", 2, 10, time.Minute)
	if err != nil || ok {
		t.Fatalf("should be denied after 2 records, ok=%v err=%v", ok, err)
	}
}

func TestWindowRecordIgnoresLimit(t *testing.T) {
	m := setupRedis(t)

	for i := 0; i < 5; i++ {
		if err := WindowRecord(t.Context(), "k", 3, time.Minute); err != nil {
			t.Fatalf("WindowRecord: %v", err)
		}
	}
	if n, _ := m.List("k"); len(n) != 3 {
		t.Fatalf("list should be trimmed to 3, got %d", len(n))
	}
}

func TestWindowLegacyEntriesTreatedAsExpired(t *testing.T) {
	m := setupRedis(t)

	// 旧版本以格式化时间字符串存储
	for i := 0; i < 3; i++ {
		m.Lpush("k", "2026-10-08T10:00:00.000Z")
	}
	if !mustTake(t, "k", 3, 10) {
		t.Fatal("unparseable legacy entries should be treated as expired")
	}
}

func TestWindowUnlimitedAndNoRedis(t *testing.T) {
	m := setupRedis(t)

	for i := 0; i < 5; i++ {
		if !mustTake(t, "k", 0, 10) {
			t.Fatal("maxCount=0 should allow")
		}
	}
	if m.Exists("k") {
		t.Fatal("maxCount=0 must not write")
	}

	common.RDB = nil
	if !mustTake(t, "k", 1, 10) || !mustTake(t, "k", 1, 10) {
		t.Fatal("nil RDB should allow")
	}
}

func TestWindowTakeConcurrent(t *testing.T) {
	setupRedis(t)

	const limit, workers = 10, 50
	var allowed atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := WindowTake(t.Context(), "k", limit, 10, time.Minute)
			if err != nil {
				t.Errorf("WindowTake: %v", err)
				return
			}
			if ok {
				allowed.Add(1)
			}
		}()
	}
	wg.Wait()
	if got := allowed.Load(); got != limit {
		t.Fatalf("expected exactly %d allowed, got %d", limit, got)
	}
}
