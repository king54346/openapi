package unimplemented

import (
	"net/http"

	gin "github.com/king54346/gin-tiny"
)

// Package unimplemented 集中承载明确不在本项目实现范围内的接口（统一返回 501 Not Implemented）：
//   - 计费、倍率、充值与订阅
//   - 用户安全、二次验证、2FA 与 Passkey
//   - 未对接的 WebStar 插件包分发

func replyNotImplemented(c gin.Context, name string, detail string) {
	msg := "controller." + name + " is not implemented yet"
	if detail != "" {
		msg += ": " + detail
	}
	c.JSON(http.StatusNotImplemented, gin.H{
		"error":   "not_implemented",
		"message": msg,
	})
}

func Admin2FAStats(c gin.Context) {
	replyNotImplemented(c, "Admin2FAStats", "two-factor authentication is out of scope")
}

func AdminCompleteTopUp(c gin.Context) {
	replyNotImplemented(c, "AdminCompleteTopUp", "top-up is out of scope")
}

func AdminDisable2FA(c gin.Context) {
	replyNotImplemented(c, "AdminDisable2FA", "two-factor authentication is out of scope")
}

func AdminResetPasskey(c gin.Context) {
	replyNotImplemented(c, "AdminResetPasskey", "passkey is out of scope")
}

func DeleteSelf(c gin.Context) {
	replyNotImplemented(c, "DeleteSelf", "account deletion is out of scope")
}

func DownloadWebStarPackage(c gin.Context) {
	replyNotImplemented(c, "DownloadWebStarPackage", "webstar plugin package is out of scope")
}

func FetchUpstreamRatios(c gin.Context) {
	replyNotImplemented(c, "FetchUpstreamRatios", "ratio sync is out of scope")
}

func GetAllTopUps(c gin.Context) {
	replyNotImplemented(c, "GetAllTopUps", "top-up is out of scope")
}

func GetPeakValleyTimeInfo(c gin.Context) {
	replyNotImplemented(c, "GetPeakValleyTimeInfo", "peak/valley billing is out of scope")
}

func GetPricing(c gin.Context) {
	replyNotImplemented(c, "GetPricing", "pricing is out of scope")
}

func GetRatioConfig(c gin.Context) {
	replyNotImplemented(c, "GetRatioConfig", "ratio config is out of scope")
}

func GetSubscriptionPlans(c gin.Context) {
	replyNotImplemented(c, "GetSubscriptionPlans", "subscription is out of scope")
}

func GetSubscriptionSelf(c gin.Context) {
	replyNotImplemented(c, "GetSubscriptionSelf", "subscription is out of scope")
}

func GetSyncableChannels(c gin.Context) {
	replyNotImplemented(c, "GetSyncableChannels", "ratio sync is out of scope")
}

func GetVerificationStatus(c gin.Context) {
	replyNotImplemented(c, "GetVerificationStatus", "secure verification is out of scope")
}

func PasskeyLoginBegin(c gin.Context) {
	replyNotImplemented(c, "PasskeyLoginBegin", "passkey is out of scope")
}

func PasskeyLoginFinish(c gin.Context) {
	replyNotImplemented(c, "PasskeyLoginFinish", "passkey is out of scope")
}

func ResetModelRatio(c gin.Context) {
	replyNotImplemented(c, "ResetModelRatio", "ratio config is out of scope")
}

func UniversalVerify(c gin.Context) {
	replyNotImplemented(c, "UniversalVerify", "secure verification is out of scope")
}

func UpdateSelf(c gin.Context) {
	replyNotImplemented(c, "UpdateSelf", "self profile update is out of scope")
}

func UpdateSubscriptionPreference(c gin.Context) {
	replyNotImplemented(c, "UpdateSubscriptionPreference", "subscription is out of scope")
}

func Verify2FALogin(c gin.Context) {
	replyNotImplemented(c, "Verify2FALogin", "two-factor authentication is out of scope")
}
