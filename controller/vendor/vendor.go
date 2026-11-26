package vendor

import (
	"strconv"
	"strings"

	"openapi/common"
	ctrlcommon "openapi/controller/common"
	"openapi/model"
	vendorsvc "openapi/service/vendor"

	gin "github.com/king54346/gin-tiny"
)

func handleVendorErr(c gin.Context, err error) {
	if msg, ok := vendorsvc.UnwrapBusinessErr(err); ok {
		ctrlcommon.ErrorMsg(c, msg)
		return
	}
	ctrlcommon.Error(c, err)
}

func GetAllVendors(c gin.Context) {
	page := common.GetPageQuery(c)
	vendors, total, err := vendorsvc.ListVendors(page.GetStartIdx(), page.GetPageSize())
	if err != nil {
		ctrlcommon.Error(c, err)
		return
	}
	ctrlcommon.Success(c, ctrlcommon.PageResult(page, vendors, total))
}

func SearchVendors(c gin.Context) {
	var status *int
	if s := strings.TrimSpace(c.Query("status")); s != "" {
		if v, err := strconv.Atoi(s); err == nil {
			status = &v
		}
	}
	page := common.GetPageQuery(c)
	vendors, total, err := vendorsvc.SearchVendors(c.Query("keyword"), status, page.GetStartIdx(), page.GetPageSize())
	if err != nil {
		ctrlcommon.Error(c, err)
		return
	}
	ctrlcommon.Success(c, ctrlcommon.PageResult(page, vendors, total))
}

func GetVendorMeta(c gin.Context) {
	id, ok := ctrlcommon.ParamID(c)
	if !ok {
		return
	}
	v, err := vendorsvc.GetVendor(id)
	if err != nil {
		handleVendorErr(c, err)
		return
	}
	ctrlcommon.Success(c, v)
}

func CreateVendorMeta(c gin.Context) {
	var v model.Vendor
	if err := c.ShouldBindJSON(&v); err != nil {
		ctrlcommon.ErrorMsg(c, "invalid request: "+err.Error())
		return
	}
	created, err := vendorsvc.CreateVendor(v)
	if err != nil {
		handleVendorErr(c, err)
		return
	}
	ctrlcommon.Success(c, created)
}

func UpdateVendorMeta(c gin.Context) {
	var req vendorsvc.UpdateVendorInput
	if err := c.ShouldBindJSON(&req); err != nil {
		ctrlcommon.ErrorMsg(c, "invalid request: "+err.Error())
		return
	}
	updated, err := vendorsvc.UpdateVendor(req)
	if err != nil {
		handleVendorErr(c, err)
		return
	}
	ctrlcommon.Success(c, updated)
}

func DeleteVendorMeta(c gin.Context) {
	id, ok := ctrlcommon.ParamID(c)
	if !ok {
		return
	}
	if err := vendorsvc.DeleteVendor(id); err != nil {
		handleVendorErr(c, err)
		return
	}
	ctrlcommon.OK(c)
}
