package channels

import (
	"openapi/common"
	"openapi/constant"
	"openapi/setting"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	gin "github.com/king54346/gin-tiny"
)

var affinitySessionHeaders = []string{
	"X-Session-Id",
	"X-Conversation-Id",
	"Conversation-Id",
	"X-Prompt-Cache-Key",
}

// affinitySessionQueries 会话标识候选查询参数。

var affinitySessionQueries = []string{"session_id", "conversation_id", "prompt_cache_key"}

// defaultAffinityTTL / defaultAffinityMaxEntries 兜底值，配置非法时使用。

const (
	defaultAffinityTTL        = 5 * time.Minute
	defaultAffinityMaxEntries = 100000
)

type affinityEntry struct {
	channelID int
	expireAt  time.Time
}

// AffinityStats 亲和性缓存统计。

type AffinityStats struct {
	Enabled    bool    `json:"enabled"`
	Entries    int     `json:"entries"`
	MaxEntries int     `json:"max_entries"`
	TTLSeconds int     `json:"ttl_seconds"`
	Hits       uint64  `json:"hits"`
	Misses     uint64  `json:"misses"`
	Records    uint64  `json:"records"`
	HitRate    float64 `json:"hit_rate"`
}

// AffinityUsageStat 单个渠道的亲和性使用次数。

type AffinityUsageStat struct {
	ChannelID int    `json:"channel_id"`
	Count     uint64 `json:"count"`
}

var affinityCache = struct {
	mu      sync.Mutex
	entries map[string]affinityEntry
	usage   map[int]uint64
	hits    uint64
	misses  uint64
	records uint64
}{entries: map[string]affinityEntry{}, usage: map[int]uint64{}}

// affinityEnabled 亲和性是否开启。

func affinityEnabled() bool {
	return setting.GetGlobalSettings().ChannelAffinityEnabled
}

// affinityTTL 亲和记录有效期。

func affinityTTL() time.Duration {
	seconds := setting.GetGlobalSettings().ChannelAffinityTTLSeconds
	if seconds <= 0 {
		return defaultAffinityTTL
	}
	return time.Duration(seconds) * time.Second
}

// affinityMaxEntries 缓存条目上限。

func affinityMaxEntries() int {
	maxEntries := setting.GetGlobalSettings().ChannelAffinityMaxEntries
	if maxEntries <= 0 {
		return defaultAffinityMaxEntries
	}
	return maxEntries
}

// buildAffinityKey 生成亲和 key：用户 + 模型 + 会话标识（会话标识缺失时退化为前两者）。

func buildAffinityKey(c gin.Context, modelName string) string {
	userID := 0
	if c != nil {
		userID = common.GetContextKeyInt(c, constant.ContextKeyUserId)
		if userID == 0 {
			userID = c.GetInt("id")
		}
	}
	key := strings.Join([]string{strconv.Itoa(userID), modelName}, "|")
	if c == nil || c.Request() == nil {
		return key
	}
	for _, header := range affinitySessionHeaders {
		if value := strings.TrimSpace(c.Request().Header.Get(header)); value != "" {
			return key + "|" + header + ":" + value
		}
	}
	for _, query := range affinitySessionQueries {
		if value := strings.TrimSpace(c.Query(query)); value != "" {
			return key + "|" + query + ":" + value
		}
	}
	return key
}

// GetPreferredChannelByAffinity 查询会话上次命中的渠道；未开启、无记录或已过期时返回 false。

func GetPreferredChannelByAffinity(c gin.Context, modelName string) (int, bool) {
	if !affinityEnabled() || modelName == "" {
		return 0, false
	}
	key := buildAffinityKey(c, modelName)
	now := time.Now()

	affinityCache.mu.Lock()
	defer affinityCache.mu.Unlock()
	entry, ok := affinityCache.entries[key]
	if !ok || now.After(entry.expireAt) {
		if ok {
			delete(affinityCache.entries, key)
		}
		affinityCache.misses++
		return 0, false
	}
	affinityCache.hits++
	return entry.channelID, true
}

// RecordChannelAffinity 记录本次成功请求使用的渠道，供后续同会话请求复用。

