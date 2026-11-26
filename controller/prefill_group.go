package controller

import (
	"encoding/json"
	"errors"
	"fmt"
	"openapi/common"
	"openapi/model"
	"strings"

	gin "github.com/king54346/gin-tiny"
)

// 预填充内容组管理（管理员）。items 为任意 JSON，按原样存取。

const (
	maxPrefillGroupNameLength = 128
	maxPrefillGroupDescLength = 255
)

// GetPrefillGroups 分页列出预填充组，可按 type 过滤。
func GetPrefillGroups(c gin.Context) {
	page := common.GetPageQuery(c)
	groups, total, err := model.ListPrefillGroups(strings.TrimSpace(c.Query("type")), page.GetStartIdx(), page.GetPageSize())
	if err != nil {
		apiError(c, err)
		return
	}
	apiSuccess(c, pageResult(page, groups, total))
}

// prefillGroupRequest 新建/修改预填充组的请求体。
type prefillGroupRequest struct {
	Id          int             `json:"id"`
	Name        string          `json:"name"`
	Type        string          `json:"type"`
	Items       json.RawMessage `json:"items"`
	Description string          `json:"description"`
}

// validate 校验并规整字段。
func (r *prefillGroupRequest) validate() error {
	r.Name = strings.TrimSpace(r.Name)
	r.Type = strings.TrimSpace(r.Type)
	if r.Name == "" {
		return errors.New("name is required")
	}
	if len(r.Name) > maxPrefillGroupNameLength {
		return fmt.Errorf("name must be at most %d characters", maxPrefillGroupNameLength)
	}
	if r.Type == "" {
		return errors.New("type is required")
	}
	if len([]rune(r.Description)) > maxPrefillGroupDescLength {
		return fmt.Errorf("description must be at most %d characters", maxPrefillGroupDescLength)
	}
	if len(r.Items) > 0 && !json.Valid(r.Items) {
		return errors.New("items must be valid json")
	}
	return nil
}

// CreatePrefillGroup 新建预填充组。
func CreatePrefillGroup(c gin.Context) {
	var req prefillGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiErrorMsg(c, "invalid request: "+err.Error())
		return
	}
	if err := req.validate(); err != nil {
		apiErrorMsg(c, err.Error())
		return
	}
	duplicated, err := model.IsPrefillGroupNameDuplicated(0, req.Name)
	if err != nil {
		apiError(c, err)
		return
	}
	if duplicated {
		apiErrorMsg(c, "prefill group name already exists")
		return
	}
	group := &model.PrefillGroup{
		Name:        req.Name,
		Type:        req.Type,
		Items:       req.Items,
		Description: req.Description,
	}
	if err := group.Insert(); err != nil {
		apiError(c, err)
		return
	}
	apiSuccess(c, group)
}

// UpdatePrefillGroup 修改预填充组。
func UpdatePrefillGroup(c gin.Context) {
	var req prefillGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Id <= 0 {
		apiErrorMsg(c, "invalid request")
		return
	}
	if err := req.validate(); err != nil {
		apiErrorMsg(c, err.Error())
		return
	}
	group, err := model.GetPrefillGroupById(req.Id)
	if err != nil {
		if err == model.ErrPrefillGroupNotFound {
			apiErrorMsg(c, err.Error())
			return
		}
		apiError(c, err)
		return
	}
	duplicated, err := model.IsPrefillGroupNameDuplicated(req.Id, req.Name)
	if err != nil {
		apiError(c, err)
		return
	}
	if duplicated {
		apiErrorMsg(c, "prefill group name already exists")
		return
	}
	group.Name = req.Name
	group.Type = req.Type
	group.Items = req.Items
	group.Description = req.Description
	if err := group.Update(); err != nil {
		apiError(c, err)
		return
	}
	apiSuccess(c, group)
}

// DeletePrefillGroup 软删除预填充组。
func DeletePrefillGroup(c gin.Context) {
	id, ok := paramID(c)
	if !ok {
		return
	}
	group, err := model.GetPrefillGroupById(id)
	if err != nil {
		if err == model.ErrPrefillGroupNotFound {
			apiErrorMsg(c, err.Error())
			return
		}
		apiError(c, err)
		return
	}
	if err := group.Delete(); err != nil {
		apiError(c, err)
		return
	}
	apiOK(c)
}
