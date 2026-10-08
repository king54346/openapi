package doubao

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"openapi/common"
	"openapi/constant"
	"openapi/dto"
	"openapi/model"
	"openapi/relay/channel"
	relaycommon "openapi/relay/common"
	"openapi/service"

	gin "github.com/king54346/gin-tiny"
	"github.com/pkg/errors"
)

// ============================
// Request / Response structures
// ============================

type ContentItem struct {
	Type     string          `json:"type"`                // "text", "image_url", "video_url" or legacy "video"
	Text     string          `json:"text,omitempty"`      // for text type
	ImageURL *ImageURL       `json:"image_url,omitempty"` // for image_url type
	VideoURL *VideoReference `json:"video_url,omitempty"` // for video_url type
	Video    *VideoReference `json:"video,omitempty"`     // legacy draft video field
	Role     string          `json:"role,omitempty"`      // reference_image / reference_video / first_frame / last_frame
}

type ImageURL struct {
	URL string `json:"url"`
}

type VideoReference struct {
	URL string `json:"url"` // Draft video URL
}

type requestPayload struct {
	Model                 string         `json:"model"`
	Content               []ContentItem  `json:"content"`
	CallbackURL           string         `json:"callback_url,omitempty"`
	ReturnLastFrame       *dto.BoolValue `json:"return_last_frame,omitempty"`
	ServiceTier           string         `json:"service_tier,omitempty"`
	ExecutionExpiresAfter dto.IntValue   `json:"execution_expires_after,omitempty"`
	GenerateAudio         *dto.BoolValue `json:"generate_audio,omitempty"`
	Draft                 *dto.BoolValue `json:"draft,omitempty"`
	Resolution            string         `json:"resolution,omitempty"`
	Ratio                 string         `json:"ratio,omitempty"`
	Duration              dto.IntValue   `json:"duration,omitempty"`
	Frames                dto.IntValue   `json:"frames,omitempty"`
	Seed                  dto.IntValue   `json:"seed,omitempty"`
	CameraFixed           *dto.BoolValue `json:"camera_fixed,omitempty"`
	Watermark             *dto.BoolValue `json:"watermark,omitempty"`
	OmniReferenceTaskType string         `json:"omni_reference_task_type,omitempty"`
	OutputFormat          string         `json:"output_format,omitempty"`
}

type responsePayload struct {
	ID string `json:"id"` // task_id
}

type responseTask struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Status  string `json:"status"`
	Content struct {
		VideoURL string `json:"video_url"`
	} `json:"content"`
	Seed            int    `json:"seed"`
	Resolution      string `json:"resolution"`
	Duration        int    `json:"duration"`
	Ratio           string `json:"ratio"`
	FramesPerSecond int    `json:"framespersecond"`
	ServiceTier     string `json:"service_tier"`
	Usage           struct {
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	CreatedAt int64 `json:"created_at"`
	UpdatedAt int64 `json:"updated_at"`
}

// ============================
// Adaptor implementation
// ============================

type TaskAdaptor struct {
	ChannelType int
	apiKey      string
	baseURL     string
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
	a.baseURL = info.ChannelBaseUrl
	a.apiKey = info.ApiKey
}

// ValidateRequestAndSetAction 解析并校验请求体，设置默认 action
func (a *TaskAdaptor) ValidateRequestAndSetAction(c gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	return relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionGenerate)
}

// BuildRequestURL constructs the upstream URL.
func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	return fmt.Sprintf("%s/api/v3/contents/generations/tasks", a.baseURL), nil
}

// BuildRequestHeader sets required headers.
func (a *TaskAdaptor) BuildRequestHeader(c gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	return nil
}

// BuildRequestBody converts request into Doubao specific format.
func (a *TaskAdaptor) BuildRequestBody(c gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil, err
	}

	body, err := a.convertToRequestPayload(&req)
	if err != nil {
		return nil, errors.Wrap(err, "convert request payload failed")
	}
	info.UpstreamModelName = body.Model
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

// DoRequest delegates to common helper.
func (a *TaskAdaptor) DoRequest(c gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

// DoResponse handles upstream response, returns taskID etc.
func (a *TaskAdaptor) DoResponse(c gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}
	_ = resp.Body.Close()

	// Parse Doubao response
	var dResp responsePayload
	if err := json.Unmarshal(responseBody, &dResp); err != nil {
		taskErr = service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
		return
	}

	if dResp.ID == "" {
		taskErr = service.TaskErrorWrapper(fmt.Errorf("task_id is empty"), "invalid_response", http.StatusInternalServerError)
		return
	}

	ov := dto.NewOpenAIVideo()
	ov.ID = dResp.ID
	ov.TaskID = dResp.ID
	ov.CreatedAt = time.Now().Unix()
	ov.Model = info.OriginModelName

	c.JSON(http.StatusOK, ov)
	return dResp.ID, responseBody, nil
}

// FetchTask fetch task status
func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}

	uri := fmt.Sprintf("%s/api/v3/contents/generations/tasks/%s", baseUrl, taskID)

	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

