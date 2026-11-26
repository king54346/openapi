package unimplemented

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	gin "github.com/king54346/gin-tiny"
)

func callStub(t *testing.T, h gin.HandlerFunc) (int, map[string]string) {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	h(c)
	res := w.Result()
	defer res.Body.Close()
	var body map[string]string
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode stub response: %v", err)
	}
	return res.StatusCode, body
}

func TestStubsReturn501(t *testing.T) {
	stubs := map[string]gin.HandlerFunc{
		"GetPricing":                   GetPricing,
		"GetRatioConfig":               GetRatioConfig,
		"ResetModelRatio":              ResetModelRatio,
		"GetAllTopUps":                 GetAllTopUps,
		"AdminCompleteTopUp":           AdminCompleteTopUp,
		"GetSubscriptionPlans":         GetSubscriptionPlans,
		"GetSubscriptionSelf":          GetSubscriptionSelf,
		"UpdateSubscriptionPreference": UpdateSubscriptionPreference,
		"UpdateSelf":                   UpdateSelf,
		"DeleteSelf":                   DeleteSelf,
		"Verify2FALogin":               Verify2FALogin,
		"PasskeyLoginBegin":            PasskeyLoginBegin,
		"PasskeyLoginFinish":           PasskeyLoginFinish,
		"UniversalVerify":              UniversalVerify,
		"GetVerificationStatus":        GetVerificationStatus,
		"Admin2FAStats":                Admin2FAStats,
		"AdminDisable2FA":              AdminDisable2FA,
		"AdminResetPasskey":            AdminResetPasskey,
		"FetchUpstreamRatios":          FetchUpstreamRatios,
		"GetSyncableChannels":          GetSyncableChannels,
		"GetPeakValleyTimeInfo":        GetPeakValleyTimeInfo,
		"DownloadWebStarPackage":       DownloadWebStarPackage,
	}
	for name, h := range stubs {
		code, body := callStub(t, h)
		if code != http.StatusNotImplemented {
			t.Fatalf("%s: expected 501, got %d", name, code)
		}
		if body["error"] != "not_implemented" {
			t.Fatalf("%s: unexpected body: %v", name, body)
		}
	}
}
