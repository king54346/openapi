package moonshot

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	channelconstant "openapi/constant"
	"openapi/dto"
	"openapi/relay/channel"
	"openapi/relay/channel/claude"
	"openapi/relay/channel/openai"
	relaycommon "openapi/relay/common"
	"openapi/relay/constant"
	"openapi/types"

	gin "github.com/king54346/gin-tiny"
)

type Adaptor struct{}

func (a *Adaptor) ConvertAudioRequest(c gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	return nil, channel.ErrNotImplemented
}

func (a *Adaptor) ConvertImageRequest(c gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	adaptor := openai.Adaptor{}
	return adaptor.ConvertImageRequest(c, info, request)
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	baseURL := strings.TrimRight(info.ChannelBaseUrl, "/")
	if baseURL == "" {
		baseURL = channelconstant.ChannelBaseURLs[channelconstant.ChannelTypeMoonshot]
	}

	if specialPlan, ok := channelconstant.ChannelSpecialBases[baseURL]; ok {
		if info.RelayFormat == types.RelayFormatClaude && specialPlan.ClaudeBaseURL != "" {
			return fmt.Sprintf("%s/v1/messages", specialPlan.ClaudeBaseURL), nil
		}
		if info.RelayFormat == types.RelayFormatOpenAI && specialPlan.OpenAIBaseURL != "" {
			return fmt.Sprintf("%s/chat/completions", specialPlan.OpenAIBaseURL), nil
		}
	}

	if info.RelayFormat == types.RelayFormatClaude {
		cleanBase := strings.TrimSuffix(baseURL, "/v1")
		return fmt.Sprintf("%s/anthropic/v1/messages", cleanBase), nil
	}

	v1Base := baseURL
	if !strings.HasSuffix(v1Base, "/v1") {
		v1Base += "/v1"
	}
	switch info.RelayMode {
	case constant.RelayModeRerank:
		return v1Base + "/rerank", nil
	case constant.RelayModeEmbeddings:
		return v1Base + "/embeddings", nil
	case constant.RelayModeCompletions:
		return v1Base + "/completions", nil
	default:
		return v1Base + "/chat/completions", nil
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
	req.Set("Authorization", fmt.Sprintf("Bearer %s", info.ApiKey))
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(c gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	return request, nil
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	return nil, channel.ErrNotImplemented
}

func (a *Adaptor) DoRequest(c gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

func (a *Adaptor) ConvertRerankRequest(c gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return request, nil
}

func (a *Adaptor) ConvertEmbeddingRequest(c gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return request, nil
}

func (a *Adaptor) DoResponse(c gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.StarAPIError) {
	if info.RelayFormat == types.RelayFormatClaude {
		if resp == nil {
			return nil, types.NewError(fmt.Errorf("upstream response is nil"), types.ErrorCodeBadResponseStatusCode)
		}
		defer resp.Body.Close()
		return claude.PassThroughClaude(c, resp, info)
	}
	adaptor := openai.Adaptor{}
	return adaptor.DoResponse(c, resp, info)
}

func (a *Adaptor) GetModelList() []string { return ModelList }
func (a *Adaptor) GetChannelName() string { return ChannelName }