func (a *TaskAdaptor) GetModelList() []string {
	return ModelList
}

func (a *TaskAdaptor) GetChannelName() string {
	return ChannelName
}

func (a *TaskAdaptor) convertToRequestPayload(req *relaycommon.TaskSubmitReq) (*requestPayload, error) {
	r := requestPayload{
		Model:                 req.Model,
		Content:               []ContentItem{},
		Ratio:                 req.Ratio,
		OmniReferenceTaskType: req.OmniReferenceTaskType,
		OutputFormat:          req.OutputFormat,
	}
	if req.GenerateAudio != nil {
		generateAudio := dto.BoolValue(*req.GenerateAudio)
		r.GenerateAudio = &generateAudio
	}

	// 顶层 duration/seconds 透传为上游时长，metadata 中的 duration 仍可覆盖
	if seconds := req.GetDurationSeconds(); seconds > 0 {
		r.Duration = dto.IntValue(seconds)
	}

	// 官方 content 优先；旧版 prompt/images 会转换为相同的上游结构。
	hasContentText := false
	for _, item := range req.Content {
		converted := ContentItem{
			Type: item.Type,
			Text: item.Text,
			Role: item.Role,
		}
		if item.ImageURL != nil {
			converted.ImageURL = &ImageURL{URL: item.ImageURL.URL}
			if converted.Role == "" {
				converted.Role = "reference_image"
			}
		}
		if item.VideoURL != nil {
			converted.VideoURL = &VideoReference{URL: item.VideoURL.URL}
			if converted.Role == "" {
				converted.Role = "reference_video"
			}
		}
		if item.Type == "text" && strings.TrimSpace(item.Text) != "" {
			hasContentText = true
		}
		r.Content = append(r.Content, converted)
	}

	if req.Prompt != "" && !hasContentText {
		r.Content = append(r.Content, ContentItem{
			Type: "text",
			Text: req.Prompt,
		})
	}

	if len(req.Content) == 0 {
		images := req.Images
		if len(images) == 0 && req.Image != "" {
			images = []string{req.Image}
		}
		for _, imgURL := range images {
			if strings.TrimSpace(imgURL) == "" {
				continue
			}
			r.Content = append(r.Content, ContentItem{
				Type: "image_url",
				Role: "reference_image",
				ImageURL: &ImageURL{
					URL: imgURL,
				},
			})
		}
	}

	metadata := req.Metadata
	medaBytes, err := json.Marshal(metadata)
	if err != nil {
		return nil, errors.Wrap(err, "metadata marshal metadata failed")
	}
	err = json.Unmarshal(medaBytes, &r)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal metadata failed")
	}

	return &r, nil
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	resTask := responseTask{}
	if err := json.Unmarshal(respBody, &resTask); err != nil {
		return nil, errors.Wrap(err, "unmarshal task result failed")
	}

	taskResult := relaycommon.TaskInfo{
		Code: 0,
	}

	// Map Doubao status to internal status
	switch resTask.Status {
	case "pending", "queued":
		taskResult.Status = model.TaskStatusQueued
		taskResult.Progress = "10%"
	case "processing", "running":
		taskResult.Status = model.TaskStatusInProgress
		taskResult.Progress = "50%"
	case "succeeded":
		taskResult.Status = model.TaskStatusSuccess
		taskResult.Progress = "100%"
		taskResult.Url = resTask.Content.VideoURL
		// 解析 usage 信息用于按倍率计费
		taskResult.CompletionTokens = resTask.Usage.CompletionTokens
		taskResult.TotalTokens = resTask.Usage.TotalTokens
	case "failed":
		taskResult.Status = model.TaskStatusFailure
		taskResult.Progress = "100%"
		taskResult.Reason = "task failed"
	default:
		// Unknown status, treat as processing
		taskResult.Status = model.TaskStatusInProgress
		taskResult.Progress = "30%"
	}

	return &taskResult, nil
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(originTask *model.Task) ([]byte, error) {
	var dResp responseTask
	if err := json.Unmarshal(originTask.Data, &dResp); err != nil {
		return nil, errors.Wrap(err, "unmarshal doubao task data failed")
	}

	openAIVideo := dto.NewOpenAIVideo()
	openAIVideo.ID = originTask.TaskID
	openAIVideo.TaskID = originTask.TaskID
	openAIVideo.Status = originTask.Status.ToVideoStatus()
	openAIVideo.SetProgressStr(originTask.Progress)
	openAIVideo.SetMetadata("url", dResp.Content.VideoURL)
	openAIVideo.CreatedAt = originTask.CreatedAt
	openAIVideo.CompletedAt = originTask.UpdatedAt
	openAIVideo.Model = originTask.Properties.OriginModelName

	if dResp.Status == "failed" {
		openAIVideo.Error = &dto.OpenAIVideoError{
			Message: "task failed",
			Code:    "failed",
		}
	}

	jsonData, _ := common.Marshal(openAIVideo)
	return jsonData, nil
}
