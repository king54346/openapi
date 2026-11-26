package relay

import (
	"net/http"

	"openapi/common"
	"openapi/constant"
	"openapi/logger"
	relaycommon "openapi/relay/common"
	"openapi/relay/helper"
	channelsvc "openapi/service/channel"
	relaysvc "openapi/service/relay"
	tokensvc "openapi/service/token"
	"openapi/types"

	"github.com/gorilla/websocket"
	gin "github.com/king54346/gin-tiny"
)

var wsUpgrader = websocket.Upgrader{
	Subprotocols: []string{"realtime"},
	CheckOrigin:  func(r *http.Request) bool { return true },
}

func Relay(c gin.Context, relayFormat types.RelayFormat) {
	var ws *websocket.Conn
	if relayFormat == types.RelayFormatOpenAIRealtime {
		conn, err := wsUpgrader.Upgrade(c.Response(), c.Request(), nil)
		if err != nil {
			logger.LogError(c, "websocket upgrade failed: "+err.Error())
			return
		}
		ws = conn
		defer ws.Close()
	}

	var apiErr *types.StarAPIError
	defer func() {
		if apiErr != nil {
			relaysvc.WriteRelayError(c, ws, apiErr)
		}
	}()

	request, err := helper.GetAndValidateRequest(c, relayFormat)
	if err != nil {
		apiErr = relaysvc.RequestBodyError(err)
		return
	}
	info, err := relaycommon.GenRelayInfo(c, relayFormat, request, ws)
	if err != nil {
		apiErr = types.NewErrorWithStatusCode(err, types.ErrorCodeGenRelayInfoFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		return
	}
	if promptTokens, err := tokensvc.EstimateRequestToken(c, request.GetTokenCountMeta(), info); err != nil {
		logger.LogWarn(c, "estimate prompt tokens failed: "+err.Error())
	} else {
		info.SetEstimatePromptTokens(promptTokens)
	}
	defer relaysvc.LogRetryPath(c)

	for attempt := 0; ; attempt++ {
		channelID := common.GetContextKeyInt(c, constant.ContextKeyChannelId)
		relaysvc.AddUsedChannel(c, channelID)
		apiErr = relaysvc.RelayOnce(c, info, relayFormat)
		if apiErr == nil {
			channelsvc.RecordChannelSuccess(channelID)
			return
		}
		relaysvc.ProcessChannelError(c, info, apiErr, attempt)

		if !relaysvc.ShouldRetry(c, apiErr, common.RetryTimes-attempt) {
			return
		}
		if _, selectErr := relaysvc.SelectRetryChannel(c, info.OriginModelName, attempt+1); selectErr != nil {
			logger.LogError(c, selectErr.Error())
			return
		}
	}
}
