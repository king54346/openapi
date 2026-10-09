package ali

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"openapi/common"
	"openapi/dto"
	relaycommon "openapi/relay/common"
	"openapi/service"
	"openapi/types"

	gin "github.com/king54346/gin-tiny"
)

// isAliChatASRModel 检测是否为阿里 Chat 格式的 ASR 模型
// 例如 qwen3-asr-flash、qwen-asr-*
func isAliChatASRModel(modelName string) bool {
	lower := strings.ToLower(modelName)
	return strings.Contains(lower, "-asr-") || strings.HasSuffix(lower, "-asr")
}

// aliASRRequest 是阿里 Chat ASR 请求体结构
type aliASRRequest struct {
	Model      string          `json:"model"`
	Messages   []aliASRMessage `json:"messages"`
	Stream     bool            `json:"stream,omitempty"`
	ASROptions *aliASROptions  `json:"asr_options,omitempty"`
}

type aliASRMessage struct {
	Role    string          `json:"role"`
	Content []aliASRContent `json:"content"`
}

type aliASRContent struct {
	Type       string       `json:"type"`
	InputAudio *aliASRAudio `json:"input_audio,omitempty"`
}

type aliASRAudio struct {
	Data string `json:"data"` // Data URI: data:<mime>;base64,<data>
}

type aliASROptions struct {
	Language  string `json:"language,omitempty"`
	EnableITN bool   `json:"enable_itn"`
}

// audioExtToMIMEType 根据扩展名返回音频 MIME 类型
func audioExtToMIMEType(ext string) string {
	switch strings.ToLower(ext) {
	case ".mp3":
		return "audio/mpeg"
	case ".wav":
		return "audio/wav"
	case ".flac":
		return "audio/flac"
	case ".ogg", ".oga":
		return "audio/ogg"
	case ".opus":
		return "audio/ogg"
	case ".m4a":
		return "audio/mp4"
	case ".aac":
		return "audio/aac"
	case ".webm":
		return "audio/webm"
	default:
		return "audio/mpeg"
	}
}

// convertAudioToAliChatASR 将 multipart 音频文件转换为阿里 Chat ASR 请求格式
func convertAudioToAliChatASR(c gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	formData, err := common.ParseMultipartFormReusable(c)
	if err != nil {
		return nil, fmt.Errorf("error parsing multipart form: %w", err)
	}

	fileHeaders := formData.File["file"]
	if len(fileHeaders) == 0 {
		return nil, fmt.Errorf("file is required")
	}

	fileHeader := fileHeaders[0]
	file, err := fileHeader.Open()
	if err != nil {
		return nil, fmt.Errorf("error opening audio file: %w", err)
	}
	defer file.Close()

	fileData, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("error reading audio file: %w", err)
	}

	ext := filepath.Ext(fileHeader.Filename)
	mimeType := audioExtToMIMEType(ext)
	b64Data := base64.StdEncoding.EncodeToString(fileData)
	dataURI := fmt.Sprintf("data:%s;base64,%s", mimeType, b64Data)

	// 从表单中读取 language 参数
	language := ""
	if vals, ok := formData.Value["language"]; ok && len(vals) > 0 {
		language = vals[0]
	}

	asrReq := aliASRRequest{
		Model: strings.ToLower(request.Model),
		Messages: []aliASRMessage{
			{
				Role: "user",
				Content: []aliASRContent{
					{
						Type:       "input_audio",
						InputAudio: &aliASRAudio{Data: dataURI},
					},
				},
			},
		},
		ASROptions: &aliASROptions{
			Language:  language,
			EnableITN: false,
		},
	}

	jsonData, err := common.Marshal(asrReq)
	if err != nil {
		return nil, fmt.Errorf("error marshalling ASR request: %w", err)
	}

	// 更新请求的 Content-Type，供后续 header 设置使用
	c.Request().Header.Set("Content-Type", "application/json")
	return bytes.NewReader(jsonData), nil
}

// aliChatASRResponseHandler 处理阿里 Chat ASR 的响应，提取文本并以标准 STT 格式返回
func aliChatASRResponseHandler(c gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*types.StarAPIError, *dto.Usage) {
	defer service.CloseResponseBodyGracefully(resp)

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError), nil
	}

	// 解析 chat completion 格式的响应
	var chatResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage *dto.Usage `json:"usage"`
	}

	if err := common.Unmarshal(responseBody, &chatResp); err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError), nil
	}

	text := ""
	if len(chatResp.Choices) > 0 {
		text = chatResp.Choices[0].Message.Content
	}

	// 转换为标准 STT 响应格式 {"text": "..."}
	sttResponse := dto.AudioResponse{Text: text}
	sttBody, err := common.Marshal(sttResponse)
	if err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError), nil
	}

	service.IOCopyBytesGracefully(c, resp, sttBody)

	usage := chatResp.Usage
	if usage == nil {
		usage = &dto.Usage{}
	}
	if usage.PromptTokens == 0 && usage.InputTokens > 0 {
		usage.PromptTokens = usage.InputTokens
	}
	if usage.CompletionTokens == 0 && usage.OutputTokens > 0 {
		usage.CompletionTokens = usage.OutputTokens
	}
	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	return nil, usage
}
