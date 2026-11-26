package modelmeta

import (
	ctrlcommon "openapi/controller/common"
	metasvc "openapi/service/modelmeta"

	gin "github.com/king54346/gin-tiny"
)

func ChannelListModels(c gin.Context) {
	ctrlcommon.Success(c, metasvc.ChannelListModels())
}

func DashboardListModels(c gin.Context) {
	ctrlcommon.Success(c, metasvc.DashboardListModels())
}

func EnabledListModels(c gin.Context) {
	ctrlcommon.Success(c, metasvc.EnabledListModels())
}
