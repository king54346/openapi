package relay

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"openapi/common"
	"openapi/constant"
	"openapi/dto"
	"openapi/model"
	"openapi/relay/channel"
	relaycommon "openapi/relay/common"
	relayconstant "openapi/relay/constant"
	"openapi/service"

	gin "github.com/king54346/gin-tiny"
)

/*
Task 任务通过平台、Action 区分任务
*/
func RelayTaskSubmit(c gin.Context, info *relaycommon.RelayInfo) (taskErr *dto.TaskError) {
	info.InitChannelMeta(c)
	// 确保 TaskRelayInfo 已初始化，避免访问内嵌字段时空指针
	if info.TaskRelayInfo == nil {
		info.TaskRelayInfo = &relaycommon.TaskRelayInfo{}
	}
	path := c.Request().URL.Path
	if strings.Contains(path, "/v1/videos/") && strings.HasSuffix(path, "/remix") {
		info.Action = constant.TaskActionRemix
	}

	// 提取 remix 任务的 video_id
	if info.Action == constant.TaskActionRemix {
		videoID := c.Param("video_id")
		if strings.TrimSpace(videoID) == "" {
			return service.TaskErrorWrapperLocal(fmt.Errorf("video_id is required"), "invalid_request", http.StatusBadRequest)
		}
		info.OriginTaskID = videoID
	}

	platform := constant.TaskPlatform(c.GetString("platform"))

	// 基于原始任务（如 remix）时，沿用原任务的模型和渠道
	if info.OriginTaskID != "" {
		originTask, exist, err := model.GetByTaskId(info.UserId, info.OriginTaskID)
		if err != nil {
			return service.TaskErrorWrapper(err, "get_origin_task_failed", http.StatusInternalServerError)
		}
		if !exist {
			return service.TaskErrorWrapperLocal(errors.New("task_origin_not_exist"), "task_not_exist", http.StatusBadRequest)
		}
		if info.OriginModelName == "" {
			if originTask.Properties.OriginModelName != "" {
				info.OriginModelName = originTask.Properties.OriginModelName
			} else if originTask.Properties.UpstreamModelName != "" {
				info.OriginModelName = originTask.Properties.UpstreamModelName
			} else {
				var taskData map[string]interface{}
				_ = common.Unmarshal(originTask.Data, &taskData)
				if m, ok := taskData["model"].(string); ok && m != "" {
					info.OriginModelName = m
					platform = originTask.Platform
				}
			}
		}
		if originTask.ChannelId != info.ChannelId {
			channelModel, err := model.GetChannelById(originTask.ChannelId, true)
			if err != nil {
				return service.TaskErrorWrapperLocal(err, "channel_not_found", http.StatusBadRequest)
			}
			if channelModel.Status != common.ChannelStatusEnabled {
				return service.TaskErrorWrapperLocal(errors.New("the channel of the origin task is disabled"), "task_channel_disable", http.StatusBadRequest)
			}
			key, _, apiErr := channelModel.GetNextEnabledKey()
			if apiErr != nil {
				return service.TaskErrorWrapper(apiErr, "channel_no_available_key", apiErr.StatusCode)
			}
			common.SetContextKey(c, constant.ContextKeyChannelKey, key)
			common.SetContextKey(c, constant.ContextKeyChannelType, channelModel.Type)
			common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, channelModel.GetBaseURL())
			common.SetContextKey(c, constant.ContextKeyChannelId, originTask.ChannelId)

			info.ChannelBaseUrl = channelModel.GetBaseURL()
			info.ChannelId = originTask.ChannelId
			info.ChannelType = channelModel.Type
			info.ApiKey = key
			platform = originTask.Platform
		}
	}
	if platform == "" {
		platform = GetTaskPlatform(c)
	}

	info.InitChannelMeta(c)
	adaptor := GetTaskAdaptor(platform)
	if adaptor == nil {
		return service.TaskErrorWrapperLocal(fmt.Errorf("invalid api platform: %s", platform), "invalid_api_platform", http.StatusBadRequest)
	}
	adaptor.Init(info)
	// 获取并校验任务请求
	if taskErr = adaptor.ValidateRequestAndSetAction(c, info); taskErr != nil {
		return
	}

	modelName := info.OriginModelName
	if modelName == "" {
		modelName = service.CoverTaskActionToModelName(platform, info.Action)
	}

	// auto 分组时，Distribute 中间件会把实际选中的分组放到 ContextKeyAutoGroup
	if autoGroup, exists := common.GetContextKey(c, constant.ContextKeyAutoGroup); exists {
		if groupStr, ok := autoGroup.(string); ok && groupStr != "" {
			info.UsingGroup = groupStr
		}
	}

	requestBody, err := adaptor.BuildRequestBody(c, info)
	if err != nil {
		return service.TaskErrorWrapper(err, "build_request_failed", http.StatusInternalServerError)
	}
	resp, err := adaptor.DoRequest(c, info, requestBody)
	if err != nil {
		return service.TaskErrorWrapper(err, "do_request_failed", http.StatusInternalServerError)
	}
	if resp != nil && resp.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(resp.Body)
		service.CloseResponseBodyGracefully(resp)
		return service.TaskErrorWrapper(fmt.Errorf("%s", string(responseBody)), "fail_to_fetch_task", resp.StatusCode)
	}

	taskID, taskData, taskErr := adaptor.DoResponse(c, resp, info)
	if taskErr != nil {
		return
	}

	task := model.InitTask(platform, info)
	task.TaskID = taskID
	task.Data = taskData
	task.Action = info.Action
	if err = task.Insert(); err != nil {
		return service.TaskErrorWrapper(err, "insert_task_failed", http.StatusInternalServerError)
	}

	info.OriginModelName = modelName
	recordUsage(c, info, &dto.Usage{}, map[string]any{
		"action":  info.Action,
		"task_id": taskID,
	})
	return nil
}

