package gemini

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"openapi/common"
	"openapi/constant"
	"openapi/dto"
	"openapi/relay/channel"
	relaycommon "openapi/relay/common"
	relayconstant "openapi/relay/constant"
	"openapi/types"

	gin "github.com/king54346/gin-tiny"
)

// ModelList 内置的 Google Gemini 模型清单。
var ModelList = []string{
	"gemini-2.5-pro",
	"gemini-2.5-flash",
	"gemini-2.0-flash",
	"gemini-2.0-flash-lite",
	"gemini-1.5-pro",
	"gemini-1.5-flash",
	"text-embedding-004",
	"gemini-embedding-001",
}

const ChannelName = "Google Gemini"

type Adaptor struct{}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	baseURL := strings.TrimSuffix(info.ChannelBaseUrl, "/")
	if baseURL == "" {
		baseURL = constant.ChannelBaseURLs[constant.ChannelTypeGemini]
	}
	version := strings.Trim(strings.TrimSpace(info.ApiVersion), "/")
	if version == "" {
		version = "v1beta"
	}
	model := info.UpstreamModelName
	if model == "" {
		model = info.OriginModelName
	}

	action := "generateContent"
	if info.RelayMode == relayconstant.RelayModeEmbeddings {
		inputs := 1
		if embReq, ok := info.Request.(*dto.EmbeddingRequest); ok {
			inputs = len(embReq.ParseInput())
		}
		if inputs > 1 {
			action = "batchEmbedContents"
		} else {
			action = "embedContent"
		}
		return fmt.Sprintf("%s/%s/models/%s:%s", baseURL, version, model, action), nil
	}

	if gemReq, ok := info.Request.(*dto.GeminiChatRequest); ok && gemReq.Action != "" {
		action = gemReq.Action
	} else if info.IsStream {
		action = "streamGenerateContent"
	}

	url := fmt.Sprintf("%s/%s/models/%s:%s", baseURL, version, model, action)
	if action == "streamGenerateContent" || info.IsStream {
		url += "?alt=sse"
	}
	return url, nil
}

func (a *Adaptor) SetupRequestHeader(c gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	req.Set("x-goog-api-key", info.ApiKey)
	req.Set("Content-Type", "application/json")
	req.Del("Authorization")
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(c gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	return dto.OpenAIRequestToGeminiRequest(request)
}

func (a *Adaptor) ConvertRerankRequest(c gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, channel.ErrNotImplemented
}

func (a *Adaptor) ConvertEmbeddingRequest(c gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	inputs := request.ParseInput()
	if len(inputs) == 0 {
		return nil, fmt.Errorf("embedding input is empty")
	}
	model := info.UpstreamModelName
	if model == "" {
		model = request.Model
	}
	if !strings.HasPrefix(model, "models/") {
		model = "models/" + model
	}
	if len(inputs) == 1 {
		return &dto.GeminiEmbeddingRequest{
			Model:                model,
			Content:              dto.GeminiChatContent{Parts: []dto.GeminiPart{{Text: inputs[0]}}},
			OutputDimensionality: request.Dimensions,
		}, nil
	}
	batch := &dto.GeminiBatchEmbeddingRequest{
		Requests: make([]dto.GeminiEmbeddingRequest, 0, len(inputs)),
	}
	for _, text := range inputs {
		batch.Requests = append(batch.Requests, dto.GeminiEmbeddingRequest{
			Model:                model,
			Content:              dto.GeminiChatContent{Parts: []dto.GeminiPart{{Text: text}}},
			OutputDimensionality: request.Dimensions,
		})
	}
	return batch, nil
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

	if info.RelayMode == relayconstant.RelayModeEmbeddings {
		return handleGeminiEmbeddingResponse(c, resp, info)
	}
	if info.RelayFormat == types.RelayFormatGemini {
		return passThroughGeminiResponse(c, resp, info)
	}
	return convertGeminiResponseForOpenAI(c, resp, info)
}

func (a *Adaptor) GetModelList() []string { return ModelList }
func (a *Adaptor) GetChannelName() string { return ChannelName }

func handleGeminiEmbeddingResponse(c gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.StarAPIError) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeReadResponseBodyFailed)
	}
	var gemResp dto.GeminiEmbeddingResponse
	if err := common.Unmarshal(body, &gemResp); err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	var items []dto.OpenAIEmbeddingResponseItem
	if gemResp.Embedding != nil {
		items = append(items, dto.OpenAIEmbeddingResponseItem{
			Object:    "embedding",
			Index:     0,
			Embedding: gemResp.Embedding.Values,
		})
	}
	for i, emb := range gemResp.Embeddings {
		items = append(items, dto.OpenAIEmbeddingResponseItem{
			Object:    "embedding",
			Index:     i,
			Embedding: emb.Values,
		})
	}
	usage := &dto.Usage{
		PromptTokens: info.GetEstimatePromptTokens(),
		TotalTokens:  info.GetEstimatePromptTokens(),
	}
	out := dto.OpenAIEmbeddingResponse{
		Object: "list",
		Data:   items,
		Model:  info.OriginModelName,
		Usage:  *usage,
	}
	c.JSON(http.StatusOK, out)
	return usage, nil
}

