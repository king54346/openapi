package gemini_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"openapi/common"
	"openapi/constant"
	"openapi/dto"
	"openapi/model"
	"openapi/relay/channel/gemini"
	relaycommon "openapi/relay/common"
	relayconstant "openapi/relay/constant"
	"openapi/router"

	"github.com/glebarez/sqlite"
	gin "github.com/king54346/gin-tiny"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestGeminiAdaptorURLAndHeaders(t *testing.T) {
	a := &gemini.Adaptor{}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeGemini,
			ChannelBaseUrl:    "https://generativelanguage.googleapis.com",
			ApiKey:            "gem-secret",
			UpstreamModelName: "gemini-2.5-flash",
		},
		RelayMode: relayconstant.RelayModeChatCompletions,
		IsStream:  true,
	}
	url, err := a.GetRequestURL(info)
	if err != nil {
		t.Fatalf("GetRequestURL: %v", err)
	}
	if url != "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:streamGenerateContent?alt=sse" {
		t.Fatalf("unexpected stream URL: %s", url)
	}

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.SetRequest(httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil))
	hdr := http.Header{}
	hdr.Set("Authorization", "Bearer user-token")
	if err := a.SetupRequestHeader(c, &hdr, info); err != nil {
		t.Fatalf("SetupRequestHeader: %v", err)
	}
	if hdr.Get("x-goog-api-key") != "gem-secret" || hdr.Get("Authorization") != "" {
		t.Fatalf("unexpected headers: %v", hdr)
	}

	embReq := dto.EmbeddingRequest{Model: "text-embedding-004", Input: []any{"a", "b"}}
	info.RelayMode = relayconstant.RelayModeEmbeddings
	info.UpstreamModelName = "text-embedding-004"
	info.Request = &embReq
	embURL, err := a.GetRequestURL(info)
	if err != nil || !strings.HasSuffix(embURL, "/v1beta/models/text-embedding-004:batchEmbedContents") {
		t.Fatalf("unexpected batch embedding URL: %s (%v)", embURL, err)
	}
	converted, err := a.ConvertEmbeddingRequest(c, info, embReq)
	if err != nil {
		t.Fatalf("ConvertEmbeddingRequest: %v", err)
	}
	batch, ok := converted.(*dto.GeminiBatchEmbeddingRequest)
	if !ok || len(batch.Requests) != 2 || batch.Requests[0].Model != "models/text-embedding-004" {
		t.Fatalf("unexpected converted batch embedding: %+v", converted)
	}
}

