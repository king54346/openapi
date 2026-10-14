package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"openapi/common"
	"openapi/constant"
	"openapi/setting"

	gin "github.com/king54346/gin-tiny"
	"github.com/redis/go-redis/v9"
	"github.com/tidwall/gjson"
)

// 渠道亲和性：按配置规则从请求中取出用户 / 会话 / 请求标识，在 TTL 内优先复用该标识上次成功的渠道。
//
// 一次请求的流程：
//  1. Distribute 调 GetPreferredChannelByAffinity：匹配规则、取标识、查绑定（计一次请求）；
//  2. 绑定的渠道仍可用时 Distribute 直接使用它并调 MarkChannelAffinityUsed（计一次命中）；
//  3. 转发失败时 OnChannelAffinityFailure 解除失效的绑定，规则要求时 ShouldSkipRetryByAffinity 阻止换渠道；
//  4. 请求成功后 RecordChannelAffinity 把最终成功的渠道写回绑定并续期；
//  5. 记录用量时 RecordChannelAffinityUsage 累计 token。
//
// 绑定启用 Redis 时存 Redis（多节点共享），否则存本地内存；命中与用量统计为本节点内存统计。

const channelAffinityKeyPrefix = "channel_affinity:"

// channelAffinityState 单次请求的亲和性状态，存于上下文 ContextKeyChannelAffinity。
type channelAffinityState struct {
	rule      string
	keyHash   string
	cacheKey  string
	model     string
	group     string
	ttl       time.Duration
	skipRetry bool

	boundChannelID int  // 查询时已绑定的渠道，0 表示无绑定
	hit            bool // 本次请求实际使用了绑定的渠道
	usageRecorded  bool
}

func getAffinityState(c gin.Context) *channelAffinityState {
	if c == nil {
		return nil
	}
	state, _ := common.GetContextKeyType[*channelAffinityState](c, constant.ContextKeyChannelAffinity)
	return state
}

// ---------------------------------------------------------------------------
// 请求流程
// ---------------------------------------------------------------------------

// GetPreferredChannelByAffinity 匹配亲和性规则并返回已绑定的渠道 ID。
// 规则命中（取到标识）时会在上下文中记录状态，后续成功时据此写回绑定。
func GetPreferredChannelByAffinity(c gin.Context, modelName, usingGroup string) (int, bool) {
	cfg := setting.GetChannelAffinitySetting()
	if c == nil || !cfg.Enabled || modelName == "" {
		return 0, false
	}
	path := c.Request().URL.Path
	for i := range cfg.Rules {
		rule := &cfg.Rules[i]
		if !rule.Enabled || !rule.MatchModel(modelName) || !rule.MatchPath(path) {
			continue
		}
		value := extractAffinityValue(c, rule)
		if value == "" {
			continue
		}
		state := newAffinityState(cfg, rule, value, modelName, usingGroup)
		common.SetContextKey(c, constant.ContextKeyChannelAffinity, state)
		if channelID, ok := affinityStore().get(state.cacheKey); ok {
			state.boundChannelID = channelID
		}
		affinityStats.onRequest(state)
		return state.boundChannelID, state.boundChannelID > 0
	}
	return 0, false
}

// MarkChannelAffinityUsed 标记本次请求使用了亲和性绑定的渠道。
func MarkChannelAffinityUsed(c gin.Context, _ string, channelID int) {
	state := getAffinityState(c)
	if state == nil || state.hit || channelID != state.boundChannelID {
		return
	}
	state.hit = true
	affinityStats.onHit(state)
}

// RecordChannelAffinity 请求成功后把最终使用的渠道写回绑定并续期。
func RecordChannelAffinity(c gin.Context, channelID int) {
	state := getAffinityState(c)
	if state == nil || channelID <= 0 {
		return
	}
	affinityStore().set(state.cacheKey, channelID, state.ttl)
	affinityStats.onSuccess(state, channelID)
}

