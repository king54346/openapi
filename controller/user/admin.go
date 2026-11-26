package user

import (
	"openapi/common"
	ctrlcommon "openapi/controller/common"
	usersvc "openapi/service/user"

	gin "github.com/king54346/gin-tiny"
)

func handleUserErr(c gin.Context, err error) {
	if msg, ok := usersvc.IsBusinessErr(err); ok {
		ctrlcommon.ErrorMsg(c, msg)
		return
	}
	ctrlcommon.Error(c, err)
}

func GetAllUsers(c gin.Context) {
	page := common.GetPageQuery(c)
	users, total, err := usersvc.ListUsers(page.GetStartIdx(), page.GetPageSize())
	if err != nil {
		ctrlcommon.Error(c, err)
		return
	}
	ctrlcommon.Success(c, ctrlcommon.PageResult(page, users, total))
}

func SearchUsers(c gin.Context) {
	page := common.GetPageQuery(c)
	users, total, err := usersvc.SearchUsers(c.Query("keyword"), page.GetStartIdx(), page.GetPageSize())
	if err != nil {
		ctrlcommon.Error(c, err)
		return
	}
	ctrlcommon.Success(c, ctrlcommon.PageResult(page, users, total))
}

func GetUser(c gin.Context) {
	id, ok := ctrlcommon.ParamID(c)
	if !ok {
		return
	}
	if id == c.GetInt("id") {
		GetSelf(c)
		return
	}
	u, err := usersvc.LoadManagedUser(c.GetInt("id"), c.GetInt("role"), id)
	if err != nil {
		handleUserErr(c, err)
		return
	}
	ctrlcommon.Success(c, u)
}

func CreateUser(c gin.Context) {
	var req usersvc.CreateUserInput
	if err := c.ShouldBindJSON(&req); err != nil {
		ctrlcommon.ErrorMsg(c, "invalid request: "+err.Error())
		return
	}
	u, err := usersvc.CreateUser(c.GetInt("role"), req)
	if err != nil {
		handleUserErr(c, err)
		return
	}
	ctrlcommon.Success(c, u)
}

func UpdateUser(c gin.Context) {
	var req usersvc.UpdateUserInput
	if err := c.ShouldBindJSON(&req); err != nil || req.Id <= 0 {
		ctrlcommon.ErrorMsg(c, "invalid request")
		return
	}
	u, err := usersvc.UpdateUser(c.GetInt("id"), c.GetInt("role"), req)
	if err != nil {
		handleUserErr(c, err)
		return
	}
	ctrlcommon.Success(c, u)
}

func DeleteUser(c gin.Context) {
	id, ok := ctrlcommon.ParamID(c)
	if !ok {
		return
	}
	if err := usersvc.DeleteUser(c.GetInt("id"), c.GetInt("role"), id); err != nil {
		handleUserErr(c, err)
		return
	}
	ctrlcommon.OK(c)
}

func ManageUser(c gin.Context) {
	var req struct {
		Id     int    `json:"id"`
		Action string `json:"action"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Id <= 0 {
		ctrlcommon.ErrorMsg(c, "invalid request")
		return
	}
	deleted, u, err := usersvc.ManageUser(c.GetInt("id"), c.GetInt("role"), req.Id, req.Action)
	if err != nil {
		handleUserErr(c, err)
		return
	}
	if deleted {
		ctrlcommon.OK(c)
		return
	}
	ctrlcommon.Success(c, gin.H{"role": u.Role, "status": u.Status})
}
