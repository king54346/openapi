package zhipu

import (
	"bufio"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"openapi/common"
	channelconstant "openapi/constant"
	"openapi/dto"
	"openapi/relay/channel"
	relaycommon "openapi/relay/common"
	"openapi/relay/helper"
	httpclient "openapi/service/http"
	"openapi/types"

	gin "github.com/king54346/gin-tiny"
)

type ZhipuMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ZhipuRequest struct {
	Prompt      []ZhipuMessage `json:"prompt"`
	Temperature *float64       `json:"temperature,omitempty"`
	TopP        *float64       `json:"top_p,omitempty"`
	RequestId   string         `json:"request_id,omitempty"`
	Incremental bool           `json:"incremental,omitempty"`
}

type ZhipuResponseData struct {
	TaskId     string         `json:"task_id"`
	RequestId  string         `json:"request_id"`
	TaskStatus string         `json:"task_status"`
	Choices    []ZhipuMessage `json:"choices"`
	dto.Usage  `json:"usage"`
}

type ZhipuResponse struct {
	Code    int               `json:"code"`
	Msg     string            `json:"msg"`
	Success bool              `json:"success"`
	Data    ZhipuResponseData `json:"data"`
}

type ZhipuStreamMetaResponse struct {
	RequestId  string `json:"request_id"`
	TaskId     string `json:"task_id"`
	TaskStatus string `json:"task_status"`
	dto.Usage  `json:"usage"`
}

type zhipuTokenData struct {
	Token      string
	ExpiryTime time.Time
}

var (
	zhipuTokens sync.Map
	expSeconds  int64 = 24 * 3600
)

// getZhipuToken 使用标准库 HMAC-SHA256 签发智谱 v3 JWT Token（带 24h 内存缓存）。
func getZhipuToken(apikey string) string {
	if data, ok := zhipuTokens.Load(apikey); ok {
		tokenData := data.(zhipuTokenData)
		if time.Now().Before(tokenData.ExpiryTime) {
			return tokenData.Token
		}
	}

	split := strings.Split(apikey, ".")
	if len(split) != 2 || split[0] == "" || split[1] == "" {
		common.SysLog("invalid zhipu v3 key format")
		return apikey
	}
	id, secret := split[0], split[1]

	now := time.Now()
	expiryTime := now.Add(time.Duration(expSeconds) * time.Second)
	headerBytes, err := common.Marshal(map[string]any{
		"alg":       "HS256",
		"sign_type": "SIGN",
	})
	if err != nil {
		return ""
	}
	payloadBytes, err := common.Marshal(map[string]any{
		"api_key":   id,
		"exp":       expiryTime.UnixMilli(),
		"timestamp": now.UnixMilli(),
	})
	if err != nil {
		return ""
	}

	unsigned := base64.RawURLEncoding.EncodeToString(headerBytes) + "." +
		base64.RawURLEncoding.EncodeToString(payloadBytes)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(unsigned))
	tokenString := unsigned + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	zhipuTokens.Store(apikey, zhipuTokenData{
		Token:      tokenString,
		ExpiryTime: expiryTime,
	})
	return tokenString
}

type Adaptor struct{}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	baseURL := strings.TrimRight(info.ChannelBaseUrl, "/")
	if baseURL == "" {
		baseURL = channelconstant.ChannelBaseURLs[channelconstant.ChannelTypeZhipu]
	}
	method := "invoke"
	if info.IsStream {
		method = "sse-invoke"
	}
	return fmt.Sprintf("%s/api/paas/v3/model-api/%s/%s", baseURL, info.UpstreamModelName, method), nil
}