// OnChannelAffinityFailure 绑定的渠道转发失败（渠道侧错误）时解除绑定，下次请求重新选择渠道。
func OnChannelAffinityFailure(c gin.Context, channelID int) {
	state := getAffinityState(c)
	if state == nil || !state.hit || channelID != state.boundChannelID {
		return
	}
	affinityStore().del(state.cacheKey)
	affinityStats.onFailure(state)
}

// ShouldSkipRetryByAffinity 命中亲和渠道且规则配置了 skip_retry_on_failure 时不换渠道重试。
func ShouldSkipRetryByAffinity(c gin.Context) bool {
	state := getAffinityState(c)
	return state != nil && state.hit && state.skipRetry
}

// RecordChannelAffinityUsage 累计亲和性请求的 token 用量，每个请求只记一次。
func RecordChannelAffinityUsage(c gin.Context, promptTokens, completionTokens, cachedTokens int) {
	state := getAffinityState(c)
	if state == nil || state.usageRecorded {
		return
	}
	state.usageRecorded = true
	affinityStats.onUsage(state, int64(promptTokens), int64(completionTokens), int64(cachedTokens))
}

// ChannelAffinityLogInfo 写入日志 other 字段的亲和性信息，未匹配规则时返回 nil。
func ChannelAffinityLogInfo(c gin.Context) map[string]any {
	state := getAffinityState(c)
	if state == nil {
		return nil
	}
	return map[string]any{
		"rule":             state.rule,
		"key_hash":         state.keyHash,
		"hit":              state.hit,
		"bound_channel_id": state.boundChannelID,
	}
}

func newAffinityState(cfg *setting.ChannelAffinitySetting, rule *setting.ChannelAffinityRule, value, modelName, usingGroup string) *channelAffinityState {
	sum := sha256.Sum256([]byte(value))
	state := &channelAffinityState{
		rule:      rule.Name,
		keyHash:   hex.EncodeToString(sum[:8]),
		skipRetry: rule.SkipRetryOnFailure,
	}
	if rule.IncludeModel {
		state.model = modelName
	}
	if rule.IncludeUsingGroup {
		state.group = usingGroup
	}
	ttl := rule.TTLSeconds
	if ttl <= 0 {
		ttl = cfg.DefaultTTLSeconds
	}
	state.ttl = time.Duration(ttl) * time.Second
	// 规则名不含 ':'，按规则前缀清理时不会误删
	state.cacheKey = channelAffinityKeyPrefix + state.rule + ":" + state.group + ":" + state.model + ":" + state.keyHash
	return state
}

// extractAffinityValue 按规则的来源顺序取第一个非空且满足 value_regex 的标识值。
func extractAffinityValue(c gin.Context, rule *setting.ChannelAffinityRule) string {
	for _, src := range rule.KeySources {
		value := strings.TrimSpace(readAffinitySource(c, src))
		if value != "" && rule.MatchValue(value) {
			return value
		}
	}
	return ""
}

