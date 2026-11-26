package channel

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"openapi/common"
	"openapi/constant"
	"openapi/model"
	"openapi/relay"
	relaycommon "openapi/relay/common"
	relayconstant "openapi/relay/constant"
	"openapi/relay/helper"
	"openapi/types"

	gin "github.com/king54346/gin-tiny"
)

const (
	channelTestTimeout       = 60 * time.Second
	channelTestConcurrency   = 4
	channelTestTypeAllModels = "all_models"
)

var channelTestAllRunning atomic.Bool

type ChannelTestResult struct {
	Model      string
	Path       string
	Latency    time.Duration
	Err        *types.StarAPIError
	Skipped    bool
	SkipReason string
	UsingKey   string
}

func (r ChannelTestResult) OK() bool { return !r.Skipped && r.Err == nil }

var expensiveModelMarkers = []string{"seedream", "seedance", "dall-e", "gpt-image", "sora", "wan2", "kling", "vidu", "hailuo", "image"}

func resolveTestModel(ch *model.Channel, testModel string) string {
	if testModel = strings.TrimSpace(testModel); testModel != "" {
		return testModel
	}
	if ch.TestModel != nil && strings.TrimSpace(*ch.TestModel) != "" {
		return strings.TrimSpace(*ch.TestModel)
	}
	for _, m := range ch.GetModels() {
		if m = strings.TrimSpace(m); m != "" {
			return m
		}
	}
	return ""
}

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

func TestChannelRaw(ch *model.Channel, testModel string) ChannelTestResult {
	done := make(chan ChannelTestResult, 1)
	go func() { done <- runChannelTest(ch, testModel) }()
	select {
	case res := <-done:
		return res
	case <-time.After(channelTestTimeout):
		return ChannelTestResult{
			Model:   resolveTestModel(ch, testModel),
			Latency: channelTestTimeout,
			Err: types.NewErrorWithStatusCode(fmt.Errorf("channel test timed out after %s", channelTestTimeout),
				types.ErrorCodeDoRequestFailed, http.StatusGatewayTimeout),
		}
	}
}

func executeRelayOnce(c gin.Context, info *relaycommon.RelayInfo, format types.RelayFormat) *types.StarAPIError {
	if format == types.RelayFormatOpenAIRealtime {
		return relay.WssHelper(c, info)
	}
	if format == types.RelayFormatClaude {
		return relay.ClaudeHelper(c, info)
	}
	if format == types.RelayFormatGemini {
		return relay.GeminiHelper(c, info)
	}
	switch info.RelayMode {
	case relayconstant.RelayModeImagesGenerations, relayconstant.RelayModeImagesEdits, relayconstant.RelayModeEdits:
		return relay.ImageHelper(c, info)
	case relayconstant.RelayModeAudioSpeech, relayconstant.RelayModeAudioTranscription, relayconstant.RelayModeAudioTranslation:
		return relay.AudioHelper(c, info)
	case relayconstant.RelayModeEmbeddings:
		return relay.EmbeddingHelper(c, info)
	case relayconstant.RelayModeRerank:
		return relay.RerankHelper(c, info)
	case relayconstant.RelayModeResponses, relayconstant.RelayModeResponsesCompact:
		return relay.ResponsesHelper(c, info)
	}
	return relay.TextHelper(c, info)
}

func runChannelTest(ch *model.Channel, testModel string) (res ChannelTestResult) {
	res.Model = resolveTestModel(ch, testModel)
	if !constant.IsRelaySupportedChannelType(ch.Type) {
		res.Skipped, res.SkipReason = true, fmt.Sprintf("channel type %d has no relay implementation", ch.Type)
		return res
	}
	if _, ok := constant.ChannelType2APIType(ch.Type); !ok {
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

	payload, _ := common.Marshal(body)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(payload)))
	req.Header.Set("Content-Type", "application/json")
	c.SetRequest(req)
	defer common.CleanupBodyStorage(c)
	c.Set("token_name", "channel-test")
	c.Set("username", "system")

	start := time.Now()
	defer func() { res.Latency = time.Since(start) }()

	if apiErr := SetupContextForSelectedChannel(c, ch, res.Model); apiErr != nil {
		res.Err = apiErr
		return res
	}
	res.UsingKey = common.GetContextKeyString(c, constant.ContextKeyChannelKey)
	common.SetContextKey(c, constant.ContextKeyRequestStartTime, start)

	request, err := helper.GetAndValidateRequest(c, format)
	if err != nil {
		if common.IsRequestBodyTooLargeError(err) {
			res.Err = types.NewErrorWithStatusCode(err, types.ErrorCodeReadRequestBodyFailed, http.StatusRequestEntityTooLarge, types.ErrOptionWithSkipRetry())
		} else {
			res.Err = types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		}
		return res
	}
	info, err := relaycommon.GenRelayInfo(c, format, request, nil)
	if err != nil {
		res.Err = types.NewError(err, types.ErrorCodeGenRelayInfoFailed, types.ErrOptionWithSkipRetry())
		return res
	}
	info.IsChannelTest = true
	res.Err = executeRelayOnce(c, info, format)
	return res
}

