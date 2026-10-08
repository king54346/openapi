package relay

import (
	"errors"
	"fmt"
	"net/http"

	"openapi/common"
	"openapi/dto"
	relaycommon "openapi/relay/common"
	"openapi/relay/helper"
	"openapi/service"
	"openapi/types"

	gin "github.com/king54346/gin-tiny"
)

func AudioHelper(c gin.Context, info *relaycommon.RelayInfo) (apiError *types.StarAPIError) {
	info.InitChannelMeta(c)

	audioReq, ok := info.Request.(*dto.AudioRequest)
	if !ok {
		return types.NewError(errors.New("invalid request type"), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}

	request, err := common.DeepCopy(audioReq)
	if err != nil {
		return types.NewError(fmt.Errorf("failed to copy request to AudioRequest: %w", err), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}

	err = helper.ModelMappedHelper(c, info, request)
	if err != nil {
		return types.NewError(err, types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())
	}

	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return types.NewError(fmt.Errorf("invalid api type: %d", info.ApiType), types.ErrorCodeInvalidApiType, types.ErrOptionWithSkipRetry())
	}
	adaptor.Init(info)

	ioReader, err := adaptor.ConvertAudioRequest(c, info, *request)
	if err != nil {
		return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}

	resp, err := adaptor.DoRequest(c, info, ioReader)
	if err != nil {
		return types.NewError(err, types.ErrorCodeDoRequestFailed)
	}
	statusCodeMappingStr := c.GetString("status_code_mapping")

	var httpResp *http.Response
	if resp != nil {
		httpResp = resp.(*http.Response)
		if httpResp.StatusCode != http.StatusOK {
			apiError = service.RelayErrorHandler(c.Request().Context(), httpResp, false)
			// reset status code 重置状态码
			service.ResetStatusCode(apiError, statusCodeMappingStr)
			return apiError
		}
	}

	usage, apiError := adaptor.DoResponse(c, httpResp, info)
	if apiError != nil {
		// reset status code 重置状态码
		service.ResetStatusCode(apiError, statusCodeMappingStr)
		return apiError
	}
	recordUsage(c, info, usage.(*dto.Usage), nil)

	return nil
}
