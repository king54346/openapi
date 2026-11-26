package task

import (
	"context"
	"errors"
	"fmt"
	"time"

	"openapi/common"
	"openapi/constant"
	"openapi/logger"
	"openapi/model"
	"openapi/relay"
)

const taskPollInterval = 15 * time.Second

func StartTaskPolling() {
	if !common.GetEnvOrDefaultBool("UPDATE_TASK", common.IsMasterNode) {
		common.SysLog("task polling disabled (UPDATE_TASK=false or slave node)")
		return
	}
	go UpdateTaskBulk()
	common.SysLog(fmt.Sprintf("task polling started, interval %s", taskPollInterval))
}

func UpdateTaskBulk() {
	for {
		time.Sleep(taskPollInterval)
		UpdateUnfinishedTasks(context.Background())
	}
}

func UpdateUnfinishedTasks(ctx context.Context) {
	tasks := model.GetAllUnFinishSyncTasks(constant.TaskQueryLimit)
	if len(tasks) == 0 {
		return
	}

	var invalidIds, timeoutIds []int64
	pending := make(map[constant.TaskPlatform]map[int][]*model.Task)
	timeoutBefore := time.Now().Add(-time.Duration(constant.TaskTimeoutMinutes) * time.Minute).Unix()
	for _, t := range tasks {
		switch {
		case t.TaskID == "":
			invalidIds = append(invalidIds, t.ID)
		case constant.TaskTimeoutMinutes > 0 && t.SubmitTime > 0 && t.SubmitTime < timeoutBefore:
			timeoutIds = append(timeoutIds, t.ID)
		default:
			if pending[t.Platform] == nil {
				pending[t.Platform] = make(map[int][]*model.Task)
			}
			pending[t.Platform][t.ChannelId] = append(pending[t.Platform][t.ChannelId], t)
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

func UpdateVideoTaskAll(ctx context.Context, platform constant.TaskPlatform, byChannel map[int][]*model.Task) {
	for channelId, tasks := range byChannel {
		logger.LogInfo(ctx, fmt.Sprintf("channel #%d (platform %s) pending tasks: %d", channelId, platform, len(tasks)))
		for _, t := range tasks {
			err := relay.SyncTaskFromUpstream(t)
			if err == nil {
				continue
			}
			if errors.Is(err, relay.ErrTaskChannelNotFound) {
				ids := make([]int64, 0, len(tasks))
				for _, item := range tasks {
					ids = append(ids, item.ID)
				}
				failTasksByID(ctx, ids, fmt.Sprintf("channel #%d no longer exists", channelId))
				break
			}
			logger.LogError(ctx, fmt.Sprintf("failed to update task %s: %v", t.TaskID, err))
		}
	}
}
