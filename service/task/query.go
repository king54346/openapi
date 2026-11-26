package task

import (
	"openapi/model"
)

func ListAllTasks(startIdx, pageSize int, params model.SyncTaskQueryParams) ([]*model.Task, int64) {
	items := model.TaskGetAllTasks(startIdx, pageSize, params)
	total := model.TaskCountAllTasks(params)
	return items, total
}

func ListUserTasks(userID, startIdx, pageSize int, params model.SyncTaskQueryParams) ([]*model.Task, int64) {
	items := model.TaskGetAllUserTask(userID, startIdx, pageSize, params)
	total := model.TaskCountAllUserTask(userID, params)
	return items, total
}
