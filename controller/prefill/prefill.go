package prefill

import (
	"openapi/common"
	ctrlcommon "openapi/controller/common"
	prefillsvc "openapi/service/prefill"

	gin "github.com/king54346/gin-tiny"
)

func handlePrefillErr(c gin.Context, err error) {
	if msg, ok := prefillsvc.UnwrapValidationErr(err); ok {
		ctrlcommon.ErrorMsg(c, msg)
		return
	}
	ctrlcommon.Error(c, err)
}

func GetPrefillGroups(c gin.Context) {
	page := common.GetPageQuery(c)
	groups, total, err := prefillsvc.ListPrefillGroups(c.Query("type"), page.GetStartIdx(), page.GetPageSize())
	if err != nil {
		ctrlcommon.Error(c, err)
		return
	}
	ctrlcommon.Success(c, ctrlcommon.PageResult(page, groups, total))
}

func CreatePrefillGroup(c gin.Context) {
	var req prefillsvc.PrefillGroupInput
	if err := c.ShouldBindJSON(&req); err != nil {
		ctrlcommon.ErrorMsg(c, "invalid request: "+err.Error())
		return
	}
	group, err := prefillsvc.CreatePrefillGroup(req)
	if err != nil {
		handlePrefillErr(c, err)
		return
	}
	ctrlcommon.Success(c, group)
}

func UpdatePrefillGroup(c gin.Context) {
	var req prefillsvc.PrefillGroupInput
	if err := c.ShouldBindJSON(&req); err != nil || req.Id <= 0 {
		ctrlcommon.ErrorMsg(c, "invalid request")
		return
	}
	group, err := prefillsvc.UpdatePrefillGroup(req)
	if err != nil {
		handlePrefillErr(c, err)
		return
	}
	ctrlcommon.Success(c, group)
}

func DeletePrefillGroup(c gin.Context) {
	id, ok := ctrlcommon.ParamID(c)
	if !ok {
		return
	}
	if err := prefillsvc.DeletePrefillGroup(id); err != nil {
		handlePrefillErr(c, err)
		return
	}
	ctrlcommon.OK(c)
}
