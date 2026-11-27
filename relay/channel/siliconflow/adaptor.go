package siliconflow

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"openapi/common"
	channelconstant "openapi/constant"
	"openapi/dto"
	"openapi/relay/channel"
	"openapi/relay/channel/openai"
	relaycommon "openapi/relay/common"
	"openapi/relay/constant"
	httpclient "openapi/service/http"
	"openapi/types"

	gin "github.com/king54346/gin-tiny"
	"github.com/samber/lo"
)

type SFTokens struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type SFMeta struct {
	Tokens SFTokens `json:"tokens"`
}

type SFRerankResponse struct {
	Results []dto.RerankResponseResult `json:"results"`
	Meta    SFMeta                     `json:"meta"`
}

type SFImageRequest struct {
	Model             string  `json:"model"`
	Prompt            string  `json:"prompt"`
	NegativePrompt    string  `json:"negative_prompt,omitempty"`
	ImageSize         string  `json:"image_size,omitempty"`
	BatchSize         uint    `json:"batch_size,omitempty"`
	Seed              uint64  `json:"seed,omitempty"`
	NumInferenceSteps uint    `json:"num_inference_steps,omitempty"`
	GuidanceScale     float64 `json:"guidance_scale,omitempty"`
	Cfg               float64 `json:"cfg,omitempty"`
	Image             string  `json:"image,omitempty"`
	Image2            string  `json:"image2,omitempty"`
	Image3            string  `json:"image3,omitempty"`
}

type Adaptor struct{}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	baseURL := strings.TrimRight(info.ChannelBaseUrl, "/")
	if baseURL == "" {
		baseURL = channelconstant.ChannelBaseURLs[channelconstant.ChannelTypeSiliconFlow]
	}
	if info.RelayMode == constant.RelayModeRerank {
		if strings.HasSuffix(baseURL, "/v1") {
			return baseURL + "/rerank", nil
		}
		return baseURL + "/v1/rerank", nil
	}
	return relaycommon.GetFullRequestURL(baseURL, info.RequestURLPath, info.ChannelType), nil
}

func (a *Adaptor) SetupRequestHeader(c gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	req.Set("Authorization", fmt.Sprintf("Bearer %s", info.ApiKey))
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(c gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	// SiliconFlow FIM 请求即使使用 prefix/suffix 也要求提供非空 messages 数组
	if request != nil && (request.Prefix != nil || request.Suffix != nil) && len(request.Messages) == 0 {
		request.Messages = []dto.Message{
			{
				Role:    "user",
				Content: "",
			},
		}
	}
	return request, nil
}

func (a *Adaptor) ConvertRerankRequest(c gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return request, nil
}

func (a *Adaptor) ConvertEmbeddingRequest(c gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return request, nil
}

func (a *Adaptor) ConvertAudioRequest(c gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	adaptor := openai.Adaptor{}
	return adaptor.ConvertAudioRequest(c, info, request)
}

func (a *Adaptor) ConvertImageRequest(c gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	sfRequest := &SFImageRequest{}
	if len(request.ExtraFields) > 0 {
		if err := common.Unmarshal(request.ExtraFields, sfRequest); err != nil {
			sfRequest = &SFImageRequest{}
		}
	}
	sfRequest.Model = request.Model
	sfRequest.Prompt = request.Prompt
	if sfRequest.ImageSize == "" {
		sfRequest.ImageSize = request.Size
	}
	if sfRequest.BatchSize == 0 {
		sfRequest.BatchSize = lo.FromPtrOr(request.N, uint(0))
	}
	return sfRequest, nil
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	return nil, channel.ErrNotImplemented
}

func (a *Adaptor) DoRequest(c gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	adaptor := openai.Adaptor{}
	return adaptor.DoRequest(c, info, requestBody)
}

func (a *Adaptor) DoResponse(c gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.StarAPIError) {
	if info.RelayMode == constant.RelayModeRerank {
		return siliconflowRerankHandler(c, resp)
	}
	adaptor := openai.Adaptor{}
	return adaptor.DoResponse(c, resp, info)
}

func (a *Adaptor) GetModelList() []string { return ModelList }
func (a *Adaptor) GetChannelName() string { return ChannelName }

func siliconflowRerankHandler(c gin.Context, resp *http.Response) (*dto.Usage, *types.StarAPIError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	httpclient.CloseResponseBodyGracefully(resp)

	var sfResp SFRerankResponse
	if err := common.Unmarshal(responseBody, &sfResp); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	usage := &dto.Usage{
		PromptTokens:     sfResp.Meta.Tokens.InputTokens,
		CompletionTokens: sfResp.Meta.Tokens.OutputTokens,
		TotalTokens:      sfResp.Meta.Tokens.InputTokens + sfResp.Meta.Tokens.OutputTokens,
	}
	rerankResp := &dto.RerankResponse{
		Results: sfResp.Results,
		Usage:   *usage,
	}

	jsonResp, err := common.Marshal(rerankResp)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	c.Data(resp.StatusCode, "application/json; charset=utf-8", jsonResp)
	return usage, nil
}
