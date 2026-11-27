package channel

import (
	ctrlcommon "openapi/controller/common"
	channelsvc "openapi/service/channel"

	gin "github.com/king54346/gin-tiny"
)

// GetChannelAffinityCacheStats 获取频道亲和力缓存统计信息
func GetChannelAffinityCacheStats(c gin.Context) {
	ctrlcommon.Success(c, channelsvc.GetAffinityCacheStats())
}

// ClearChannelAffinityCache 清除频道亲和力缓存
func ClearChannelAffinityCache(c gin.Context) {
	channelsvc.ClearAffinityCache()
	ctrlcommon.OK(c)
}

// GetChannelAffinityUsageCacheStats 获取频道亲和力使用缓存统计信息
func GetChannelAffinityUsageCacheStats(c gin.Context) {
	ctrlcommon.Success(c, gin.H{
		"items": channelsvc.GetAffinityUsageStats(),
		"stats": channelsvc.GetAffinityCacheStats(),
	})
}
