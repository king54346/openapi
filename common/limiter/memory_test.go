package limiter

import "testing"

func TestInMemoryRateLimiter(t *testing.T) {
	var l InMemoryRateLimiter
	l.Init(0)

	for i := 0; i < 3; i++ {
		if !l.Request("k", 3, 60) {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
	if l.Request("k", 3, 60) {
		t.Fatal("4th request should be denied")
	}
	if !l.Request("other", 3, 60) {
		t.Fatal("different key should be independent")
	}
	// duration=0 时窗口立即滑过
	if !l.Request("k", 3, 0) {
		t.Fatal("zero duration should allow")
	}
}

func TestInMemoryRateLimiterUnlimited(t *testing.T) {
	var l InMemoryRateLimiter
	l.Init(0)

	// 曾经 maxRequestNum=0 第二次请求会越界 panic
	for i := 0; i < 3; i++ {
		if !l.Request("k", 0, 60) {
			t.Fatal("maxRequestNum=0 should allow")
		}
	}
}
