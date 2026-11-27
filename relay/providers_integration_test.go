package relay_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"openapi/common"
	"openapi/constant"
	"openapi/dto"
	"openapi/model"
	"openapi/relay"
	relaycommon "openapi/relay/common"
	relayconstant "openapi/relay/constant"
	channelsvc "openapi/service/channel"
	httpclient "openapi/service/http"
	"openapi/types"

	"github.com/glebarez/sqlite"
	gin "github.com/king54346/gin-tiny"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func runInGinContext(t *testing.T, fn func(c gin.Context)) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	e := gin.New()
	e.POST("/test", func(c gin.Context) {
		fn(c)
	})
	e.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/test", nil))
	return w
}

func TestZhipuKimiSiliconFlowAdaptors(t *testing.T) {
	httpclient.InitHttpClient()

	// 1. 验证四种渠道均已注册 APIType 与 Adaptor
	for _, ct := range []int{
		constant.ChannelTypeZhipu,
		constant.ChannelTypeZhipu_v4,
		constant.ChannelTypeMoonshot,
		constant.ChannelTypeSiliconFlow,
	} {
		apiType, ok := constant.ChannelType2APIType(ct)
		if !ok {
			t.Fatalf("ChannelType2APIType(%d) not registered", ct)
		}
		adp := relay.GetAdaptor(apiType)
		if adp == nil || len(adp.GetModelList()) == 0 {
			t.Fatalf("GetAdaptor(%d) returned nil or empty models", apiType)
		}
	}

	// 2. 智谱 ZhipuV4：URL 路由（含 glm-coding-plan + Anthropic 兼容端点）、请求转换（TopP 截断 + base64 前缀剥离）与图像响应
	t.Run("ZhipuV4", func(t *testing.T) {
		apiType, _ := constant.ChannelType2APIType(constant.ChannelTypeZhipu_v4)
		adp := relay.GetAdaptor(apiType)

		info := &relaycommon.RelayInfo{
			ChannelMeta: &relaycommon.ChannelMeta{
				ChannelType:    constant.ChannelTypeZhipu_v4,
				ChannelBaseUrl: "",
			},
			RelayFormat: types.RelayFormatOpenAI,
			RelayMode:   relayconstant.RelayModeChatCompletions,
			StartTime:   time.Now(),
		}
		u1, _ := adp.GetRequestURL(info)
		if u1 != "https://open.bigmodel.cn/api/paas/v4/chat/completions" {
			t.Fatalf("unexpected ZhipuV4 chat URL: %s", u1)
		}

		info.ChannelBaseUrl = "glm-coding-plan"
		info.RelayFormat = types.RelayFormatClaude
		u2, _ := adp.GetRequestURL(info)
		if u2 != "https://open.bigmodel.cn/api/anthropic/v1/messages" {
			t.Fatalf("unexpected glm-coding-plan Claude URL: %s", u2)
		}

		topP := 1.0
		converted, err := adp.ConvertOpenAIRequest(nil, info, &dto.GeneralOpenAIRequest{
			Model: "glm-4v-plus",
			TopP:  &topP,
			Stop:  "END",
			Messages: []dto.Message{
				{
					Role: "user",
					Content: []any{
						map[string]any{"type": "text", "text": "describe"},
						map[string]any{
							"type":      "image_url",
							"image_url": map[string]any{"url": "data:image/png;base64,QUJDRA=="},
						},
					},
				},
			},
		})
		if err != nil {
			t.Fatalf("ZhipuV4 ConvertOpenAIRequest: %v", err)
		}
		rawReq, _ := common.Marshal(converted)
		if !bytes.Contains(rawReq, []byte(`"top_p":0.99`)) || bytes.Contains(rawReq, []byte("data:image/png;base64,")) || !bytes.Contains(rawReq, []byte("QUJDRA==")) {
			t.Fatalf("unexpected ZhipuV4 converted JSON: %s", string(rawReq))
		}

		info.RelayFormat = types.RelayFormatOpenAI
		info.RelayMode = relayconstant.RelayModeImagesGenerations
		w := runInGinContext(t, func(c gin.Context) {
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"created":1700000001,"data":[{"b64_json":"aW1nX2RhdGE="}]}`)),
			}
			if _, apiErr := adp.DoResponse(c, resp, info); apiErr != nil {
				t.Fatalf("ZhipuV4 image DoResponse: %v", apiErr)
			}
		})
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"b64_json":"aW1nX2RhdGE="`) {
			t.Fatalf("unexpected ZhipuV4 image response: %d %s", w.Code, w.Body.String())
		}
	})

	// 3. 智谱 Zhipu v3：JWT HS256 签名头 + 非流式与 SSE 流式 (data:/meta:) 转换
	t.Run("ZhipuV3", func(t *testing.T) {
		apiType, _ := constant.ChannelType2APIType(constant.ChannelTypeZhipu)
		adp := relay.GetAdaptor(apiType)
		info := &relaycommon.RelayInfo{
			ChannelMeta: &relaycommon.ChannelMeta{
				ChannelType:       constant.ChannelTypeZhipu,
				ApiKey:            "kid123.secret456",
				UpstreamModelName: "chatglm_turbo",
			},
			IsStream: true,
		}
		u, _ := adp.GetRequestURL(info)
		if u != "https://open.bigmodel.cn/api/paas/v3/model-api/chatglm_turbo/sse-invoke" {
			t.Fatalf("unexpected Zhipu v3 stream URL: %s", u)
		}

		_ = runInGinContext(t, func(c gin.Context) {
			hdr := make(http.Header)
			if err := adp.SetupRequestHeader(c, &hdr, info); err != nil {
				t.Fatalf("Zhipu v3 SetupRequestHeader: %v", err)
			}
			parts := strings.Split(hdr.Get("Authorization"), ".")
			if len(parts) != 3 {
				t.Fatalf("expected 3-segment HS256 JWT token, got %q", hdr.Get("Authorization"))
			}
		})

		wStream := runInGinContext(t, func(c gin.Context) {
			sseBody := "data:你好\ndata:世界\nmeta:{\"request_id\":\"r1\",\"task_id\":\"t1\",\"task_status\":\"SUCCESS\",\"usage\":{\"prompt_tokens\":4,\"completion_tokens\":2,\"total_tokens\":6}}\n"
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(sseBody)),
			}
			usageAny, apiErr := adp.DoResponse(c, resp, info)
			if apiErr != nil {
				t.Fatalf("Zhipu v3 stream DoResponse: %v", apiErr)
			}
			uObj, ok := usageAny.(*dto.Usage)
			if !ok || uObj == nil || uObj.TotalTokens != 6 {
				t.Fatalf("unexpected Zhipu v3 stream usage: %+v", usageAny)
			}
		})
		if !strings.Contains(wStream.Body.String(), "你好") || !strings.Contains(wStream.Body.String(), "[DONE]") {
			t.Fatalf("unexpected Zhipu v3 SSE output: %s", wStream.Body.String())
		}
	})

	// 4. Kimi / Moonshot：kimi-coding-plan 与 Claude /anthropic/v1/messages 路由
	t.Run("KimiMoonshot", func(t *testing.T) {
		apiType, _ := constant.ChannelType2APIType(constant.ChannelTypeMoonshot)
		adp := relay.GetAdaptor(apiType)

		info := &relaycommon.RelayInfo{
			ChannelMeta: &relaycommon.ChannelMeta{
				ChannelType:    constant.ChannelTypeMoonshot,
				ChannelBaseUrl: "kimi-coding-plan",
			},
			RelayFormat: types.RelayFormatOpenAI,
		}
		if u, _ := adp.GetRequestURL(info); u != "https://api.kimi.com/coding/v1/chat/completions" {
			t.Fatalf("unexpected kimi-coding-plan OpenAI URL: %s", u)
		}
		info.RelayFormat = types.RelayFormatClaude
		if u, _ := adp.GetRequestURL(info); u != "https://api.kimi.com/coding/v1/messages" {
			t.Fatalf("unexpected kimi-coding-plan Claude URL: %s", u)
		}
		info.ChannelBaseUrl = "https://api.moonshot.cn"
		if u, _ := adp.GetRequestURL(info); u != "https://api.moonshot.cn/anthropic/v1/messages" {
			t.Fatalf("unexpected Moonshot native Claude URL: %s", u)
		}
	})

	// 5. 硅基流动 SiliconFlow：FIM 补齐、图像参数转换、Rerank 响应及余额查询
	t.Run("SiliconFlow", func(t *testing.T) {
		apiType, _ := constant.ChannelType2APIType(constant.ChannelTypeSiliconFlow)
		adp := relay.GetAdaptor(apiType)

		info := &relaycommon.RelayInfo{
			ChannelMeta: &relaycommon.ChannelMeta{
				ChannelType: constant.ChannelTypeSiliconFlow,
			},
			RelayMode:      relayconstant.RelayModeRerank,
			RequestURLPath: "/v1/rerank",
		}
		if u, _ := adp.GetRequestURL(info); u != "https://api.siliconflow.cn/v1/rerank" {
			t.Fatalf("unexpected SiliconFlow rerank URL: %s", u)
		}

		fimReq := &dto.GeneralOpenAIRequest{Model: "Qwen/Qwen2.5-Coder-32B-Instruct", Prefix: "func add("}
		_, _ = adp.ConvertOpenAIRequest(nil, info, fimReq)
		if len(fimReq.Messages) != 1 {
			t.Fatalf("expected SiliconFlow FIM to populate empty user message, got %+v", fimReq.Messages)
		}

		n := uint(2)
		imgConverted, err := adp.ConvertImageRequest(nil, info, dto.ImageRequest{
			Model:       "black-forest-labs/FLUX.1-schnell",
			Prompt:      "mountain",
			Size:        "1024x1024",
			N:           &n,
			ExtraFields: json.RawMessage(`{"num_inference_steps":20}`),
		})
		if err != nil {
			t.Fatalf("SiliconFlow ConvertImageRequest: %v", err)
		}
		imgBytes, _ := common.Marshal(imgConverted)
		if !bytes.Contains(imgBytes, []byte(`"image_size":"1024x1024"`)) ||
			!bytes.Contains(imgBytes, []byte(`"batch_size":2`)) ||
			!bytes.Contains(imgBytes, []byte(`"num_inference_steps":20`)) {
			t.Fatalf("unexpected SiliconFlow image payload: %s", string(imgBytes))
		}

		wRerank := runInGinContext(t, func(c gin.Context) {
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(
					`{"results":[{"index":0,"relevance_score":0.98}],"meta":{"tokens":{"input_tokens":15,"output_tokens":3}}}`,
				)),
			}
			usageAny, apiErr := adp.DoResponse(c, resp, info)
			if apiErr != nil {
				t.Fatalf("SiliconFlow rerank DoResponse: %v", apiErr)
			}
			uObj := usageAny.(*dto.Usage)
			if uObj.PromptTokens != 15 || uObj.CompletionTokens != 3 || uObj.TotalTokens != 18 {
				t.Fatalf("unexpected SiliconFlow rerank usage: %+v", uObj)
			}
		})
		if wRerank.Code != http.StatusOK || !strings.Contains(wRerank.Body.String(), `"relevance_score":0.98`) {
			t.Fatalf("unexpected SiliconFlow rerank body: %s", wRerank.Body.String())
		}

		// 验证硅基流动 /v1/user/info 余额查询
		mockSF := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1/user/info" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":20000,"message":"ok","status":true,"data":{"totalBalance":"88.65"}}`))
		}))
		defer mockSF.Close()

		db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{Logger: logger.Discard})
		if err != nil {
			t.Fatal(err)
		}
		sqlDB, _ := db.DB()
		defer sqlDB.Close()
		if err := model.InitDB(db, nil); err != nil {
			t.Fatal(err)
		}
		baseURL := mockSF.URL
		sfCh := &model.Channel{
			Type:    constant.ChannelTypeSiliconFlow,
			Name:    "sf-balance",
			Status:  common.ChannelStatusEnabled,
			Key:     "sk-sf-test",
			BaseURL: &baseURL,
			Models:  "BAAI/bge-m3",
		}
		if err := sfCh.Insert(); err != nil {
			t.Fatal(err)
		}
		bal, err := channelsvc.UpdateChannelBalance(sfCh.Id)
		if err != nil || bal != 88.65 {
			t.Fatalf("expected SiliconFlow balance 88.65, got %v (err=%v)", bal, err)
		}
	})
}
