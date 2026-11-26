package controller

import (
	"openapi/common"
	"openapi/model"
	"strconv"
	"strings"

	gin "github.com/king54346/gin-tiny"
)

// 供应商元数据管理接口（需管理员），参照 new-api controller/vendor.go。

// vendorWithCount 供应商详情附带其模型数量。
type vendorWithCount struct {
	*model.Vendor
	ModelCount int64 `json:"model_count"`
}

// GetAllVendors 分页列出供应商。
func GetAllVendors(c gin.Context) {
	page := common.GetPageQuery(c)
	vendors, total, err := model.ListVendors(page.GetStartIdx(), page.GetPageSize())
	if err != nil {
		apiError(c, err)
		return
	}
	apiSuccess(c, pageResult(page, vendors, total))
}

// SearchVendors 按 keyword（名称/描述）与 status 分页搜索供应商。
func SearchVendors(c gin.Context) {
	var status *int
	if s := strings.TrimSpace(c.Query("status")); s != "" {
		if v, err := strconv.Atoi(s); err == nil {
			status = &v
		}
	}
	page := common.GetPageQuery(c)
	vendors, total, err := SearchVendors(strings.TrimSpace(c.Query("keyword")), status,
		page.GetStartIdx(), page.GetPageSize())
	if err != nil {
		apiError(c, err)
		return
	}
	apiSuccess(c, pageResult(page, vendors, total))
}

// GetVendorMeta 单个供应商，附带模型数量。
func GetVendorMeta(c gin.Context) {
	id, ok := paramID(c)
	if !ok {
		return
	}
	vendor, err := model.GetVendorById(id)
	if err != nil {
		vendorNotFoundOrError(c, err)
		return
	}
	counts, _ := model.GetVendorModelCounts()
	count := counts[int64(id)]
	apiSuccess(c, vendorWithCount{Vendor: vendor, ModelCount: count})
}

func vendorNotFoundOrError(c gin.Context, err error) {
	if err == model.ErrVendorNotFound {
		apiErrorMsg(c, "vendor not found")
		return
	}
	apiError(c, err)
}

// checkVendorName 校验名称非空且不重名，失败时已写回错误。
func checkVendorName(c gin.Context, id int, name string) bool {
	if strings.TrimSpace(name) == "" {
		apiErrorMsg(c, "vendor name is required")
		return false
	}
	dup, err := model.IsVendorNameDuplicated(id, name)
	if err != nil {
		apiError(c, err)
		return false
	}
	if dup {
		apiErrorMsg(c, "vendor name already exists")
		return false
	}
	return true
}

// CreateVendorMeta 新建供应商，body {name, description, icon, status}。
func CreateVendorMeta(c gin.Context) {
	var v model.Vendor
	if err := c.ShouldBindJSON(&v); err != nil {
		apiErrorMsg(c, "invalid request: "+err.Error())
		return
	}
	v.Id = 0
	v.Name = strings.TrimSpace(v.Name)
	if !checkVendorName(c, 0, v.Name) {
		return
	}
	if err := v.Insert(); err != nil {
		apiError(c, err)
		return
	}
	apiSuccess(c, &v)
}

// UpdateVendorMeta 修改供应商，body {id, name?, description?, icon?, status?}。
func UpdateVendorMeta(c gin.Context) {
	var req struct {
		Id          int     `json:"id"`
		Name        *string `json:"name"`
		Description *string `json:"description"`
		Icon        *string `json:"icon"`
		Status      *int    `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		apiErrorMsg(c, "invalid request: "+err.Error())
		return
	}
	if req.Id <= 0 {
		apiErrorMsg(c, "id is required")
		return
	}
	vendor, err := model.GetVendorById(req.Id)
	if err != nil {
		vendorNotFoundOrError(c, err)
		return
	}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if !checkVendorName(c, req.Id, name) {
			return
		}
		vendor.Name = name
	}
	if req.Description != nil {
		vendor.Description = *req.Description
	}
	if req.Icon != nil {
		vendor.Icon = *req.Icon
	}
	if req.Status != nil {
		vendor.Status = *req.Status
	}
	if err := vendor.Update(); err != nil {
		apiError(c, err)
		return
	}
	updated, err := model.GetVendorById(req.Id)
	if err != nil {
		apiError(c, err)
		return
	}
	apiSuccess(c, updated)
}

// DeleteVendorMeta 软删除供应商。
func DeleteVendorMeta(c gin.Context) {
	id, ok := paramID(c)
	if !ok {
		return
	}
	vendor, err := model.GetVendorById(id)
	if err != nil {
		vendorNotFoundOrError(c, err)
		return
	}
	if err := vendor.Delete(); err != nil {
		apiError(c, err)
		return
	}
	apiOK(c)
}
