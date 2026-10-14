package controller

import (
	"strconv"

	"openapi/service"
	"openapi/setting"

	gin "github.com/king54346/gin-tiny"
)

// 渠道亲和性管理接口：配置读写、绑定缓存概况与清理、命中与用量统计。

// GetChannelAffinitySetting 当前亲和性配置。
func GetChannelAffinitySetting(c gin.Context) {
	apiSuccess(c, setting.GetChannelAffinitySetting())
}

// UpdateChannelAffinitySetting 整体替换亲和性配置，校验失败（如正则非法）时不生效。
func UpdateChannelAffinitySetting(c gin.Context) {
	var req setting.ChannelAffinitySetting
	if err := c.ShouldBindJSON(&req); err != nil {
		apiErrorMsg(c, "invalid request: "+err.Error())
		return
	}
	if err := setting.UpdateChannelAffinitySetting(req); err != nil {
		apiErrorMsg(c, err.Error())
		return
	}
	apiSuccess(c, setting.GetChannelAffinitySetting())
}

// GetChannelAffinityCacheStats 绑定缓存概况：存储后端、总绑定数、各规则绑定数。
func GetChannelAffinityCacheStats(c gin.Context) {
	apiSuccess(c, service.GetChannelAffinityCacheStats())
}

// ClearChannelAffinityCache 清除绑定，?rule= 指定规则，不传清除全部；?reset_usage=true 同时清空用量统计。
func ClearChannelAffinityCache(c gin.Context) {
	deleted := service.ClearChannelAffinityBindings(c.Query("rule"))
	if c.Query("reset_usage") == "true" {
		service.ResetChannelAffinityUsageStats()
	}
	apiSuccess(c, gin.H{"deleted": deleted})
}

// GetChannelAffinityUsageCacheStats 命中情况、请求次数与 token 用量。
// 查询参数：rule 按规则过滤，limit 返回标识条数（默认 100），sort 为 last_seen / requests / hits / tokens。
func GetChannelAffinityUsageCacheStats(c gin.Context) {
	limit, _ := strconv.Atoi(c.Query("limit"))
	apiSuccess(c, service.GetChannelAffinityUsageStats(service.ChannelAffinityUsageQuery{
		Rule:   c.Query("rule"),
		Limit:  limit,
		SortBy: c.Query("sort"),
	}))
}
