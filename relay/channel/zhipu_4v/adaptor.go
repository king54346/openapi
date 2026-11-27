package zhipu_4v

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"openapi/common"
	channelconstant "openapi/constant"
	"openapi/dto"
	"openapi/logger"
	"openapi/relay/channel"
	"openapi/relay/channel/claude"
	"openapi/relay/channel/openai"
	relaycommon "openapi/relay/common"
	relayconstant "openapi/relay/constant"
	"openapi/service/files"
	httpclient "openapi/service/http"
	"openapi/types"

	gin "github.com/king54346/gin-tiny"
)

type Adaptor struct{}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	baseURL := strings.TrimRight(info.ChannelBaseUrl, "/")
	if baseURL == "" {
		baseURL = channelconstant.ChannelBaseURLs[channelconstant.ChannelTypeZhipu_v4]
	}
	specialPlan, hasSpecialPlan := channelconstant.ChannelSpecialBases[baseURL]

	if info.RelayFormat == types.RelayFormatClaude {
		if hasSpecialPlan && specialPlan.ClaudeBaseURL != "" {
			return fmt.Sprintf("%s/v1/messages", specialPlan.ClaudeBaseURL), nil
		}
		cleanBase := strings.TrimSuffix(baseURL, "/api/paas/v4")
		return fmt.Sprintf("%s/api/anthropic/v1/messages", cleanBase), nil
	}

	v4Base := baseURL
	if !hasSpecialPlan && !strings.HasSuffix(v4Base, "/api/paas/v4") {
		v4Base += "/api/paas/v4"
	}

	switch info.RelayMode {
	case relayconstant.RelayModeEmbeddings:
		if hasSpecialPlan && specialPlan.OpenAIBaseURL != "" {
			return fmt.Sprintf("%s/embeddings", specialPlan.OpenAIBaseURL), nil
		}
		return v4Base + "/embeddings", nil
	case relayconstant.RelayModeImagesGenerations:
		return v4Base + "/images/generations", nil
	default:
		if hasSpecialPlan && specialPlan.OpenAIBaseURL != "" {
			return fmt.Sprintf("%s/chat/completions", specialPlan.OpenAIBaseURL), nil
		}
		return v4Base + "/chat/completions", nil
	}
}

