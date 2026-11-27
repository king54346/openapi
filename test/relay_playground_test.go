package e2e_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// TestRelayMultiProtocolAndPlaygroundAPIs 在运行中的后端上注册临时 DeepSeek 与自定义 OpenAI 渠道，
// 完整覆盖模型列表、OpenAI Chat/Responses/Embeddings/Rerank、Anthropic Claude、Google Gemini 及 Playground 接口，
// 测试结束后自动清理临时渠道与令牌。
func TestRelayMultiProtocolAndPlaygroundAPIs(t *testing.T) {
	admin := newLoggedInAdminClient(t)
	anon := newLiveClient(t)

	// 1. 拉取两个真实上游的可用模型列表
	dsKey := deepSeekTestKey(t)
	xzURL := customOpenAIBaseURL(t)
	xzKey := customOpenAITestKey(t)

	dsModels := mustDecode[[]string](t, admin.MustOK(http.MethodPost, "/api/channel/fetch_models", map[string]any{
		"type": 43,
		"key":  dsKey,
	}))
	dsModel := "deepseek-flash"
	if len(dsModels) > 0 {
		dsModel = dsModels[0]
	}

	// 为 Playground 定义独立模型别名，通过 model_mapping 定向打到本次临时渠道，不影响线上渠道
	pgChatAlias := uniqueName("e2e-pg-chat")
	pgEmbedAlias := uniqueName("e2e-pg-embed")
	pgRerankAlias := uniqueName("e2e-pg-rerank")

	dsAdd := mustDecode[struct {
		IDs []int `json:"ids"`
	}](t, admin.MustOK(http.MethodPost, "/api/channel/", map[string]any{
		"mode": "single",
		"channel": map[string]any{
			"name":          uniqueName("e2e_relay_ds"),
			"type":          43,
			"key":           dsKey,
			"models":        dsModel + "," + pgChatAlias,
			"model_mapping": fmt.Sprintf(`{%q:%q}`, pgChatAlias, dsModel),
			"auto_ban":      0,
		},
	}))
	dsChannelID := dsAdd.IDs[0]

	xzChatModel := "Qwen/Qwen2.5-7B-Instruct"
	xzEmbedModel := "BAAI/bge-large-en-v1.5"
	xzRerankModel := "BAAI/bge-reranker-v2-m3"

	xzAdd := mustDecode[struct {
		IDs []int `json:"ids"`
	}](t, admin.MustOK(http.MethodPost, "/api/channel/", map[string]any{
		"mode": "single",
		"channel": map[string]any{
			"name":          uniqueName("e2e_relay_xz"),
			"type":          1,
			"base_url":      xzURL,
			"key":           xzKey,
			"models":        strings.Join([]string{xzChatModel, xzEmbedModel, xzRerankModel, pgEmbedAlias, pgRerankAlias}, ","),
			"model_mapping": fmt.Sprintf(`{%q:%q,%q:%q}`, pgEmbedAlias, xzEmbedModel, pgRerankAlias, xzRerankModel),
			"auto_ban":      0,
		},
	}))
	xzChannelID := xzAdd.IDs[0]

	tk := mustDecode[struct {
		ID  int    `json:"id"`
		Key string `json:"key"`
	}](t, admin.MustOK(http.MethodPost, "/api/token/", map[string]any{
		"name":            uniqueName("e2e_relay_tk"),
		"unlimited_quota": true,
		"expired_time":    -1,
	}))

	t.Cleanup(func() {
		_, _, _ = admin.DoAPI(http.MethodDelete, fmt.Sprintf("/api/token/%d", tk.ID), nil, nil)
		_, _, _ = admin.DoAPI(http.MethodPost, "/api/channel/batch", map[string]any{
			"ids": []int{dsChannelID, xzChannelID},
		}, nil)
	})

	baseKey := "sk-" + tk.Key
	dsPinnedKey := fmt.Sprintf("sk-%s-%d", tk.Key, dsChannelID)
	xzPinnedKey := fmt.Sprintf("sk-%s-%d", tk.Key, xzChannelID)

	// ==================== 2. 模型列表接口 ====================
	t.Run("ModelsEndpoints", func(t *testing.T) {
		bearer := map[string]string{"Authorization": "Bearer " + baseKey}
		for _, path := range []string{
			"/v1/models",
			"/v1/models/" + pgChatAlias,
			"/v1beta/models",
			"/v1beta/models/" + pgChatAlias,
			"/v1beta/openai/models",
		} {
			code, _, body := anon.RawDo(http.MethodGet, path, nil, bearer)
			if code != http.StatusOK {
				t.Fatalf("GET %s failed: HTTP %d %s", path, code, string(body))
			}
		}

		// Claude 头识别 /v1/models
		code, _, _ := anon.RawDo(http.MethodGet, "/v1/models", nil, map[string]string{
			"x-api-key":         baseKey,
			"anthropic-version": "2023-06-01",
		})
		if code != http.StatusOK {
			t.Fatalf("Claude GET /v1/models expected 200, got %d", code)
		}

		// Gemini 头识别 /v1/models
		code, _, _ = anon.RawDo(http.MethodGet, "/v1/models", nil, map[string]string{
			"x-goog-api-key": baseKey,
		})
		if code != http.StatusOK {
			t.Fatalf("Gemini GET /v1/models expected 200, got %d", code)
		}
	})

	// 公共多协议验证闭包：在指定渠道与模型上验证 OpenAI 同步/流式、Responses 同步/流式、Claude 同步/流式、Gemini 同步/流式
	verifyMultiProtocolSuite := func(t *testing.T, pinnedKey, modelName string) {
		oaiHdr := map[string]string{"Authorization": "Bearer " + pinnedKey}

		// 1) OpenAI /v1/chat/completions (非流式)
		code, _, body := anon.RawDo(http.MethodPost, "/v1/chat/completions", map[string]any{
			"model":      modelName,
			"max_tokens": 256,
			"messages":   []map[string]string{{"role": "user", "content": "Reply PONG only."}},
		}, oaiHdr)
		if code != http.StatusOK || !strings.Contains(string(body), `"choices"`) {
			t.Fatalf("POST /v1/chat/completions sync (%s): HTTP %d %s", modelName, code, string(body))
		}

		// 2) OpenAI /v1/chat/completions (SSE 流式) + /v1beta/openai/chat/completions
		for _, path := range []string{"/v1/chat/completions", "/v1beta/openai/chat/completions"} {
			code, _, body = anon.RawDo(http.MethodPost, path, map[string]any{
				"model":      modelName,
				"max_tokens": 256,
				"stream":     true,
				"messages":   []map[string]string{{"role": "user", "content": "Say OK."}},
			}, oaiHdr)
			if code != http.StatusOK || !strings.Contains(string(body), "data: [DONE]") {
				t.Fatalf("POST %s SSE (%s): HTTP %d %s", path, modelName, code, string(body))
			}
		}

		// 3) OpenAI /v1/responses (非流式 + SSE 流式)
		code, _, body = anon.RawDo(http.MethodPost, "/v1/responses", map[string]any{
			"model":             modelName,
			"max_output_tokens": 256,
			"input":             "Reply 1+1=2 only.",
		}, oaiHdr)
		if code != http.StatusOK || !strings.Contains(string(body), `"object":"response"`) {
			t.Fatalf("POST /v1/responses sync (%s): HTTP %d %s", modelName, code, string(body))
		}

		code, _, body = anon.RawDo(http.MethodPost, "/v1/responses", map[string]any{
			"model":             modelName,
			"max_output_tokens": 256,
			"stream":            true,
			"input":             "Say OK.",
		}, oaiHdr)
		if code != http.StatusOK || !strings.Contains(string(body), "response.completed") {
			t.Fatalf("POST /v1/responses stream (%s): HTTP %d %s", modelName, code, string(body))
		}

		// 4) Anthropic Claude /v1/messages (非流式 + SSE 流式)
		claudeHdr := map[string]string{
			"x-api-key":         pinnedKey,
			"anthropic-version": "2023-06-01",
		}
		code, _, body = anon.RawDo(http.MethodPost, "/v1/messages", map[string]any{
			"model":      modelName,
			"max_tokens": 256,
			"messages":   []map[string]string{{"role": "user", "content": "Reply HELLO."}},
		}, claudeHdr)
		if code != http.StatusOK || !strings.Contains(string(body), `"type":"message"`) {
			t.Fatalf("POST /v1/messages sync (%s): HTTP %d %s", modelName, code, string(body))
		}

		code, _, body = anon.RawDo(http.MethodPost, "/v1/messages", map[string]any{
			"model":      modelName,
			"max_tokens": 256,
			"stream":     true,
			"messages":   []map[string]string{{"role": "user", "content": "Reply HELLO."}},
		}, claudeHdr)
		if code != http.StatusOK || !strings.Contains(string(body), "message_stop") {
			t.Fatalf("POST /v1/messages stream (%s): HTTP %d %s", modelName, code, string(body))
		}

		// 5) Google Gemini /v1beta/models/*:generateContent & :streamGenerateContent
		gemHdr := map[string]string{"x-goog-api-key": pinnedKey}
		code, _, body = anon.RawDo(http.MethodPost, fmt.Sprintf("/v1beta/models/%s:generateContent", modelName), map[string]any{
			"contents":         []map[string]any{{"role": "user", "parts": []map[string]string{{"text": "Reply HI."}}}},
			"generationConfig": map[string]any{"maxOutputTokens": 256},
		}, gemHdr)
		if code != http.StatusOK || !strings.Contains(string(body), `"candidates"`) {
			t.Fatalf("POST /v1beta/models/%s:generateContent: HTTP %d %s", modelName, code, string(body))
		}

		code, _, body = anon.RawDo(http.MethodPost, fmt.Sprintf("/v1beta/models/%s:streamGenerateContent?alt=sse", modelName), map[string]any{
			"contents":         []map[string]any{{"role": "user", "parts": []map[string]string{{"text": "Reply HI."}}}},
			"generationConfig": map[string]any{"maxOutputTokens": 256},
		}, gemHdr)
		if code != http.StatusOK || !strings.Contains(string(body), `"candidates"`) {
			t.Fatalf("POST /v1beta/models/%s:streamGenerateContent: HTTP %d %s", modelName, code, string(body))
		}
	}

	t.Run("DeepSeekMultiProtocol", func(t *testing.T) {
		verifyMultiProtocolSuite(t, dsPinnedKey, dsModel)
	})

	t.Run("CustomOpenAIMultiProtocolAndEmbeddingsRerank", func(t *testing.T) {
		verifyMultiProtocolSuite(t, xzPinnedKey, xzChatModel)

		xzHdr := map[string]string{"Authorization": "Bearer " + xzPinnedKey}
		// /v1/embeddings & /v1beta/openai/embeddings & /v1/engines/:model/embeddings
		for _, embPath := range []string{
			"/v1/embeddings",
			"/v1beta/openai/embeddings",
		} {
			code, _, body := anon.RawDo(http.MethodPost, embPath, map[string]any{
				"model": xzEmbedModel,
				"input": "hello openapi",
			}, xzHdr)
			if code != http.StatusOK || !strings.Contains(string(body), `"embedding"`) {
				t.Fatalf("POST %s failed: HTTP %d %s", embPath, code, string(body))
			}
		}

		// /v1/rerank
		code, _, body := anon.RawDo(http.MethodPost, "/v1/rerank", map[string]any{
			"model":     xzRerankModel,
			"query":     "apple",
			"documents": []string{"apple is a fruit", "golang http server"},
		}, xzHdr)
		if code != http.StatusOK || !strings.Contains(string(body), `"results"`) {
			t.Fatalf("POST /v1/rerank failed: HTTP %d %s", code, string(body))
		}
	})

	// ==================== 3. Playground 接口（Session 鉴权） ====================
	t.Run("PlaygroundEndpoints", func(t *testing.T) {
		// 未登录访问 Playground 应返回 401
		code, _, _ := anon.RawDo(http.MethodPost, "/pg/chat/completions", map[string]any{
			"model":    pgChatAlias,
			"messages": []map[string]string{{"role": "user", "content": "hi"}},
		}, nil)
		if code != http.StatusUnauthorized {
			t.Fatalf("unauthenticated Playground expected 401, got %d", code)
		}

		// 已登录管理员直接调用 /pg/chat/completions、/pg/embeddings、/pg/rerank
		code, _, body := admin.RawDo(http.MethodPost, "/pg/chat/completions", map[string]any{
			"model":      pgChatAlias,
			"max_tokens": 256,
			"messages":   []map[string]string{{"role": "user", "content": "Say PG_OK"}},
		}, nil)
		if code != http.StatusOK || !strings.Contains(string(body), `"choices"`) {
			t.Fatalf("POST /pg/chat/completions failed: HTTP %d %s", code, string(body))
		}

		code, _, body = admin.RawDo(http.MethodPost, "/pg/embeddings", map[string]any{
			"model": pgEmbedAlias,
			"input": "playground embedding test",
		}, nil)
		if code != http.StatusOK || !strings.Contains(string(body), `"embedding"`) {
			t.Fatalf("POST /pg/embeddings failed: HTTP %d %s", code, string(body))
		}

		code, _, body = admin.RawDo(http.MethodPost, "/pg/rerank", map[string]any{
			"model":     pgRerankAlias,
			"query":     "banana",
			"documents": []string{"yellow fruit", "database index"},
		}, nil)
		if code != http.StatusOK || !strings.Contains(string(body), `"results"`) {
			t.Fatalf("POST /pg/rerank failed: HTTP %d %s", code, string(body))
		}
	})

	// ==================== 4. 异常与边界状态码契约 ====================
	t.Run("ErrorContracts", func(t *testing.T) {
		// 无效令牌 -> 401
		if code, _, _ := anon.RawDo(http.MethodGet, "/v1/models", nil, map[string]string{"Authorization": "Bearer sk-invalid-token"}); code != http.StatusUnauthorized {
			t.Fatalf("invalid token expected 401, got %d", code)
		}
		// 缺少必填 messages -> 400
		if code, _, _ := anon.RawDo(http.MethodPost, "/v1/chat/completions", map[string]any{"model": pgChatAlias}, map[string]string{"Authorization": "Bearer " + baseKey}); code != http.StatusBadRequest {
			t.Fatalf("missing messages expected 400, got %d", code)
		}
		// 不存在的模型 -> 503
		if code, _, _ := anon.RawDo(http.MethodPost, "/v1/chat/completions", map[string]any{
			"model":    "nonexistent-model-xyz-999",
			"messages": []map[string]string{{"role": "user", "content": "hi"}},
		}, map[string]string{"Authorization": "Bearer " + baseKey}); code != http.StatusServiceUnavailable {
			t.Fatalf("nonexistent model expected 503, got %d", code)
		}
		// 未知 API 路由 -> 404 JSON
		code, _, raw := anon.RawDo(http.MethodGet, "/api/unknown_route_xyz", nil, nil)
		var errBody map[string]any
		_ = json.Unmarshal(raw, &errBody)
		if code != http.StatusNotFound || errBody["error"] != "not_found" {
			t.Fatalf("unknown API route expected 404 JSON, got %d %s", code, string(raw))
		}
	})
}
