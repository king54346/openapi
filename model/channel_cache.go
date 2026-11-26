package model

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"sort"
	"strings"
	"sync"
	"time"

	"openapi/common"
	"openapi/constant"
)

// channelCache 已启用且有转发实现的渠道快照，定期从 channels 表整体重建。
// 分组概念已移除：按模型索引，所有渠道对所有请求可见。
type channelCache struct {
	byID map[int]*Channel
	// model -> 渠道列表（按优先级降序）
	byModel map[string][]*Channel
}

var (
	channelCacheMu sync.RWMutex
	channelsCache  *channelCache
)

var errChannelCacheNotReady = errors.New("channel cache is not initialized")

// InitChannelCache 从数据库加载渠道并重建缓存。
func InitChannelCache() error {
	var channels []*Channel
	if err := DB.Where("status = ?", common.ChannelStatusEnabled).Find(&channels).Error; err != nil {
		return err
	}

	cache := &channelCache{
		byID:    make(map[int]*Channel, len(channels)),
		byModel: make(map[string][]*Channel),
	}
	var skipped []string
	for _, ch := range channels {
		if !constant.IsRelaySupportedChannelType(ch.Type) {
			skipped = append(skipped, fmt.Sprintf("%d(%s,type=%d)", ch.Id, ch.Name, ch.Type))
			continue
		}
		cache.byID[ch.Id] = ch
		for _, modelName := range ch.GetModels() {
			if modelName = strings.TrimSpace(modelName); modelName != "" {
				cache.byModel[modelName] = append(cache.byModel[modelName], ch)
			}
		}
	}
	for _, list := range cache.byModel {
		sort.SliceStable(list, func(i, j int) bool {
			return list[i].GetPriority() > list[j].GetPriority()
		})
	}

	channelCacheMu.Lock()
	channelsCache = cache
	channelCacheMu.Unlock()

	if len(skipped) > 0 {
		common.SysLog("channels skipped (no relay implementation for type): " + strings.Join(skipped, ", "))
	}
	common.SysLog(fmt.Sprintf("channel cache loaded: %d enabled channels", len(cache.byID)))
	return nil
}

// SyncChannelCache 每 frequency 秒重建一次渠道缓存（阻塞，需在 goroutine 中运行）。
func SyncChannelCache(frequency int) {
	if frequency <= 0 {
		frequency = 60
	}
	for {
		time.Sleep(time.Duration(frequency) * time.Second)
		if err := InitChannelCache(); err != nil {
			common.SysError("failed to sync channel cache: " + err.Error())
		}
	}
}

func getChannelCache() *channelCache {
	channelCacheMu.RLock()
	defer channelCacheMu.RUnlock()
	return channelsCache
}

// CacheGetChannel 按 ID 取已启用渠道；缓存未命中时回源数据库（可能返回已禁用渠道，由调用方判断状态）。
func CacheGetChannel(channelID int) (*Channel, error) {
	if cache := getChannelCache(); cache != nil {
		if ch, ok := cache.byID[channelID]; ok {
			return ch, nil
		}
	}
	return GetChannelById(channelID, true)
}

// IsChannelEnabledForGroupModel 渠道是否启用并支持该模型（分组参数仅为兼容调用方保留，已忽略）。
func IsChannelEnabledForGroupModel(group, modelName string, channelID int) bool {
	cache := getChannelCache()
	if cache == nil {
		return false
	}
	for _, ch := range cache.byModel[modelName] {
		if ch.Id == channelID {
			return true
		}
	}
	return false
}

// GetRandomSatisfiedChannel 选出支持该模型的渠道：
// 先按优先级分档（retry 次数越大，档位越低，超出后停在最低档），档内按权重随机。
// 没有可用渠道时返回 (nil, nil)。
func GetRandomSatisfiedChannel(group, modelName string, retry int) (*Channel, error) {
	_ = group // 分组已移除
	cache := getChannelCache()
	if cache == nil {
		return nil, errChannelCacheNotReady
	}
	candidates := cache.byModel[modelName]
	if len(candidates) == 0 {
		return nil, nil
	}

	// candidates 已按优先级降序，取出第 retry 档
	priorities := make([]int64, 0, len(candidates))
	for _, ch := range candidates {
		if p := ch.GetPriority(); len(priorities) == 0 || priorities[len(priorities)-1] != p {
			priorities = append(priorities, p)
		}
	}
	target := priorities[min(max(retry, 0), len(priorities)-1)]

	tier := make([]*Channel, 0, len(candidates))
	totalWeight := 0
	for _, ch := range candidates {
		if ch.GetPriority() == target {
			tier = append(tier, ch)
			// 权重整体 +10，让权重为 0 的渠道也有机会被选中
			totalWeight += ch.GetWeight() + 10
		}
	}

	r := rand.IntN(totalWeight)
	for _, ch := range tier {
		r -= ch.GetWeight() + 10
		if r < 0 {
			return ch, nil
		}
	}
	return tier[len(tier)-1], nil
}

// RefreshChannelCache 渠道被修改后立即重建缓存，失败只记日志（下个同步周期会再试）。
func RefreshChannelCache() {
	if err := InitChannelCache(); err != nil {
		common.SysError("failed to refresh channel cache: " + err.Error())
	}
}

// GetEnabledModels 返回所有已启用渠道提供的模型名（去重、已排序）。
func GetEnabledModels() []string {
	cache := getChannelCache()
	if cache == nil {
		return nil
	}
	names := make([]string, 0, len(cache.byModel))
	for modelName := range cache.byModel {
		names = append(names, modelName)
	}
	sort.Strings(names)
	return names
}

// GetModelChannels 返回支持该模型的渠道（按优先级降序的副本）。
func GetModelChannels(modelName string) []*Channel {
	cache := getChannelCache()
	if cache == nil {
		return nil
	}
	list := cache.byModel[modelName]
	out := make([]*Channel, len(list))
	copy(out, list)
	return out
}