func readAffinitySource(c gin.Context, src setting.ChannelAffinityKeySource) string {
	switch src.Type {
	case setting.AffinityKeySourceHeader:
		return c.Request().Header.Get(src.Key)
	case setting.AffinityKeySourceQuery:
		return c.Query(src.Key)
	case setting.AffinityKeySourceContextInt:
		if v := common.GetContextKeyInt(c, constant.ContextKey(src.Key)); v != 0 {
			return strconv.Itoa(v)
		}
	case setting.AffinityKeySourceContextString:
		return common.GetContextKeyString(c, constant.ContextKey(src.Key))
	case setting.AffinityKeySourceJSON:
		if strings.Contains(c.Request().Header.Get("Content-Type"), "multipart/form-data") {
			return ""
		}
		body, err := common.GetRequestBody(c)
		if err != nil || len(body) == 0 {
			return ""
		}
		result := gjson.GetBytes(body, src.Path)
		if result.Type == gjson.String || result.Type == gjson.Number {
			return result.String()
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// 绑定存储
// ---------------------------------------------------------------------------

type affinityBindingStore interface {
	get(key string) (int, bool)
	set(key string, channelID int, ttl time.Duration)
	del(key string)
	// clear 删除以 prefix 开头的绑定，返回删除数量
	clear(prefix string) int
	// countByRule 各规则当前的绑定数
	countByRule() map[string]int
	backend() string
}

var memoryAffinityStore = newMemoryAffinityStore()

func affinityStore() affinityBindingStore {
	if common.RedisEnabled && common.RDB != nil {
		return redisAffinityStore{}
	}
	return memoryAffinityStore
}

type memoryAffinityEntry struct {
	channelID int
	expireAt  time.Time
}

type memoryAffinityBindings struct {
	mu      sync.Mutex
	entries map[string]memoryAffinityEntry
}

func newMemoryAffinityStore() *memoryAffinityBindings {
	return &memoryAffinityBindings{entries: make(map[string]memoryAffinityEntry)}
}

func (m *memoryAffinityBindings) backend() string { return "memory" }

func (m *memoryAffinityBindings) get(key string) (int, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry, ok := m.entries[key]
	if !ok {
		return 0, false
	}
	if time.Now().After(entry.expireAt) {
		delete(m.entries, key)
		return 0, false
	}
	return entry.channelID, true
}

func (m *memoryAffinityBindings) set(key string, channelID int, ttl time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries[key] = memoryAffinityEntry{channelID: channelID, expireAt: time.Now().Add(ttl)}
	if maxEntries := setting.GetChannelAffinitySetting().MaxEntries; len(m.entries) > maxEntries {
		m.evictLocked(maxEntries)
	}
}

// evictLocked 先清过期项，仍超限时按过期时间淘汰最早的一批（续期即视为最近使用），降到上限的 90%。
func (m *memoryAffinityBindings) evictLocked(maxEntries int) {
	now := time.Now()
	for k, e := range m.entries {
		if now.After(e.expireAt) {
			delete(m.entries, k)
		}
	}
	if len(m.entries) <= maxEntries {
		return
	}
	keys := make([]string, 0, len(m.entries))
	for k := range m.entries {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return m.entries[keys[i]].expireAt.Before(m.entries[keys[j]].expireAt) })
	target := maxEntries * 9 / 10
	for _, k := range keys[:len(keys)-target] {
		delete(m.entries, k)
	}
}

func (m *memoryAffinityBindings) del(key string) {
	m.mu.Lock()
	delete(m.entries, key)
	m.mu.Unlock()
}

func (m *memoryAffinityBindings) clear(prefix string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for k := range m.entries {
		if strings.HasPrefix(k, prefix) {
			delete(m.entries, k)
			n++
		}
	}
	return n
}

func (m *memoryAffinityBindings) countByRule() map[string]int {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	out := map[string]int{}
	for k, e := range m.entries {
		if now.After(e.expireAt) {
			continue
		}
		out[ruleOfCacheKey(k)]++
	}
	return out
}

type redisAffinityStore struct{}

const redisAffinityTimeout = 200 * time.Millisecond

func (redisAffinityStore) backend() string { return "redis" }

func (redisAffinityStore) get(key string) (int, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), redisAffinityTimeout)
	defer cancel()
	id, err := common.RDB.Get(ctx, key).Int()
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			common.SysError("channel affinity redis get failed: " + err.Error())
		}
		return 0, false
	}
	return id, id > 0
}

func (redisAffinityStore) set(key string, channelID int, ttl time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), redisAffinityTimeout)
	defer cancel()
	if err := common.RDB.Set(ctx, key, channelID, ttl).Err(); err != nil {
		common.SysError("channel affinity redis set failed: " + err.Error())
	}
}

func (redisAffinityStore) del(key string) {
	ctx, cancel := context.WithTimeout(context.Background(), redisAffinityTimeout)
	defer cancel()
	if err := common.RDB.Del(ctx, key).Err(); err != nil {
		common.SysError("channel affinity redis del failed: " + err.Error())
	}
}

func (redisAffinityStore) scan(prefix string, fn func(keys []string)) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var cursor uint64
	for {
		keys, next, err := common.RDB.Scan(ctx, cursor, prefix+"*", 1000).Result()
		if err != nil {
			common.SysError("channel affinity redis scan failed: " + err.Error())
			return
		}
		if len(keys) > 0 {
			fn(keys)
		}
		if cursor = next; cursor == 0 {
			return
		}
	}
}