func (a *Adaptor) SetupRequestHeader(c gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	if info.RelayFormat == types.RelayFormatClaude {
		req.Set("x-api-key", info.ApiKey)
		ver := strings.TrimSpace(c.Request().Header.Get("anthropic-version"))
		if ver == "" {
			ver = "2023-06-01"
		}
		req.Set("anthropic-version", ver)
	}
	req.Set("Authorization", "Bearer "+info.ApiKey)
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(c gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	return requestOpenAI2ZhipuV4(*request), nil
}

func (a *Adaptor) ConvertRerankRequest(c gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, channel.ErrNotImplemented
}

func (a *Adaptor) ConvertEmbeddingRequest(c gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return request, nil
}

func (a *Adaptor) ConvertAudioRequest(c gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	return nil, channel.ErrNotImplemented
}

func (a *Adaptor) ConvertImageRequest(c gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	return request, nil
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	return nil, channel.ErrNotImplemented
}

func (a *Adaptor) DoRequest(c gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

func (a *Adaptor) DoResponse(c gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.StarAPIError) {
	if info.RelayFormat == types.RelayFormatClaude {
		if resp == nil {
			return nil, types.NewError(fmt.Errorf("upstream response is nil"), types.ErrorCodeBadResponseStatusCode)
		}
		defer resp.Body.Close()
		return claude.PassThroughClaude(c, resp, info)
	}
	if info.RelayMode == relayconstant.RelayModeImagesGenerations {
		return zhipu4vImageHandler(c, resp, info)
	}
	adaptor := openai.Adaptor{}
	return adaptor.DoResponse(c, resp, info)
}

func (a *Adaptor) GetModelList() []string { return ModelList }
func (a *Adaptor) GetChannelName() string { return ChannelName }

func requestOpenAI2ZhipuV4(request dto.GeneralOpenAIRequest) *dto.GeneralOpenAIRequest {
	topP := request.TopP
	if topP != nil && *topP >= 1 {
		clamped := 0.99
		topP = &clamped
	}

	messages := make([]dto.Message, 0, len(request.Messages))
	for _, message := range request.Messages {
		if !message.IsStringContent() {
			mediaMessages := message.ParseContent()
			for j, mediaMessage := range mediaMessages {
				if mediaMessage.Type == dto.ContentTypeImageURL {
					imageUrl := mediaMessage.GetImageMedia()
					if imageUrl != nil && strings.HasPrefix(imageUrl.Url, "data:image/") {
						if idx := strings.Index(imageUrl.Url, ","); idx != -1 {
							imageUrl.Url = imageUrl.Url[idx+1:]
						}
					}
					mediaMessage.ImageUrl = imageUrl
					mediaMessages[j] = mediaMessage
				}
			}
			message.SetMediaContent(mediaMessages)
		}
		messages = append(messages, dto.Message{
			Role:       message.Role,
			Content:    message.Content,
			ToolCalls:  message.ToolCalls,
			ToolCallId: message.ToolCallId,
		})
	}

	var stop []string
	if str, ok := request.Stop.(string); ok && str != "" {
		stop = []string{str}
	} else if arr, ok := request.Stop.([]string); ok {
		stop = arr
	}

	maxTokens := request.MaxTokens
	if (maxTokens == nil || *maxTokens == 0) && request.MaxCompletionTokens != nil {
		maxTokens = request.MaxCompletionTokens
	}

	out := &dto.GeneralOpenAIRequest{
		Model:         request.Model,
		Stream:        request.Stream,
		StreamOptions: request.StreamOptions,
		Messages:      messages,
		Temperature:   request.Temperature,
		TopP:          topP,
		MaxTokens:     maxTokens,
		Tools:         request.Tools,
		ToolChoice:    request.ToolChoice,
		THINKING:      request.THINKING,
	}
	if len(stop) > 0 {
		out.Stop = stop
	}
	return out
}

type zhipuImageResponse struct {
	Created *int64           `json:"created,omitempty"`
	Data    []zhipuImageData `json:"data,omitempty"`
	Usage   *dto.Usage       `json:"usage,omitempty"`
	Error   *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type zhipuImageData struct {
	Url      string `json:"url,omitempty"`
	ImageUrl string `json:"image_url,omitempty"`
	B64Json  string `json:"b64_json,omitempty"`
	B64Image string `json:"b64_image,omitempty"`
}

func zhipu4vImageHandler(c gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.StarAPIError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	httpclient.CloseResponseBodyGracefully(resp)

	var zhipuResp zhipuImageResponse
	if err := common.Unmarshal(responseBody, &zhipuResp); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if zhipuResp.Error != nil && zhipuResp.Error.Message != "" {
		return nil, types.WithOpenAIError(types.OpenAIError{
			Message: zhipuResp.Error.Message,
			Type:    "zhipu_image_error",
			Code:    zhipuResp.Error.Code,
		}, resp.StatusCode)
	}

	type openAIImageItem struct {
		Url     string `json:"url,omitempty"`
		B64Json string `json:"b64_json,omitempty"`
	}
	payload := struct {
		Created int64             `json:"created"`
		Data    []openAIImageItem `json:"data"`
	}{
		Created: info.StartTime.Unix(),
	}
	if zhipuResp.Created != nil && *zhipuResp.Created != 0 {
		payload.Created = *zhipuResp.Created
	}

	for _, data := range zhipuResp.Data {
		b64 := data.B64Json
		if b64 == "" {
			b64 = data.B64Image
		}
		imgURL := data.Url
		if imgURL == "" {
			imgURL = data.ImageUrl
		}
		if b64 == "" && imgURL != "" {
			if _, downloaded, dlErr := files.GetImageFromUrl(imgURL); dlErr == nil && downloaded != "" {
				b64 = downloaded
			} else {
				// 若外链下载受限则保留原始 URL 返回给客户端
				logger.LogInfo(c, "zhipu_image_fallback_to_url: "+imgURL)
				payload.Data = append(payload.Data, openAIImageItem{Url: imgURL})
				continue
			}
		}
		if b64 != "" {
			payload.Data = append(payload.Data, openAIImageItem{B64Json: b64})
		}
	}

	jsonResp, err := common.Marshal(payload)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	c.Data(resp.StatusCode, "application/json; charset=utf-8", jsonResp)
	if zhipuResp.Usage != nil {
		return zhipuResp.Usage, nil
	}
	return &dto.Usage{}, nil
}
