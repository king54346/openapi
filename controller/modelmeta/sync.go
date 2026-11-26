package modelmeta

import (
	"strings"

	"openapi/common"
	ctrlcommon "openapi/controller/common"
	metasvc "openapi/service/modelmeta"

	gin "github.com/king54346/gin-tiny"
)

func GetMissingModels(c gin.Context) {
	missing, err := metasvc.GetMissingModels()
	if err != nil {
		ctrlcommon.Error(c, err)
		return
	}
	ctrlcommon.Success(c, gin.H{"models": missing, "total": len(missing)})
}

func SyncUpstreamPreview(c gin.Context) {
	channelIds := make([]int, 0)
	for _, raw := range strings.Split(c.Query("channel_ids"), ",") {
		if id := common.String2Int(strings.TrimSpace(raw)); id > 0 {
			channelIds = append(channelIds, id)
		}
	}
	createList, updateList, total, err := metasvc.SyncUpstreamPreview(channelIds)
	if err != nil {
		ctrlcommon.Error(c, err)
		return
	}
	ctrlcommon.Success(c, gin.H{
		"create": createList,
		"update": updateList,
		"total":  total,
	})
}

func SyncUpstreamModels(c gin.Context) {
	var req struct {
		ChannelIds []int `json:"channel_ids"`
		DryRun     bool  `json:"dry_run"`
	}
	_ = c.ShouldBindJSON(&req)
	res, err := metasvc.SyncUpstreamModels(req.ChannelIds, req.DryRun)
	if err != nil {
		ctrlcommon.Error(c, err)
		return
	}
	ctrlcommon.Success(c, res)
}
