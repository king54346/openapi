package relay

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"openapi/constant"
	"openapi/relay/channel"
	relaycommon "openapi/relay/common"
	"openapi/types"

	gin "github.com/king54346/gin-tiny"
)

func TestConvertRequestError(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.SetRequest(httptest.NewRequest(http.MethodPost, "/v1/responses", nil))
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeDeepSeek}}

	unsupported := convertRequestError(c, info, fmt.Errorf("wrap: %w", channel.ErrNotImplemented))
	if unsupported.StatusCode != http.StatusBadRequest || !types.IsSkipRetryError(unsupported) {
		t.Fatalf("unsupported endpoint: status=%d skipRetry=%v", unsupported.StatusCode, types.IsSkipRetryError(unsupported))
	}
	if msg := unsupported.Error(); msg != "channel type DeepSeek does not support /v1/responses" {
		t.Fatalf("message: %q", msg)
	}

	other := convertRequestError(c, info, errors.New("bad field"))
	if other.GetErrorCode() != types.ErrorCodeConvertRequestFailed || !types.IsSkipRetryError(other) {
		t.Fatalf("other convert error: code=%s skipRetry=%v", other.GetErrorCode(), types.IsSkipRetryError(other))
	}
}
