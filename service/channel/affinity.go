package channel

import (
	"context"
	"errors"
	"openapi/common"
	"openapi/constant"
	"openapi/setting"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	gin "github.com/king54346/gin-tiny"
	"github.com/redis/go-redis/v9"
)

/**
  亲和性缓存机制(命中统计与各渠道命中计数统一写入 Redis，多实例负载均衡下同一对话稳定命中同一上游渠道（支持前缀缓存复用），异常或未启用时无缝降级本地内存)
	 1. 提取会话指纹： 用户ID | 模型名 | 会话标识 拼出唯一 Key 若客户端没传会话 ID，则按 用户ID | 模型名 聚合。
	 2. 启用 Redis 时，亲和性缓存跨节点共享 节点选渠道前先去 Redis GET openapi:affinity:entry:<会话指纹> 拿到渠道；异常时自动回退本地缓存。
	 3. 亲和性缓存可用于同一用户在同一会话中连续请求时，优先使用上次成功的渠道，减少渠道切换，提高响应速度。
	 4. 亲和性缓存支持统计命中率、使用次数等指标，便于监控和优化。

*/

var affinitySessionHeaders = []string{
	"X-Session-Id",
	"X-Conversation-Id",
	"Conversation-Id",
	"X-Prompt-Cache-Key",
}

// affinitySessionQueries 会话标识候选查询参数。
var affinitySessionQueries = []string{"session_id", "conversation_id", "prompt_cache_key"}

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

func affinityEnabled() bool {
	return setting.GetGlobalSettings().ChannelAffinityEnabled
}

func affinityTTL() time.Duration {
	seconds := setting.GetGlobalSettings().ChannelAffinityTTLSeconds
	if seconds <= 0 {
		return defaultAffinityTTL
	}
	return time.Duration(seconds) * time.Second
}

func affinityMaxEntries() int {
	maxEntries := setting.GetGlobalSettings().ChannelAffinityMaxEntries
	if maxEntries <= 0 {
		return defaultAffinityMaxEntries
	}
	return maxEntries
}

func affinityEntryRedisKey(key string) string {
	return common.RedisKey("affinity", "entry", key)
}

func affinityStatsRedisKey() string {
	return common.RedisKey("affinity", "stats")
}

func affinityUsageRedisKey() string {
	return common.RedisKey("affinity", "usage")
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

// GetPreferredChannelByAffinity 查询会话上次命中的渠道；启用 Redis 时跨节点共享，异常时自动回退本地缓存。
func GetPreferredChannelByAffinity(c gin.Context, modelName string) (int, bool) {
	if !affinityEnabled() || modelName == "" {
		return 0, false
	}
	key := buildAffinityKey(c, modelName)

	if common.RedisEnabled && common.RDB != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		val, err := common.RDB.Get(ctx, affinityEntryRedisKey(key)).Result()
		if err == nil {
			if channelID, convErr := strconv.Atoi(val); convErr == nil && channelID > 0 {
				_ = common.RDB.HIncrBy(ctx, affinityStatsRedisKey(), "hits", 1).Err()
				return channelID, true
			}
		} else if errors.Is(err, redis.Nil) {
			_ = common.RDB.HIncrBy(ctx, affinityStatsRedisKey(), "misses", 1).Err()
			return 0, false
		} else {
			common.SysError("redis affinity get failed, falling back to memory: " + err.Error())
		}
	}

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

// RecordChannelAffinity 记录本次成功请求使用的渠道，供后续同会话请求跨节点复用。
func RecordChannelAffinity(c gin.Context, channelID int) {
	if !affinityEnabled() || channelID <= 0 {
		return
	}
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
	if common.RedisEnabled && common.RDB != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := common.RDB.HIncrBy(ctx, affinityUsageRedisKey(), strconv.Itoa(channelID), 1).Err(); err == nil {
			return
		}
	}
	affinityCache.mu.Lock()
	defer affinityCache.mu.Unlock()
	affinityCache.usage[channelID]++
}

func storeAffinity(key string, channelID int) {
	ttl := affinityTTL()
	if common.RedisEnabled && common.RDB != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		pipe := common.RDB.Pipeline()
		pipe.Set(ctx, affinityEntryRedisKey(key), strconv.Itoa(channelID), ttl)
		pipe.HIncrBy(ctx, affinityStatsRedisKey(), "records", 1)
		pipe.HIncrBy(ctx, affinityUsageRedisKey(), strconv.Itoa(channelID), 1)
		if _, err := pipe.Exec(ctx); err == nil {
			return
		} else {
			common.SysError("redis affinity store failed, falling back to memory: " + err.Error())
		}
	}

	now := time.Now()
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

