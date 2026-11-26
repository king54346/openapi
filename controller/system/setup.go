package system

import (
	"errors"
	"strings"

	ctrlcommon "openapi/controller/common"
	"openapi/model"
	systemsvc "openapi/service/system"

	gin "github.com/king54346/gin-tiny"
)

func GetSetup(c gin.Context) {
	status, err := systemsvc.GetSetupStatus()
	if err != nil {
		ctrlcommon.Error(c, err)
		return
	}
	ctrlcommon.Success(c, status)
}

func PostSetup(c gin.Context) {
	var req systemsvc.SetupInput
	if err := c.ShouldBindJSON(&req); err != nil {
		ctrlcommon.ErrorMsg(c, "invalid request: "+err.Error())
		return
	}
	root, err := systemsvc.InitializeSystem(req)
	if err != nil {
		if errors.Is(err, systemsvc.ErrValidation) {
			ctrlcommon.ErrorMsg(c, strings.TrimPrefix(err.Error(), systemsvc.ErrValidation.Error()+": "))
			return
		}
		if errors.Is(err, model.ErrAlreadyInitialized) || errors.Is(err, model.ErrUsernameTaken) {
			ctrlcommon.ErrorMsg(c, err.Error())
			return
		}
		ctrlcommon.Error(c, err)
		return
	}
	ctrlcommon.Success(c, gin.H{"id": root.Id, "username": root.Username, "role": root.Role})
}
