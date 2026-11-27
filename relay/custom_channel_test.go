package relay_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"openapi/constant"
	"openapi/model"
	"openapi/relay"
	relaycommon "openapi/relay/common"
	channelsvc "openapi/service/channel"
)

func TestCustomChannelType8RelayAndChannelTest(t *testing.T) {
	if !constant.IsRelaySupportedChannelType(constant.ChannelTypeCustom) {
		t.Fatal("expected ChannelTypeCustom (8) to be supported for relay")
	}
	apiType, ok := constant.ChannelType2APIType(constant.ChannelTypeCustom)
	if !ok || apiType != constant.APITypeOpenAI {
		t.Fatalf("expected ChannelTypeCustom (8) -> APITypeOpenAI, got (%d, %v)", apiType, ok)
	}
	if relay.GetAdaptor(apiType) == nil {
		t.Fatal("expected non-nil adaptor for ChannelTypeCustom (8)")
	}

	// 验证末尾带 '#' 的完全自定义地址不会拼接默认路径
	exact := relaycommon.GetFullRequestURL("https://proxy.example.com/custom/chat#", "/v1/chat/completions", constant.ChannelTypeCustom)
	if exact != "https://proxy.example.com/custom/chat" {
		t.Fatalf("expected '#' suffix to preserve exact custom URL, got %s", exact)
	}

	// 验证渠道测试（TestChannelRaw）对 type=8 渠道真实发起 OpenAI 兼容请求而非 skipped
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected request path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-custom","object":"chat.completion","model":"deepseek-flash","choices":[{"index":0,"message":{"role":"assistant","content":"pong"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":1,"total_tokens":4}}`))
	}))
	defer mockUpstream.Close()

	baseURL := mockUpstream.URL
	ch := &model.Channel{
		Id:      88,
		Type:    constant.ChannelTypeCustom, // type = 8
		Name:    "custom-openai-relay",
		Key:     "sk-custom-test-key",
		BaseURL: &baseURL,
		Models:  "deepseek-flash",
	}

	res := channelsvc.TestChannelRaw(ch, "deepseek-flash")
	if res.Skipped {
		t.Fatalf("expected channel type 8 NOT to be skipped, got skip reason: %s", res.SkipReason)
	}
	if res.Err != nil {
		t.Fatalf("expected channel type 8 test to succeed, got error: %v", res.Err)
	}
}