func (s redisAffinityStore) clear(prefix string) int {
	n := 0
	s.scan(prefix, func(keys []string) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		deleted, err := common.RDB.Del(ctx, keys...).Result()
		if err != nil {
			common.SysError("channel affinity redis del failed: " + err.Error())
			return
		}
		n += int(deleted)
	})
	return n
}

func (s redisAffinityStore) countByRule() map[string]int {
	out := map[string]int{}
	s.scan(channelAffinityKeyPrefix, func(keys []string) {
		for _, k := range keys {
			out[ruleOfCacheKey(k)]++
		}
	})
	return out
}

func ruleOfCacheKey(key string) string {
	rest := strings.TrimPrefix(key, channelAffinityKeyPrefix)
	if i := strings.IndexByte(rest, ':'); i >= 0 {
		return rest[:i]
	}
	return rest
}

// ClearChannelAffinityBindings 清除绑定，rule 为空时清除全部，返回删除数量。
func ClearChannelAffinityBindings(rule string) int {
	prefix := channelAffinityKeyPrefix
	if rule != "" {
		prefix += rule + ":"
	}
	return affinityStore().clear(prefix)
}

// ChannelAffinityCacheStats 绑定缓存概况。
type ChannelAffinityCacheStats struct {
	Enabled      bool                   `json:"enabled"`
	Backend      string                 `json:"backend"`
	TotalEntries int                    `json:"total_entries"`
	MaxEntries   int                    `json:"max_entries"`
	Rules        []ChannelAffinityRuleN `json:"rules"`
}

// ChannelAffinityRuleN 单条规则的绑定数。
type ChannelAffinityRuleN struct {
	Name       string `json:"name"`
	Enabled    bool   `json:"enabled"`
	TTLSeconds int    `json:"ttl_seconds"`
	Entries    int    `json:"entries"`
}

// GetChannelAffinityCacheStats 当前绑定缓存概况（含已从配置中移除但仍有绑定的规则）。
func GetChannelAffinityCacheStats() ChannelAffinityCacheStats {
	cfg := setting.GetChannelAffinitySetting()
	store := affinityStore()
	counts := store.countByRule()
	stats := ChannelAffinityCacheStats{
		Enabled:    cfg.Enabled,
		Backend:    store.backend(),
		MaxEntries: cfg.MaxEntries,
		Rules:      make([]ChannelAffinityRuleN, 0, len(cfg.Rules)),
	}
	for _, r := range cfg.Rules {
		ttl := r.TTLSeconds
		if ttl <= 0 {
			ttl = cfg.DefaultTTLSeconds
		}
		stats.Rules = append(stats.Rules, ChannelAffinityRuleN{Name: r.Name, Enabled: r.Enabled, TTLSeconds: ttl, Entries: counts[r.Name]})
		delete(counts, r.Name)
	}
	for name, n := range counts {
		stats.Rules = append(stats.Rules, ChannelAffinityRuleN{Name: name, Entries: n})
	}
	for _, r := range stats.Rules {
		stats.TotalEntries += r.Entries
	}
	return stats
}

// ---------------------------------------------------------------------------
// 命中、请求次数与 token 用量统计（本节点内存）
// ---------------------------------------------------------------------------

