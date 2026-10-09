package controller

import (
	"net/http"
	"strconv"

	"openapi/common"

	gin "github.com/king54346/gin-tiny"
)

// 管理接口统一返回 HTTP 200 + {success, message, data}，与 new-api 前端约定一致。

func apiSuccess(c gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": data})
}

func apiOK(c gin.Context) {
	c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
}

func apiErrorMsg(c gin.Context, msg string) {
	c.JSON(http.StatusOK, gin.H{"success": false, "message": msg})
}

// apiError 返回错误；数据库等内部错误只记日志，对外给通用提示。
func apiError(c gin.Context, err error) {
	common.SysError("api error: " + err.Error())
	apiErrorMsg(c, "internal error, please check server logs")
}

// pageResult 分页接口的统一返回体，调用方可追加字段（如 type_counts）。
// 分页参数由 common.GetPageQuery 解析。
func pageResult(p *common.PageInfo, items any, total int64) gin.H {
	return gin.H{"items": items, "total": total, "page": p.Page, "page_size": p.PageSize}
}

// paramID 解析路径参数 :id，非法时写回错误并返回 false。
func paramID(c gin.Context) (int, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		apiErrorMsg(c, "invalid id")
		return 0, false
	}
	return id, true
}

// idsRequest 批量操作的请求体。
type idsRequest struct {
	Ids []int `json:"ids"`
}
