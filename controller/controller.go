package controller

// Package controller 提供 HTTP 处理函数。
//
// 本文件是尚未实现的接口（统一返回 501 Not Implemented），实现后移到对应文件：
// relay.go 转发与模型列表，channel.go 渠道管理，token.go 令牌管理，user.go 用户与登录。

import (
	"net/http"

	gin "github.com/king54346/gin-tiny"
)

// notImplemented 统一返回 501，标记该接口尚未实现真实逻辑。
func notImplemented(c gin.Context, name string, detail string) {
	msg := "controller." + name + " is not implemented yet"
	if detail != "" {
		msg += ": " + detail
	}
	c.JSON(http.StatusNotImplemented, gin.H{
		"error":   "not_implemented",
		"message": msg,
	})
}

// AddTokenAccessTemplate（桩实现：501）。
func AddTokenAccessTemplate(c gin.Context) {
	notImplemented(c, "AddTokenAccessTemplate", "")
}

// Admin2FAStats（桩实现：501）。
func Admin2FAStats(c gin.Context) {
	notImplemented(c, "Admin2FAStats", "")
}

// AdminCompleteTopUp（桩实现：501）。
func AdminCompleteTopUp(c gin.Context) {
	notImplemented(c, "AdminCompleteTopUp", "")
}

// AdminDisable2FA（桩实现：501）。
func AdminDisable2FA(c gin.Context) {
	notImplemented(c, "AdminDisable2FA", "")
}

// AdminResetPasskey（桩实现：501）。
func AdminResetPasskey(c gin.Context) {
	notImplemented(c, "AdminResetPasskey", "")
}

// BatchRetestChannelModels（桩实现：501）。
func BatchRetestChannelModels(c gin.Context) {
	notImplemented(c, "BatchRetestChannelModels", "")
}

// ClearChannelAffinityCache（桩实现：501）。
func ClearChannelAffinityCache(c gin.Context) {
	notImplemented(c, "ClearChannelAffinityCache", "")
}

// ClearChannelTestRecords（桩实现：501）。
func ClearChannelTestRecords(c gin.Context) {
	notImplemented(c, "ClearChannelTestRecords", "")
}

// CreatePrefillGroup（桩实现：501）。
func CreatePrefillGroup(c gin.Context) {
	notImplemented(c, "CreatePrefillGroup", "")
}

// CreateVendorMeta（桩实现：501）。
func CreateVendorMeta(c gin.Context) {
	notImplemented(c, "CreateVendorMeta", "")
}

// DeleteChannelTestRecords（桩实现：501）。
func DeleteChannelTestRecords(c gin.Context) {
	notImplemented(c, "DeleteChannelTestRecords", "")
}

// DeletePrefillGroup（桩实现：501）。
func DeletePrefillGroup(c gin.Context) {
	notImplemented(c, "DeletePrefillGroup", "")
}

// DeleteSelf（桩实现：501）。
func DeleteSelf(c gin.Context) {
	notImplemented(c, "DeleteSelf", "")
}

// DeleteTokenAccessTemplate（桩实现：501）。
func DeleteTokenAccessTemplate(c gin.Context) {
	notImplemented(c, "DeleteTokenAccessTemplate", "")
}

// DeleteVendorMeta（桩实现：501）。
func DeleteVendorMeta(c gin.Context) {
	notImplemented(c, "DeleteVendorMeta", "")
}

// DownloadWebStarPackage（桩实现：501）。
func DownloadWebStarPackage(c gin.Context) {
	notImplemented(c, "DownloadWebStarPackage", "")
}

// FetchUpstreamRatios（桩实现：501）。
func FetchUpstreamRatios(c gin.Context) {
	notImplemented(c, "FetchUpstreamRatios", "")
}

// FixChannelsAbilities（桩实现：501）。
func FixChannelsAbilities(c gin.Context) {
	notImplemented(c, "FixChannelsAbilities", "")
}

// GetAbout（桩实现：501）。
func GetAbout(c gin.Context) {
	notImplemented(c, "GetAbout", "")
}

// GetAllTopUps（桩实现：501）。
func GetAllTopUps(c gin.Context) {
	notImplemented(c, "GetAllTopUps", "")
}

// GetAllVendors（桩实现：501）。
func GetAllVendors(c gin.Context) {
	notImplemented(c, "GetAllVendors", "")
}

// GetChannelAffinityCacheStats（桩实现：501）。
func GetChannelAffinityCacheStats(c gin.Context) {
	notImplemented(c, "GetChannelAffinityCacheStats", "")
}