// ChannelAffinityCounters 命中与用量计数。
type ChannelAffinityCounters struct {
	// Requests 匹配到规则并取到标识的请求数
	Requests int64 `json:"requests"`
	// Hits 使用了已绑定渠道的请求数
	Hits int64 `json:"hits"`
	// Misses 未使用绑定渠道（无绑定或绑定渠道不可用）的请求数
	Misses int64 `json:"misses"`
	// Successes 成功并写回绑定的请求数
	Successes int64 `json:"successes"`
	// Failures 绑定渠道转发失败而解除绑定的次数
	Failures int64 `json:"failures"`
	// Switches 成功渠道与原绑定不同、绑定被改写的次数
	Switches         int64   `json:"switches"`
	HitRate          float64 `json:"hit_rate"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	CachedTokens     int64   `json:"cached_tokens"`
	TotalTokens      int64   `json:"total_tokens"`
}

func (s *ChannelAffinityCounters) finalize() {
	s.Misses = s.Requests - s.Hits
	s.TotalTokens = s.PromptTokens + s.CompletionTokens
	if s.Requests > 0 {
		s.HitRate = float64(s.Hits) / float64(s.Requests)
	}
}

// ChannelAffinityRuleUsage 单条规则的累计统计。
type ChannelAffinityRuleUsage struct {
	Rule string `json:"rule"`
	ChannelAffinityCounters
}

// ChannelAffinityKeyUsage 单个标识（规则 + 分组 + 模型 + 标识哈希）的累计统计。
type ChannelAffinityKeyUsage struct {
	Rule      string    `json:"rule"`
	Group     string    `json:"group,omitempty"`
	Model     string    `json:"model,omitempty"`
	KeyHash   string    `json:"key_hash"`
	ChannelID int       `json:"channel_id"`
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
	ChannelAffinityCounters
}

// ChannelAffinityUsageStats 用量统计查询结果。
type ChannelAffinityUsageStats struct {
	Since      time.Time                  `json:"since"`
	Total      ChannelAffinityCounters    `json:"total"`
	Rules      []ChannelAffinityRuleUsage `json:"rules"`
	KeyCount   int                        `json:"key_count"`
	Keys       []ChannelAffinityKeyUsage  `json:"keys"`
	MaxEntries int                        `json:"max_entries"`
}

type channelAffinityStatsStore struct {
	mu    sync.Mutex
	since time.Time
	rules map[string]*ChannelAffinityCounters
	keys  map[string]*ChannelAffinityKeyUsage
}

var affinityStats = newChannelAffinityStatsStore()

func newChannelAffinityStatsStore() *channelAffinityStatsStore {
	return &channelAffinityStatsStore{
		since: time.Now(),
		rules: make(map[string]*ChannelAffinityCounters),
		keys:  make(map[string]*ChannelAffinityKeyUsage),
	}
}

// update 在锁内对规则与标识两级计数执行 fn。
func (s *channelAffinityStatsStore) update(state *channelAffinityState, fn func(c *ChannelAffinityCounters, k *ChannelAffinityKeyUsage)) {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	rule := s.rules[state.rule]
	if rule == nil {
		rule = &ChannelAffinityCounters{}
		s.rules[state.rule] = rule
	}
	key := s.keys[state.cacheKey]
	if key == nil {
		key = &ChannelAffinityKeyUsage{Rule: state.rule, Group: state.group, Model: state.model, KeyHash: state.keyHash, FirstSeen: now, LastSeen: now}
		s.keys[state.cacheKey] = key
		if maxEntries := setting.GetChannelAffinitySetting().MaxUsageEntries; len(s.keys) > maxEntries {
			s.evictLocked(maxEntries)
		}
	}
	key.LastSeen = now
	fn(rule, key)
	fn(&key.ChannelAffinityCounters, key)
}

// evictLocked 淘汰最久未出现的标识，降到上限的 90%；规则级累计不受影响。
func (s *channelAffinityStatsStore) evictLocked(maxEntries int) {
	keys := make([]string, 0, len(s.keys))
	for k := range s.keys {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return s.keys[keys[i]].LastSeen.Before(s.keys[keys[j]].LastSeen) })
	for _, k := range keys[:len(keys)-maxEntries*9/10] {
		delete(s.keys, k)
	}
}

func (s *channelAffinityStatsStore) onRequest(state *channelAffinityState) {
	s.update(state, func(c *ChannelAffinityCounters, k *ChannelAffinityKeyUsage) {
		c.Requests++
		if state.boundChannelID > 0 {
			k.ChannelID = state.boundChannelID
		}
	})
}

func (s *channelAffinityStatsStore) onHit(state *channelAffinityState) {
	s.update(state, func(c *ChannelAffinityCounters, _ *ChannelAffinityKeyUsage) { c.Hits++ })
}

func (s *channelAffinityStatsStore) onSuccess(state *channelAffinityState, channelID int) {
	s.update(state, func(c *ChannelAffinityCounters, k *ChannelAffinityKeyUsage) {
		c.Successes++
		if state.boundChannelID > 0 && state.boundChannelID != channelID {
			c.Switches++
		}
		k.ChannelID = channelID
	})
}

func (s *channelAffinityStatsStore) onFailure(state *channelAffinityState) {
	s.update(state, func(c *ChannelAffinityCounters, k *ChannelAffinityKeyUsage) {
		c.Failures++
		k.ChannelID = 0
	})
}

func (s *channelAffinityStatsStore) onUsage(state *channelAffinityState, prompt, completion, cached int64) {
	s.update(state, func(c *ChannelAffinityCounters, _ *ChannelAffinityKeyUsage) {
		c.PromptTokens += prompt
		c.CompletionTokens += completion
		c.CachedTokens += cached
	})
}

// ChannelAffinityUsageQuery 用量统计查询条件。
type ChannelAffinityUsageQuery struct {
	Rule  string // 只看某条规则，空为全部
	Limit int    // 返回的标识条数，<=0 时为 100
	// SortBy 标识排序：last_seen（默认）/ requests / hits / tokens
	SortBy string
}

// GetChannelAffinityUsageStats 汇总命中、请求次数与 token 用量。
func GetChannelAffinityUsageStats(q ChannelAffinityUsageQuery) ChannelAffinityUsageStats {
	if q.Limit <= 0 {
		q.Limit = 100
	}
	s := affinityStats
	s.mu.Lock()
	out := ChannelAffinityUsageStats{
		Since:      s.since,
		Rules:      make([]ChannelAffinityRuleUsage, 0, len(s.rules)),
		MaxEntries: setting.GetChannelAffinitySetting().MaxUsageEntries,
	}
	for name, c := range s.rules {
		if q.Rule != "" && name != q.Rule {
			continue
		}
		r := ChannelAffinityRuleUsage{Rule: name, ChannelAffinityCounters: *c}
		r.finalize()
		out.Rules = append(out.Rules, r)
		out.Total.Requests += c.Requests
		out.Total.Hits += c.Hits
		out.Total.Successes += c.Successes
		out.Total.Failures += c.Failures
		out.Total.Switches += c.Switches
		out.Total.PromptTokens += c.PromptTokens
		out.Total.CompletionTokens += c.CompletionTokens
		out.Total.CachedTokens += c.CachedTokens
	}
	keys := make([]ChannelAffinityKeyUsage, 0, len(s.keys))
	for _, k := range s.keys {
		if q.Rule != "" && k.Rule != q.Rule {
			continue
		}
		item := *k
		item.finalize()
		keys = append(keys, item)
	}
	s.mu.Unlock()

	out.Total.finalize()
	sort.Slice(out.Rules, func(i, j int) bool { return out.Rules[i].Rule < out.Rules[j].Rule })
	less := func(a, b *ChannelAffinityKeyUsage) bool { return a.LastSeen.After(b.LastSeen) }
	switch q.SortBy {
	case "requests":
		less = func(a, b *ChannelAffinityKeyUsage) bool { return a.Requests > b.Requests }
	case "hits":
		less = func(a, b *ChannelAffinityKeyUsage) bool { return a.Hits > b.Hits }
	case "tokens":
		less = func(a, b *ChannelAffinityKeyUsage) bool { return a.TotalTokens > b.TotalTokens }
	}
	sort.SliceStable(keys, func(i, j int) bool { return less(&keys[i], &keys[j]) })
	out.KeyCount = len(keys)
	if len(keys) > q.Limit {
		keys = keys[:q.Limit]
	}
	out.Keys = keys
	return out
}

// ResetChannelAffinityUsageStats 清空用量统计。
func ResetChannelAffinityUsageStats() {
	s := affinityStats
	s.mu.Lock()
	s.since = time.Now()
	s.rules = make(map[string]*ChannelAffinityCounters)
	s.keys = make(map[string]*ChannelAffinityKeyUsage)
	s.mu.Unlock()
}
