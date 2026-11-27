package e2e_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

// TestOutOfScopeEndpointsReturn501 验证明确不实现的计费/倍率/充值/订阅、用户安全/2FA/Passkey 及插件包接口均返回 501 Not Implemented。
func TestOutOfScopeEndpointsReturn501(t *testing.T) {
	admin := newLoggedInAdminClient(t)

	cases := []struct {
		method string
		path   string
	}{
		// 计费、倍率、充值与订阅
		{http.MethodGet, "/api/pricing"},
		{http.MethodGet, "/api/ratio_config"},
		{http.MethodPost, "/api/option/rest_model_ratio"},
		{http.MethodGet, "/api/option/peak_valley_time"},
		{http.MethodGet, "/api/ratio_sync/channels"},
		{http.MethodPost, "/api/ratio_sync/fetch"},
		{http.MethodGet, "/api/user/topup"},
		{http.MethodPost, "/api/user/topup/complete"},
		{http.MethodGet, "/api/subscription/plans"},
		{http.MethodGet, "/api/subscription/self"},
		{http.MethodPut, "/api/subscription/self/preference"},

		// 用户安全、二次验证、2FA 与 Passkey
		{http.MethodPost, "/api/verify"},
		{http.MethodGet, "/api/verify/status"},
		{http.MethodPost, "/api/user/login/2fa"},
		{http.MethodPost, "/api/user/passkey/login/begin"},
		{http.MethodPost, "/api/user/passkey/login/finish"},
		{http.MethodPut, "/api/user/self"},
		{http.MethodDelete, "/api/user/self"},
		{http.MethodDelete, "/api/user/999999/reset_passkey"},
		{http.MethodGet, "/api/user/2fa/stats"},
		{http.MethodDelete, "/api/user/999999/2fa"},

		// WebStar 插件包下载
		{http.MethodGet, "/api/token/webstar_package"},
	}

	for _, tc := range cases {
		code, _, raw := admin.RawDo(tc.method, tc.path, map[string]any{}, nil)
		if code != http.StatusNotImplemented {
			t.Errorf("%s %s: expected 501 Not Implemented, got %d (%s)", tc.method, tc.path, code, string(raw))
		}
		var body map[string]any
		if err := json.Unmarshal(raw, &body); err != nil || body["error"] != "not_implemented" {
			t.Errorf("%s %s: expected error=not_implemented, got %s", tc.method, tc.path, string(raw))
		}
	}
}