func (a *Adaptor) SetupRequestHeader(c gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	req.Set("Authorization", getZhipuToken(info.ApiKey))
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(c gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	topP := request.TopP
	if topP != nil && *topP >= 1 {
		clamped := 0.99
		topP = &clamped
	}
	messages := make([]ZhipuMessage, 0, len(request.Messages))
	for _, message := range request.Messages {
		if message.Role == "system" {
			messages = append(messages,
				ZhipuMessage{Role: "system", Content: message.StringContent()},
				ZhipuMessage{Role: "user", Content: "Okay"},
			)
		} else {
			messages = append(messages, ZhipuMessage{
				Role:    message.Role,
				Content: message.StringContent(),
			})
		}
	}
	return &ZhipuRequest{
		Prompt:      messages,
		Temperature: request.Temperature,
		TopP:        topP,
		Incremental: false,
	}, nil
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
	if info.IsStream {
		return zhipuStreamHandler(c, info, resp)
	}
	return zhipuHandler(c, info, resp)
}

func (a *Adaptor) GetModelList() []string { return ModelList }
func (a *Adaptor) GetChannelName() string { return ChannelName }

func zhipuHandler(c gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.StarAPIError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	httpclient.CloseResponseBodyGracefully(resp)

	var zhipuResp ZhipuResponse
	if err := common.Unmarshal(responseBody, &zhipuResp); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if !zhipuResp.Success {
		return nil, types.WithOpenAIError(types.OpenAIError{
			Message: zhipuResp.Msg,
			Code:    zhipuResp.Code,
		}, resp.StatusCode)
	}

	fullText := dto.OpenAITextResponse{
		Id:      zhipuResp.Data.TaskId,
		Object:  "chat.completion",
		Created: common.GetTimestamp(),
		Model:   info.UpstreamModelName,
		Choices: make([]dto.OpenAITextResponseChoice, 0, len(zhipuResp.Data.Choices)),
		Usage:   zhipuResp.Data.Usage,
	}
	for i, choice := range zhipuResp.Data.Choices {
		finish := ""
		if i == len(zhipuResp.Data.Choices)-1 {
			finish = "stop"
		}
		fullText.Choices = append(fullText.Choices, dto.OpenAITextResponseChoice{
			Index: i,
			Message: dto.Message{
				Role:    choice.Role,
				Content: strings.Trim(choice.Content, "\""),
			},
			FinishReason: finish,
		})
	}

	jsonResp, err := common.Marshal(fullText)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	c.Data(resp.StatusCode, "application/json; charset=utf-8", jsonResp)
	return &fullText.Usage, nil
}

func zhipuStreamHandler(c gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.StarAPIError) {
	defer httpclient.CloseResponseBodyGracefully(resp)
	helper.SetEventStreamHeaders(c)

	var usage *dto.Usage
	scanner := bufio.NewScanner(resp.Body)
	scanner.Split(bufio.ScanLines)
	stopReason := "stop"

	for scanner.Scan() {
		line := scanner.Text()
		if len(line) < 5 {
			continue
		}
		switch {
		case strings.HasPrefix(line, "data:"):
			text := line[5:]
			var choice dto.ChatCompletionsStreamResponseChoice
			choice.Delta.SetContentString(text)
			chunk := dto.ChatCompletionsStreamResponse{
				Object:  "chat.completion.chunk",
				Created: common.GetTimestamp(),
				Model:   info.UpstreamModelName,
				Choices: []dto.ChatCompletionsStreamResponseChoice{choice},
			}
			_ = helper.ObjectData(c, chunk)
		case strings.HasPrefix(line, "meta:"):
			var meta ZhipuStreamMetaResponse
			if err := common.Unmarshal([]byte(line[5:]), &meta); err == nil {
				usage = &meta.Usage
				var choice dto.ChatCompletionsStreamResponseChoice
				choice.Delta.SetContentString("")
				choice.FinishReason = &stopReason
				chunk := dto.ChatCompletionsStreamResponse{
					Id:      meta.RequestId,
					Object:  "chat.completion.chunk",
					Created: common.GetTimestamp(),
					Model:   info.UpstreamModelName,
					Choices: []dto.ChatCompletionsStreamResponseChoice{choice},
				}
				_ = helper.ObjectData(c, chunk)
			}
		}
	}
	helper.Done(c)
	return usage, nil
}