func RecordChannelAffinity(c gin.Context, channelID int) {
	if !affinityEnabled() || channelID <= 0 {
		return
	}
	// 亲和 key 需要模型名：取分发阶段写入上下文的原始模型名
	modelName := common.GetContextKeyString(c, constant.ContextKeyOriginalModel)
	if modelName == "" {
		return
	}
	storeAffinity(buildAffinityKey(c, modelName), channelID)
}

// MarkChannelAffinityUsed 记一次亲和性命中（渠道来自 GetPreferredChannelByAffinity）。

func MarkChannelAffinityUsed(c gin.Context, channelID int) {
	if !affinityEnabled() || channelID <= 0 {
		return
	}
	affinityCache.mu.Lock()
	defer affinityCache.mu.Unlock()
	affinityCache.usage[channelID]++
}

// storeAffinity 写入亲和记录，必要时先清理过期项，并在超出容量时淘汰最早过期的条目。

func storeAffinity(key string, channelID int) {
	now := time.Now()
	ttl := affinityTTL()
	maxEntries := affinityMaxEntries()

	affinityCache.mu.Lock()
	defer affinityCache.mu.Unlock()

	affinityCache.records++
	affinityCache.usage[channelID]++
	if len(affinityCache.entries) >= maxEntries {
		purgeExpiredAffinityLocked(now)
	}
	for len(affinityCache.entries) >= maxEntries {
		oldestKey, oldestExpire := "", time.Time{}
		for k, entry := range affinityCache.entries {
			if oldestKey == "" || entry.expireAt.Before(oldestExpire) {
				oldestKey, oldestExpire = k, entry.expireAt
			}
		}
		if oldestKey == "" {
			break
		}
		delete(affinityCache.entries, oldestKey)
	}
	affinityCache.entries[key] = affinityEntry{channelID: channelID, expireAt: now.Add(ttl)}
}

// purgeExpiredAffinityLocked 删除已过期条目，调用方需持有写锁。

func purgeExpiredAffinityLocked(now time.Time) {
	for key, entry := range affinityCache.entries {
		if now.After(entry.expireAt) {
			delete(affinityCache.entries, key)
		}
	}
}

// GetAffinityCacheStats 亲和性缓存统计（条目数、命中率等）。

func GetAffinityCacheStats() AffinityStats {
	affinityCache.mu.Lock()
	defer affinityCache.mu.Unlock()
	purgeExpiredAffinityLocked(time.Now())

	total := affinityCache.hits + affinityCache.misses
	stats := AffinityStats{
		Enabled:    affinityEnabled(),
		Entries:    len(affinityCache.entries),
		MaxEntries: affinityMaxEntries(),
		TTLSeconds: int(affinityTTL().Seconds()),
		Hits:       affinityCache.hits,
		Misses:     affinityCache.misses,
		Records:    affinityCache.records,
	}
	if total > 0 {
		stats.HitRate = float64(affinityCache.hits) / float64(total)
	}
	return stats
}

// GetAffinityUsageStats 各渠道的亲和性使用次数（按次数降序）。

func GetAffinityUsageStats() []AffinityUsageStat {
	affinityCache.mu.Lock()
	defer affinityCache.mu.Unlock()

	stats := make([]AffinityUsageStat, 0, len(affinityCache.usage))
	for channelID, count := range affinityCache.usage {
		stats = append(stats, AffinityUsageStat{ChannelID: channelID, Count: count})
	}
	sort.Slice(stats, func(i, j int) bool {
		if stats[i].Count != stats[j].Count {
			return stats[i].Count > stats[j].Count
		}
		return stats[i].ChannelID < stats[j].ChannelID
	})
	return stats
}

// ClearAffinityCache 清空亲和性缓存与统计。

func ClearAffinityCache() {
	affinityCache.mu.Lock()
	defer affinityCache.mu.Unlock()
	affinityCache.entries = map[string]affinityEntry{}
	affinityCache.usage = map[int]uint64{}
	affinityCache.hits, affinityCache.misses, affinityCache.records = 0, 0, 0
}

// 渠道健康：按渠道统计连续失败次数，达到阈值或遇到致命错误（密钥失效、欠费等）时
// 自动禁用渠道（状态改为 ChannelStatusAutoDisabled，需管理员手动启用）。
// 只统计渠道侧问题，请求本身有误（400 等）不计数；渠道关闭 auto_ban 时不处理。
