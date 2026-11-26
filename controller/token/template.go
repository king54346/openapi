package token

import (
	"openapi/common"
	ctrlcommon "openapi/controller/common"
	tokensvc "openapi/service/token"

	gin "github.com/king54346/gin-tiny"
)

func GetUserTokenAccessTemplates(c gin.Context) {
	page := common.GetPageQuery(c)
	templates, total, err := tokensvc.ListTemplates(c.GetInt("id"), page.GetStartIdx(), page.GetPageSize())
	if err != nil {
		ctrlcommon.Error(c, err)
		return
	}
	ctrlcommon.Success(c, ctrlcommon.PageResult(page, templates, total))
}

func AddTokenAccessTemplate(c gin.Context) {
	var req tokensvc.TemplateInput
	if err := c.ShouldBindJSON(&req); err != nil {
		ctrlcommon.ErrorMsg(c, "invalid request: "+err.Error())
		return
	}
	tpl, err := tokensvc.CreateTemplate(c.GetInt("id"), req)
	if err != nil {
		handleTokenErr(c, err)
		return
	}
	ctrlcommon.Success(c, tpl)
}

func UpdateTokenAccessTemplate(c gin.Context) {
	var req tokensvc.TemplateInput
	if err := c.ShouldBindJSON(&req); err != nil || req.Id <= 0 {
		ctrlcommon.ErrorMsg(c, "invalid request")
		return
	}
	tpl, err := tokensvc.UpdateTemplate(c.GetInt("id"), req)
	if err != nil {
		handleTokenErr(c, err)
		return
	}
	ctrlcommon.Success(c, tpl)
}

func DeleteTokenAccessTemplate(c gin.Context) {
	id, ok := ctrlcommon.ParamID(c)
	if !ok {
		return
	}
	if err := tokensvc.DeleteTemplate(c.GetInt("id"), id); err != nil {
		handleTokenErr(c, err)
		return
	}
	ctrlcommon.OK(c)
}
