package controller

import (
	"errors"
	"strconv"
	"strings"

	"openapi/common"
	"openapi/model"

	gin "github.com/king54346/gin-tiny"
)

// 模型元数据管理接口（需管理员），参照 new-api controller/model_meta.go。
// 附加信息（提供该模型的渠道、可用分组、规则匹配到的模型）来自渠道缓存；本项目不做计费，没有定价刷新。

// GetAllModelsMeta 分页列出模型元数据，附带各供应商的模型数量。
func GetAllModelsMeta(c gin.Context) {
	pageInfo := common.GetPageQuery(c)
	models, total, err := model.GetAllModels(pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		apiError(c, err)
		return
	}
	model.EnrichModels(models)
	vendorCounts, _ := model.GetVendorModelCounts()
	apiSuccess(c, gin.H{
		"items":         models,
		"total":         total,
		"page":          pageInfo.GetPage(),
		"page_size":     pageInfo.GetPageSize(),
		"vendor_counts": vendorCounts,
	})
}

// GetModelTags 所有模型使用过的标签。
func GetModelTags(c gin.Context) {
	tags, err := model.GetAllModelTags()
	if err != nil {
		apiError(c, err)
		return
	}
	apiSuccess(c, tags)
}

// SearchModelsMeta 按 keyword（名称/描述/标签）、vendor（id 或名称）、tags（逗号分隔，需全部命中）搜索。
func SearchModelsMeta(c gin.Context) {
	var tags []string
	for _, t := range strings.Split(c.Query("tags"), ",") {
		if t = strings.TrimSpace(t); t != "" {
			tags = append(tags, t)
		}
	}
	pageInfo := common.GetPageQuery(c)
	models, total, err := model.SearchModels(c.Query("keyword"), c.Query("vendor"), tags, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		apiError(c, err)
		return
	}
	model.EnrichModels(models)
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(models)
	apiSuccess(c, pageInfo)
}

func modelMetaNotFoundOrError(c gin.Context, err error) {
	if errors.Is(err, model.ErrModelMetaNotFound) {
		apiErrorMsg(c, "model not found")
		return
	}
	apiError(c, err)
}

// GetModelMeta 单个模型元数据。
func GetModelMeta(c gin.Context) {
	id, ok := paramID(c)
	if !ok {
		return
	}
	m, err := model.GetModelMetaById(id)
	if err != nil {
		modelMetaNotFoundOrError(c, err)
		return
	}
	model.EnrichModels([]*model.Model{m})
	apiSuccess(c, m)
}

// checkModelName 校验名称非空且不与其他模型重名，失败时已写回错误。
func checkModelName(c gin.Context, id int, name string) bool {
	if strings.TrimSpace(name) == "" {
		apiErrorMsg(c, "model_name is required")
		return false
	}
	dup, err := model.IsModelNameDuplicated(id, name)
	if err != nil {
		apiError(c, err)
		return false
	}
	if dup {
		apiErrorMsg(c, "model_name already exists")
		return false
	}
	return true
}

// CreateModelMeta 新建模型元数据。
func CreateModelMeta(c gin.Context) {
	var m model.Model
	if err := c.ShouldBindJSON(&m); err != nil {
		apiErrorMsg(c, "invalid request: "+err.Error())
		return
	}
	m.Id = 0
	m.ModelName = strings.TrimSpace(m.ModelName)
	if !checkModelName(c, 0, m.ModelName) {
		return
	}
	if err := m.Insert(); err != nil {
		apiError(c, err)
		return
	}
	apiSuccess(c, &m)
}

// UpdateModelMeta 修改模型元数据；?status_only=true 时只修改状态。
func UpdateModelMeta(c gin.Context) {
	var m model.Model
	if err := c.ShouldBindJSON(&m); err != nil {
		apiErrorMsg(c, "invalid request: "+err.Error())
		return
	}
	if m.Id <= 0 {
		apiErrorMsg(c, "id is required")
		return
	}
	if _, err := model.GetModelMetaById(m.Id); err != nil {
		modelMetaNotFoundOrError(c, err)
		return
	}
	if c.Query("status_only") == "true" {
		if err := model.UpdateModelMetaStatus(m.Id, m.Status); err != nil {
			apiError(c, err)
			return
		}
	} else {
		m.ModelName = strings.TrimSpace(m.ModelName)
		if !checkModelName(c, m.Id, m.ModelName) {
			return
		}
		if err := m.Update(); err != nil {
			apiError(c, err)
			return
		}
	}
	updated, err := model.GetModelMetaById(m.Id)
	if err != nil {
		apiError(c, err)
		return
	}
	apiSuccess(c, updated)
}

// DeleteModelMeta 删除模型元数据（软删除）。
func DeleteModelMeta(c gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		apiErrorMsg(c, "invalid id")
		return
	}
	if err := model.DeleteModelMetaById(id); err != nil {
		apiError(c, err)
		return
	}
	apiOK(c)
}