var fetchRespBuilders = map[int]func(c gin.Context) (respBody []byte, taskResp *dto.TaskError){
	relayconstant.RelayModeVideoFetchByID: videoFetchByIDRespBodyBuilder,
}

func RelayTaskFetch(c gin.Context, relayMode int) (taskResp *dto.TaskError) {
	respBuilder, ok := fetchRespBuilders[relayMode]
	if !ok {
		return service.TaskErrorWrapperLocal(errors.New("invalid_relay_mode"), "invalid_relay_mode", http.StatusBadRequest)
	}

	respBody, taskErr := respBuilder(c)
	if taskErr != nil {
		return taskErr
	}
	if len(respBody) == 0 {
		respBody = []byte("{\"code\":\"success\",\"data\":null}")
	}

	c.Response().Header().Set("Content-Type", "application/json")
	if _, err := io.Copy(c.Response(), bytes.NewBuffer(respBody)); err != nil {
		return service.TaskErrorWrapper(err, "copy_response_body_failed", http.StatusInternalServerError)
	}
	return nil
}

func videoFetchByIDRespBodyBuilder(c gin.Context) (respBody []byte, taskResp *dto.TaskError) {
	taskId := c.Param("task_id")
	if taskId == "" {
		taskId = c.GetString("task_id")
	}
	userId := c.GetInt("id")

	originTask, exist, err := model.GetByTaskId(userId, taskId)
	if err != nil {
		return nil, service.TaskErrorWrapper(err, "get_task_failed", http.StatusInternalServerError)
	}
	if !exist {
		return nil, service.TaskErrorWrapperLocal(errors.New("task_not_exist"), "task_not_exist", http.StatusBadRequest)
	}

	// 没有后台轮询，未结束的任务在查询时向上游同步一次最新状态；同步失败时返回库中已有的状态
	if !originTask.Status.IsFinished() {
		if err := syncTaskFromUpstream(c, originTask); err != nil {
			common.SysLog(fmt.Sprintf("sync task %s from upstream failed: %v", originTask.TaskID, err))
		}
	}

	// /v1/videos/{task_id} 返回 OpenAI Video 格式；/v1/video/generations/{task_id} 返回任务原始数据
	if strings.HasPrefix(c.Request().RequestURI, "/v1/videos/") {
		adaptor := GetTaskAdaptor(originTask.Platform)
		if adaptor == nil {
			return nil, service.TaskErrorWrapperLocal(fmt.Errorf("invalid channel id: %d", originTask.ChannelId), "invalid_channel_id", http.StatusBadRequest)
		}
		converter, ok := adaptor.(channel.OpenAIVideoConverter)
		if !ok {
			return nil, service.TaskErrorWrapperLocal(fmt.Errorf("not_implemented:%s", originTask.Platform), "not_implemented", http.StatusNotImplemented)
		}
		openAIVideoData, err := converter.ConvertToOpenAIVideo(originTask)
		if err != nil {
			return nil, service.TaskErrorWrapper(err, "convert_to_openai_video_failed", http.StatusInternalServerError)
		}
		return openAIVideoData, nil
	}

	respBody, err = common.Marshal(dto.TaskResponse[any]{
		Code: "success",
		Data: TaskModel2Dto(originTask),
	})
	if err != nil {
		return nil, service.TaskErrorWrapper(err, "marshal_response_failed", http.StatusInternalServerError)
	}
	return respBody, nil
}

// TaskModel2Dto 转成对外的任务数据（PrivateData 含渠道 key，不输出）。
func TaskModel2Dto(task *model.Task) *dto.TaskDto {
	return &dto.TaskDto{
		ID:         task.ID,
		CreatedAt:  task.CreatedAt,
		UpdatedAt:  task.UpdatedAt,
		Platform:   string(task.Platform),
		UserId:     task.UserId,
		Group:      task.Group,
		ChannelId:  task.ChannelId,
		Quota:      task.Quota,
		Properties: task.Properties,
		TaskID:     task.TaskID,
		Action:     task.Action,
		Status:     string(task.Status),
		FailReason: task.FailReason,
		ResultURL:  task.GetResultURL(),
		SubmitTime: task.SubmitTime,
		StartTime:  task.StartTime,
		FinishTime: task.FinishTime,
		Progress:   task.Progress,
		Data:       task.Data, // 上游原始数据，可能包含 id、model、status、content、usage 等
	}
}