func purgeExpiredAffinityLocked(now time.Time) {
	for key, entry := range affinityCache.entries {
		if now.After(entry.expireAt) {
			delete(affinityCache.entries, key)
		}
	}
}

// GetAffinityCacheStats 亲和性缓存统计（启用 Redis 时汇总集群全局统计）。
func GetAffinityCacheStats() AffinityStats {
	if common.RedisEnabled && common.RDB != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		rawStats, err := common.RDB.HGetAll(ctx, affinityStatsRedisKey()).Result()
		if err == nil {
			hits, _ := strconv.ParseUint(rawStats["hits"], 10, 64)
			misses, _ := strconv.ParseUint(rawStats["misses"], 10, 64)
			records, _ := strconv.ParseUint(rawStats["records"], 10, 64)
			entries := countRedisAffinityEntries(ctx)
			total := hits + misses
			stats := AffinityStats{
				Enabled:    affinityEnabled(),
				Entries:    entries,
				MaxEntries: affinityMaxEntries(),
				TTLSeconds: int(affinityTTL().Seconds()),
				Hits:       hits,
				Misses:     misses,
				Records:    records,
			}
			if total > 0 {
				stats.HitRate = float64(hits) / float64(total)
			}
			return stats
		}
	}

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

func countRedisAffinityEntries(ctx context.Context) int {
	pattern := common.RedisKey("affinity", "entry", "*")
	var cursor uint64
	count := 0
	for {
		keys, nextCursor, err := common.RDB.Scan(ctx, cursor, pattern, 200).Result()
		if err != nil {
			break
		}
		count += len(keys)
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
	return count
}

// GetAffinityUsageStats 各渠道的亲和性使用次数（按次数降序）。
func GetAffinityUsageStats() []AffinityUsageStat {
	if common.RedisEnabled && common.RDB != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		rawUsage, err := common.RDB.HGetAll(ctx, affinityUsageRedisKey()).Result()
		if err == nil {
			stats := make([]AffinityUsageStat, 0, len(rawUsage))
			for chStr, cntStr := range rawUsage {
				chID, err1 := strconv.Atoi(chStr)
				cnt, err2 := strconv.ParseUint(cntStr, 10, 64)
				if err1 == nil && err2 == nil && cnt > 0 {
					stats = append(stats, AffinityUsageStat{ChannelID: chID, Count: cnt})
				}
			}
			sortAffinityUsageStats(stats)
			return stats
		}
	}

	affinityCache.mu.Lock()
	defer affinityCache.mu.Unlock()

	stats := make([]AffinityUsageStat, 0, len(affinityCache.usage))
	for channelID, count := range affinityCache.usage {
		stats = append(stats, AffinityUsageStat{ChannelID: channelID, Count: count})
	}
	sortAffinityUsageStats(stats)
	return stats
}

func sortAffinityUsageStats(stats []AffinityUsageStat) {
	sort.Slice(stats, func(i, j int) bool {
		if stats[i].Count != stats[j].Count {
			return stats[i].Count > stats[j].Count
		}
		return stats[i].ChannelID < stats[j].ChannelID
	})
}

// ClearAffinityCache 清空亲和性缓存与统计（同时清理 Redis 与本地缓存）。
func ClearAffinityCache() {
	if common.RedisEnabled && common.RDB != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		pattern := common.RedisKey("affinity", "entry", "*")
		var cursor uint64
		for {
			keys, nextCursor, err := common.RDB.Scan(ctx, cursor, pattern, 200).Result()
			if err != nil {
				break
			}
			if len(keys) > 0 {
				_ = common.RDB.Del(ctx, keys...).Err()
			}
			cursor = nextCursor
			if cursor == 0 {
				break
			}
		}
		_ = common.RDB.Del(ctx, affinityStatsRedisKey(), affinityUsageRedisKey()).Err()
	}

	affinityCache.mu.Lock()
	defer affinityCache.mu.Unlock()
	affinityCache.entries = map[string]affinityEntry{}
	affinityCache.usage = map[int]uint64{}
	affinityCache.hits, affinityCache.misses, affinityCache.records = 0, 0, 0
}