func passThroughGeminiResponse(c gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.StarAPIError) {
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") || info.IsStream {
		return streamPassThroughGemini(c, resp, info)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeReadResponseBodyFailed)
	}
	var gemResp dto.GeminiChatResponse
	if err := common.Unmarshal(body, &gemResp); err != nil {
		// embedContent 等非对话响应原样返回
		c.Data(http.StatusOK, "application/json; charset=utf-8", body)
		return &dto.Usage{PromptTokens: info.GetEstimatePromptTokens(), TotalTokens: info.GetEstimatePromptTokens()}, nil
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", body)
	return dto.GeminiUsageToUsage(gemResp.UsageMetadata), nil
}

func streamPassThroughGemini(c gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.StarAPIError) {
	w := c.Response()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	var lastUsage *dto.Usage
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		info.SetFirstResponseTime()
		if strings.HasPrefix(line, "data:") {
			payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			var chunk dto.GeminiChatResponse
			if payload != "" && common.Unmarshal([]byte(payload), &chunk) == nil && chunk.UsageMetadata != nil {
				lastUsage = dto.GeminiUsageToUsage(chunk.UsageMetadata)
			}
		}
		_, _ = fmt.Fprintf(w, "%s\n", line)
		w.Flush()
	}
	if lastUsage == nil {
		lastUsage = &dto.Usage{PromptTokens: info.GetEstimatePromptTokens(), TotalTokens: info.GetEstimatePromptTokens()}
	}
	return lastUsage, nil
}

func convertGeminiResponseForOpenAI(c gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.StarAPIError) {
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") || info.IsStream {
		return convertGeminiStreamToOpenAI(c, resp, info)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeReadResponseBodyFailed)
	}
	var gemResp dto.GeminiChatResponse
	if err := common.Unmarshal(body, &gemResp); err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	oaiResp, usage := dto.GeminiResponseToOpenAIResponse(&gemResp, info.OriginModelName)
	c.JSON(http.StatusOK, oaiResp)
	return usage, nil
}

func convertGeminiStreamToOpenAI(c gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.StarAPIError) {
	w := c.Response()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	streamID := "chatcmpl-" + common.GetRandomString(16)
	created := time.Now().Unix()
	var finalUsage *dto.Usage
	sentRole := false

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		var gemChunk dto.GeminiChatResponse
		if err := common.Unmarshal([]byte(payload), &gemChunk); err != nil {
			continue
		}
		info.SetFirstResponseTime()
		if gemChunk.UsageMetadata != nil {
			finalUsage = dto.GeminiUsageToUsage(gemChunk.UsageMetadata)
		}

		for _, cand := range gemChunk.Candidates {
			var delta dto.ChatCompletionsStreamResponseChoiceDelta
			if !sentRole {
				delta.Role = "assistant"
				sentRole = true
			}
			for j, part := range cand.Content.Parts {
				if part.Thought {
					delta.SetReasoningContent(part.Text)
					continue
				}
				if part.Text != "" {
					existing := delta.GetContentString()
					delta.SetContentString(existing + part.Text)
				}
				if part.FunctionCall != nil {
					callID := part.FunctionCall.ID
					if callID == "" {
						callID = fmt.Sprintf("call_%d_%d", cand.Index, j)
					}
					argsBytes, _ := common.Marshal(part.FunctionCall.Args)
					idx := j
					delta.ToolCalls = append(delta.ToolCalls, dto.ToolCallResponse{
						Index: &idx,
						ID:    callID,
						Type:  "function",
						Function: dto.FunctionResponse{
							Name:      part.FunctionCall.Name,
							Arguments: string(argsBytes),
						},
					})
				}
			}

			var finishPtr *string
			if cand.FinishReason != "" {
				fr := "stop"
				if len(delta.ToolCalls) > 0 {
					fr = "tool_calls"
				} else if strings.EqualFold(cand.FinishReason, "MAX_TOKENS") {
					fr = "length"
				}
				finishPtr = &fr
			}

			chunkObj := map[string]any{
				"id":      streamID,
				"object":  "chat.completion.chunk",
				"created": created,
				"model":   info.OriginModelName,
				"choices": []dto.ChatCompletionsStreamResponseChoice{{
					Index:        cand.Index,
					Delta:        delta,
					FinishReason: finishPtr,
				}},
			}
			b, _ := common.Marshal(chunkObj)
			_, _ = fmt.Fprintf(w, "data: %s\n\n", b)
			w.Flush()
		}
	}

	if finalUsage != nil && info.ShouldIncludeUsage {
		usageChunk := map[string]any{
			"id":      streamID,
			"object":  "chat.completion.chunk",
			"created": created,
			"model":   info.OriginModelName,
			"choices": []any{},
			"usage":   finalUsage,
		}
		b, _ := common.Marshal(usageChunk)
		_, _ = fmt.Fprintf(w, "data: %s\n\n", b)
		w.Flush()
	}
	_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	w.Flush()

	if finalUsage == nil {
		finalUsage = &dto.Usage{PromptTokens: info.GetEstimatePromptTokens(), TotalTokens: info.GetEstimatePromptTokens()}
	}
	return finalUsage, nil
}
