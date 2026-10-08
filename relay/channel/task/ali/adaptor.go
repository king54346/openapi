package ali

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	"openapi/common"
	"openapi/constant"
	"openapi/dto"
	"openapi/logger"
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

// AliVideoRequest 阿里通义万相视频生成请求
type AliVideoRequest struct {
	Model      string              `json:"model"`
	Input      AliVideoInput       `json:"input"`
	Parameters *AliVideoParameters `json:"parameters,omitempty"`
}

// AliVideoMediaItem wan2.7-i2v-2026 系列的媒体输入项
type AliVideoMediaItem struct {
	Type string `json:"type"` // "first_frame" / "last_frame" / "first_clip" / "driving_audio"
	URL  string `json:"url"`
}

// AliVideoInput 视频输入参数
type AliVideoInput struct {
	Prompt         string              `json:"prompt,omitempty"`          // 文本提示词
	ImgURL         string              `json:"img_url,omitempty"`         // 首帧图像URL或Base64（图生视频）
	FirstFrameURL  string              `json:"first_frame_url,omitempty"` // 首帧图片URL（首尾帧生视频）
	LastFrameURL   string              `json:"last_frame_url,omitempty"`  // 尾帧图片URL（首尾帧生视频）
	AudioURL       string              `json:"audio_url,omitempty"`       // 音频URL（wan2.5支持）
	NegativePrompt string              `json:"negative_prompt,omitempty"` // 反向提示词
	Template       string              `json:"template,omitempty"`        // 视频特效模板
	Media          []AliVideoMediaItem `json:"media,omitempty"`           // wan2.7-i2v-2026 系列媒体数组
}

// AliVideoParameters 视频参数
type AliVideoParameters struct {
	Resolution   string `json:"resolution,omitempty"`    // 分辨率: 480P/720P/1080P（图生视频、首尾帧生视频）
	Ratio        string `json:"ratio,omitempty"`         // 宽高比: 16:9/9:16/1:1（wan2.7-t2v 系列）
	Size         string `json:"size,omitempty"`          // 尺寸: 如 "832*480"（文生视频）
	Duration     int    `json:"duration,omitempty"`      // 时长: 3-15秒
	PromptExtend bool   `json:"prompt_extend,omitempty"` // 是否开启prompt智能改写
	Watermark    bool   `json:"watermark,omitempty"`     // 是否添加水印
	Audio        *bool  `json:"audio,omitempty"`         // 是否添加音频（wan2.5）
	Seed         int    `json:"seed,omitempty"`          // 随机数种子
}

// AliVideoResponse 阿里通义万相响应
type AliVideoResponse struct {
	Output    AliVideoOutput `json:"output"`
	RequestID string         `json:"request_id"`
	Code      string         `json:"code,omitempty"`
	Message   string         `json:"message,omitempty"`
	Usage     *AliUsage      `json:"usage,omitempty"`
}

// AliVideoOutput 输出信息
type AliVideoOutput struct {
	TaskID        string `json:"task_id"`
	TaskStatus    string `json:"task_status"`
	SubmitTime    string `json:"submit_time,omitempty"`
	ScheduledTime string `json:"scheduled_time,omitempty"`
	EndTime       string `json:"end_time,omitempty"`
	OrigPrompt    string `json:"orig_prompt,omitempty"`
	ActualPrompt  string `json:"actual_prompt,omitempty"`
	VideoURL      string `json:"video_url,omitempty"`
	Code          string `json:"code,omitempty"`
	Message       string `json:"message,omitempty"`
}

// AliUsage 使用统计
type AliUsage struct {
	Duration   int `json:"duration,omitempty"`
	VideoCount int `json:"video_count,omitempty"`
	SR         int `json:"SR,omitempty"`
}