// GetChannelAffinityUsageCacheStats（桩实现：501）。
func GetChannelAffinityUsageCacheStats(c gin.Context) {
	notImplemented(c, "GetChannelAffinityUsageCacheStats", "")
}

// GetChannelTestRecords（桩实现：501）。
func GetChannelTestRecords(c gin.Context) {
	notImplemented(c, "GetChannelTestRecords", "")
}

// GetGroups（桩实现：501）。
func GetGroups(c gin.Context) {
	notImplemented(c, "GetGroups", "")
}

// GetHomePageContent（桩实现：501）。
func GetHomePageContent(c gin.Context) {
	notImplemented(c, "GetHomePageContent", "")
}

// GetMissingModels（桩实现：501）。
func GetMissingModels(c gin.Context) {
	notImplemented(c, "GetMissingModels", "")
}

// GetNotice（桩实现：501）。
func GetNotice(c gin.Context) {
	notImplemented(c, "GetNotice", "")
}

// GetOptions（桩实现：501）。
func GetOptions(c gin.Context) {
	notImplemented(c, "GetOptions", "")
}

// GetPeakValleyTimeInfo（桩实现：501）。
func GetPeakValleyTimeInfo(c gin.Context) {
	notImplemented(c, "GetPeakValleyTimeInfo", "")
}

// GetPrefillGroups（桩实现：501）。
func GetPrefillGroups(c gin.Context) {
	notImplemented(c, "GetPrefillGroups", "")
}

// GetPricing（桩实现：501）。
func GetPricing(c gin.Context) {
	notImplemented(c, "GetPricing", "")
}

// GetPrivacyPolicy（桩实现：501）。
func GetPrivacyPolicy(c gin.Context) {
	notImplemented(c, "GetPrivacyPolicy", "")
}

// GetRatioConfig（桩实现：501）。
func GetRatioConfig(c gin.Context) {
	notImplemented(c, "GetRatioConfig", "")
}

// GetSetup（桩实现：501）。
func GetSetup(c gin.Context) {
	notImplemented(c, "GetSetup", "")
}

// GetStatus（桩实现：501）。
func GetStatus(c gin.Context) {
	notImplemented(c, "GetStatus", "")
}

// GetSubscriptionPlans（桩实现：501）。
func GetSubscriptionPlans(c gin.Context) {
	notImplemented(c, "GetSubscriptionPlans", "")
}

// GetSubscriptionSelf（桩实现：501）。
func GetSubscriptionSelf(c gin.Context) {
	notImplemented(c, "GetSubscriptionSelf", "")
}

// GetSyncableChannels（桩实现：501）。
func GetSyncableChannels(c gin.Context) {
	notImplemented(c, "GetSyncableChannels", "")
}

// GetTokenAnalytics（桩实现：501）。
func GetTokenAnalytics(c gin.Context) {
	notImplemented(c, "GetTokenAnalytics", "")
}

// GetTokenUsage（桩实现：501）。
func GetTokenUsage(c gin.Context) {
	notImplemented(c, "GetTokenUsage", "")
}

// GetUptimeKumaStatus（桩实现：501）。
func GetUptimeKumaStatus(c gin.Context) {
	notImplemented(c, "GetUptimeKumaStatus", "")
}

// GetUserAgreement（桩实现：501）。
func GetUserAgreement(c gin.Context) {
	notImplemented(c, "GetUserAgreement", "")
}

// GetUserGroups（桩实现：501）。
func GetUserGroups(c gin.Context) {
	notImplemented(c, "GetUserGroups", "")
}

// GetUserModels（桩实现：501）。
func GetUserModels(c gin.Context) {
	notImplemented(c, "GetUserModels", "")
}

// GetUserTokenAccessTemplates（桩实现：501）。
func GetUserTokenAccessTemplates(c gin.Context) {
	notImplemented(c, "GetUserTokenAccessTemplates", "")
}

// GetVendorMeta（桩实现：501）。
func GetVendorMeta(c gin.Context) {
	notImplemented(c, "GetVendorMeta", "")
}

// GetVerificationStatus（桩实现：501）。
func GetVerificationStatus(c gin.Context) {
	notImplemented(c, "GetVerificationStatus", "")
}

// MigrateConsoleSetting（桩实现：501）。
func MigrateConsoleSetting(c gin.Context) {
	notImplemented(c, "MigrateConsoleSetting", "")
}

// PasskeyLoginBegin（桩实现：501）。
func PasskeyLoginBegin(c gin.Context) {
	notImplemented(c, "PasskeyLoginBegin", "")
}

// PasskeyLoginFinish（桩实现：501）。
func PasskeyLoginFinish(c gin.Context) {
	notImplemented(c, "PasskeyLoginFinish", "")
}

