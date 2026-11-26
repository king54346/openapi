package relay

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"openapi/common"
	"openapi/constant"
	"openapi/dto"
	"openapi/logger"
	"openapi/model"
	corerelay "openapi/relay"
	relaycommon "openapi/relay/common"
	relayconstant "openapi/relay/constant"
	"openapi/relay/helper"
	channelsvc "openapi/service/channel"
	"openapi/types"

	"github.com/gorilla/websocket"
	gin "github.com/king54346/gin-tiny"
)

func RequestBodyError(err error) *types.StarAPIError {
	if common.IsRequestBodyTooLargeError(err) {
		return types.NewErrorWithStatusCode(err, types.ErrorCodeReadRequestBodyFailed, http.StatusRequestEntityTooLarge, types.ErrOptionWithSkipRetry())
	}
	return types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
}

func SelectRetryChannel(c gin.Context, modelName string, retry int) (*model.Channel, *types.StarAPIError) {
	ch, err := CacheGetRandomSatisfiedChannel(&RetryParam{
		Ctx:       c,
		ModelName: modelName,
		Retry:     &retry,
	})
	if err != nil {
		return nil, types.NewError(fmt.Errorf("failed to get channel for model %s (retry): %w", modelName, err),
			types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
	}
	if ch == nil {
		return nil, types.NewError(fmt.Errorf("no available channel for model %s (retry)", modelName),
			types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
	}
	if setupErr := channelsvc.SetupContextForSelectedChannel(c, ch, modelName); setupErr != nil {
		return nil, setupErr
	}
	logger.LogInfo(c, fmt.Sprintf("retrying with channel #%d", ch.Id))
	return ch, nil
}

func AddUsedChannel(c gin.Context, channelID int) {
	c.Set("use_channel", append(c.GetStringSlice("use_channel"), strconv.Itoa(channelID)))
}

func LogRetryPath(c gin.Context) {
	if used := c.GetStringSlice("use_channel"); len(used) > 1 {
		logger.LogInfo(c, "retry path: "+strings.Join(used, "->"))
	}
}

func ProcessChannelError(c gin.Context, info *relaycommon.RelayInfo, apiErr *types.StarAPIError, attempt int) {
	channelID := common.GetContextKeyInt(c, constant.ContextKeyChannelId)
	logger.LogError(c, fmt.Sprintf("relay failed (channel #%d, attempt %d, status %d): %s",
		channelID, attempt+1, apiErr.StatusCode, apiErr.MaskSensitiveError()))
	RecordRelayErrorLog(c, info, apiErr, channelID, attempt)
	channelsvc.RecordChannelFailure(channelID,
		common.GetContextKeyString(c, constant.ContextKeyChannelKey),
		common.GetContextKeyBool(c, constant.ContextKeyChannelAutoBan),
		apiErr)
}

func RecordRelayErrorLog(c gin.Context, info *relaycommon.RelayInfo, apiErr *types.StarAPIError, channelID, attempt int) {
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
		info.IsStream, other)
}

func RelayOnce(c gin.Context, info *relaycommon.RelayInfo, relayFormat types.RelayFormat) *types.StarAPIError {
	if relayFormat == types.RelayFormatOpenAIRealtime {
		return corerelay.WssHelper(c, info)
	}
	if relayFormat == types.RelayFormatClaude {
		return corerelay.ClaudeHelper(c, info)
	}
	if relayFormat == types.RelayFormatGemini {
		return corerelay.GeminiHelper(c, info)
	}
	switch info.RelayMode {
	case relayconstant.RelayModeImagesGenerations, relayconstant.RelayModeImagesEdits, relayconstant.RelayModeEdits:
		return corerelay.ImageHelper(c, info)
	case relayconstant.RelayModeAudioSpeech, relayconstant.RelayModeAudioTranscription, relayconstant.RelayModeAudioTranslation:
		return corerelay.AudioHelper(c, info)
	case relayconstant.RelayModeEmbeddings:
		return corerelay.EmbeddingHelper(c, info)
	case relayconstant.RelayModeRerank:
		return corerelay.RerankHelper(c, info)
	case relayconstant.RelayModeResponses, relayconstant.RelayModeResponsesCompact:
		return corerelay.ResponsesHelper(c, info)
	}
	return corerelay.TextHelper(c, info)
}

func ShouldRetry(c gin.Context, apiErr *types.StarAPIError, remaining int) bool {
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
	return RetryableStatus(apiErr.StatusCode)
}

func RetryableStatus(code int) bool {
	switch {
	case code == http.StatusTooManyRequests, code == http.StatusTemporaryRedirect:
		return true
	case code/100 == 5:
		return code != http.StatusGatewayTimeout && code != 524
	case code == http.StatusBadRequest, code == http.StatusRequestTimeout, code/100 == 2:
		return false
	}
	return true
}

func WriteRelayError(c gin.Context, ws *websocket.Conn, apiErr *types.StarAPIError) {
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

func ShouldRetryTaskRelay(c gin.Context, taskErr *dto.TaskError, remaining int) bool {
	if taskErr == nil || remaining <= 0 || taskErr.LocalError {
		return false
	}
	if c.Response().Written() {
		return false
	}
	if _, ok := c.Get(string(constant.ContextKeyTokenSpecificChannelId)); ok {
		return false
	}
	return RetryableStatus(taskErr.StatusCode)
}
