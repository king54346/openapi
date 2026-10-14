package controller

import (
	"openapi/common"

	gin "github.com/king54346/gin-tiny"
)

// GetFetchSetting 当前 SSRF 防护配置。
func GetFetchSetting(c gin.Context) {
	apiSuccess(c, common.GetFetchSetting())
}

// UpdateFetchSetting 整体替换 SSRF 防护配置，端口或 IP 名单非法时不生效。
func UpdateFetchSetting(c gin.Context) {
	var req common.FetchSetting
	if err := c.ShouldBindJSON(&req); err != nil {
		apiErrorMsg(c, "invalid request: "+err.Error())
		return
	}
	if err := common.UpdateFetchSetting(req); err != nil {
		apiErrorMsg(c, err.Error())
		return
	}
	apiSuccess(c, common.GetFetchSetting())
}
