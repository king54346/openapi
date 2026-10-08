package relay

import (
	"fmt"

	"openapi/dto"
	relaycommon "openapi/relay/common"
	"openapi/service"
	"openapi/types"

	"github.com/gorilla/websocket"
	gin "github.com/king54346/gin-tiny"
)

func WssHelper(c gin.Context, info *relaycommon.RelayInfo) (apiError *types.StarAPIError) {
	info.InitChannelMeta(c)

	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return types.NewError(fmt.Errorf("invalid api type: %d", info.ApiType), types.ErrorCodeInvalidApiType, types.ErrOptionWithSkipRetry())
	}
	adaptor.Init(info)
	//var requestBody io.Reader
	//firstWssRequest, _ := c.Get("first_wss_request")
	//requestBody = bytes.NewBuffer(firstWssRequest.([]byte))

	statusCodeMappingStr := c.GetString("status_code_mapping")
	resp, err := adaptor.DoRequest(c, info, nil)
	if err != nil {
		return types.NewError(err, types.ErrorCodeDoRequestFailed)
	}

	if resp != nil {
		info.TargetWs = resp.(*websocket.Conn)
		defer info.TargetWs.Close()
	}

	usage, apiError := adaptor.DoResponse(c, nil, info)
	if apiError != nil {
		// reset status code 重置状态码
		service.ResetStatusCode(apiError, statusCodeMappingStr)
		return apiError
	}
	recordUsage(c, info, realtimeUsageToUsage(usage.(*dto.RealtimeUsage)), nil)
	return nil
}

// realtimeUsageToUsage 把 realtime 的 input/output 用量转换为通用 Usage
func realtimeUsageToUsage(u *dto.RealtimeUsage) *dto.Usage {
	if u == nil {
		return nil
	}
	usage := &dto.Usage{
		PromptTokens:     u.InputTokens,
		CompletionTokens: u.OutputTokens,
		TotalTokens:      u.TotalTokens,
	}
	usage.PromptTokensDetails.CachedTokens = u.InputTokenDetails.CachedTokens
	usage.PromptTokensDetails.TextTokens = u.InputTokenDetails.TextTokens
	usage.PromptTokensDetails.AudioTokens = u.InputTokenDetails.AudioTokens
	usage.CompletionTokenDetails.TextTokens = u.OutputTokenDetails.TextTokens
	usage.CompletionTokenDetails.AudioTokens = u.OutputTokenDetails.AudioTokens
	return usage
}
