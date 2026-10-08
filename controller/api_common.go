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

const (
	defaultPageSize = 10
	maxPageSize     = 100
)

// pageQuery 解析分页参数 p（从 1 开始）与 page_size。
type pageQuery struct {
	Page     int
	PageSize int
}

func getPageQuery(c gin.Context) pageQuery {
	page, _ := strconv.Atoi(c.Query("p"))
	if page < 1 {
		page = 1
	}
	size, _ := strconv.Atoi(c.Query("page_size"))
	if size <= 0 {
		size = defaultPageSize
	}
	if size > maxPageSize {
		size = maxPageSize
	}
	return pageQuery{Page: page, PageSize: size}
}

func (p pageQuery) offset() int { return (p.Page - 1) * p.PageSize }

func (p pageQuery) result(items any, total int64) gin.H {
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
