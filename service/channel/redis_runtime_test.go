package channel

import (
	"net/http"
	"testing"

	"openapi/common"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func setupMiniredisForChannelSvc(t *testing.T) *miniredis.Miniredis {
	t.Helper()
	mr := miniredis.RunT(t)
	oldRDB, oldEnabled := common.RDB, common.RedisEnabled
	common.RDB = redis.NewClient(&redis.Options{Addr: mr.Addr()})
	common.RedisEnabled = true
	t.Cleanup(func() {
		ClearAffinityCache()
		_ = common.RDB.Close()
		common.RDB = oldRDB
		common.RedisEnabled = oldEnabled
	})
	return mr
}

func TestRedisCrossNodeSessionAffinity(t *testing.T) {
	setupMiniredisForChannelSvc(t)
	enableAffinity(t, true, 300)
	ClearAffinityCache()

	// 模拟节点 A 记录会话亲和性（清空本地 map 以证明读取完全来自共享 Redis）
	ctxA := newAffinityContext(t, "shared-sess-99")
	RecordChannelAffinity(ctxA, 12)
	affinityCache.mu.Lock()
	affinityCache.entries = map[string]affinityEntry{}
	affinityCache.mu.Unlock()

	// 模拟节点 B 处理同一会话的后续请求，从 Redis 命中渠道 12
	ctxB := newAffinityContext(t, "shared-sess-99")
	chID, found := GetPreferredChannelByAffinity(ctxB, "gpt-4o")
	if !found || chID != 12 {
		t.Fatalf("expected Redis cross-node affinity hit channel 12, got %d %v", chID, found)
	}
	MarkChannelAffinityUsed(ctxB, 12)

	stats := GetAffinityCacheStats()
	if !stats.Enabled || stats.Entries != 1 || stats.Hits != 1 || stats.Records != 1 {
		t.Fatalf("unexpected Redis affinity stats: %+v", stats)
	}
	usage := GetAffinityUsageStats()
	if len(usage) != 1 || usage[0].ChannelID != 12 || usage[0].Count != 2 {
		t.Fatalf("unexpected Redis affinity usage: %+v", usage)
	}

	ClearAffinityCache()
	if statsAfter := GetAffinityCacheStats(); statsAfter.Entries != 0 || statsAfter.Hits != 0 {
		t.Fatalf("expected empty Redis affinity stats after clear, got %+v", statsAfter)
	}
}

func TestRedisClusterWideHealthFailureCounter(t *testing.T) {
	setupHealthDB(t)
	setupMiniredisForChannelSvc(t)
	insertChannel(t, 1)

	// 阈值设为 3：模拟节点 A 失败 2 次后清空进程内存计数，节点 B 再失败第 3 次仍能累加触发集群熔断
	RecordChannelFailure(1, "k", true, statusErr(http.StatusServiceUnavailable, "node A err 1"))
	RecordChannelFailure(1, "k", true, statusErr(http.StatusServiceUnavailable, "node A err 2"))
	channelFailures.Lock()
	channelFailures.counts = make(map[int]int)
	channelFailures.Unlock()

	if channelStatus(t, 1) != common.ChannelStatusEnabled {
		t.Fatal("channel should still be enabled at 2 failures")
	}
	if !RecordChannelFailure(1, "k", true, statusErr(http.StatusServiceUnavailable, "node B err 3")) {
		t.Fatal("3rd cluster-wide failure via Redis should disable the channel")
	}
	if channelStatus(t, 1) != common.ChannelStatusAutoDisabled {
		t.Fatal("channel status in DB should be auto-disabled")
	}
}
