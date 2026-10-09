package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"

	"openapi/common"
	"openapi/dto"
	"openapi/logger"
	"openapi/types"
)

// RelayErrorHandler 把上游的非 2xx 响应转换为 StarAPIError，并关闭响应体
func RelayErrorHandler(ctx context.Context, resp *http.Response, showBodyWhenFail bool) (apiErr *types.StarAPIError) {
	apiErr = types.InitOpenAIError(types.ErrorCodeBadResponseStatusCode, resp.StatusCode)

	responseBody, err := io.ReadAll(resp.Body)
	CloseResponseBodyGracefully(resp)
	if err != nil {
		return
	}
	buildErrWithBody := func(message string) error {
		if message == "" {
			return fmt.Errorf("bad response status code %d, body: %s", resp.StatusCode, string(responseBody))
		}
		return fmt.Errorf("bad response status code %d, message: %s, body: %s", resp.StatusCode, message, string(responseBody))
	}

	var errResponse dto.GeneralErrorResponse
	if err = common.Unmarshal(responseBody, &errResponse); err != nil {
		if showBodyWhenFail {
			apiErr.Err = buildErrWithBody("")
		} else {
			logger.LogError(ctx, fmt.Sprintf("bad response status code %d, body: %s", resp.StatusCode, string(responseBody)))
			apiErr.Err = fmt.Errorf("bad response status code %d", resp.StatusCode)
		}
		return
	}

	if oaiError := errResponse.TryToOpenAIError(); oaiError != nil {
		apiErr = types.WithOpenAIError(*oaiError, resp.StatusCode)
		if showBodyWhenFail {
			apiErr.Err = buildErrWithBody(apiErr.Error())
		}
		return
	}
	apiErr = types.NewOpenAIError(errors.New(errResponse.ToMessage()), types.ErrorCodeBadResponseStatusCode, resp.StatusCode)
	if showBodyWhenFail {
		apiErr.Err = buildErrWithBody(apiErr.Error())
	}
	return
}

// ResetStatusCode 按渠道配置的状态码映射（例如 {"429":"503"}）改写返回给下游的状态码
func ResetStatusCode(apiErr *types.StarAPIError, statusCodeMappingStr string) {
	if apiErr == nil || statusCodeMappingStr == "" || statusCodeMappingStr == "{}" {
		return
	}
	if apiErr.StatusCode == http.StatusOK {
		return
	}
	statusCodeMapping := make(map[string]any)
	if err := common.Unmarshal([]byte(statusCodeMappingStr), &statusCodeMapping); err != nil {
		return
	}
	if value, ok := statusCodeMapping[strconv.Itoa(apiErr.StatusCode)]; ok {
		if intCode, ok := parseStatusCodeMappingValue(value); ok {
			apiErr.StatusCode = intCode
		}
	}
}

func parseStatusCodeMappingValue(value any) (int, bool) {
	switch v := value.(type) {
	case string:
		if v == "" {
			return 0, false
		}
		statusCode, err := strconv.Atoi(v)
		if err != nil {
			return 0, false
		}
		return statusCode, true
	case float64:
		if v != math.Trunc(v) {
			return 0, false
		}
		return int(v), true
	case int:
		return v, true
	case json.Number:
		statusCode, err := strconv.Atoi(v.String())
		if err != nil {
			return 0, false
		}
		return statusCode, true
	default:
		return 0, false
	}
}

func TaskErrorWrapperLocal(err error, code string, statusCode int) *dto.TaskError {
	taskErr := TaskErrorWrapper(err, code, statusCode)
	taskErr.LocalError = true
	return taskErr
}

func TaskErrorWrapper(err error, code string, statusCode int) *dto.TaskError {
	text := err.Error()
	lowerText := strings.ToLower(text)
	if strings.Contains(lowerText, "post") || strings.Contains(lowerText, "dial") || strings.Contains(lowerText, "http") {
		common.SysLog(fmt.Sprintf("error: %s", text))
		// 避免把上游地址、密钥等内部信息暴露给调用方
		text = common.MaskSensitiveInfo(text)
	}
	return &dto.TaskError{
		Code:       code,
		Message:    text,
		StatusCode: statusCode,
		Error:      err,
	}
}