type AliMetadata struct {
	// Input 相关
	AudioURL       string `json:"audio_url,omitempty"`       // 音频URL
	ImgURL         string `json:"img_url,omitempty"`         // 图片URL（图生视频）
	FirstFrameURL  string `json:"first_frame_url,omitempty"` // 首帧图片URL（首尾帧生视频）
	LastFrameURL   string `json:"last_frame_url,omitempty"`  // 尾帧图片URL（首尾帧生视频）
	NegativePrompt string `json:"negative_prompt,omitempty"` // 反向提示词
	Template       string `json:"template,omitempty"`        // 视频特效模板

	// Parameters 相关
	Resolution   *string `json:"resolution,omitempty"`    // 分辨率: 480P/720P/1080P
	Ratio        *string `json:"ratio,omitempty"`         // 宽高比: 16:9/9:16/1:1
	Size         *string `json:"size,omitempty"`          // 尺寸: 如 "832*480"
	Duration     *int    `json:"duration,omitempty"`      // 时长
	PromptExtend *bool   `json:"prompt_extend,omitempty"` // 是否开启prompt智能改写
	Watermark    *bool   `json:"watermark,omitempty"`     // 是否添加水印
	Audio        *bool   `json:"audio,omitempty"`         // 是否添加音频
	Seed         *int    `json:"seed,omitempty"`          // 随机数种子
}

// ============================
// Adaptor implementation
// ============================

type TaskAdaptor struct {
	ChannelType int
	apiKey      string
	baseURL     string
	aliReq      *AliVideoRequest
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
	a.baseURL = info.ChannelBaseUrl
	a.apiKey = info.ApiKey
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c gin.Context, info *relaycommon.RelayInfo) (taskErr *dto.TaskError) {
	// 阿里通义万相支持 JSON 格式，不使用 multipart
	var taskReq relaycommon.TaskSubmitReq
	if err := common.UnmarshalBodyReusable(c, &taskReq); err != nil {
		return service.TaskErrorWrapper(err, "unmarshal_task_request_failed", http.StatusBadRequest)
	}
	aliReq, err := a.convertToAliRequest(info, taskReq)
	if err != nil {
		return service.TaskErrorWrapper(err, "convert_to_ali_request_failed", http.StatusInternalServerError)
	}
	a.aliReq = aliReq
	logger.LogJson(c, "ali video request body", aliReq)
	if taskErr = relaycommon.ValidateMultipartDirect(c, info); taskErr != nil {
		return taskErr
	}
	// 阿里万相模型名带 i2v 一律视为图生视频，覆盖通用逻辑的兜底（防止仅靠 metadata.input.media 传图时被识别成文生）
	if strings.Contains(taskReq.Model, "i2v") {
		info.Action = constant.TaskActionGenerate
	}
	return nil
}

func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	return fmt.Sprintf("%s/api/v1/services/aigc/video-generation/video-synthesis", a.baseURL), nil
}

// BuildRequestHeader sets required headers for Ali API
func (a *TaskAdaptor) BuildRequestHeader(c gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-DashScope-Async", "enable") // 阿里异步任务必须设置
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	bodyBytes, err := common.Marshal(a.aliReq)
	if err != nil {
		return nil, errors.Wrap(err, "marshal_ali_request_failed")
	}

	return bytes.NewReader(bodyBytes), nil
}

var (
	size480p = []string{
		"832*480",
		"480*832",
		"624*624",
	}
	size720p = []string{
		"1280*720",
		"720*1280",
		"960*960",
		"1088*832",
		"832*1088",
	}
	size1080p = []string{
		"1920*1080",
		"1080*1920",
		"1440*1440",
		"1632*1248",
		"1248*1632",
	}
)

func isVideoURL(url string) bool {
	lower := strings.ToLower(url)
	for _, ext := range []string{".mp4", ".mov", ".avi", ".mkv", ".webm", ".m4v"} {
		if strings.Contains(lower, ext) {
			return true
		}
	}
	return false
}

// isMediaArrayModel 判断是否使用 input.media 数组格式（而非 img_url）
// 目前包括：wan2.7-i2v 系列、happyhorse-1.1-i2v
func isMediaArrayModel(model string) bool {
	return strings.HasPrefix(model, "wan2.7-i2v") || model == "happyhorse-1.1-i2v"
}

// isWan27I2v2026 保留别名，兼容现有调用
// Deprecated: 使用 isMediaArrayModel
func isWan27I2v2026(model string) bool {
	return isMediaArrayModel(model)
}

