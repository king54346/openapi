package task

import (
	"strconv"

	"openapi/common"
	"openapi/constant"
	ctrlcommon "openapi/controller/common"
	"openapi/model"
	tasksvc "openapi/service/task"

	gin "github.com/king54346/gin-tiny"
)

func parseTaskQuery(c gin.Context) model.SyncTaskQueryParams {
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	return model.SyncTaskQueryParams{
		Platform:       constant.TaskPlatform(c.Query("platform")),
		TaskID:         c.Query("task_id"),
		Status:         c.Query("status"),
		Action:         c.Query("action"),
		StartTimestamp: startTimestamp,
		EndTimestamp:   endTimestamp,
	}
}

func GetAllTask(c gin.Context) {
	page := common.GetPageQuery(c)
	params := parseTaskQuery(c)
	params.ChannelID = c.Query("channel_id")
	items, total := tasksvc.ListAllTasks(page.GetStartIdx(), page.GetPageSize(), params)
	ctrlcommon.Success(c, ctrlcommon.PageResult(page, items, total))
}

func GetUserTask(c gin.Context) {
	page := common.GetPageQuery(c)
	params := parseTaskQuery(c)
	items, total := tasksvc.ListUserTasks(c.GetInt("id"), page.GetStartIdx(), page.GetPageSize(), params)
	ctrlcommon.Success(c, ctrlcommon.PageResult(page, items, total))
}
