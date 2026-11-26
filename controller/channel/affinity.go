package channel

import (
	ctrlcommon "openapi/controller/common"
	channelsvc "openapi/service/channel"

	gin "github.com/king54346/gin-tiny"
)

func GetChannelAffinityCacheStats(c gin.Context) {
	ctrlcommon.Success(c, channelsvc.GetAffinityCacheStats())
}

func ClearChannelAffinityCache(c gin.Context) {
	channelsvc.ClearAffinityCache()
	ctrlcommon.OK(c)
}

func GetChannelAffinityUsageCacheStats(c gin.Context) {
	ctrlcommon.Success(c, gin.H{
		"items": channelsvc.GetAffinityUsageStats(),
		"stats": channelsvc.GetAffinityCacheStats(),
	})
}
