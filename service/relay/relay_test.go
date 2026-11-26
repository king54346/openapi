package relay

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"openapi/common"
	"openapi/constant"
	"openapi/dto"
	"openapi/types"

	gin "github.com/king54346/gin-tiny"
)

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
		if got := ShouldRetry(c, tc.err, tc.remaining); got != tc.want {
			t.Errorf("%s: ShouldRetry = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestShouldRetryTaskRelay(t *testing.T) {
	cases := []struct {
		name      string
		err       *dto.TaskError
		remaining int
		specific  bool
		want      bool
	}{
		{"429", &dto.TaskError{StatusCode: http.StatusTooManyRequests}, 1, false, true},
		{"502", &dto.TaskError{StatusCode: http.StatusBadGateway}, 1, false, true},
		{"504", &dto.TaskError{StatusCode: http.StatusGatewayTimeout}, 1, false, false},
		{"400", &dto.TaskError{StatusCode: http.StatusBadRequest}, 1, false, false},
		{"local error", &dto.TaskError{StatusCode: http.StatusInternalServerError, LocalError: true}, 1, false, false},
		{"no retries left", &dto.TaskError{StatusCode: http.StatusBadGateway}, 0, false, false},
		{"specific channel", &dto.TaskError{StatusCode: http.StatusBadGateway}, 1, true, false},
		{"nil", nil, 1, false, false},
	}
	for _, tc := range cases {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		if tc.specific {
			c.Set(string(constant.ContextKeyTokenSpecificChannelId), "3")
		}
		if got := ShouldRetryTaskRelay(c, tc.err, tc.remaining); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestRequestBodyError(t *testing.T) {
	if e := RequestBodyError(fmt.Errorf("wrap: %w", common.ErrRequestBodyTooLarge)); e.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("too large: %d", e.StatusCode)
	}
	if e := RequestBodyError(errors.New("field messages is required")); e.StatusCode != http.StatusBadRequest || !types.IsSkipRetryError(e) {
		t.Fatalf("bad request: %d", e.StatusCode)
	}
}

func TestRetryPathTracking(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	AddUsedChannel(c, 3)
	AddUsedChannel(c, 999)
	if got := c.GetStringSlice("use_channel"); len(got) != 2 || got[0] != "3" || got[1] != "999" {
		t.Fatalf("use_channel = %v", got)
	}
}

func TestShouldNotRetryAfterResponseWritten(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.String(http.StatusOK, "partial")
	err := types.NewErrorWithStatusCode(errors.New("x"), types.ErrorCodeBadResponseStatusCode, http.StatusServiceUnavailable)
	if ShouldRetry(c, err, 1) {
		t.Fatal("should not retry after response is written")
	}
}

func TestFormatMatchingModelName(t *testing.T) {
	cases := map[string]string{
		"gpt-4o":                      "gpt-4o",
		"thinking-claude-3-7-sonnet":  "claude-3-7-sonnet",
		"deepseek-r1-online-high":     "deepseek-r1",
		"gpt-4o-search-nothinking":    "gpt-4o",
		"g-123456789-my-custom-agent": "gpt-4-gizmo-*",
		"gpt-4-gizmo-g-abc":           "gpt-4-gizmo-*",
	}
	for in, want := range cases {
		if got := FormatMatchingModelName(in); got != want {
			t.Errorf("FormatMatchingModelName(%q) = %q, want %q", in, got, want)
		}
	}
}
