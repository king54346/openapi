package middleware

import (
	"errors"
	"net/http"
	"strings"

	"openapi/common"
	"openapi/common/system"
	"openapi/constant"
	"openapi/types"

	gin "github.com/king54346/gin-tiny"
)

func abortOverloaded(c gin.Context, apiErr *types.StarAPIError) {
	if strings.HasPrefix(c.Request().URL.Path, "/v1/messages") {
		c.JSON(apiErr.StatusCode, gin.H{
			"error": gin.H{
				"message": apiErr.Error(),
				"type":    "overloaded_error",
			},
		})
	} else {
		c.JSON(apiErr.StatusCode, gin.H{
			"error": apiErr.ToOpenAIError(),
		})
	}
	c.Abort()
}

// SystemPerformanceCheck 检查系统 CPU/内存/磁盘性能阈值中间件。
func SystemPerformanceCheck() gin.HandlerFunc {
	return func(c gin.Context) {
		if err := checkSystemPerformance(); err != nil {
			abortOverloaded(c, err)
			return
		}
		c.Next()
	}
}

// AdmissionControl 在途请求准入控制与平滑背压中间件（置于鉴权之后以支持高优先白名单识别）。
func AdmissionControl() gin.HandlerFunc {
	return func(c gin.Context) {
		username := common.GetContextKeyString(c, constant.ContextKeyUserName)
		if username == "" {
			username = c.GetString("username")
		}
		tokenName := c.GetString("token_name")
		userGroup := c.GetString("group")
		isPriority := system.MatchAdmissionPriority(username, tokenName, userGroup)

		release, err := system.AcquireAdmission(c.Request().Context(), isPriority)
		if err != nil {
			abortOverloaded(
				c,
				types.NewErrorWithStatusCode(err, "system_admission_overloaded", http.StatusServiceUnavailable),
			)
			return
		}
		defer release()
		c.Next()
	}
}

func checkSystemPerformance() *types.StarAPIError {
	config := common.GetPerformanceMonitorConfig()
	if !config.Enabled {
		return nil
	}

	status := system.GetSystemStatus()

	if config.CPUThreshold > 0 && int(status.CPUUsage) > config.CPUThreshold {
		return types.NewErrorWithStatusCode(errors.New("system cpu overloaded"), "system_cpu_overloaded", http.StatusServiceUnavailable)
	}
	if config.MemoryThreshold > 0 && int(status.MemoryUsage) > config.MemoryThreshold {
		return types.NewErrorWithStatusCode(errors.New("system memory overloaded"), "system_memory_overloaded", http.StatusServiceUnavailable)
	}
	if config.DiskThreshold > 0 && int(status.DiskUsage) > config.DiskThreshold {
		return types.NewErrorWithStatusCode(errors.New("system disk overloaded"), "system_disk_overloaded", http.StatusServiceUnavailable)
	}
	return nil
}
