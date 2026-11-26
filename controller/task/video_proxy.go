package task

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"openapi/logger"
	tasksvc "openapi/service/task"

	gin "github.com/king54346/gin-tiny"
)

func VideoProxy(c gin.Context) {
	start := time.Now()
	resp, err := tasksvc.OpenUpstreamVideo(
		c.Request().Context(),
		c.GetInt("id"),
		c.Param("task_id"),
		c.Request().Header.Get("Range"),
		c.Request().Header.Get("If-Range"),
		c.Request().Header.Get("User-Agent"),
	)
	if err != nil {
		var httpErr *tasksvc.ProxyHTTPError
		if errors.As(err, &httpErr) {
			videoProxyError(c, httpErr.Status, httpErr.Message)
			return
		}
		videoProxyError(c, http.StatusInternalServerError, err.Error())
		return
	}
	defer resp.Body.Close()

	for _, header := range tasksvc.VideoProxyForwardHeaders {
		if value := resp.Header.Get(header); value != "" {
			c.Response().Header().Set(header, value)
		}
	}
	c.Status(resp.StatusCode)

	written, copyErr := io.CopyBuffer(c.Response(), resp.Body, make([]byte, tasksvc.VideoProxyBufferSize))
	if copyErr != nil {
		logger.LogDebug(c, "video proxy copy interrupted: "+copyErr.Error())
		return
	}
	logger.LogInfo(c, "video proxy finished: "+strconv.FormatInt(written, 10)+" bytes in "+time.Since(start).Round(time.Millisecond).String())
}

func videoProxyError(c gin.Context, status int, message string) {
	if c.Response().Written() {
		return
	}
	c.JSON(status, gin.H{
		"error": gin.H{
			"message": message,
			"type":    "invalid_request_error",
		},
	})
}
