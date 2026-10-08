package controller

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"openapi/constant"
	"openapi/types"

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
		"GetStatus":      func(c gin.Context) { GetStatus(c) },
		"GetAbout":       func(c gin.Context) { GetAbout(c) },
		"ClearDiskCache": func(c gin.Context) { ClearDiskCache(c) },
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

func TestRelayRejectsUnsupportedFormat(t *testing.T) {
	for _, format := range []types.RelayFormat{types.RelayFormatClaude, types.RelayFormatGemini} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		Relay(c, format)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%s: expected 400, got %d", format, w.Code)
		}
		var body struct {
			Error types.OpenAIError `json:"error"`
		}
		if err := json.NewDecoder(w.Body).Decode(&body); err != nil || body.Error.Message == "" {
			t.Fatalf("%s: expected OpenAI error body, err=%v body=%+v", format, err, body)
		}
	}
}

func TestShouldRetry(t *testing.T) {
	statusErr := func(code int, opts ...types.StarAPIErrorOptions) *types.StarAPIError {
		return types.NewErrorWithStatusCode(errors.New("x"), types.ErrorCodeBadResponseStatusCode, code, opts...)
	}
	cases := []struct {
		name      string
		err       *types.StarAPIError
		remaining int
		specific  bool
		want      bool
	}{
		{"429", statusErr(http.StatusTooManyRequests), 1, false, true},
		{"503", statusErr(http.StatusServiceUnavailable), 1, false, true},
		{"401 bad key", statusErr(http.StatusUnauthorized), 1, false, true},
		{"504 timeout", statusErr(http.StatusGatewayTimeout), 1, false, false},
		{"400 bad request", statusErr(http.StatusBadRequest), 1, false, false},
		{"no retries left", statusErr(http.StatusServiceUnavailable), 0, false, false},
		{"skip retry", statusErr(http.StatusServiceUnavailable, types.ErrOptionWithSkipRetry()), 1, false, false},
		{"specific channel", statusErr(http.StatusServiceUnavailable), 1, true, false},
		{"nil error", nil, 1, false, false},
	}
	for _, tc := range cases {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		if tc.specific {
			c.Set(string(constant.ContextKeyTokenSpecificChannelId), "3")
		}
		if got := shouldRetry(c, tc.err, tc.remaining); got != tc.want {
			t.Errorf("%s: shouldRetry = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestShouldNotRetryAfterResponseWritten(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.String(http.StatusOK, "partial")
	err := types.NewErrorWithStatusCode(errors.New("x"), types.ErrorCodeBadResponseStatusCode, http.StatusServiceUnavailable)
	if shouldRetry(c, err, 1) {
		t.Fatal("should not retry after response is written")
	}
}
