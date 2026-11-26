package common

import (
	"net/http"
	"strconv"

	appcommon "openapi/common"

	gin "github.com/king54346/gin-tiny"
)

// 管理接口统一返回 HTTP 200 + {success, message, data}，与前端约定一致。

// Success 返回成功响应并携带 data。
func Success(c gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": data})
}

// SuccessMsg 返回带自定义提示语和数据的成功响应。
func SuccessMsg(c gin.Context, message string, data any) {
	c.JSON(http.StatusOK, gin.H{"success": true, "message": message, "data": data})
}

// OK 返回无 data 的成功响应。
func OK(c gin.Context) {
	c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
}

// ErrorMsg 返回业务错误提示。
func ErrorMsg(c gin.Context, msg string) {
	c.JSON(http.StatusOK, gin.H{"success": false, "message": msg})
}

// ErrorWithData 返回携带附加数据的失败响应。
func ErrorWithData(c gin.Context, msg string, data any) {
	c.JSON(http.StatusOK, gin.H{"success": false, "message": msg, "data": data})
}

// Error 记录内部错误日志并返回通用错误提示，避免泄露数据库或内部细节。
func Error(c gin.Context, err error) {
	appcommon.SysError("api error: " + err.Error())
	ErrorMsg(c, "internal error, please check server logs")
}

// PageResult 构造分页接口的统一返回体，调用方可继续追加附加统计字段。
func PageResult(p *appcommon.PageInfo, items any, total int64) gin.H {
	return gin.H{"items": items, "total": total, "page": p.Page, "page_size": p.PageSize}
}

// ParamID 解析路径参数 :id，非法时写回错误响应并返回 false。
func ParamID(c gin.Context) (int, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		ErrorMsg(c, "invalid id")
		return 0, false
	}
	return id, true
}

// IdsRequest 批量按 ID 操作的通用请求体。
type IdsRequest struct {
	Ids []int `json:"ids"`
}
