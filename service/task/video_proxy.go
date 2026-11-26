package task

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"

	"openapi/common"
	"openapi/logger"
	"openapi/model"
	httpclient "openapi/service/http"
)

const VideoProxyBufferSize = 64 << 10

var VideoProxyForwardHeaders = []string{
	"Content-Type",
	"Content-Length",
	"Content-Range",
	"Accept-Ranges",
	"Content-Disposition",
	"Last-Modified",
	"ETag",
}

type ProxyHTTPError struct {
	Status  int
	Message string
}

func (e *ProxyHTTPError) Error() string { return e.Message }

// OpenUpstreamVideo 校验任务归属与完成状态，并向上游发起流式拉取请求（支持 Range）。
func OpenUpstreamVideo(ctx context.Context, userID int, taskID, rangeHeader, ifRange, userAgent string) (*http.Response, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return nil, &ProxyHTTPError{Status: http.StatusBadRequest, Message: "task_id is required"}
	}
	if userID <= 0 {
		return nil, &ProxyHTTPError{Status: http.StatusUnauthorized, Message: "invalid token"}
	}

	t, found, err := model.GetByTaskId(userID, taskID)
	if err != nil {
		common.SysError("failed to query task: " + err.Error())
		return nil, &ProxyHTTPError{Status: http.StatusInternalServerError, Message: "failed to query task"}
	}
	if !found {
		return nil, &ProxyHTTPError{Status: http.StatusNotFound, Message: "task not found"}
	}
	if t.Status != model.TaskStatusSuccess {
		return nil, &ProxyHTTPError{Status: http.StatusBadRequest, Message: "task is not finished successfully, current status: " + string(t.Status)}
	}
	resultURL := strings.TrimSpace(t.GetResultURL())
	if resultURL == "" {
		return nil, &ProxyHTTPError{Status: http.StatusNotFound, Message: "task has no result url"}
	}

	proxyURL := ""
	if t.ChannelId > 0 {
		if ch, channelErr := model.GetChannelById(t.ChannelId, true); channelErr == nil {
			proxyURL = ch.GetSetting().Proxy
		} else {
			logger.LogWarn(ctx, "failed to load channel for video proxy: "+channelErr.Error())
		}
	}
	client, err := httpclient.GetHttpClientWithProxy(proxyURL)
	if err != nil {
		common.SysError("failed to create http client for video proxy: " + err.Error())
		return nil, &ProxyHTTPError{Status: http.StatusInternalServerError, Message: "failed to create http client"}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, resultURL, nil)
	if err != nil {
		return nil, &ProxyHTTPError{Status: http.StatusBadRequest, Message: "invalid result url"}
	}
	if rangeHeader != "" {
		req.Header.Set("Range", rangeHeader)
	}
	if ifRange != "" {
		req.Header.Set("If-Range", ifRange)
	}
	if userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	}

	resp, err := client.Do(req)
	if err != nil {
		logger.LogError(ctx, "video proxy request failed: "+common.MaskSensitiveInfo(err.Error()))
		return nil, &ProxyHTTPError{Status: http.StatusBadGateway, Message: "failed to fetch video from upstream"}
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		_ = resp.Body.Close()
		logger.LogError(ctx, "video proxy upstream returned "+resp.Status+" : "+common.MaskSensitiveInfo(string(body)))
		return nil, &ProxyHTTPError{Status: http.StatusBadGateway, Message: "upstream returned status " + strconv.Itoa(resp.StatusCode)}
	}
	return resp, nil
}
