package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	gin "github.com/king54346/gin-tiny"
)

func TestPassthroughStubsReturnHandler(t *testing.T) {
	mws := map[string]func() gin.HandlerFunc{
		"DecompressRequestMiddleware": DecompressRequestMiddleware,
		"DisableCache":                DisableCache,
		"ModelRequestRateLimit":       ModelRequestRateLimit,
		"SecureVerificationRequired":  SecureVerificationRequired,
		"TurnstileCheck":              TurnstileCheck,
	}
	for name, factory := range mws {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.SetRequest(httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil))
		h := factory()
		if h == nil {
			t.Fatalf("%s returned nil handler", name)
		}
		h(c)
	}
}

func TestAuthFactoriesReturnHandler(t *testing.T) {
	mws := map[string]func() gin.HandlerFunc{
		"TokenAuth":   TokenAuth,
		"UserAuth":    UserAuth,
		"AdminAuth":   AdminAuth,
		"RootAuth":    RootAuth,
		"TryUserAuth": TryUserAuth,
	}
	for name, factory := range mws {
		if factory() == nil {
			t.Fatalf("%s returned nil handler", name)
		}
	}
}

func TestAbortWithOpenAiMessageAborts(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	abortWithOpenAiMessage(c, 403, "forbidden")
	if w.Code != 403 {
		t.Fatalf("expected 403, got %d", w.Code)
	}
	if !c.IsAborted() {
		t.Fatal("expected context to be aborted")
	}
}

func TestNormalizeBearerKey(t *testing.T) {
	if got := normalizeBearerKey("Bearer sk-abc"); got != "sk-abc" {
		t.Fatalf("expected sk-abc, got %q", got)
	}
	if got := normalizeBearerKey("bearer xyz"); got != "xyz" {
		t.Fatalf("expected case-insensitive prefix trim, got %q", got)
	}
	if got := normalizeBearerKey("  plain  "); got != "plain" {
		t.Fatalf("expected trimmed plain key, got %q", got)
	}
}

func TestToInt(t *testing.T) {
	if v, ok := toInt(42); !ok || v != 42 {
		t.Fatalf("int failed: %v %v", v, ok)
	}
	if v, ok := toInt("7"); !ok || v != 7 {
		t.Fatalf("string int failed: %v %v", v, ok)
	}
	if _, ok := toInt("abc"); ok {
		t.Fatal("non-numeric string should fail")
	}
	if _, ok := toInt(nil); ok {
		t.Fatal("nil should fail")
	}
}
