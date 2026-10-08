package service

import (
	"fmt"
	"net/http"

	"openapi/common"
	"openapi/logger"

	gin "github.com/king54346/gin-tiny"
)

func CloseResponseBodyGracefully(httpResponse *http.Response) {
	if httpResponse == nil || httpResponse.Body == nil {
		return
	}
	if err := httpResponse.Body.Close(); err != nil {
		common.SysError("failed to close response body: " + err.Error())
	}
}

// IOCopyBytesGracefully 把已经读取并处理好的响应体写回下游，并透传上游响应头。
// 必须在响应体解析成功之后再设置响应头，否则解析失败时已无法改为返回错误响应。
func IOCopyBytesGracefully(c gin.Context, src *http.Response, data []byte) {
	w := c.Response()
	if w == nil {
		return
	}
	if src != nil {
		for k, v := range src.Header {
			if k == "Content-Length" {
				continue
			}
			w.Header().Set(k, v[0])
		}
	}
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))

	if src != nil {
		w.WriteHeader(src.StatusCode)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	if _, err := w.Write(data); err != nil {
		logger.LogError(c, fmt.Sprintf("failed to copy response body: %s", err.Error()))
	}
	w.Flush()
}
