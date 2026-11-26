package channel

import (
	ctrlcommon "openapi/controller/common"
	channelsvc "openapi/service/channel"

	gin "github.com/king54346/gin-tiny"
)

func TestChannel(c gin.Context) {
	id, ok := ctrlcommon.ParamID(c)
	if !ok {
		return
	}
	_, res, err := channelsvc.TestChannelByID(id, c.Query("model"))
	if err != nil {
		handleChannelErr(c, err)
		return
	}
	data := gin.H{"model": res.Model, "path": res.Path, "time": res.Latency.Seconds()}
	switch {
	case res.Skipped:
		ctrlcommon.ErrorWithData(c, "skipped: "+res.SkipReason, data)
	case res.Err != nil:
		ctrlcommon.ErrorWithData(c, res.Err.MaskSensitiveErrorWithStatusCode(), data)
	default:
		ctrlcommon.Success(c, data)
	}
}

func TestChannelAllModels(c gin.Context) {
	id, ok := ctrlcommon.ParamID(c)
	if !ok {
		return
	}
	res, err := channelsvc.TestChannelAllModels(id)
	if err != nil {
		handleChannelErr(c, err)
		return
	}
	ctrlcommon.Success(c, res)
}

func TestAllChannels(c gin.Context) {
	count, err := channelsvc.TestAllChannels()
	if err != nil {
		handleChannelErr(c, err)
		return
	}
	ctrlcommon.Success(c, gin.H{"started": true, "channels": count})
}
