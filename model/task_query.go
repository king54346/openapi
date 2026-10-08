package model

import (
	"openapi/constant"

	"gorm.io/gorm"
)

// defaultUnfinishedTaskLimit limit 未配置（<=0）时每轮最多处理的任务数
const defaultUnfinishedTaskLimit = 1000

// GetAllUnFinishSyncTasks 取未结束的任务（按 id 升序），供后台轮询同步状态。
func GetAllUnFinishSyncTasks(limit int) []*Task {
	if limit <= 0 {
		limit = defaultUnfinishedTaskLimit
	}
	var tasks []*Task
	err := DB.Where("status NOT IN ?", []TaskStatus{TaskStatusSuccess, TaskStatusFailure}).
		Order("id").Limit(limit).Find(&tasks).Error
	if err != nil {
		return nil
	}
	return tasks
}

// TaskBulkUpdate 按上游任务 ID 批量更新。
func TaskBulkUpdate(taskIds []string, params map[string]any) error {
	if len(taskIds) == 0 {
		return nil
	}
	return DB.Model(&Task{}).Where("task_id IN ?", taskIds).Updates(params).Error
}

// TaskBulkUpdateByID 按主键批量更新。
func TaskBulkUpdateByID(ids []int64, params map[string]any) error {
	if len(ids) == 0 {
		return nil
	}
	return DB.Model(&Task{}).Where("id IN ?", ids).Updates(params).Error
}

// SyncTaskQueryParams 任务列表的筛选条件，零值表示不筛选。
type SyncTaskQueryParams struct {
	Platform       constant.TaskPlatform
	ChannelID      string
	TaskID         string
	UserID         string
	Action         string
	Status         string
	StartTimestamp int64
	EndTimestamp   int64
}

func taskListQuery(params SyncTaskQueryParams) *gorm.DB {
	query := DB.Model(&Task{})
	if params.ChannelID != "" {
		query = query.Where("channel_id = ?", params.ChannelID)
	}
	if params.Platform != "" {
		query = query.Where("platform = ?", params.Platform)
	}
	if params.UserID != "" {
		query = query.Where("user_id = ?", params.UserID)
	}
	if params.TaskID != "" {
		query = query.Where("task_id = ?", params.TaskID)
	}
	if params.Action != "" {
		query = query.Where("action = ?", params.Action)
	}
	if params.Status != "" {
		query = query.Where("status = ?", params.Status)
	}
	if params.StartTimestamp != 0 {
		query = query.Where("submit_time >= ?", params.StartTimestamp)
	}
	if params.EndTimestamp != 0 {
		query = query.Where("submit_time <= ?", params.EndTimestamp)
	}
	return query
}

// TaskGetAllTasks 管理员分页查询任务（按 id 倒序）。
func TaskGetAllTasks(startIdx, num int, params SyncTaskQueryParams) []*Task {
	var tasks []*Task
	if err := taskListQuery(params).Order("id desc").Limit(num).Offset(startIdx).Find(&tasks).Error; err != nil {
		return nil
	}
	return tasks
}

// TaskCountAllTasks 管理员查询的任务总数。
func TaskCountAllTasks(params SyncTaskQueryParams) int64 {
	var total int64
	_ = taskListQuery(params).Count(&total).Error
	return total
}

// TaskGetAllUserTask 用户分页查询自己的任务（按 id 倒序）。
func TaskGetAllUserTask(userId, startIdx, num int, params SyncTaskQueryParams) []*Task {
	var tasks []*Task
	err := taskListQuery(params).Where("user_id = ?", userId).
		Order("id desc").Limit(num).Offset(startIdx).Find(&tasks).Error
	if err != nil {
		return nil
	}
	return tasks
}

// TaskCountAllUserTask 用户自己的任务总数。
func TaskCountAllUserTask(userId int, params SyncTaskQueryParams) int64 {
	var total int64
	_ = taskListQuery(params).Where("user_id = ?", userId).Count(&total).Error
	return total
}