func ShouldDisableAfterTest(apiErr *types.StarAPIError) bool {
	if apiErr == nil || types.IsSkipRetryError(apiErr) || apiErr.StatusCode == http.StatusTooManyRequests {
		return false
	}
	code := apiErr.StatusCode
	return types.IsChannelError(apiErr) || code == http.StatusUnauthorized || code == http.StatusForbidden ||
		code >= http.StatusInternalServerError
}

func TestChannelByID(id int, modelParam string) (*model.Channel, ChannelTestResult, error) {
	ch, err := model.GetChannelById(id, true)
	if err != nil {
		return nil, ChannelTestResult{}, ErrChannelNotFound
	}
	res := TestChannelRaw(ch, modelParam)
	if !res.Skipped {
		ch.UpdateResponseTime(res.Latency.Milliseconds())
	}
	return ch, res, nil
}

type TestRecordResult struct {
	Model     string `json:"model"`
	Success   bool   `json:"success"`
	Skipped   bool   `json:"skipped"`
	LatencyMS int    `json:"latency_ms"`
	Error     string `json:"error,omitempty"`
}

func RecordChannelTestResult(ch *model.Channel, modelName, testType string, res ChannelTestResult) error {
	record := &model.ChannelTestRecord{
		CreatedAt:    common.GetTimestamp(),
		ChannelId:    ch.Id,
		ChannelName:  ch.Name,
		ChannelTag:   ch.GetTag(),
		ModelName:    modelName,
		Status:       model.ChannelTestStatusSuccess,
		ResponseTime: int(res.Latency.Milliseconds()),
		TestType:     testType,
	}
	switch {
	case res.Skipped:
		record.Status = model.ChannelTestStatusFail
		record.ErrorMessage = "skipped: " + res.SkipReason
	case res.Err != nil:
		record.Status = model.ChannelTestStatusFail
		record.ErrorMessage = res.Err.MaskSensitiveErrorWithStatusCode()
	}
	return record.Insert()
}

func TestChannelAllModels(id int) (map[string]any, error) {
	ch, err := model.GetChannelById(id, true)
	if err != nil {
		return nil, ErrChannelNotFound
	}
	models := ch.GetModels()
	items := make([]TestRecordResult, 0, len(models))
	passed, failed, skipped := 0, 0, 0
	for _, modelName := range models {
		modelName = strings.TrimSpace(modelName)
		if modelName == "" {
			continue
		}
		res := TestChannelRaw(ch, modelName)
		item := TestRecordResult{
			Model:     modelName,
			Success:   res.OK(),
			LatencyMS: int(res.Latency.Milliseconds()),
		}
		switch {
		case res.Skipped:
			item.Skipped = true
			item.Error = res.SkipReason
			skipped++
		case res.Err != nil:
			item.Error = res.Err.MaskSensitiveErrorWithStatusCode()
			failed++
		default:
			passed++
		}
		items = append(items, item)
		if err := RecordChannelTestResult(ch, modelName, channelTestTypeAllModels, res); err != nil {
			common.SysError("failed to record channel test result: " + err.Error())
		}
	}
	return map[string]any{
		"items":   items,
		"passed":  passed,
		"failed":  failed,
		"skipped": skipped,
	}, nil
}

func enabledChannels() ([]*model.Channel, error) {
	var channels []*model.Channel
	err := model.DB.Where("status = ?", common.ChannelStatusEnabled).Order("id").Find(&channels).Error
	return channels, err
}

func TestAllChannels() (int, error) {
	if !channelTestAllRunning.CompareAndSwap(false, true) {
		return 0, validationErr("channel test is already running")
	}
	channels, err := enabledChannels()
	if err != nil {
		channelTestAllRunning.Store(false)
		return 0, err
	}
	go func() {
		defer channelTestAllRunning.Store(false)
		sem := make(chan struct{}, channelTestConcurrency)
		var wg sync.WaitGroup
		for _, ch := range channels {
			wg.Add(1)
			sem <- struct{}{}
			go func(ch *model.Channel) {
				defer func() {
					<-sem
					wg.Done()
				}()
				res := TestChannelRaw(ch, "")
				if err := RecordChannelTestResult(ch, res.Model, "all_channels", res); err != nil {
					common.SysError("failed to record channel test result: " + err.Error())
				}
			}(ch)
		}
		wg.Wait()
		common.SysLog(fmt.Sprintf("all-channel test finished: %d channels", len(channels)))
	}()
	return len(channels), nil
}
