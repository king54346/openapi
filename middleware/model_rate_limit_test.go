package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"openapi/common/limiter"
	"openapi/setting"

	gin "github.com/king54346/gin-tiny"
)

func serveModelRateLimit(t *testing.T, setup func(c gin.Context)) *httptest.ResponseRecorder {
	t.Helper()
	r := gin.New()
	r.Use(ModelRequestRateLimit())
	r.GET("/v1/chat/completions", func(c gin.Context) {
		setup(c)
		c.String(http.StatusOK, "ok")
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestModelRateLimitDisabledPassthrough(t *testing.T) {
	setting.GetGlobalSettings().ModelRequestRateLimitEnabled = false
	w := serveModelRateLimit(t, func(c gin.Context) {})
	if w.Code != http.StatusOK {
		t.Fatalf("disabled limiter should passthrough, got %d", w.Code)
	}
}

func TestModelRateLimitMemoryPassthrough(t *testing.T) {
	cfg := setting.GetGlobalSettings()
	cfg.ModelRequestRateLimitEnabled = true
	cfg.ModelRequestRateLimitDurationMinutes = 1
	cfg.ModelRequestRateLimitCount = 100
	cfg.ModelRequestRateLimitSuccessCount = 100
	defer func() { cfg.ModelRequestRateLimitEnabled = false }()

	w := serveModelRateLimit(t, func(c gin.Context) {})
	if w.Code != http.StatusOK {
		t.Fatalf("memory stub limiter should passthrough, got %d", w.Code)
	}
}

func TestModelRateLimitCheckAndRecord(t *testing.T) {
	allowed, err := limiter.WindowAllow(t.Context(), "k", 0, 60, time.Minute)
	if err != nil || !allowed {
		t.Fatalf("maxCount=0 should allow, allowed=%v err=%v", allowed, err)
	}
	if err := limiter.WindowRecord(t.Context(), "k", 0, time.Minute); err != nil {
		t.Fatalf("maxCount=0 record should be no-op, err=%v", err)
	}

	if got := modelRateLimitExceededMessage("limit %d per %d min", 5); got == "" {
		t.Fatal("expected non-empty message")
	}

	if window := modelRateLimitWindow(); window <= 0 {
		t.Fatalf("expected positive window, got %v", window)
	}
}

func TestGroupRateLimitRoundTrip(t *testing.T) {
	setting.SetGroupRateLimit("test-group", 10, 5)
	total, success, found := setting.GetGroupRateLimit("test-group")
	if !found || total != 10 || success != 5 {
		t.Fatalf("unexpected group limit: %d %d %v", total, success, found)
	}
	if _, _, found := setting.GetGroupRateLimit("no-such-group"); found {
		t.Fatal("unknown group should not be found")
	}
}
