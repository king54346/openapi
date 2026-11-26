package controller

import (
	"net/http"

	gin "github.com/king54346/gin-tiny"
)

// Package controller 提供 HTTP 处理函数。
//
// 明确不实现的接口（统一返回 501 Not Implemented）：
//   - 计费、充值与订阅：GetPricing、GetRatioConfig、ResetModelRatio、FetchUpstreamRatios、
//     GetSyncableChannels、GetPeakValleyTimeInfo、GetAllTopUps、AdminCompleteTopUp、
//     GetSubscriptionPlans、GetSubscriptionSelf、UpdateSubscriptionPreference
//   - 用户安全与双因素认证：UpdateSelf、DeleteSelf、Verify2FALogin、PasskeyLoginBegin、
//     PasskeyLoginFinish、UniversalVerify、GetVerificationStatus、Admin2FAStats、
//     AdminDisable2FA、AdminResetPasskey
//   - 未对接的插件包分发：DownloadWebStarPackage
//
// 其余接口分别实现在 relay.go（转发与模型列表）、channel.go（渠道管理）、token.go（令牌管理）、
// user.go（用户与登录）、option.go（系统配置）、system.go（初始化与状态）、group.go（分组与模型）、
// playground.go（体验中心）、video_proxy.go（视频内容代理）、channel_ops.go（渠道运维）、
// model_sync.go（模型同步）、vendor.go、prefill_group.go、token_template.go、token_usage.go 等文件。

// notImplemented 统一返回 501，标记该接口不在本项目实现范围内。
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

// Admin2FAStats（桩实现：501，双因素认证不在实现范围内）。
func Admin2FAStats(c gin.Context) {
	notImplemented(c, "Admin2FAStats", "two-factor authentication is out of scope")
}

// AdminCompleteTopUp（桩实现：501，充值不在实现范围内）。
func AdminCompleteTopUp(c gin.Context) {
	notImplemented(c, "AdminCompleteTopUp", "top-up is out of scope")
}

// AdminDisable2FA（桩实现：501，双因素认证不在实现范围内）。
func AdminDisable2FA(c gin.Context) {
	notImplemented(c, "AdminDisable2FA", "two-factor authentication is out of scope")
}

// AdminResetPasskey（桩实现：501，Passkey 不在实现范围内）。
func AdminResetPasskey(c gin.Context) {
	notImplemented(c, "AdminResetPasskey", "passkey is out of scope")
}

// DeleteSelf（桩实现：501，账号注销不在实现范围内）。
func DeleteSelf(c gin.Context) {
	notImplemented(c, "DeleteSelf", "account deletion is out of scope")
}

// DownloadWebStarPackage（桩实现：501，插件包分发不在实现范围内）。
func DownloadWebStarPackage(c gin.Context) {
	notImplemented(c, "DownloadWebStarPackage", "webstar plugin package is out of scope")
}

// FetchUpstreamRatios（桩实现：501，倍率同步不在实现范围内）。
func FetchUpstreamRatios(c gin.Context) {
	notImplemented(c, "FetchUpstreamRatios", "ratio sync is out of scope")
}

// GetAllTopUps（桩实现：501，充值记录不在实现范围内）。
func GetAllTopUps(c gin.Context) {
	notImplemented(c, "GetAllTopUps", "top-up is out of scope")
}

// GetPeakValleyTimeInfo（桩实现：501，峰谷计费不在实现范围内）。
func GetPeakValleyTimeInfo(c gin.Context) {
	notImplemented(c, "GetPeakValleyTimeInfo", "peak/valley billing is out of scope")
}

// GetPricing（桩实现：501，计费价格不在实现范围内）。
func GetPricing(c gin.Context) {
	notImplemented(c, "GetPricing", "pricing is out of scope")
}

// GetRatioConfig（桩实现：501，倍率配置不在实现范围内）。
func GetRatioConfig(c gin.Context) {
	notImplemented(c, "GetRatioConfig", "ratio config is out of scope")
}

// GetSubscriptionPlans（桩实现：501，订阅不在实现范围内）。
func GetSubscriptionPlans(c gin.Context) {
	notImplemented(c, "GetSubscriptionPlans", "subscription is out of scope")
}

// GetSubscriptionSelf（桩实现：501，订阅不在实现范围内）。
func GetSubscriptionSelf(c gin.Context) {
	notImplemented(c, "GetSubscriptionSelf", "subscription is out of scope")
}

// GetSyncableChannels（桩实现：501，倍率同步不在实现范围内）。
func GetSyncableChannels(c gin.Context) {
	notImplemented(c, "GetSyncableChannels", "ratio sync is out of scope")
}

// GetVerificationStatus（桩实现：501，二次验证不在实现范围内）。
func GetVerificationStatus(c gin.Context) {
	notImplemented(c, "GetVerificationStatus", "secure verification is out of scope")
}

// PasskeyLoginBegin（桩实现：501，Passkey 不在实现范围内）。
func PasskeyLoginBegin(c gin.Context) {
	notImplemented(c, "PasskeyLoginBegin", "passkey is out of scope")
}

// PasskeyLoginFinish（桩实现：501，Passkey 不在实现范围内）。
func PasskeyLoginFinish(c gin.Context) {
	notImplemented(c, "PasskeyLoginFinish", "passkey is out of scope")
}

// ResetModelRatio（桩实现：501，倍率配置不在实现范围内）。
func ResetModelRatio(c gin.Context) {
	notImplemented(c, "ResetModelRatio", "ratio config is out of scope")
}

// UniversalVerify（桩实现：501，二次验证不在实现范围内）。
func UniversalVerify(c gin.Context) {
	notImplemented(c, "UniversalVerify", "secure verification is out of scope")
}

// UpdateSelf（桩实现：501，个人信息自助修改不在实现范围内）。
func UpdateSelf(c gin.Context) {
	notImplemented(c, "UpdateSelf", "self profile update is out of scope")
}

// UpdateSubscriptionPreference（桩实现：501，订阅不在实现范围内）。
func UpdateSubscriptionPreference(c gin.Context) {
	notImplemented(c, "UpdateSubscriptionPreference", "subscription is out of scope")
}

// Verify2FALogin（桩实现：501，双因素认证不在实现范围内）。
func Verify2FALogin(c gin.Context) {
	notImplemented(c, "Verify2FALogin", "two-factor authentication is out of scope")
}
