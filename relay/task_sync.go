package relay

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"openapi/common"
	"openapi/constant"
	"openapi/dto"
	"openapi/model"
	relaycommon "openapi/relay/common"
	"openapi/service"

	gin "github.com/king54346/gin-tiny"
	"gorm.io/gorm"
)

// 任务进度的默认展示值，上游未返回进度时使用
const (
	taskProgressSubmitted  = "10%"
	taskProgressQueued     = "20%"
	taskProgressInProgress = "30%"
	taskProgressComplete   = "100%"
)

// ErrTaskChannelNotFound 任务所属渠道已被删除，任务无法再向上游查询。
var ErrTaskChannelNotFound = errors.New("task channel not found")

// syncTaskFromUpstream 请求内（查询任务时）同步一次任务状态。
func syncTaskFromUpstream(_ gin.Context, task *model.Task) error {
	return SyncTaskFromUpstream(task)
}

// SyncTaskFromUpstream 向上游查询任务的最新状态并写回数据库，供查询接口与后台轮询共用。
// 只更新状态、进度、时间、结果 URL、失败原因和原始数据，不涉及计费。
func SyncTaskFromUpstream(task *model.Task) error {
	adaptor := GetTaskAdaptor(task.Platform)
	if adaptor == nil {
		return fmt.Errorf("unsupported task platform: %s", task.Platform)
	}
	ch, err := model.GetChannelById(task.ChannelId, true)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("%w: #%d", ErrTaskChannelNotFound, task.ChannelId)
		}
		return fmt.Errorf("get channel %d: %w", task.ChannelId, err)
	}

	baseURL := ch.GetBaseURL()
	if baseURL == "" {
		baseURL = constant.ChannelBaseURLs[ch.Type]
	}
	key := ch.Key
	if task.PrivateData.Key != "" {
		key = task.PrivateData.Key
	}

	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
		ChannelType:    ch.Type,
		ChannelId:      ch.Id,
		ChannelBaseUrl: baseURL,
		ApiKey:         key,
	}}
	adaptor.Init(info)

	resp, err := adaptor.FetchTask(baseURL, key, map[string]any{
		"task_id":    task.TaskID,
		"action":     task.Action,
		"model_name": task.Properties.OriginModelName,
	}, ch.GetSetting().Proxy)
	if err != nil {
		return fmt.Errorf("fetch task: %w", err)
	}
	defer service.CloseResponseBodyGracefully(resp)
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read fetch response: %w", err)
	}

	taskResult, err := adaptor.ParseTaskResult(responseBody)
	if err != nil {
		return fmt.Errorf("parse task result: %w", err)
	}
	if taskResult.Status == "" {
		// 上游返回的是错误而不是任务状态：429 视为暂时性错误，保持原状态
		var errorResult dto.GeneralErrorResponse
		if err = common.Unmarshal(responseBody, &errorResult); err == nil {
			if openaiError := errorResult.TryToOpenAIError(); openaiError != nil && fmt.Sprint(openaiError.Code) == "429" {
				return nil
			}
		}
		return errors.New("upstream returned empty task status: " + string(responseBody))
	}
	if !knownTaskStatuses[taskResult.Status] {
		return fmt.Errorf("unknown task status %q", taskResult.Status)
	}

	applyTaskResult(task, taskResult, responseBody)
	return task.Update()
}

var knownTaskStatuses = map[string]bool{
	model.TaskStatusSubmitted:  true,
	model.TaskStatusQueued:     true,
	model.TaskStatusInProgress: true,
	model.TaskStatusSuccess:    true,
	model.TaskStatusFailure:    true,
}

func applyTaskResult(task *model.Task, taskResult *relaycommon.TaskInfo, responseBody []byte) {
	now := time.Now().Unix()
	task.Data = redactVideoResponseBody(responseBody)
	task.Status = model.TaskStatus(taskResult.Status)
	switch taskResult.Status {
	case model.TaskStatusSubmitted:
		task.Progress = taskProgressSubmitted
	case model.TaskStatusQueued:
		task.Progress = taskProgressQueued
	case model.TaskStatusInProgress:
		task.Progress = taskProgressInProgress
		if task.StartTime == 0 {
			task.StartTime = now
		}
	case model.TaskStatusSuccess:
		task.Progress = taskProgressComplete
		if task.FinishTime == 0 {
			task.FinishTime = now
		}
		// data: URI（base64 视频）不适合作为结果 URL，只保留在 Data 中
		if taskResult.Url != "" && !strings.HasPrefix(taskResult.Url, "data:") {
			task.PrivateData.ResultURL = taskResult.Url
		}
	case model.TaskStatusFailure:
		task.Progress = taskProgressComplete
		if task.FinishTime == 0 {
			task.FinishTime = now
		}
		task.FailReason = taskResult.Reason
	}
	if taskResult.Progress != "" {
		task.Progress = taskResult.Progress
	}
}

// redactVideoResponseBody 去掉上游响应中内联的 base64 视频，避免把大字段写进数据库。
func redactVideoResponseBody(body []byte) []byte {
	var m map[string]any
	if err := common.Unmarshal(body, &m); err != nil {
		return body
	}
	resp, _ := m["response"].(map[string]any)
	if resp == nil {
		return body
	}
	delete(resp, "bytesBase64Encoded")
	if v, ok := resp["video"].(string); ok {
		resp["video"] = truncateBase64(v)
	}
	if vs, ok := resp["videos"].([]any); ok {
		for i := range vs {
			if vm, ok := vs[i].(map[string]any); ok {
				delete(vm, "bytesBase64Encoded")
			}
		}
	}
	b, err := common.Marshal(m)
	if err != nil {
		return body
	}
	return b
}

func truncateBase64(s string) string {
	const maxKeep = 256
	if len(s) <= maxKeep {
		return s
	}
	return s[:maxKeep] + "..."
}