func (a *TaskAdaptor) convertToAliRequest(info *relaycommon.RelayInfo, req relaycommon.TaskSubmitReq) (*AliVideoRequest, error) {
	input := AliVideoInput{Prompt: req.Prompt}

	if isWan27I2v2026(req.Model) {
		// wan2.7-i2v 系列全部使用 media 数组格式（input.media 必填）
		// 优先取 InputReference（URL），其次取 Image（base64）
		imgSrc := req.InputReference
		if imgSrc == "" {
			imgSrc = req.Image
		}
		if imgSrc != "" {
			mediaType := "first_frame"
			if isVideoURL(imgSrc) {
				mediaType = "first_clip"
			}
			input.Media = append(input.Media, AliVideoMediaItem{Type: mediaType, URL: imgSrc})
		}
		// 注：metadata 中的 input.media 会在后续合并覆盖，允许前端传入多段媒体（first_clip + last_frame）
	} else {
		// 其他 i2v 模型继续使用 img_url
		if req.InputReference != "" {
			input.ImgURL = req.InputReference
		} else if req.Image != "" {
			input.ImgURL = req.Image
		}
	}

	aliReq := &AliVideoRequest{
		Model: req.Model,
		Input: input,
		Parameters: &AliVideoParameters{
			PromptExtend: true, // 默认开启智能改写
			Watermark:    false,
		},
	}

	// wan2.7-t2v 整个系列使用 resolution + ratio 格式（不使用 size）
	isWan27T2v := strings.HasPrefix(req.Model, "wan2.7-t2v")

	// 处理分辨率映射
	if req.Size != "" {
		if strings.Contains(req.Size, "*") {
			aliReq.Parameters.Size = req.Size
		} else if strings.Contains(req.Size, ":") {
			// ratio 格式，如 "16:9"
			aliReq.Parameters.Ratio = req.Size
		} else {
			resolution := strings.ToUpper(req.Size)
			if !strings.HasSuffix(resolution, "P") {
				resolution = resolution + "P"
			}
			aliReq.Parameters.Resolution = resolution
		}
	} else {
		// 根据模型设置默认分辨率
		if isWan27T2v {
			// wan2.7-t2v 系列：resolution + ratio
			aliReq.Parameters.Resolution = "720P"
			aliReq.Parameters.Ratio = "16:9"
		} else if strings.Contains(req.Model, "t2v") { // text to video
			if strings.HasPrefix(req.Model, "wan2.6") {
				aliReq.Parameters.Size = "1920*1080"
			} else if strings.HasPrefix(req.Model, "wan2.5") {
				aliReq.Parameters.Size = "1920*1080"
			} else if strings.HasPrefix(req.Model, "wan2.2") {
				aliReq.Parameters.Size = "1920*1080"
			} else {
				aliReq.Parameters.Size = "1280*720"
			}
		} else {
			if strings.HasPrefix(req.Model, "wan2.7") {
				aliReq.Parameters.Resolution = "1080P"
			} else if strings.HasPrefix(req.Model, "wan2.6") {
				aliReq.Parameters.Resolution = "1080P"
			} else if strings.HasPrefix(req.Model, "wan2.5") {
				aliReq.Parameters.Resolution = "1080P"
			} else if strings.HasPrefix(req.Model, "wan2.2-i2v-flash") {
				aliReq.Parameters.Resolution = "720P"
			} else if strings.HasPrefix(req.Model, "wan2.2-i2v-plus") {
				aliReq.Parameters.Resolution = "1080P"
			} else {
				aliReq.Parameters.Resolution = "720P"
			}
		}
	}

	// 处理时长
	if seconds := req.GetDurationSeconds(); seconds > 0 {
		aliReq.Parameters.Duration = seconds
	} else {
		aliReq.Parameters.Duration = 5 // 默认5秒
	}

	// 从 metadata 中提取额外参数
	if req.Metadata != nil {
		if metadataBytes, err := common.Marshal(req.Metadata); err == nil {
			err = common.Unmarshal(metadataBytes, aliReq)
			if err != nil {
				return nil, errors.Wrap(err, "unmarshal metadata failed")
			}
		} else {
			return nil, errors.Wrap(err, "marshal metadata failed")
		}
	}

	if aliReq.Model != req.Model {
		return nil, errors.New("can't change model with metadata")
	}

	// media 数组格式的模型（wan2.7-i2v 系列、happyhorse-1.1-i2v）必须提供 media 数组
	if isWan27I2v2026(aliReq.Model) && len(aliReq.Input.Media) == 0 {
		return nil, errors.New("该模型必须提供 input.media（首帧/视频 URL 或上传图片）")
	}

	return aliReq, nil
}

