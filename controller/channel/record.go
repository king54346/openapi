package channel

import (
	"strings"

	"openapi/common"
	ctrlcommon "openapi/controller/common"
	"openapi/model"
	channelsvc "openapi/service/channel"

	gin "github.com/king54346/gin-tiny"
)

func testRecordFilterFromQuery(c gin.Context) model.ChannelTestRecordFilter {
	filter := model.ChannelTestRecordFilter{
		ChannelId:  common.String2Int(c.Query("channel_id")),
		ModelName:  strings.TrimSpace(c.Query("model_name")),
		TestType:   strings.TrimSpace(c.Query("test_type")),
		ChannelTag: strings.TrimSpace(c.Query("channel_tag")),
		StartTime:  int64(common.String2Int(c.Query("start_time"))),
		EndTime:    int64(common.String2Int(c.Query("end_time"))),
	}
	if status := c.Query("status"); status != "" {
		parsed := common.String2Int(status)
		filter.Status = &parsed
	}
	return filter
}

func GetChannelTestRecords(c gin.Context) {
	page := common.GetPageQuery(c)
	records, total, err := channelsvc.ListTestRecords(testRecordFilterFromQuery(c), page.GetStartIdx(), page.GetPageSize())
	if err != nil {
		ctrlcommon.Error(c, err)
		return
	}
	ctrlcommon.Success(c, ctrlcommon.PageResult(page, records, total))
}

func DeleteChannelTestRecords(c gin.Context) {
	deleted, err := channelsvc.DeleteTestRecords(testRecordFilterFromQuery(c))
	if err != nil {
		handleChannelErr(c, err)
		return
	}
	ctrlcommon.Success(c, deleted)
}

func ClearChannelTestRecords(c gin.Context) {
	deleted, err := channelsvc.ClearTestRecords()
	if err != nil {
		ctrlcommon.Error(c, err)
		return
	}
	ctrlcommon.Success(c, deleted)
}

func RetestChannelModel(c gin.Context) {
	id, ok := ctrlcommon.ParamID(c)
	if !ok {
		return
	}
	record, err := channelsvc.RetestChannelModel(id)
	if err != nil {
		handleChannelErr(c, err)
		return
	}
	ctrlcommon.Success(c, record)
}

func BatchRetestChannelModels(c gin.Context) {
	var req ctrlcommon.IdsRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Ids) == 0 {
		ctrlcommon.ErrorMsg(c, "ids is required")
		return
	}
	total, passed, failed := channelsvc.BatchRetestChannelModels(req.Ids)
	ctrlcommon.Success(c, gin.H{"total": total, "passed": passed, "failed": failed})
}

func RetestFilteredRecords(c gin.Context) {
	var req struct {
		ChannelId  int    `json:"channel_id"`
		ModelName  string `json:"model_name"`
		Status     *int   `json:"status"`
		TestType   string `json:"test_type"`
		ChannelTag string `json:"channel_tag"`
	}
	_ = c.ShouldBindJSON(&req)
	filter := model.ChannelTestRecordFilter{
		ChannelId:  req.ChannelId,
		ModelName:  strings.TrimSpace(req.ModelName),
		Status:     req.Status,
		TestType:   strings.TrimSpace(req.TestType),
		ChannelTag: strings.TrimSpace(req.ChannelTag),
	}
	total, passed, failed, err := channelsvc.RetestFilteredRecords(filter)
	if err != nil {
		handleChannelErr(c, err)
		return
	}
	ctrlcommon.Success(c, gin.H{"total": total, "passed": passed, "failed": failed})
}