// Playground（桩实现：501）。
func Playground(c gin.Context) {
	notImplemented(c, "Playground", "")
}

// PlaygroundAudio（桩实现：501）。
func PlaygroundAudio(c gin.Context) {
	notImplemented(c, "PlaygroundAudio", "")
}

// PlaygroundEmbedding（桩实现：501）。
func PlaygroundEmbedding(c gin.Context) {
	notImplemented(c, "PlaygroundEmbedding", "")
}

// PlaygroundImage（桩实现：501）。
func PlaygroundImage(c gin.Context) {
	notImplemented(c, "PlaygroundImage", "")
}

// PlaygroundRerank（桩实现：501）。
func PlaygroundRerank(c gin.Context) {
	notImplemented(c, "PlaygroundRerank", "")
}

// PlaygroundTask（桩实现：501）。
func PlaygroundTask(c gin.Context) {
	notImplemented(c, "PlaygroundTask", "")
}

// PostSetup（桩实现：501）。
func PostSetup(c gin.Context) {
	notImplemented(c, "PostSetup", "")
}

// RemoveModelFromChannel（桩实现：501）。
func RemoveModelFromChannel(c gin.Context) {
	notImplemented(c, "RemoveModelFromChannel", "")
}

// ResetModelRatio（桩实现：501）。
func ResetModelRatio(c gin.Context) {
	notImplemented(c, "ResetModelRatio", "")
}

// RetestChannelModel（桩实现：501）。
func RetestChannelModel(c gin.Context) {
	notImplemented(c, "RetestChannelModel", "")
}

// RetestFilteredRecords（桩实现：501）。
func RetestFilteredRecords(c gin.Context) {
	notImplemented(c, "RetestFilteredRecords", "")
}

// SearchVendors（桩实现：501）。
func SearchVendors(c gin.Context) {
	notImplemented(c, "SearchVendors", "")
}

// SyncUpstreamModels（桩实现：501）。
func SyncUpstreamModels(c gin.Context) {
	notImplemented(c, "SyncUpstreamModels", "")
}

// SyncUpstreamPreview（桩实现：501）。
func SyncUpstreamPreview(c gin.Context) {
	notImplemented(c, "SyncUpstreamPreview", "")
}

// TestAllChannels（桩实现：501）。
func TestAllChannels(c gin.Context) {
	notImplemented(c, "TestAllChannels", "")
}

// TestChannelAllModels（桩实现：501）。
func TestChannelAllModels(c gin.Context) {
	notImplemented(c, "TestChannelAllModels", "")
}

// TestStatus（桩实现：501）。
func TestStatus(c gin.Context) {
	notImplemented(c, "TestStatus", "")
}

// UniversalVerify（桩实现：501）。
func UniversalVerify(c gin.Context) {
	notImplemented(c, "UniversalVerify", "")
}

// UpdateAllChannelsBalance（桩实现：501）。
func UpdateAllChannelsBalance(c gin.Context) {
	notImplemented(c, "UpdateAllChannelsBalance", "")
}

// UpdateChannelBalance（桩实现：501）。
func UpdateChannelBalance(c gin.Context) {
	notImplemented(c, "UpdateChannelBalance", "")
}

// UpdateOption（桩实现：501）。
func UpdateOption(c gin.Context) {
	notImplemented(c, "UpdateOption", "")
}

// UpdatePrefillGroup（桩实现：501）。
func UpdatePrefillGroup(c gin.Context) {
	notImplemented(c, "UpdatePrefillGroup", "")
}

// UpdateSelf（桩实现：501）。
func UpdateSelf(c gin.Context) {
	notImplemented(c, "UpdateSelf", "")
}

// UpdateSubscriptionPreference（桩实现：501）。
func UpdateSubscriptionPreference(c gin.Context) {
	notImplemented(c, "UpdateSubscriptionPreference", "")
}

// UpdateTokenAccessTemplate（桩实现：501）。
func UpdateTokenAccessTemplate(c gin.Context) {
	notImplemented(c, "UpdateTokenAccessTemplate", "")
}

// UpdateVendorMeta（桩实现：501）。
func UpdateVendorMeta(c gin.Context) {
	notImplemented(c, "UpdateVendorMeta", "")
}

// Verify2FALogin（桩实现：501）。
func Verify2FALogin(c gin.Context) {
	notImplemented(c, "Verify2FALogin", "")
}

// VideoProxy（桩实现：501）。
func VideoProxy(c gin.Context) {
	notImplemented(c, "VideoProxy", "")
}
