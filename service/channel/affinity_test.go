package channel

import (
	"net/http"
	"net/http/httptest"
	"openapi/common"
	"openapi/constant"
	"openapi/service/files"
	"openapi/setting"
	"os"
	"path/filepath"
	"testing"
	"time"

	gin "github.com/king54346/gin-tiny"
)

// newAffinityContext 构造一个带用户、模型与会话标识的测试上下文。
func newAffinityContext(t *testing.T, sessionID string) gin.Context {
	t.Helper()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.SetRequest(httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil))
	if sessionID != "" {
		c.Request().Header.Set("X-Session-Id", sessionID)
	}
	common.SetContextKey(c, constant.ContextKeyUserId, 1)
	common.SetContextKey(c, constant.ContextKeyOriginalModel, "gpt-4o")
	return c
}

func enableAffinity(t *testing.T, enabled bool, ttlSeconds int) {
	t.Helper()
	setting.UpdateGlobalSettings(func(s *setting.GlobalSettings) {
		s.ChannelAffinityEnabled = enabled
		s.ChannelAffinityTTLSeconds = ttlSeconds
		s.ChannelAffinityMaxEntries = 100
	})
	t.Cleanup(func() {
		setting.UpdateGlobalSettings(func(s *setting.GlobalSettings) {
			s.ChannelAffinityEnabled = false
			s.ChannelAffinityTTLSeconds = 300
			s.ChannelAffinityMaxEntries = 100000
		})
		ClearAffinityCache()
	})
}

func TestAffinityRecordAndHit(t *testing.T) {
	enableAffinity(t, true, 300)
	ClearAffinityCache()

	c := newAffinityContext(t, "sess-1")
	if _, found := GetPreferredChannelByAffinity(c, "gpt-4o"); found {
		t.Fatal("no affinity record yet")
	}
	RecordChannelAffinity(c, 7)

	channelID, found := GetPreferredChannelByAffinity(c, "gpt-4o")
	if !found || channelID != 7 {
		t.Fatalf("expected affinity hit on channel 7, got %d %v", channelID, found)
	}

	// 不同会话 / 不同模型不共享亲和记录
	if _, found := GetPreferredChannelByAffinity(newAffinityContext(t, "sess-2"), "gpt-4o"); found {
		t.Fatal("other session must not share affinity")
	}
	if _, found := GetPreferredChannelByAffinity(c, "gpt-4o-mini"); found {
		t.Fatal("other model must not share affinity")
	}

	stats := GetAffinityCacheStats()
	if !stats.Enabled || stats.Entries != 1 || stats.Records != 1 || stats.Hits != 1 || stats.Misses != 3 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	if stats.HitRate <= 0 || stats.HitRate > 1 {
		t.Fatalf("unexpected hit rate: %f", stats.HitRate)
	}
}

func TestAffinityDisabledByDefault(t *testing.T) {
	enableAffinity(t, false, 300)
	ClearAffinityCache()

	c := newAffinityContext(t, "sess-1")
	RecordChannelAffinity(c, 7)
	if _, found := GetPreferredChannelByAffinity(c, "gpt-4o"); found {
		t.Fatal("affinity must be ignored when disabled")
	}
	if stats := GetAffinityCacheStats(); stats.Entries != 0 || stats.Records != 0 {
		t.Fatalf("unexpected stats when disabled: %+v", stats)
	}
}

func TestAffinityExpires(t *testing.T) {
	enableAffinity(t, true, 1)
	ClearAffinityCache()

	c := newAffinityContext(t, "sess-expire")
	storeAffinity("expired-key", 9)
	// 手动把过期时间调到过去，模拟 TTL 到期
	affinityCache.mu.Lock()
	entry := affinityCache.entries["expired-key"]
	entry.expireAt = time.Now().Add(-time.Minute)
	affinityCache.entries["expired-key"] = entry
	affinityCache.mu.Unlock()

	if _, found := GetPreferredChannelByAffinity(c, "gpt-4o"); found {
		t.Fatal("expired entry must not hit")
	}
	if stats := GetAffinityCacheStats(); stats.Entries != 0 {
		t.Fatalf("expired entry should be purged: %+v", stats)
	}
}

func TestAffinityClearAndUsageStats(t *testing.T) {
	enableAffinity(t, true, 300)
	ClearAffinityCache()

	c := newAffinityContext(t, "sess-usage")
	RecordChannelAffinity(c, 3)
	MarkChannelAffinityUsed(c, 3)
	MarkChannelAffinityUsed(c, 5)

	usage := GetAffinityUsageStats()
	if len(usage) != 2 || usage[0].ChannelID != 3 || usage[0].Count != 2 || usage[1].ChannelID != 5 {
		t.Fatalf("unexpected usage stats: %+v", usage)
	}

	ClearAffinityCache()
	if stats := GetAffinityCacheStats(); stats.Entries != 0 || stats.Records != 0 || stats.Hits != 0 || stats.Misses != 0 {
		t.Fatalf("cache should be cleared: %+v", stats)
	}
	if len(GetAffinityUsageStats()) != 0 {
		t.Fatal("usage stats should be cleared")
	}
}

func TestRegisterTempFileAndCleanup(t *testing.T) {
	dir := t.TempDir()
	kept := filepath.Join(dir, "keep.txt")
	removed := filepath.Join(dir, "temp.txt")
	if err := os.WriteFile(kept, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(removed, []byte("temp"), 0o600); err != nil {
		t.Fatal(err)
	}

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	files.RegisterTempFile(c, removed)
	files.RegisterTempFile(c, removed) // 重复登记不会重复删除，也不会报错
	files.RegisterTempFile(c, "")      // 空路径忽略
	files.RegisterTempFile(c, filepath.Join(dir, "does-not-exist.txt"))

	files.CleanupFileSources(c)
	if _, err := os.Stat(removed); !os.IsNotExist(err) {
		t.Fatalf("temp file should be removed, err=%v", err)
	}
	if _, err := os.Stat(kept); err != nil {
		t.Fatalf("unrelated file must be kept: %v", err)
	}
	// 重复调用是安全的
	files.CleanupFileSources(c)
}