func TestMultiProtocolRelayEndToEnd(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := model.InitDB(db, nil); err != nil {
		t.Fatal(err)
	}

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		switch {
		// Native Gemini upstream
		case strings.Contains(r.URL.Path, "/v1beta/models/gemini-2.5-flash:generateContent"):
			if r.Header.Get("x-goog-api-key") != "gem-key" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"responseId":"g1","modelVersion":"gemini-2.5-flash","candidates":[{"index":0,"content":{"role":"model","parts":[{"text":"hello from gemini"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":4,"candidatesTokenCount":3,"totalTokenCount":7}}`)

		case strings.Contains(r.URL.Path, "/v1beta/models/gemini-2.5-flash:streamGenerateContent"):
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(w, "data: {\"candidates\":[{\"index\":0,\"content\":{\"role\":\"model\",\"parts\":[{\"text\":\"gem \"}]}}]}\n\n")
			_, _ = fmt.Fprint(w, "data: {\"candidates\":[{\"index\":0,\"content\":{\"role\":\"model\",\"parts\":[{\"text\":\"stream\"}]},\"finishReason\":\"STOP\"}],\"usageMetadata\":{\"promptTokenCount\":3,\"candidatesTokenCount\":2,\"totalTokenCount\":5}}\n\n")

		// OpenAI/DeepSeek upstream (also used for Gemini->OpenAI cross-protocol & /v1/responses fallback)
		case strings.HasSuffix(r.URL.Path, "/chat/completions"):
			if bytes.Contains(body, []byte(`"stream":true`)) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, "data: {\"id\":\"c1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"deepseek-chat\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"ds \"}}]}\n\n")
				_, _ = io.WriteString(w, "data: {\"id\":\"c1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"deepseek-chat\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":6,\"completion_tokens\":2,\"total_tokens\":8}}\n\ndata: [DONE]\n\n")
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"id":"c1","object":"chat.completion","created":1,"model":"deepseek-chat","choices":[{"index":0,"message":{"role":"assistant","content":"hello from deepseek"},"finish_reason":"stop"}],"usage":{"prompt_tokens":6,"completion_tokens":3,"total_tokens":9}}`)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(upstream.Close)

	gemBase := upstream.URL
	dsBase := upstream.URL
	for _, ch := range []*model.Channel{
		{Type: constant.ChannelTypeGemini, Key: "gem-key", Name: "gem", Status: common.ChannelStatusEnabled, Models: "gemini-2.5-flash", BaseURL: &gemBase},
		{Type: constant.ChannelTypeDeepSeek, Key: "ds-key", Name: "ds", Status: common.ChannelStatusEnabled, Models: "deepseek-chat", BaseURL: &dsBase},
	} {
		if err := ch.Insert(); err != nil {
			t.Fatal(err)
		}
	}
	if err := model.InitChannelCache(); err != nil {
		t.Fatal(err)
	}

	u := &model.User{Username: "u1", Role: common.RoleCommonUser}
	if err := u.Insert("password123"); err != nil {
		t.Fatal(err)
	}
	tk := &model.Token{UserId: u.Id, Name: "tk", UnlimitedQuota: true, ExpiredTime: -1}
	if err := tk.Insert(); err != nil {
		t.Fatal(err)
	}

	engine := gin.New()
	router.SetRelayRouter(engine)

	doRelay := func(method, path string, body any) *httptest.ResponseRecorder {
		var r io.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			r = bytes.NewReader(b)
		}
		req := httptest.NewRequest(method, path, r)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer sk-"+tk.Key)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		return w
	}

	// 1. Native Gemini POST /v1beta/models/gemini-2.5-flash:generateContent -> Gemini channel
	w := doRelay(http.MethodPost, "/v1beta/models/gemini-2.5-flash:generateContent", map[string]any{
		"contents": []map[string]any{{"role": "user", "parts": []map[string]string{{"text": "hi"}}}},
	})
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "hello from gemini") {
		t.Fatalf("native gemini generateContent: %d %s", w.Code, w.Body.String())
	}

	// 2. OpenAI Chat Completions (stream) -> Gemini channel
	w = doRelay(http.MethodPost, "/v1/chat/completions", map[string]any{
		"model":    "gemini-2.5-flash",
		"stream":   true,
		"messages": []map[string]string{{"role": "user", "content": "hi"}},
	})
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"content":"gem "`) || !strings.Contains(w.Body.String(), "[DONE]") {
		t.Fatalf("openai stream -> gemini channel: %d %s", w.Code, w.Body.String())
	}

	// 3. Gemini protocol request -> DeepSeek (OpenAI-compatible) channel cross-protocol conversion
	w = doRelay(http.MethodPost, "/v1beta/models/deepseek-chat:generateContent", map[string]any{
		"contents": []map[string]any{{"role": "user", "parts": []map[string]string{{"text": "hi"}}}},
	})
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "hello from deepseek") || !strings.Contains(w.Body.String(), `"candidates"`) {
		t.Fatalf("gemini -> deepseek cross-protocol: %d %s", w.Code, w.Body.String())
	}

	// 4. OpenAI /v1/responses non-stream & stream -> DeepSeek channel via automatic ChatCompletions fallback
	w = doRelay(http.MethodPost, "/v1/responses", map[string]any{
		"model": "deepseek-chat",
		"input": "hello",
	})
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"object":"response"`) || !strings.Contains(w.Body.String(), "hello from deepseek") {
		t.Fatalf("responses non-stream fallback: %d %s", w.Code, w.Body.String())
	}

	w = doRelay(http.MethodPost, "/v1/responses", map[string]any{
		"model":  "deepseek-chat",
		"stream": true,
		"input":  "hello",
	})
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "response.output_text.delta") || !strings.Contains(w.Body.String(), "response.completed") {
		t.Fatalf("responses stream fallback: %d %s", w.Code, w.Body.String())
	}
}
