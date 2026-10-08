package ali

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"

	"openapi/common"
	"openapi/dto"
	"openapi/logger"
	relaycommon "openapi/relay/common"
	"openapi/service"
	"openapi/types"

	gin "github.com/king54346/gin-tiny"
)

// isAliMultimodalTTSModel 检测是否为阿里多模态 TTS 模型（如 qwen3-tts-flash、qwen-tts-*）
func isAliMultimodalTTSModel(modelName string) bool {
	lower := strings.ToLower(modelName)
	return strings.Contains(lower, "-tts-") || strings.HasSuffix(lower, "-tts")
}

// aliTTSInput 是阿里多模态 TTS 请求的 input 字段
type aliTTSInput struct {
	Text         string `json:"text"`
	Voice        string `json:"voice,omitempty"`
	LanguageType string `json:"language_type,omitempty"`
}

// aliTTSRequest 是阿里多模态 TTS 请求体
type aliTTSRequest struct {
	Model string      `json:"model"`
	Input aliTTSInput `json:"input"`
}

// aliTTSAudio 是响应中 output.audio 字段
type aliTTSAudio struct {
	Data      string `json:"data"`
	URL       string `json:"url"`
	ID        string `json:"id"`
	ExpiresAt int64  `json:"expires_at"`
}

// aliTTSOutput 是响应中 output 字段
type aliTTSOutput struct {
	Audio *aliTTSAudio `json:"audio"`
}

// aliTTSUsage 是响应中 usage 字段
type aliTTSUsage struct {
	Characters int `json:"characters"`
}

// aliTTSResponse 是阿里多模态 TTS 的完整响应
type aliTTSResponse struct {
	Output    aliTTSOutput `json:"output"`
	Usage     aliTTSUsage  `json:"usage"`
	RequestID string       `json:"request_id"`
	Code      string       `json:"code"`
	Message   string       `json:"message"`
}

// convertAudioToAliMultimodalTTS 将 OpenAI TTS 请求转换为阿里多模态 TTS 请求格式
func convertAudioToAliMultimodalTTS(request dto.AudioRequest) (io.Reader, error) {
	ttsReq := aliTTSRequest{
		Model: request.Model,
		Input: aliTTSInput{
			Text:  request.Input,
			Voice: request.Voice,
		},
	}

	jsonData, err := json.Marshal(ttsReq)
	if err != nil {
		return nil, fmt.Errorf("error marshalling TTS request: %w", err)
	}
	return bytes.NewReader(jsonData), nil
}

// aliMultimodalTTSResponseHandler 处理阿里多模态 TTS 的响应
// 从响应 JSON 中取 output.audio.url，下载音频并以二进制返回给客户端
func aliMultimodalTTSResponseHandler(c gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*types.StarAPIError, *dto.Usage) {
	defer service.CloseResponseBodyGracefully(resp)

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError), nil
	}

	var ttsResp aliTTSResponse
	if err := common.Unmarshal(responseBody, &ttsResp); err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError), nil
	}

	if ttsResp.Code != "" && ttsResp.Code != "200" {
		apiErr := fmt.Errorf("%s: %s", ttsResp.Code, ttsResp.Message)
		return types.NewOpenAIError(apiErr, types.ErrorCodeDoRequestFailed, http.StatusBadGateway), nil
	}

	if ttsResp.Output.Audio == nil || ttsResp.Output.Audio.URL == "" {
		return types.NewOpenAIError(fmt.Errorf("no audio URL in response"), types.ErrorCodeDoRequestFailed, http.StatusBadGateway), nil
	}

	// 下载音频文件
	audioResp, err := http.Get(ttsResp.Output.Audio.URL)
	if err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusBadGateway), nil
	}
	defer audioResp.Body.Close()

	audioBytes, err := io.ReadAll(audioResp.Body)
	if err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError), nil
	}

	// 阿里多模态 TTS 始终返回 WAV 格式
	c.Response().Header().Set("Content-Type", "audio/wav")
	c.Response().WriteHeader(http.StatusOK)
	if _, err := c.Response().Write(audioBytes); err != nil {
		logger.LogError(c, fmt.Sprintf("failed to write TTS audio response: %v", err))
	}

	// 计算 usage
	usage := &dto.Usage{}
	usage.PromptTokens = info.GetEstimatePromptTokens()

	// 根据音频时长计算 token（固定用 wav 格式）
	ext := ".wav"
	reader := bytes.NewReader(audioBytes)
	duration, durationErr := common.GetAudioDuration(c.Request().Context(), reader, ext)
	if durationErr != nil {
		logger.LogWarn(c, fmt.Sprintf("failed to get TTS audio duration: %v", durationErr))
		// 按字符数粗估
		completionTokens := int(math.Ceil(float64(ttsResp.Usage.Characters) / 60.0 * 1000))
		usage.CompletionTokens = completionTokens
	} else if duration > 0 {
		completionTokens := int(math.Round(math.Ceil(duration) / 60.0 * 1000))
		usage.CompletionTokens = completionTokens
	}
	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens

	return nil, usage
}
