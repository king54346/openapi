package model

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"openapi/common"
	"openapi/constant"
	"openapi/dto"
	relaycommon "openapi/relay/common"

	"gorm.io/gorm"
)

type TaskStatus string

const (
	TaskStatusNotStart TaskStatus = "NOT_START"
	// 以下为无类型常量，既可赋给 TaskStatus，也可赋给 TaskInfo.Status 这类 string 字段
	TaskStatusSubmitted  = "SUBMITTED"
	TaskStatusQueued     = "QUEUED"
	TaskStatusInProgress = "IN_PROGRESS"
	TaskStatusFailure    = "FAILURE"
	TaskStatusSuccess    = "SUCCESS"
	TaskStatusUnknown    = "UNKNOWN"
)

func (t TaskStatus) ToVideoStatus() string {
	switch t {
	case TaskStatusQueued, TaskStatusSubmitted:
		return dto.VideoStatusQueued
	case TaskStatusInProgress:
		return dto.VideoStatusInProgress
	case TaskStatusSuccess:
		return dto.VideoStatusCompleted
	case TaskStatusFailure:
		return dto.VideoStatusFailed
	default:
		return dto.VideoStatusUnknown
	}
}

// IsFinished 任务是否已经结束（成功或失败），结束后不再需要向上游查询
func (t TaskStatus) IsFinished() bool {
	return t == TaskStatusSuccess || t == TaskStatusFailure
}

type Task struct {
	ID         int64                 `json:"id" gorm:"primary_key;AUTO_INCREMENT"`
	CreatedAt  int64                 `json:"created_at" gorm:"index"`
	UpdatedAt  int64                 `json:"updated_at"`
	TaskID     string                `json:"task_id" gorm:"type:varchar(191);index"` // 上游返回的任务 ID
	Platform   constant.TaskPlatform `json:"platform" gorm:"type:varchar(30);index"`
	UserId     int                   `json:"user_id" gorm:"index"`
	Group      string                `json:"group" gorm:"type:varchar(50)"`
	ChannelId  int                   `json:"channel_id" gorm:"index"`
	Quota      int                   `json:"quota"`                                // 预留给计费使用，转发本身不写入
	Action     string                `json:"action" gorm:"type:varchar(40);index"` // 任务类型，如 generate、remixGenerate
	Status     TaskStatus            `json:"status" gorm:"type:varchar(20);index"`
	FailReason string                `json:"fail_reason"` // 失败原因；成功时历史上也用来存放结果 URL
	SubmitTime int64                 `json:"submit_time" gorm:"index"`
	StartTime  int64                 `json:"start_time" gorm:"index"`
	FinishTime int64                 `json:"finish_time" gorm:"index"`
	Progress   string                `json:"progress" gorm:"type:varchar(20);index"`
	Properties Properties            `json:"properties" gorm:"type:json"`
	// 禁止返回给用户，内部可能包含 key 等隐私信息
	PrivateData TaskPrivateData `json:"-" gorm:"column:private_data;type:json"`
	Data        json.RawMessage `json:"data" gorm:"type:json"`
}

func (t *Task) SetData(data any) {
	b, _ := common.Marshal(data)
	t.Data = b
}

func (t *Task) GetData(v any) error {
	return common.Unmarshal(t.Data, v)
}

type Properties struct {
	Input             string `json:"input"`
	UpstreamModelName string `json:"upstream_model_name,omitempty"`
	OriginModelName   string `json:"origin_model_name,omitempty"`
}

func (m *Properties) Scan(val interface{}) error {
	bytesValue, err := scanJSONBytes(val)
	if err != nil {
		return err
	}
	if len(bytesValue) == 0 {
		*m = Properties{}
		return nil
	}
	return common.Unmarshal(bytesValue, m)
}

func (m Properties) Value() (driver.Value, error) {
	if m == (Properties{}) {
		return nil, nil
	}
	return common.Marshal(m)
}

type TaskPrivateData struct {
	Key       string `json:"key,omitempty"`        // 提交任务时使用的渠道 key，查询时优先使用
	ResultURL string `json:"result_url,omitempty"` // 任务成功后的结果 URL（视频地址等）
}

func (p *TaskPrivateData) Scan(val interface{}) error {
	bytesValue, err := scanJSONBytes(val)
	if err != nil {
		return err
	}
	if len(bytesValue) == 0 {
		return nil
	}
	return common.Unmarshal(bytesValue, p)
}

func (p TaskPrivateData) Value() (driver.Value, error) {
	if p == (TaskPrivateData{}) {
		return nil, nil
	}
	return common.Marshal(p)
}

// GetResultURL 获取任务结果 URL，旧数据回退到 FailReason
func (t *Task) GetResultURL() string {
	if t.PrivateData.ResultURL != "" {
		return t.PrivateData.ResultURL
	}
	return t.FailReason
}

func InitTask(platform constant.TaskPlatform, relayInfo *relaycommon.RelayInfo) *Task {
	properties := Properties{}
	privateData := TaskPrivateData{}
	channelId := 0
	if relayInfo.ChannelMeta != nil {
		channelId = relayInfo.ChannelId
		privateData.Key = relayInfo.ApiKey
		properties.UpstreamModelName = relayInfo.UpstreamModelName
	}
	properties.OriginModelName = relayInfo.OriginModelName

	return &Task{
		UserId:      relayInfo.UserId,
		Group:       relayInfo.UsingGroup,
		SubmitTime:  time.Now().Unix(),
		Status:      TaskStatusNotStart,
		Progress:    "0%",
		ChannelId:   channelId,
		Platform:    platform,
		Properties:  properties,
		PrivateData: privateData,
	}
}

func GetByTaskId(userId int, taskId string) (*Task, bool, error) {
	if taskId == "" {
		return nil, false, nil
	}
	var task Task
	err := DB.Where("user_id = ? and task_id = ?", userId, taskId).First(&task).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &task, true, nil
}

func GetByTaskIds(userId int, taskIds []any) ([]*Task, error) {
	if len(taskIds) == 0 {
		return nil, nil
	}
	var tasks []*Task
	err := DB.Where("user_id = ? and task_id in (?)", userId, taskIds).Find(&tasks).Error
	if err != nil {
		return nil, err
	}
	return tasks, nil
}

func (t *Task) Insert() error {
	return DB.Create(t).Error
}

func (t *Task) Update() error {
	return DB.Save(t).Error
}

func (t *Task) ToOpenAIVideo() *dto.OpenAIVideo {
	openAIVideo := dto.NewOpenAIVideo()
	openAIVideo.ID = t.TaskID
	openAIVideo.Status = t.Status.ToVideoStatus()
	openAIVideo.Model = t.Properties.OriginModelName
	openAIVideo.SetProgressStr(t.Progress)
	openAIVideo.CreatedAt = t.CreatedAt
	openAIVideo.CompletedAt = t.UpdatedAt
	openAIVideo.SetMetadata("url", t.GetResultURL())
	return openAIVideo
}