// DoRequest delegates to common helper
func (a *TaskAdaptor) DoRequest(c gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

// DoResponse handles upstream response
func (a *TaskAdaptor) DoResponse(c gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}
	_ = resp.Body.Close()

	// 解析阿里响应
	var aliResp AliVideoResponse
	if err := common.Unmarshal(responseBody, &aliResp); err != nil {
		taskErr = service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
		return
	}

	// 检查错误
	if aliResp.Code != "" {
		taskErr = service.TaskErrorWrapper(fmt.Errorf("%s: %s", aliResp.Code, aliResp.Message), "ali_api_error", resp.StatusCode)
		return
	}

	if aliResp.Output.TaskID == "" {
		taskErr = service.TaskErrorWrapper(fmt.Errorf("task_id is empty"), "invalid_response", http.StatusInternalServerError)
		return
	}

	// 转换为 OpenAI 格式响应
	openAIResp := dto.NewOpenAIVideo()
	openAIResp.ID = aliResp.Output.TaskID
	openAIResp.Model = c.GetString("model")
	if openAIResp.Model == "" && info != nil {
		openAIResp.Model = info.OriginModelName
	}
	openAIResp.Status = convertAliStatus(aliResp.Output.TaskStatus)
	openAIResp.CreatedAt = common.GetTimestamp()

	// 返回 OpenAI 格式
	c.JSON(http.StatusOK, openAIResp)

	return aliResp.Output.TaskID, responseBody, nil
}

// FetchTask 查询任务状态
func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}

	uri := fmt.Sprintf("%s/api/v1/tasks/%s", baseUrl, taskID)

	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}

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

// ParseTaskResult 解析任务结果
func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	var aliResp AliVideoResponse
	if err := common.Unmarshal(respBody, &aliResp); err != nil {
		return nil, errors.Wrap(err, "unmarshal task result failed")
	}

	taskResult := relaycommon.TaskInfo{
		Code: 0,
	}

	// 状态映射
	switch aliResp.Output.TaskStatus {
	case "PENDING":
		taskResult.Status = model.TaskStatusQueued
	case "RUNNING":
		taskResult.Status = model.TaskStatusInProgress
	case "SUCCEEDED":
		taskResult.Status = model.TaskStatusSuccess
		// 阿里直接返回视频URL，不需要额外的代理端点
		taskResult.Url = aliResp.Output.VideoURL
	case "FAILED", "CANCELED", "UNKNOWN":
		taskResult.Status = model.TaskStatusFailure
		if aliResp.Message != "" {
			taskResult.Reason = aliResp.Message
		} else if aliResp.Output.Message != "" {
			taskResult.Reason = fmt.Sprintf("task failed, code: %s , message: %s", aliResp.Output.Code, aliResp.Output.Message)
		} else {
			taskResult.Reason = "task failed"
		}
	default:
		taskResult.Status = model.TaskStatusQueued
	}

	return &taskResult, nil
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(task *model.Task) ([]byte, error) {
	var aliResp AliVideoResponse
	if err := common.Unmarshal(task.Data, &aliResp); err != nil {
		return nil, errors.Wrap(err, "unmarshal ali response failed")
	}

	openAIResp := dto.NewOpenAIVideo()
	openAIResp.ID = task.TaskID
	openAIResp.Status = convertAliStatus(aliResp.Output.TaskStatus)
	openAIResp.Model = task.Properties.OriginModelName
	openAIResp.SetProgressStr(task.Progress)
	openAIResp.CreatedAt = task.CreatedAt
	openAIResp.CompletedAt = task.UpdatedAt

	// 设置视频URL（核心字段）
	openAIResp.SetMetadata("url", aliResp.Output.VideoURL)

	// 错误处理
	if aliResp.Code != "" {
		openAIResp.Error = &dto.OpenAIVideoError{
			Code:    aliResp.Code,
			Message: aliResp.Message,
		}
	} else if aliResp.Output.Code != "" {
		openAIResp.Error = &dto.OpenAIVideoError{
			Code:    aliResp.Output.Code,
			Message: aliResp.Output.Message,
		}
	}

	return common.Marshal(openAIResp)
}

func convertAliStatus(aliStatus string) string {
	switch aliStatus {
	case "PENDING":
		return dto.VideoStatusQueued
	case "RUNNING":
		return dto.VideoStatusInProgress
	case "SUCCEEDED":
		return dto.VideoStatusCompleted
	case "FAILED", "CANCELED", "UNKNOWN":
		return dto.VideoStatusFailed
	default:
		return dto.VideoStatusUnknown
	}
}
