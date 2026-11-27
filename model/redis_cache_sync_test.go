package model

import (
	"context"
	"testing"
	"time"

	"openapi/common"
	"openapi/constant"

	"github.com/alicebob/miniredis/v2"
	"github.com/glebarez/sqlite"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupRedisModelEnv(t *testing.T) *miniredis.Miniredis {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	mr := miniredis.RunT(t)
	oldRDB, oldEnabled := common.RDB, common.RedisEnabled
	common.RDB = redis.NewClient(&redis.Options{Addr: mr.Addr()})
	common.RedisEnabled = true

	ctx, cancel := context.WithCancel(context.Background())
	common.StartClusterEventBus(ctx)

	t.Cleanup(func() {
		cancel()
		common.StopClusterEventBus()
		_ = common.RDB.Close()
		common.RDB = oldRDB
		common.RedisEnabled = oldEnabled
		ClearAuthCache()
	})

	if err := InitDB(db, nil); err != nil {
		t.Fatal(err)
	}
	return mr
}

func TestRedisMultiKeyPollingUniformAndZeroDBWrite(t *testing.T) {
	setupRedisModelEnv(t)

	ch := &Channel{
		Type:   constant.ChannelTypeOpenAI,
		Name:   "multi-poll",
		Status: common.ChannelStatusEnabled,
		Key:    "k0\nk1\nk2",
		Models: "gpt-4o",
		ChannelInfo: ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 3,
			MultiKeyMode: constant.MultiKeyModePolling,
			MultiKeyStatusList: map[int]int{
				1: common.ChannelStatusManuallyDisabled, // 禁用 k1，只应在 k0 与 k2 间均匀轮询
			},
		},
	}
	if err := ch.Insert(); err != nil {
		t.Fatal(err)
	}

	var picked []string
	for range 6 {
		k, _, apiErr := ch.GetNextEnabledKey()
		if apiErr != nil {
			t.Fatalf("GetNextEnabledKey: %v", apiErr)
		}
		picked = append(picked, k)
	}
	want := []string{"k0", "k2", "k0", "k2", "k0", "k2"}
	for i := range want {
		if picked[i] != want[i] {
			t.Fatalf("polling sequence = %v, want %v", picked, want)
		}
	}
}

func TestRemoteClusterEventsInvalidateLocalCaches(t *testing.T) {
	mr := setupRedisModelEnv(t)

	u := &User{Username: "u_sync", Role: common.RoleCommonUser, Status: common.UserStatusEnabled}
	if err := u.Insert("password123"); err != nil {
		t.Fatal(err)
	}
	tk := &Token{UserId: u.Id, Name: "tk_sync", UnlimitedQuota: true, ExpiredTime: -1}
	if err := tk.Insert(); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateUserToken(tk.Key); err != nil {
		t.Fatalf("initial ValidateUserToken: %v", err)
	}

	// 直接修改底层库（模拟远端节点 B 禁用了该令牌并广播了 Pub/Sub 事件）
	DB.Model(&Token{}).Where("id = ?", tk.Id).Update("status", common.TokenStatusDisabled)

	// 尚未收到广播前，本地 L1 缓存仍命中启用态
	if _, err := ValidateUserToken(tk.Key); err != nil {
		t.Fatal("expected L1 cache hit before remote invalidation")
	}

	// 模拟远端节点发布 token:invalidate 事件
	remoteClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer remoteClient.Close()
	remoteBus := common.NewClusterEventBus(remoteClient, "")
	time.Sleep(50 * time.Millisecond)
	remoteBus.Publish(common.EventTopicTokenInvalidate, tk.Key)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := ValidateUserToken(tk.Key); err == ErrTokenDisabled {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("expected local L1 token cache to be invalidated by remote Pub/Sub event")
}
