package common

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	EventTopicTokenInvalidate     = "token:invalidate"
	EventTopicUserInvalidate      = "user:invalidate"
	EventTopicChannelRefresh      = "channel:refresh"
	EventTopicOptionRefresh       = "option:refresh"
	EventTopicModelTagsInvalidate = "model_tags:invalidate"
)

// ClusterEvent 集群缓存同步广播事件。
type ClusterEvent struct {
	NodeID string `json:"node_id"`
	Topic  string `json:"topic"`
	Key    string `json:"key,omitempty"`
}

// EventHandler 处理远端节点发来的集群事件。
type EventHandler func(key string)

// ClusterEventBus 基于 Redis Pub/Sub 的轻量跨节点事件总线（自动跳过本节点自环消息，断线自动重连）。
type ClusterEventBus struct {
	nodeID   string
	channel  string
	client   *redis.Client
	mu       sync.RWMutex
	handlers map[string][]EventHandler
	cancel   context.CancelFunc
	done     chan struct{}
}

var (
	defaultBusMu sync.RWMutex
	defaultBus   *ClusterEventBus
	globalHdlMu  sync.RWMutex
	globalHdls   = map[string][]EventHandler{}
)

// NewClusterEventBus 创建一个独立的集群事件总线实例。
func NewClusterEventBus(client *redis.Client, channelName string) *ClusterEventBus {
	if channelName == "" {
		channelName = RedisKey("cluster", "events")
	}
	return &ClusterEventBus{
		nodeID:   uuid.NewString(),
		channel:  channelName,
		client:   client,
		handlers: make(map[string][]EventHandler),
	}
}

// Subscribe 在总线实例上注册某个 topic 的回调。
func (b *ClusterEventBus) Subscribe(topic string, fn EventHandler) {
	if fn == nil {
		return
	}
	b.mu.Lock()
	b.handlers[topic] = append(b.handlers[topic], fn)
	b.mu.Unlock()
}

// Start 启动后台订阅协程。
func (b *ClusterEventBus) Start(parent context.Context) {
	if b == nil || b.client == nil {
		return
	}
	b.mu.Lock()
	if b.cancel != nil {
		b.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(parent)
	b.cancel = cancel
	b.done = make(chan struct{})
	done := b.done
	b.mu.Unlock()

	go func() {
		defer close(done)
		for {
			if ctx.Err() != nil {
				return
			}
			pubsub := b.client.Subscribe(ctx, b.channel)
			// 确认订阅就绪后再进入消息循环
			if _, err := pubsub.Receive(ctx); err != nil {
				_ = pubsub.Close()
				if ctx.Err() != nil {
					return
				}
				SysError("cluster event bus subscribe error: " + err.Error())
				select {
				case <-ctx.Done():
					return
				case <-time.After(time.Second):
					continue
				}
			}
			ch := pubsub.Channel()
		msgLoop:
			for {
				select {
				case <-ctx.Done():
					_ = pubsub.Close()
					return
				case msg, ok := <-ch:
					if !ok {
						_ = pubsub.Close()
						break msgLoop
					}
					b.dispatchRaw(msg.Payload)
				}
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(500 * time.Millisecond):
			}
		}
	}()
}

func (b *ClusterEventBus) dispatchRaw(payload string) {
	var ev ClusterEvent
	if err := Unmarshal([]byte(payload), &ev); err != nil {
		return
	}
	if ev.NodeID == b.nodeID || ev.Topic == "" {
		return
	}
	b.mu.RLock()
	fns := append([]EventHandler(nil), b.handlers[ev.Topic]...)
	b.mu.RUnlock()
	for _, fn := range fns {
		fn(ev.Key)
	}
}

// Publish 向 Redis 频道发布一条集群同步事件。
func (b *ClusterEventBus) Publish(topic, key string) {
	if b == nil || b.client == nil {
		return
	}
	payload, err := Marshal(ClusterEvent{
		NodeID: b.nodeID,
		Topic:  topic,
		Key:    key,
	})
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := b.client.Publish(ctx, b.channel, payload).Err(); err != nil && DebugEnabled {
		SysError("cluster event publish failed (" + topic + "): " + err.Error())
	}
}

// Stop 关闭订阅协程并释放连接。
func (b *ClusterEventBus) Stop() {
	if b == nil {
		return
	}
	b.mu.Lock()
	cancel := b.cancel
	done := b.done
	b.cancel = nil
	b.mu.Unlock()
	if cancel != nil {
		cancel()
		if done != nil {
			<-done
		}
	}
}

// RegisterClusterEventHandler 注册全局集群事件处理器（可在 Redis 初始化前后随时调用）。
func RegisterClusterEventHandler(topic string, fn EventHandler) {
	if fn == nil {
		return
	}
	globalHdlMu.Lock()
	globalHdls[topic] = append(globalHdls[topic], fn)
	globalHdlMu.Unlock()

	defaultBusMu.RLock()
	bus := defaultBus
	defaultBusMu.RUnlock()
	if bus != nil {
		bus.Subscribe(topic, fn)
	}
}

// StartClusterEventBus 在全局 RDB 已启用时启动默认集群事件总线。
func StartClusterEventBus(ctx context.Context) {
	if !RedisEnabled || RDB == nil {
		return
	}
	StopClusterEventBus()

	bus := NewClusterEventBus(RDB, "")
	globalHdlMu.RLock()
	for topic, fns := range globalHdls {
		for _, fn := range fns {
			bus.Subscribe(topic, fn)
		}
	}
	globalHdlMu.RUnlock()

	bus.Start(ctx)
	defaultBusMu.Lock()
	defaultBus = bus
	defaultBusMu.Unlock()
	SysLog("Redis cluster event bus started on channel " + bus.channel)
}

// StopClusterEventBus 停止默认集群事件总线。
func StopClusterEventBus() {
	defaultBusMu.Lock()
	bus := defaultBus
	defaultBus = nil
	defaultBusMu.Unlock()
	if bus != nil {
		bus.Stop()
	}
}

// PublishClusterEvent 通过默认集群事件总线广播事件；未启用 Redis 时自动静默忽略。
func PublishClusterEvent(topic, key string) {
	if !RedisEnabled {
		return
	}
	defaultBusMu.RLock()
	bus := defaultBus
	defaultBusMu.RUnlock()
	if bus != nil {
		bus.Publish(topic, key)
	}
}
