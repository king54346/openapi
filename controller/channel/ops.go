package channel

import (
	"strings"

	ctrlcommon "openapi/controller/common"
	channelsvc "openapi/service/channel"

	gin "github.com/king54346/gin-tiny"
)

func FetchUpstreamModels(c gin.Context) {
	id, ok := ctrlcommon.ParamID(c)
	if !ok {
		return
	}
	ids, err := channelsvc.FetchUpstreamModels(id)
	if err != nil {
		handleChannelErr(c, err)
		return
	}
	ctrlcommon.Success(c, ids)
}

func FetchModels(c gin.Context) {
	var req channelsvc.FetchModelsInput
	if err := c.ShouldBindJSON(&req); err != nil {
		ctrlcommon.ErrorMsg(c, "invalid request")
		return
	}
	ids, err := channelsvc.FetchModels(req)
	if err != nil {
		handleChannelErr(c, err)
		return
	}
	ctrlcommon.Success(c, ids)
}

func RemoveModelFromChannel(c gin.Context) {
	id, ok := ctrlcommon.ParamID(c)
	if !ok {
		return
	}
	modelName := strings.TrimSpace(c.Query("model"))
	remaining, err := channelsvc.RemoveModelFromChannel(id, modelName)
	if err != nil {
		handleChannelErr(c, err)
		return
	}
	ctrlcommon.Success(c, gin.H{"models": remaining, "removed": modelName})
}

func FixChannelsAbilities(c gin.Context) {
	fixed, total, err := channelsvc.FixChannelsAbilities()
	if err != nil {
		handleChannelErr(c, err)
		return
	}
	ctrlcommon.Success(c, gin.H{"fixed": fixed, "total": total})
}

func UpdateChannelBalance(c gin.Context) {
	id, ok := ctrlcommon.ParamID(c)
	if !ok {
		return
	}
	balance, err := channelsvc.UpdateChannelBalance(id)
	if err != nil {
		handleChannelErr(c, err)
		return
	}
	ctrlcommon.Success(c, gin.H{"channel_id": id, "balance": balance})
}

func UpdateAllChannelsBalance(c gin.Context) {
	res, err := channelsvc.UpdateAllChannelsBalance()
	if err != nil {
		handleChannelErr(c, err)
		return
	}
	ctrlcommon.Success(c, res)
}
