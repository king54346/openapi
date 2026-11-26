package channel

import (
	ctrlcommon "openapi/controller/common"
	channelsvc "openapi/service/channel"

	gin "github.com/king54346/gin-tiny"
)

func ManageMultiKeys(c gin.Context) {
	var req channelsvc.MultiKeyManageInput
	if err := c.ShouldBindJSON(&req); err != nil {
		ctrlcommon.ErrorMsg(c, "invalid request")
		return
	}
	message, data, err := channelsvc.ManageMultiKeys(req)
	if err != nil {
		handleChannelErr(c, err)
		return
	}
	if message != "" {
		ctrlcommon.SuccessMsg(c, message, data)
		return
	}
	ctrlcommon.Success(c, data)
}
