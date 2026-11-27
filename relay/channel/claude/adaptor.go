package claude

import (
	"fmt"
	"io"
	"net/http"
	"openapi/common"
	"openapi/constant"
	"openapi/dto"
	"openapi/logger"
	"openapi/relay/channel"
	relaycommon "openapi/relay/common"
	"openapi/types"
	"strings"

	gin "github.com/king54346/gin-tiny"
)

// Anthropic Claude 渠道适配器：/v1/messages 原生协议，以及与 OpenAI Chat Completions 的互转。

// ModelList 内置的 Claude 模型清单。
var ModelList = []string{
	"claude-opus-4-1-20250805",
	"claude-sonnet-4-5-20250929",
	"claude-sonnet-4-20250514",
	"claude-3-7-sonnet-20250219",
	"claude-3-5-sonnet-20241022",
	"claude-3-5-haiku-20241022",
	"claude-3-opus-20240229",
}

// ChannelName 渠道显示名。
const ChannelName = "Anthropic Claude"

// defaultAnthropicVersion 未指定时使用的 API 版本。
const defaultAnthropicVersion = "2023-06-01"

type Adaptor struct{}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	baseURL := strings.TrimSuffix(info.ChannelBaseUrl, "/")
	if baseURL == "" {
		baseURL = constant.ChannelBaseURLs[constant.ChannelTypeAnthropic]
	}
	version := strings.Trim(strings.TrimSpace(info.ApiVersion), "/")
	if version == "" {
		version = "v1"
	}
	return fmt.Sprintf("%s/%s/messages", baseURL, version), nil
}

func (a *Adaptor) SetupRequestHeader(c gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	req.Set("x-api-key", info.ApiKey)
	version := strings.TrimSpace(c.Request().Header.Get("anthropic-version"))
	if version == "" {
		version = defaultAnthropicVersion
	}
	req.Set("anthropic-version", version)
	// 透传 beta 能力声明（如 prompt caching、extended thinking）
	if beta := strings.TrimSpace(c.Request().Header.Get("anthropic-beta")); beta != "" {
		req.Set("anthropic-beta", beta)
	}
	req.Del("Authorization")
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(c gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, fmt.Errorf("request is nil")
	}
	return OpenAIRequestToClaudeRequest(request)
}

func (a *Adaptor) ConvertRerankRequest(c gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, channel.ErrNotImplemented
}

func (a *Adaptor) ConvertEmbeddingRequest(c gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return nil, channel.ErrNotImplemented
}

func (a *Adaptor) ConvertAudioRequest(c gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	return nil, channel.ErrNotImplemented
}

func (a *Adaptor) ConvertImageRequest(c gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	return nil, channel.ErrNotImplemented
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	return nil, channel.ErrNotImplemented
}

func (a *Adaptor) DoRequest(c gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

func (a *Adaptor) DoResponse(c gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.StarAPIError) {
	if resp == nil {
		return nil, types.NewError(fmt.Errorf("upstream response is nil"), types.ErrorCodeBadResponseStatusCode)
	}
	defer resp.Body.Close()

	// /v1/messages 原生请求：原样透传 Claude 协议
	if info.RelayFormat == types.RelayFormatClaude {
		return PassThroughClaude(c, resp, info)
	}
	// /v1/chat/completions 打到 Claude 渠道：把 Claude 响应转回 OpenAI 格式
	return convertClaudeResponseForOpenAI(c, resp, info)
}

func (a *Adaptor) GetModelList() []string { return ModelList }

func (a *Adaptor) GetChannelName() string { return ChannelName }

// PassThroughClaude 原生透传：非流式解析 usage，流式按原始字节转发（保留 event: 行）。
func PassThroughClaude(c gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (any, *types.StarAPIError) {
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		return streamPassThrough(c, resp, info)
	}
	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, types.NewError(readErr, types.ErrorCodeReadResponseBodyFailed)
	}
	var claudeResp dto.ClaudeResponse
	if unmarshalErr := common.Unmarshal(body, &claudeResp); unmarshalErr != nil {
		return nil, types.NewError(unmarshalErr, types.ErrorCodeBadResponseBody)
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", body)
	return claudeUsageToUsage(claudeResp.Usage), nil
}

// streamPassThrough 逐行转发上游 SSE，保持 Anthropic 的事件格式。
func streamPassThrough(c gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (any, *types.StarAPIError) {
	copyStreamRaw(c, resp, info)
	return nil, nil
}

// claudeUsageToUsage Claude 用量转通用用量。
func claudeUsageToUsage(usage dto.ClaudeUsage) *dto.Usage {
	out := &dto.Usage{
		PromptTokens:     usage.InputTokens,
		CompletionTokens: usage.OutputTokens,
		TotalTokens:      usage.InputTokens + usage.OutputTokens,
	}
	if usage.CacheReadInputTokens > 0 {
		out.PromptTokensDetails.CachedTokens = usage.CacheReadInputTokens
		out.PromptCacheHitTokens = usage.CacheReadInputTokens
	}
	if usage.CacheCreationInputTokens > 0 {
		out.PromptTokensDetails.CachedCreationTokens = usage.CacheCreationInputTokens
	}
	return out
}

// logUpstreamError 记录上游错误体，便于排查（不返回给客户端）。
func logUpstreamError(c gin.Context, body []byte) {
	if len(body) > 0 {
		logger.LogError(c, "claude upstream error: "+common.MaskSensitiveInfo(string(body)))
	}
}
