package channel

import (
	"strconv"

	"openapi/common"
	ctrlcommon "openapi/controller/common"
	"openapi/model"
	channelsvc "openapi/service/channel"

	gin "github.com/king54346/gin-tiny"
)

func handleChannelErr(c gin.Context, err error) {
	if msg, ok := channelsvc.UnwrapBusinessErr(err); ok {
		ctrlcommon.ErrorMsg(c, msg)
		return
	}
	ctrlcommon.Error(c, err)
}

func parseChannelFilter(c gin.Context) model.ChannelFilter {
	channelType, _ := strconv.Atoi(c.Query("type"))
	idSort, _ := strconv.ParseBool(c.Query("id_sort"))
	return model.ChannelFilter{
		Keyword: c.Query("keyword"),
		Model:   c.Query("model"),
		Status:  channelsvc.ParseStatusFilter(c.Query("status")),
		Type:    channelType,
		IdSort:  idSort,
	}
}

func GetAllChannels(c gin.Context) {
	page := common.GetPageQuery(c)
	filter := parseChannelFilter(c)
	tagMode, _ := strconv.ParseBool(c.Query("tag_mode"))

	channels, total, typeCounts, err := channelsvc.ListChannels(filter, tagMode, page.GetStartIdx(), page.GetPageSize())
	if err != nil {
		ctrlcommon.Error(c, err)
		return
	}
	result := ctrlcommon.PageResult(page, channels, total)
	result["type_counts"] = typeCounts
	ctrlcommon.Success(c, result)
}

func SearchChannels(c gin.Context) {
	GetAllChannels(c)
}

func GetChannel(c gin.Context) {
	id, ok := ctrlcommon.ParamID(c)
	if !ok {
		return
	}
	ch, err := channelsvc.GetChannel(id)
	if err != nil {
		handleChannelErr(c, err)
		return
	}
	ctrlcommon.Success(c, ch)
}

func GetChannelKey(c gin.Context) {
	id, ok := ctrlcommon.ParamID(c)
	if !ok {
		return
	}
	key, err := channelsvc.GetChannelKey(c.GetInt("id"), id)
	if err != nil {
		handleChannelErr(c, err)
		return
	}
	ctrlcommon.Success(c, gin.H{"key": key})
}

func AddChannel(c gin.Context) {
	var req channelsvc.AddChannelInput
	if err := c.ShouldBindJSON(&req); err != nil || req.Channel == nil {
		ctrlcommon.ErrorMsg(c, "invalid request")
		return
	}
	ids, err := channelsvc.AddChannels(req)
	if err != nil {
		handleChannelErr(c, err)
		return
	}
	ctrlcommon.Success(c, gin.H{"count": len(ids), "ids": ids})
}

func UpdateChannel(c gin.Context) {
	var req channelsvc.PatchChannelInput
	if err := c.ShouldBindJSON(&req); err != nil {
		ctrlcommon.ErrorMsg(c, "invalid request: "+err.Error())
		return
	}
	ch, err := channelsvc.UpdateChannel(req)
	if err != nil {
		handleChannelErr(c, err)
		return
	}
	ctrlcommon.Success(c, ch)
}

func DeleteChannel(c gin.Context) {
	id, ok := ctrlcommon.ParamID(c)
	if !ok {
		return
	}
	if err := channelsvc.DeleteChannel(id); err != nil {
		handleChannelErr(c, err)
		return
	}
	ctrlcommon.OK(c)
}

func DeleteChannelBatch(c gin.Context) {
	var req ctrlcommon.IdsRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Ids) == 0 {
		ctrlcommon.ErrorMsg(c, "ids is required")
		return
	}
	if err := channelsvc.DeleteChannelBatch(req.Ids); err != nil {
		handleChannelErr(c, err)
		return
	}
	ctrlcommon.Success(c, len(req.Ids))
}

func CopyChannel(c gin.Context) {
	id, ok := ctrlcommon.ParamID(c)
	if !ok {
		return
	}
	resetBalance := true
	if v, err := strconv.ParseBool(c.DefaultQuery("reset_balance", "true")); err == nil {
		resetBalance = v
	}
	cloneID, err := channelsvc.CopyChannel(id, c.DefaultQuery("suffix", "_复制"), resetBalance)
	if err != nil {
		handleChannelErr(c, err)
		return
	}
	ctrlcommon.Success(c, gin.H{"id": cloneID})
}

func DeleteDisabledChannel(c gin.Context) {
	rows, err := channelsvc.DeleteDisabledChannels()
	if err != nil {
		handleChannelErr(c, err)
		return
	}
	ctrlcommon.Success(c, rows)
}
