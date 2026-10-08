package controller

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"openapi/common"
	"openapi/constant"
	"openapi/dto"
	"openapi/logger"
	"openapi/middleware"
	"openapi/model"
	"openapi/relay"
	relaycommon "openapi/relay/common"
	relayconstant "openapi/relay/constant"
	"openapi/relay/helper"
	"openapi/service"
	"openapi/types"

	"github.com/gorilla/websocket"
	gin "github.com/king54346/gin-tiny"
)

var wsUpgrader = websocket.Upgrader{
	Subprotocols: []string{"realtime"},
	CheckOrigin:  func(r *http.Request) bool { return true },
}

// Relay 转发入口：解析并校验请求，生成 RelayInfo，按格式交给 relay 包处理；
// 失败且可重试时按优先级降档换渠道重试，最多 common.RetryTimes 次。
// 渠道已由 Distribute 中间件选好并写入上下文。
func Relay(c gin.Context, relayFormat types.RelayFormat) {
	var ws *websocket.Conn
	if relayFormat == types.RelayFormatOpenAIRealtime {
		conn, err := wsUpgrader.Upgrade(c.Response(), c.Request(), nil)
		if err != nil {
			// Upgrade 失败时已向客户端写回错误
			logger.LogError(c, "websocket upgrade failed: "+err.Error())
			return
		}
		ws = conn
		defer ws.Close()
	}

	var apiErr *types.StarAPIError
	defer func() {
		if apiErr != nil {
			writeRelayError(c, ws, apiErr)
		}
	}()

	if relayFormat == types.RelayFormatClaude || relayFormat == types.RelayFormatGemini {
		apiErr = types.NewErrorWithStatusCode(fmt.Errorf("request format %s is not supported", relayFormat),
			types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		return
	}

	request, err := helper.GetAndValidateRequest(c, relayFormat)
	if err != nil {
		apiErr = requestBodyError(err)
		return
	}
	info, err := relaycommon.GenRelayInfo(c, relayFormat, request, ws)
	if err != nil {
		apiErr = types.NewErrorWithStatusCode(err, types.ErrorCodeGenRelayInfoFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		return
	}
	defer logRetryPath(c)

	for attempt := 0; ; attempt++ {
		channelID := common.GetContextKeyInt(c, constant.ContextKeyChannelId)
		addUsedChannel(c, channelID)
		apiErr = relayOnce(c, info, relayFormat)
		if apiErr == nil {
			service.RecordChannelSuccess(channelID)
			return
		}
		processChannelError(c, info, apiErr, attempt)

		if !shouldRetry(c, apiErr, common.RetryTimes-attempt) {
			return
		}
		// 下一档优先级的渠道（attempt+1 档，超出后停在最低档）
		if _, selectErr := selectRetryChannel(c, info.OriginModelName, attempt+1); selectErr != nil {
			logger.LogError(c, selectErr.Error())
			return
		}
	}
}

// requestBodyError 请求解析失败：请求体过大返回 413，其余返回 400。
func requestBodyError(err error) *types.StarAPIError {
	if common.IsRequestBodyTooLargeError(err) {
		return types.NewErrorWithStatusCode(err, types.ErrorCodeReadRequestBodyFailed, http.StatusRequestEntityTooLarge, types.ErrOptionWithSkipRetry())
	}
	return types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
}

// selectRetryChannel 按 retry 档位重新选渠道并写入上下文；没有可用渠道时返回错误。
func selectRetryChannel(c gin.Context, modelName string, retry int) (*model.Channel, *types.StarAPIError) {
	group := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	channel, selectGroup, err := service.CacheGetRandomSatisfiedChannel(&service.RetryParam{
		Ctx:        c,
		ModelName:  modelName,
		TokenGroup: group,
		Retry:      &retry,
	})
	if err != nil {
		return nil, types.NewError(fmt.Errorf("failed to get channel for group %s model %s (retry): %w", selectGroup, modelName, err),
			types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
	}
	if channel == nil {
		return nil, types.NewError(fmt.Errorf("no available channel for group %s model %s (retry)", selectGroup, modelName),
			types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
	}
	if setupErr := middleware.SetupContextForSelectedChannel(c, channel, modelName); setupErr != nil {
		return nil, setupErr
	}
	logger.LogInfo(c, fmt.Sprintf("retrying with channel #%d", channel.Id))
	return channel, nil
}

// addUsedChannel 记录本次请求依次用过的渠道，用于重试路径日志与错误日志。
func addUsedChannel(c gin.Context, channelID int) {
	c.Set("use_channel", append(c.GetStringSlice("use_channel"), strconv.Itoa(channelID)))
}

// logRetryPath 发生过重试时输出渠道路径，如 "retry path: 3->5->9"。
func logRetryPath(c gin.Context) {
	if used := c.GetStringSlice("use_channel"); len(used) > 1 {
		logger.LogInfo(c, "retry path: "+strings.Join(used, "->"))
	}
}

// processChannelError 失败后的统一处理：输出日志、写错误日志、累计渠道失败（必要时自动禁用）。
func processChannelError(c gin.Context, info *relaycommon.RelayInfo, apiErr *types.StarAPIError, attempt int) {
	channelID := common.GetContextKeyInt(c, constant.ContextKeyChannelId)
	logger.LogError(c, fmt.Sprintf("relay failed (channel #%d, attempt %d, status %d): %s",
		channelID, attempt+1, apiErr.StatusCode, apiErr.MaskSensitiveError()))
	recordRelayErrorLog(c, info, apiErr, channelID, attempt)
	service.RecordChannelFailure(channelID,
		common.GetContextKeyString(c, constant.ContextKeyChannelKey),
		common.GetContextKeyBool(c, constant.ContextKeyChannelAutoBan),
		apiErr)
}

// recordRelayErrorLog 把一次失败的转发写入 logs 表（type=5），每个失败的渠道各记一条。
func recordRelayErrorLog(c gin.Context, info *relaycommon.RelayInfo, apiErr *types.StarAPIError, channelID, attempt int) {
	if !common.ErrorLogEnabled || !types.IsRecordErrorLog(apiErr) || model.LOG_DB == nil {
		return
	}
	adminInfo := map[string]any{"use_channel": c.GetStringSlice("use_channel")}
	if common.GetContextKeyBool(c, constant.ContextKeyChannelIsMultiKey) {
		adminInfo["is_multi_key"] = true
		adminInfo["multi_key_index"] = common.GetContextKeyInt(c, constant.ContextKeyChannelMultiKeyIndex)
	}
	other := map[string]any{
		"request_path": c.Request().URL.Path,
		"status_code":  apiErr.StatusCode,
		"error_code":   string(apiErr.GetErrorCode()),
		"error_type":   string(apiErr.GetErrorType()),
		"channel_id":   channelID,
		"channel_name": common.GetContextKeyString(c, constant.ContextKeyChannelName),
		"channel_type": common.GetContextKeyInt(c, constant.ContextKeyChannelType),
		"attempt":      attempt + 1,
		"admin_info":   adminInfo,
	}
	model.RecordErrorLog(c, info.UserId, channelID, info.OriginModelName, c.GetString("token_name"),
		apiErr.MaskSensitiveErrorWithStatusCode(), info.TokenId, int(time.Since(info.StartTime).Seconds()),
		info.IsStream, info.UsingGroup, other)
}

// relayOnce 用当前上下文中的渠道转发一次。
func relayOnce(c gin.Context, info *relaycommon.RelayInfo, relayFormat types.RelayFormat) *types.StarAPIError {
	if relayFormat == types.RelayFormatOpenAIRealtime {
		return relay.WssHelper(c, info)
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

// shouldRetry 判断失败后是否换渠道重试：
// 已开始向客户端输出、请求本身有误、指定了渠道或重试次数用尽时不重试。
func shouldRetry(c gin.Context, apiErr *types.StarAPIError, remaining int) bool {
	if apiErr == nil || remaining <= 0 {
		return false
	}
	if c.Response().Written() {
		return false
	}
	if _, ok := c.Get(string(constant.ContextKeyTokenSpecificChannelId)); ok {
		return false
	}
	if types.IsSkipRetryError(apiErr) {
		return false
	}
	if types.IsChannelError(apiErr) {
		return true
	}
	return retryableStatus(apiErr.StatusCode)
}

// retryableStatus 按上游状态码判断是否值得换渠道重试。
func retryableStatus(code int) bool {
	switch {
	case code == http.StatusTooManyRequests, code == http.StatusTemporaryRedirect:
		return true
	case code/100 == 5:
		// 504/524 多为上游处理超时，换渠道大概率同样超时
		return code != http.StatusGatewayTimeout && code != 524
	case code == http.StatusBadRequest, code == http.StatusRequestTimeout, code/100 == 2:
		return false
	}
	return true
}

// writeRelayError 把错误写回客户端；已开始输出（如流式响应中途出错）时不再写入。
func writeRelayError(c gin.Context, ws *websocket.Conn, apiErr *types.StarAPIError) {
	openaiErr := apiErr.ToOpenAIError()
	if requestID := c.GetString(common.RequestIdKey); requestID != "" {
		openaiErr.Message = fmt.Sprintf("%s (request id: %s)", openaiErr.Message, requestID)
	}
	if ws != nil {
		helper.WssError(c, ws, openaiErr)
		return
	}
	if c.Response().Written() {
		return
	}
	status := apiErr.StatusCode
	if status < http.StatusBadRequest {
		status = http.StatusInternalServerError
	}
	c.JSON(status, gin.H{"error": openaiErr})
}

// RelayTask 视频任务：GET 查询任务（不选渠道、不重试），其余方法提交任务。
// 提交失败且可重试时按优先级降档换渠道重试，最多 common.RetryTimes 次。
func RelayTask(c gin.Context) {
	var taskErr *dto.TaskError
	defer func() {
		if taskErr == nil || c.Response().Written() {
			return
		}
		if taskErr.StatusCode < http.StatusBadRequest {
			taskErr.StatusCode = http.StatusInternalServerError
		}
		if taskErr.StatusCode == http.StatusTooManyRequests {
			taskErr.Message = "upstream is overloaded, please try again later"
		}
		c.JSON(taskErr.StatusCode, taskErr)
	}()

	if c.Request().Method == http.MethodGet {
		taskErr = relay.RelayTaskFetch(c, relayconstant.RelayModeVideoFetchByID)
		return
	}

	info, err := relaycommon.GenRelayInfo(c, types.RelayFormatTask, nil, nil)
	if err != nil {
		taskErr = service.TaskErrorWrapperLocal(err, "gen_relay_info_failed", http.StatusInternalServerError)
		return
	}
	defer logRetryPath(c)

	for attempt := 0; ; attempt++ {
		channelID := common.GetContextKeyInt(c, constant.ContextKeyChannelId)
		addUsedChannel(c, channelID)
		taskErr = relay.RelayTaskSubmit(c, info)
		if taskErr == nil {
			service.RecordChannelSuccess(channelID)
			return
		}
		logger.LogError(c, fmt.Sprintf("task relay failed (channel #%d, attempt %d, status %d): %s",
			channelID, attempt+1, taskErr.StatusCode, taskErr.Message))

		if !shouldRetryTaskRelay(c, taskErr, common.RetryTimes-attempt) {
			return
		}
		if _, selectErr := selectRetryChannel(c, info.OriginModelName, attempt+1); selectErr != nil {
			logger.LogError(c, selectErr.Error())
			return
		}
	}
}

// shouldRetryTaskRelay 任务提交失败后是否换渠道重试，规则与普通转发一致，另外本地错误不重试。
func shouldRetryTaskRelay(c gin.Context, taskErr *dto.TaskError, remaining int) bool {
	if taskErr == nil || remaining <= 0 || taskErr.LocalError {
		return false
	}
	if c.Response().Written() {
		return false
	}
	if _, ok := c.Get(string(constant.ContextKeyTokenSpecificChannelId)); ok {
		return false
	}
	return retryableStatus(taskErr.StatusCode)
}

// openAIModel OpenAI /v1/models 返回的单个模型。
type openAIModel struct {
	Id      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

// usableModels 返回当前令牌可调用的模型：分组下有可用渠道，并满足令牌的模型黑白名单。
func usableModels(c gin.Context) []string {
	models := model.GetGroupModels(common.GetContextKeyString(c, constant.ContextKeyUsingGroup))
	if !common.GetContextKeyBool(c, constant.ContextKeyTokenModelLimitEnabled) {
		return models
	}
	limits, _ := c.Get(string(constant.ContextKeyTokenModelLimit))
	limitMap, _ := limits.(map[string]bool)
	limitType, _ := c.Get("token_model_limit_type")
	blacklist := limitType == 1

	out := models[:0]
	for _, m := range models {
		if limitMap[m] != blacklist {
			out = append(out, m)
		}
	}
	return out
}

func toOpenAIModel(name string) openAIModel {
	return openAIModel{Id: name, Object: "model", Created: common.StartTime, OwnedBy: "openapi"}
}

// ListModels 列出令牌可用的模型（统一返回 OpenAI 格式）。
func ListModels(c gin.Context, channelType int) {
	models := usableModels(c)
	data := make([]openAIModel, 0, len(models))
	for _, m := range models {
		data = append(data, toOpenAIModel(m))
	}
	c.JSON(http.StatusOK, gin.H{"object": "list", "data": data})
}

// RetrieveModel 查询单个模型是否可用（统一返回 OpenAI 格式）。
func RetrieveModel(c gin.Context, channelType int) {
	name := strings.TrimPrefix(c.Param("model"), "/")
	for _, m := range usableModels(c) {
		if m == name {
			c.JSON(http.StatusOK, toOpenAIModel(m))
			return
		}
	}
	err := types.NewErrorWithStatusCode(errors.New("model "+name+" does not exist"), types.ErrorCodeModelNotFound, http.StatusNotFound)
	c.JSON(http.StatusNotFound, gin.H{"error": err.ToOpenAIError()})
}
