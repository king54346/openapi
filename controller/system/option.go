package system

import (
	"errors"
	"strings"

	ctrlcommon "openapi/controller/common"
	"openapi/model"
	systemsvc "openapi/service/system"

	gin "github.com/king54346/gin-tiny"
)

func GetOptions(c gin.Context) {
	values, err := systemsvc.GetMaskedOptions()
	if err != nil {
		ctrlcommon.Error(c, err)
		return
	}
	ctrlcommon.Success(c, values)
}

func UpdateOption(c gin.Context) {
	var req struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		ctrlcommon.ErrorMsg(c, "invalid request: "+err.Error())
		return
	}
	key := strings.TrimSpace(req.Key)
	if err := systemsvc.UpdateOption(key, req.Value); err != nil {
		if errors.Is(err, systemsvc.ErrValidation) {
			ctrlcommon.ErrorMsg(c, strings.TrimPrefix(err.Error(), systemsvc.ErrValidation.Error()+": "))
			return
		}
		if errors.Is(err, model.ErrOptionKeyRequired) {
			ctrlcommon.ErrorMsg(c, err.Error())
			return
		}
		ctrlcommon.Error(c, err)
		return
	}
	ctrlcommon.Success(c, gin.H{"key": key})
}

func MigrateConsoleSetting(c gin.Context) {
	var req struct {
		Keys []string `json:"keys"`
	}
	_ = c.ShouldBindJSON(&req)
	migrated, skipped, err := systemsvc.MigrateConsoleSettings(req.Keys)
	if err != nil {
		ctrlcommon.Error(c, err)
		return
	}
	ctrlcommon.Success(c, gin.H{"migrated": migrated, "skipped": skipped})
}
