package cron

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"openapi/common"
	"openapi/model"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestRedisLockPreventsDuplicateMultiNodeCronTick(t *testing.T) {
	setupDB(t)
	mr := miniredis.RunT(t)
	oldRDB, oldEnabled := common.RDB, common.RedisEnabled
	common.RDB = redis.NewClient(&redis.Options{Addr: mr.Addr()})
	common.RedisEnabled = true
	t.Cleanup(func() {
		_ = common.RDB.Close()
		common.RDB = oldRDB
		common.RedisEnabled = oldEnabled
	})

	var execCount atomic.Int32
	Register("test_cluster_once", func(*model.CronJob) (string, bool) {
		execCount.Add(1)
		return "ok", true
	})

	job := &model.CronJob{
		Name:    "cluster-dedup",
		Type:    "test_cluster_once",
		Cron:    "* * * * *",
		Enabled: true,
	}
	if err := job.Insert(); err != nil {
		t.Fatal(err)
	}

	// 模拟 4 个对等节点在同一分钟触发 execute（分别错开一点时间，覆盖「前一个节点已跑完但仍在同一分钟内」的场景）
	tickMin := at("2026-10-09 13:27").Unix() / 60
	var wg sync.WaitGroup
	for i := range 4 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			time.Sleep(time.Duration(idx*15) * time.Millisecond)
			execute(job, false, tickMin)
		}(i)
	}
	wg.Wait()

	if got := execCount.Load(); got != 1 {
		t.Fatalf("expected cron job to execute exactly once across 4 nodes in the same minute, got %d", got)
	}
}
