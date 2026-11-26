package token

import (
	"openapi/common"
	ctrlcommon "openapi/controller/common"
	tokensvc "openapi/service/token"

	gin "github.com/king54346/gin-tiny"
)

func handleTokenErr(c gin.Context, err error) {
	if msg, ok := tokensvc.UnwrapBusinessErr(err); ok {
		ctrlcommon.ErrorMsg(c, msg)
		return
	}
	ctrlcommon.Error(c, err)
}

func GetAllTokens(c gin.Context) {
	page := common.GetPageQuery(c)
	tokens, total, err := tokensvc.ListUserTokens(c.GetInt("id"), page.GetStartIdx(), page.GetPageSize())
	if err != nil {
		ctrlcommon.Error(c, err)
		return
	}
	ctrlcommon.Success(c, ctrlcommon.PageResult(page, tokens, total))
}

func SearchTokens(c gin.Context) {
	page := common.GetPageQuery(c)
	tokens, total, err := tokensvc.SearchUserTokens(c.GetInt("id"), c.Query("keyword"), c.Query("token"), page.GetStartIdx(), page.GetPageSize())
	if err != nil {
		ctrlcommon.Error(c, err)
		return
	}
	ctrlcommon.Success(c, ctrlcommon.PageResult(page, tokens, total))
}

func GetToken(c gin.Context) {
	id, ok := ctrlcommon.ParamID(c)
	if !ok {
		return
	}
	t, err := tokensvc.GetUserToken(id, c.GetInt("id"))
	if err != nil {
		handleTokenErr(c, err)
		return
	}
	ctrlcommon.Success(c, t)
}

func AddToken(c gin.Context) {
	var req tokensvc.TokenInput
	if err := c.ShouldBindJSON(&req); err != nil {
		ctrlcommon.ErrorMsg(c, "invalid request: "+err.Error())
		return
	}
	t, err := tokensvc.CreateToken(c.GetInt("id"), req)
	if err != nil {
		handleTokenErr(c, err)
		return
	}
	ctrlcommon.Success(c, t)
}

func UpdateToken(c gin.Context) {
	var req tokensvc.TokenInput
	if err := c.ShouldBindJSON(&req); err != nil {
		ctrlcommon.ErrorMsg(c, "invalid request: "+err.Error())
		return
	}
	t, err := tokensvc.UpdateToken(c.GetInt("id"), req, c.Query("status_only") == "true")
	if err != nil {
		handleTokenErr(c, err)
		return
	}
	ctrlcommon.Success(c, t)
}

func DeleteToken(c gin.Context) {
	id, ok := ctrlcommon.ParamID(c)
	if !ok {
		return
	}
	if err := tokensvc.DeleteToken(c.GetInt("id"), id); err != nil {
		handleTokenErr(c, err)
		return
	}
	ctrlcommon.OK(c)
}

func DeleteTokenBatch(c gin.Context) {
	var req ctrlcommon.IdsRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Ids) == 0 {
		ctrlcommon.ErrorMsg(c, "ids is required")
		return
	}
	rows, err := tokensvc.DeleteTokenBatch(c.GetInt("id"), req.Ids)
	if err != nil {
		ctrlcommon.Error(c, err)
		return
	}
	ctrlcommon.Success(c, rows)
}
