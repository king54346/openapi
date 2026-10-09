package controller

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"openapi/common"
	"openapi/constant"
	"openapi/middleware"
	"openapi/model"
	relaycommon "openapi/relay/common"
	"openapi/relay/helper"
	"openapi/types"

	gin "github.com/king54346/gin-tiny"
)

// 渠道测试：用最小请求走一遍真实的转发流程（模型映射、参数覆盖、渠道适配器都会生效），
// 以此判断渠道是否可用。参照 new-api controller/channel-test.go。

// channelTestTimeout 单个渠道的测试超时
const channelTestTimeout = 60 * time.Second

// channelTestResult 一次渠道测试的结果
type channelTestResult struct {
	Model      string
	Path       string
	Latency    time.Duration
	Err        *types.StarAPIError // 为 nil 且未跳过表示测试通过
	Skipped    bool                // 不支持测试（如视频、图片模型），不计入成功或失败
	SkipReason string
	UsingKey   string // 本次使用的 key（多 key 渠道自动禁用时只禁用该 key）
}

func (r channelTestResult) ok() bool { return !r.Skipped && r.Err == nil }

// 这些模型按名称识别为图片 / 视频生成，测试成本高，不做自动测试
var expensiveModelMarkers = []string{"seedream", "seedance", "dall-e", "gpt-image", "sora", "wan2", "kling", "vidu", "hailuo", "image"}

// resolveTestModel 测试模型：优先使用指定模型，其次渠道的 test_model，最后取渠道第一个模型。
func resolveTestModel(channel *model.Channel, testModel string) string {
	if testModel = strings.TrimSpace(testModel); testModel != "" {
		return testModel
	}
	if channel.TestModel != nil && strings.TrimSpace(*channel.TestModel) != "" {
		return strings.TrimSpace(*channel.TestModel)
	}
	for _, m := range channel.GetModels() {
		if m = strings.TrimSpace(m); m != "" {
			return m
		}
	}
	return ""
}

// testRequestFor 按模型名决定测试接口与请求体；返回空 path 表示不支持测试。
func testRequestFor(modelName string) (path string, format types.RelayFormat, body map[string]any, skipReason string) {
	lower := strings.ToLower(modelName)
	for _, marker := range expensiveModelMarkers {
		if strings.Contains(lower, marker) {
			return "", "", nil, "image/video model is not tested automatically: " + modelName
		}
	}
	switch {
	case strings.Contains(lower, "rerank"):
		return "/v1/rerank", types.RelayFormatRerank, map[string]any{"model": modelName, "query": "hi", "documents": []string{"hi"}}, ""
	case strings.Contains(lower, "embed") || strings.Contains(lower, "bge-") || strings.HasPrefix(lower, "m3e"):
		return "/v1/embeddings", types.RelayFormatEmbedding, map[string]any{"model": modelName, "input": "hi"}, ""
	}
	return "/v1/chat/completions", types.RelayFormatOpenAI, map[string]any{
		"model":      modelName,
		"max_tokens": 16,
		"messages":   []map[string]string{{"role": "user", "content": "hi"}},
	}, ""
}

// testChannel 测试单个渠道，超过 channelTestTimeout 视为失败。
func testChannel(channel *model.Channel, testModel string) channelTestResult {
	done := make(chan channelTestResult, 1)
	go func() { done <- runChannelTest(channel, testModel) }()
	select {
	case res := <-done:
		return res
	case <-time.After(channelTestTimeout):
		return channelTestResult{
			Model:   resolveTestModel(channel, testModel),
			Latency: channelTestTimeout,
			Err: types.NewErrorWithStatusCode(fmt.Errorf("channel test timed out after %s", channelTestTimeout),
				types.ErrorCodeDoRequestFailed, http.StatusGatewayTimeout),
		}
	}
}

func runChannelTest(channel *model.Channel, testModel string) (res channelTestResult) {
	res.Model = resolveTestModel(channel, testModel)
	if !constant.IsRelaySupportedChannelType(channel.Type) {
		res.Skipped, res.SkipReason = true, fmt.Sprintf("channel type %d has no relay implementation", channel.Type)
		return res
	}
	if _, ok := constant.ChannelType2APIType(channel.Type); !ok {
		res.Skipped, res.SkipReason = true, "video-only channel is not tested automatically"
		return res
	}
	if res.Model == "" {
		res.Err = types.NewErrorWithStatusCode(errors.New("channel has no model to test"), types.ErrorCodeInvalidRequest,
			http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		return res
	}
	path, format, body, skipReason := testRequestFor(res.Model)
	if path == "" {
		res.Skipped, res.SkipReason = true, skipReason
		return res
	}
	res.Path = path

	// 构造一个内部请求：用户 id 为 0，日志中令牌名记为 channel-test
	payload, _ := common.Marshal(body)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(payload)))
	req.Header.Set("Content-Type", "application/json")
	c.SetRequest(req)
	// 内部请求不经过 BodyStorageCleanup 中间件，需自行释放请求体存储
	defer common.CleanupBodyStorage(c)
	c.Set("token_name", "channel-test")
	c.Set("username", "system")

	start := time.Now()
	defer func() { res.Latency = time.Since(start) }()

	if apiErr := middleware.SetupContextForSelectedChannel(c, channel, res.Model); apiErr != nil {
		res.Err = apiErr
		return res
	}
	res.UsingKey = common.GetContextKeyString(c, constant.ContextKeyChannelKey)
	common.SetContextKey(c, constant.ContextKeyRequestStartTime, start)

	request, err := helper.GetAndValidateRequest(c, format)
	if err != nil {
		res.Err = requestBodyError(err)
		return res
	}
	info, err := relaycommon.GenRelayInfo(c, format, request, nil)
	if err != nil {
		res.Err = types.NewError(err, types.ErrorCodeGenRelayInfoFailed, types.ErrOptionWithSkipRetry())
		return res
	}
	info.IsChannelTest = true
	res.Err = relayOnce(c, info, format)
	return res
}

// shouldDisableAfterTest 测试失败是否应禁用渠道：只看渠道侧问题，429 限流属暂时性问题不禁用。
func shouldDisableAfterTest(apiErr *types.StarAPIError) bool {
	if apiErr == nil || types.IsSkipRetryError(apiErr) || apiErr.StatusCode == http.StatusTooManyRequests {
		return false
	}
	code := apiErr.StatusCode
	return types.IsChannelError(apiErr) || code == http.StatusUnauthorized || code == http.StatusForbidden ||
		code >= http.StatusInternalServerError
}

// TestChannel 手动测试单个渠道：GET /api/channel/test/:id?model=xxx。
// 只测试并更新响应时间，不自动启停渠道。
func TestChannel(c gin.Context) {
	id, ok := paramID(c)
	if !ok {
		return
	}
	channel, err := model.GetChannelById(id, true)
	if err != nil {
		apiErrorMsg(c, "channel not found")
		return
	}
	res := testChannel(channel, c.Query("model"))
	data := gin.H{"model": res.Model, "path": res.Path, "time": res.Latency.Seconds()}
	switch {
	case res.Skipped:
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "skipped: " + res.SkipReason, "data": data})
	case res.Err != nil:
		channel.UpdateResponseTime(res.Latency.Milliseconds())
		c.JSON(http.StatusOK, gin.H{"success": false, "message": res.Err.MaskSensitiveErrorWithStatusCode(), "data": data})
	default:
		channel.UpdateResponseTime(res.Latency.Milliseconds())
		apiSuccess(c, data)
	}
}
