package controller

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"openapi/common"
	"openapi/constant"
	"openapi/logger"
	"openapi/model"
	"openapi/relay"

	gin "github.com/king54346/gin-tiny"
)

// taskPollInterval 后台轮询未完成任务的间隔
const taskPollInterval = 15 * time.Second

// UpdateTaskBulk 后台轮询未完成的异步任务（视频等），向上游同步状态。阻塞运行，需在 goroutine 中调用。
// 只同步状态与结果，不涉及计费（额度补扣/退还由计费钩子负责）。
func UpdateTaskBulk() {
	for {
		time.Sleep(taskPollInterval)
		updateUnfinishedTasks(context.Background())
	}
}

// updateUnfinishedTasks 执行一轮同步：先处理无效与超时任务，再按平台、渠道逐个向上游查询。
func updateUnfinishedTasks(ctx context.Context) {
	tasks := model.GetAllUnFinishSyncTasks(constant.TaskQueryLimit)
	if len(tasks) == 0 {
		return
	}

	var invalidIds, timeoutIds []int64
	pending := make(map[constant.TaskPlatform]map[int][]*model.Task) // platform -> channel -> tasks
	timeoutBefore := time.Now().Add(-time.Duration(constant.TaskTimeoutMinutes) * time.Minute).Unix()
	for _, task := range tasks {
		switch {
		case task.TaskID == "":
			// 提交失败、没有拿到上游任务 ID 的任务无法查询
			invalidIds = append(invalidIds, task.ID)
		case constant.TaskTimeoutMinutes > 0 && task.SubmitTime > 0 && task.SubmitTime < timeoutBefore:
			timeoutIds = append(timeoutIds, task.ID)
		default:
			if pending[task.Platform] == nil {
				pending[task.Platform] = make(map[int][]*model.Task)
			}
			pending[task.Platform][task.ChannelId] = append(pending[task.Platform][task.ChannelId], task)
		}
	}
	failTasksByID(ctx, invalidIds, "task submission failed: no upstream task id")
	failTasksByID(ctx, timeoutIds, fmt.Sprintf("task timed out after %d minutes", constant.TaskTimeoutMinutes))

	for platform, byChannel := range pending {
		UpdateVideoTaskAll(ctx, platform, byChannel)
	}
}

func failTasksByID(ctx context.Context, ids []int64, reason string) {
	if len(ids) == 0 {
		return
	}
	err := model.TaskBulkUpdateByID(ids, map[string]any{
		"status":      model.TaskStatusFailure,
		"progress":    "100%",
		"fail_reason": reason,
		"finish_time": time.Now().Unix(),
	})
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("failed to mark tasks %v as failed: %v", ids, err))
		return
	}
	logger.LogInfo(ctx, fmt.Sprintf("marked %d tasks as failed (%s)", len(ids), reason))
}

// UpdateVideoTaskAll 同步某平台下各渠道的未完成任务。
func UpdateVideoTaskAll(ctx context.Context, platform constant.TaskPlatform, byChannel map[int][]*model.Task) {
	for channelId, tasks := range byChannel {
		logger.LogInfo(ctx, fmt.Sprintf("channel #%d (platform %s) pending tasks: %d", channelId, platform, len(tasks)))
		for _, task := range tasks {
			err := relay.SyncTaskFromUpstream(task)
			if err == nil {
				continue
			}
			if errors.Is(err, relay.ErrTaskChannelNotFound) {
				// 渠道已删除，该渠道下的任务都无法再查询
				ids := make([]int64, 0, len(tasks))
				for _, t := range tasks {
					ids = append(ids, t.ID)
				}
				failTasksByID(ctx, ids, fmt.Sprintf("channel #%d no longer exists", channelId))
				break
			}
			logger.LogError(ctx, fmt.Sprintf("failed to update task %s: %v", task.TaskID, err))
		}
	}
}

// parseTaskQuery 解析任务列表的筛选参数。
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

// GetAllTask 管理员查询所有任务，额外支持 channel_id 筛选。
func GetAllTask(c gin.Context) {
	page := common.GetPageQuery(c)
	params := parseTaskQuery(c)
	params.ChannelID = c.Query("channel_id")
	items := model.TaskGetAllTasks(page.GetStartIdx(), page.GetPageSize(), params)
	total := model.TaskCountAllTasks(params)
	apiSuccess(c, pageResult(page, items, total))
}

// GetUserTask 用户查询自己的任务。
func GetUserTask(c gin.Context) {
	page := common.GetPageQuery(c)
	userId := c.GetInt("id")
	params := parseTaskQuery(c)
	items := model.TaskGetAllUserTask(userId, page.GetStartIdx(), page.GetPageSize(), params)
	total := model.TaskCountAllUserTask(userId, params)
	apiSuccess(c, pageResult(page, items, total))
}

// taskPollingEnabled 是否启动后台任务轮询（多实例部署时只需一个实例开启）。
func taskPollingEnabled() bool {
	return common.GetEnvOrDefaultBool("UPDATE_TASK", true)
}

// StartTaskPolling 按配置启动后台任务轮询。
func StartTaskPolling() {
	if !taskPollingEnabled() {
		common.SysLog("task polling disabled (UPDATE_TASK=false)")
		return
	}
	go UpdateTaskBulk()
	common.SysLog(fmt.Sprintf("task polling started, interval %s", taskPollInterval))
}
