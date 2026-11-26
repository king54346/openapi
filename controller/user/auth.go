package user

import (
	"errors"

	ctrlcommon "openapi/controller/common"
	"openapi/model"
	usersvc "openapi/service/user"

	gin "github.com/king54346/gin-tiny"
	"github.com/king54346/gin-tiny/middleware/sessions"
)

func Login(c gin.Context) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Username == "" || req.Password == "" {
		ctrlcommon.ErrorMsg(c, "username and password are required")
		return
	}
	u, err := usersvc.Login(req.Username, req.Password)
	if err != nil {
		if errors.Is(err, model.ErrPasswordIncorrect) || errors.Is(err, model.ErrUserDisabled) {
			ctrlcommon.ErrorMsg(c, err.Error())
			return
		}
		ctrlcommon.Error(c, err)
		return
	}
	session := sessions.Default(c)
	session.Set("id", u.Id)
	session.Set("username", u.Username)
	session.Set("role", u.Role)
	session.Set("status", u.Status)
	if err := session.Save(); err != nil {
		ctrlcommon.Error(c, err)
		return
	}
	ctrlcommon.Success(c, gin.H{
		"id":           u.Id,
		"username":     u.Username,
		"display_name": u.DisplayName,
		"role":         u.Role,
		"status":       u.Status,
	})
}

func Logout(c gin.Context) {
	session := sessions.Default(c)
	session.Clear()
	if err := session.Save(); err != nil {
		ctrlcommon.Error(c, err)
		return
	}
	ctrlcommon.OK(c)
}

func GetSelf(c gin.Context) {
	u, err := usersvc.GetSelf(c.GetInt("id"))
	if err != nil {
		ctrlcommon.ErrorMsg(c, "user not found")
		return
	}
	ctrlcommon.Success(c, u)
}

func GenerateAccessToken(c gin.Context) {
	token, err := usersvc.GenerateAccessToken(c.GetInt("id"))
	if err != nil {
		ctrlcommon.Error(c, err)
		return
	}
	ctrlcommon.Success(c, token)
}

func GetUserModels(c gin.Context) {
	ctrlcommon.Success(c, usersvc.GetAvailableModels())
}
