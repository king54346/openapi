package relay

import (
	"fmt"
	"net/http"

	"openapi/common"
	"openapi/constant"
	"openapi/dto"
	"openapi/logger"
	corerelay "openapi/relay"
	relaycommon "openapi/relay/common"
	relayconstant "openapi/relay/constant"
	relayerr "openapi/relay/relayerr"
	channelsvc "openapi/service/channel"
	relaysvc "openapi/service/relay"
	"openapi/types"

	gin "github.com/king54346/gin-tiny"
)

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
		taskErr = corerelay.RelayTaskFetch(c, relayconstant.RelayModeVideoFetchByID)
		return
	}

	info, err := relaycommon.GenRelayInfo(c, types.RelayFormatTask, nil, nil)
	if err != nil {
		taskErr = relayerr.TaskErrorWrapperLocal(err, "gen_relay_info_failed", http.StatusInternalServerError)
		return
	}
	defer relaysvc.LogRetryPath(c)

	for attempt := 0; ; attempt++ {
		channelID := common.GetContextKeyInt(c, constant.ContextKeyChannelId)
		relaysvc.AddUsedChannel(c, channelID)
		taskErr = corerelay.RelayTaskSubmit(c, info)
		if taskErr == nil {
			channelsvc.RecordChannelSuccess(channelID)
			return
		}
		logger.LogError(c, fmt.Sprintf("task relay failed (channel #%d, attempt %d, status %d): %s",
			channelID, attempt+1, taskErr.StatusCode, taskErr.Message))

		if !relaysvc.ShouldRetryTaskRelay(c, taskErr, common.RetryTimes-attempt) {
			return
		}
		if _, selectErr := relaysvc.SelectRetryChannel(c, info.OriginModelName, attempt+1); selectErr != nil {
			logger.LogError(c, selectErr.Error())
			return
		}
	}
}
