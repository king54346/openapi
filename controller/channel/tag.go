package channel

import (
	ctrlcommon "openapi/controller/common"
	channelsvc "openapi/service/channel"

	gin "github.com/king54346/gin-tiny"
)

func bindTag(c gin.Context) (*channelsvc.TagInput, bool) {
	var req channelsvc.TagInput
	if err := c.ShouldBindJSON(&req); err != nil || req.Tag == "" {
		ctrlcommon.ErrorMsg(c, "tag is required")
		return nil, false
	}
	return &req, true
}

func DisableTagChannels(c gin.Context) {
	req, ok := bindTag(c)
	if !ok {
		return
	}
	if err := channelsvc.DisableTagChannels(req.Tag); err != nil {
		handleChannelErr(c, err)
		return
	}
	ctrlcommon.OK(c)
}

func EnableTagChannels(c gin.Context) {
	req, ok := bindTag(c)
	if !ok {
		return
	}
	if err := channelsvc.EnableTagChannels(req.Tag); err != nil {
		handleChannelErr(c, err)
		return
	}
	ctrlcommon.OK(c)
}

func EditTagChannels(c gin.Context) {
	req, ok := bindTag(c)
	if !ok {
		return
	}
	if err := channelsvc.EditTagChannels(*req); err != nil {
		handleChannelErr(c, err)
		return
	}
	ctrlcommon.OK(c)
}

func BatchSetChannelTag(c gin.Context) {
	var req struct {
		Ids []int   `json:"ids"`
		Tag *string `json:"tag"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Ids) == 0 {
		ctrlcommon.ErrorMsg(c, "ids is required")
		return
	}
	if err := channelsvc.BatchSetChannelTag(req.Ids, req.Tag); err != nil {
		handleChannelErr(c, err)
		return
	}
	ctrlcommon.Success(c, len(req.Ids))
}

func GetTagModels(c gin.Context) {
	longest, err := channelsvc.GetTagModels(c.Query("tag"))
	if err != nil {
		handleChannelErr(c, err)
		return
	}
	ctrlcommon.Success(c, longest)
}
