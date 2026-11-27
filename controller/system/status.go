package system

import (
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
