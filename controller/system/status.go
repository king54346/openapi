package system

import (
	"net/http"

	ctrlcommon "openapi/controller/common"
	systemsvc "openapi/service/system"

	gin "github.com/king54346/gin-tiny"
)

func GetStatus(c gin.Context) {
	ctrlcommon.Success(c, systemsvc.GetPublicStatus())
}

func TestStatus(c gin.Context) {
	success, items := systemsvc.RunStatusChecks(c.Request().Context())
	ctrlcommon.Success(c, gin.H{"success": success, "items": items})
}

func GetUptimeKumaStatus(c gin.Context) {
	enabled, body, err := systemsvc.GetUptimeKumaStatus()
	if !enabled {
		ctrlcommon.Success(c, gin.H{})
		return
	}
	if err != nil {
		ctrlcommon.ErrorMsg(c, "failed to load uptime kuma status: "+err.Error())
		return
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", body)
}
