package common

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newTestMiniredis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return mr, client
}

func TestDistributedLockAcquireReleaseAndRenew(t *testing.T) {
	_, client := newTestMiniredis(t)
	ctx := context.Background()

	rel1, ok1 := AcquireLockOnClient(ctx, client, true, "job:1", 2*time.Second)
	if !ok1 {
		t.Fatal("first node should acquire lock")
	}
	if _, ok2 := AcquireLockOnClient(ctx, client, true, "job:1", 2*time.Second); ok2 {
		t.Fatal("second node must not acquire held lock")
	}

	// 释放后其他节点可重新获取
	rel1()
	rel3, ok3 := AcquireLockWithRenewOnClient(ctx, client, true, "job:1", 1500*time.Millisecond)
	if !ok3 {
		t.Fatal("should acquire lock after release")
	}
	if _, ok4 := AcquireLockOnClient(ctx, client, true, "job:1", time.Second); ok4 {
		t.Fatal("lock with renew must stay held")
	}
	rel3()
	if _, ok5 := AcquireLockOnClient(ctx, client, true, "job:1", time.Second); !ok5 {
		t.Fatal("should acquire after renewable lock release")
	}
}

func TestClusterEventBusCrossNodeDeliveryAndSelfSkip(t *testing.T) {
	_, clientA := newTestMiniredis(t)
	clientB := redis.NewClient(&redis.Options{Addr: clientA.Options().Addr})
	t.Cleanup(func() { _ = clientB.Close() })

	busA := NewClusterEventBus(clientA, "openapi:test:events")
	busB := NewClusterEventBus(clientB, "openapi:test:events")

	var recvA, recvB atomic.Int32
	gotKeyB := make(chan string, 1)

	busA.Subscribe(EventTopicTokenInvalidate, func(string) { recvA.Add(1) })
	busB.Subscribe(EventTopicTokenInvalidate, func(k string) {
		recvB.Add(1)
		gotKeyB <- k
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	busA.Start(ctx)
	busB.Start(ctx)
	defer busA.Stop()
	defer busB.Stop()

	// 等待订阅就绪
	time.Sleep(80 * time.Millisecond)

	busA.Publish(EventTopicTokenInvalidate, "tok-xyz")

	select {
	case k := <-gotKeyB:
		if k != "tok-xyz" {
			t.Fatalf("unexpected key on node B: %q", k)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for cluster event on node B")
	}

	if recvA.Load() != 0 {
		t.Fatalf("node A must ignore its own event, got %d", recvA.Load())
	}
}
