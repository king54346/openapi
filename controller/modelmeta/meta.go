package modelmeta

import (
	"strings"

	"openapi/common"
	ctrlcommon "openapi/controller/common"
	"openapi/model"
	metasvc "openapi/service/modelmeta"

	gin "github.com/king54346/gin-tiny"
)

func handleMetaErr(c gin.Context, err error) {
	if msg, ok := metasvc.UnwrapBusinessErr(err); ok {
		ctrlcommon.ErrorMsg(c, msg)
		return
	}
	ctrlcommon.Error(c, err)
}

func GetAllModelsMeta(c gin.Context) {
	pageInfo := common.GetPageQuery(c)
	models, total, vendorCounts, err := metasvc.ListModelsMeta(pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		ctrlcommon.Error(c, err)
		return
	}
	result := ctrlcommon.PageResult(pageInfo, models, total)
	result["vendor_counts"] = vendorCounts
	ctrlcommon.Success(c, result)
}

func GetModelTags(c gin.Context) {
	tags, err := metasvc.GetModelTags()
	if err != nil {
		ctrlcommon.Error(c, err)
		return
	}
	ctrlcommon.Success(c, tags)
}

func SearchModelsMeta(c gin.Context) {
	var tags []string
	for _, t := range strings.Split(c.Query("tags"), ",") {
		if t = strings.TrimSpace(t); t != "" {
			tags = append(tags, t)
		}
	}
	pageInfo := common.GetPageQuery(c)
	models, total, err := metasvc.SearchModelsMeta(c.Query("keyword"), c.Query("vendor"), tags, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		ctrlcommon.Error(c, err)
		return
	}
	ctrlcommon.Success(c, ctrlcommon.PageResult(pageInfo, models, total))
}

func GetModelMeta(c gin.Context) {
	id, ok := ctrlcommon.ParamID(c)
	if !ok {
		return
	}
	m, err := metasvc.GetModelMeta(id)
	if err != nil {
		handleMetaErr(c, err)
		return
	}
	ctrlcommon.Success(c, m)
}

func CreateModelMeta(c gin.Context) {
	var m model.Model
	if err := c.ShouldBindJSON(&m); err != nil {
		ctrlcommon.ErrorMsg(c, "invalid request: "+err.Error())
		return
	}
	created, err := metasvc.CreateModelMeta(m)
	if err != nil {
		handleMetaErr(c, err)
		return
	}
	ctrlcommon.Success(c, created)
}

func UpdateModelMeta(c gin.Context) {
	var m model.Model
	if err := c.ShouldBindJSON(&m); err != nil {
		ctrlcommon.ErrorMsg(c, "invalid request: "+err.Error())
		return
	}
	updated, err := metasvc.UpdateModelMeta(m, c.Query("status_only") == "true")
	if err != nil {
		handleMetaErr(c, err)
		return
	}
	ctrlcommon.Success(c, updated)
}

func DeleteModelMeta(c gin.Context) {
	id, ok := ctrlcommon.ParamID(c)
	if !ok {
		return
	}
	if err := metasvc.DeleteModelMeta(id); err != nil {
		ctrlcommon.Error(c, err)
		return
	}
	ctrlcommon.OK(c)
}
