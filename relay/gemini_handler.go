package relay

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	"openapi/common"
	"openapi/constant"
	"openapi/dto"
	"openapi/logger"
	relaycommon "openapi/relay/common"
	"openapi/relay/helper"
	relayerr "openapi/relay/relayerr"
	"openapi/types"

	gin "github.com/king54346/gin-tiny"
)

// GeminiHelper 处理 RelayFormatGemini 请求：
//   - /v1/engines/:model/embeddings 等嵌入请求直接复用 EmbeddingHelper
//   - 上游为 Gemini 渠道时原样透传 Gemini JSON / SSE
//   - 上游为 OpenAI/Claude 等渠道时自动做跨协议转换
func GeminiHelper(c gin.Context, info *relaycommon.RelayInfo) *types.StarAPIError {
	if _, isEmbedding := info.Request.(*dto.EmbeddingRequest); isEmbedding {
		return EmbeddingHelper(c, info)
	}

	info.InitChannelMeta(c)

	gemReq, ok := info.Request.(*dto.GeminiChatRequest)
	if !ok {
		return types.NewErrorWithStatusCode(
			fmt.Errorf("invalid request type, expected *dto.GeminiChatRequest, got %T", info.Request),
			types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry(),
		)
	}

	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return types.NewError(fmt.Errorf("invalid api type: %d", info.ApiType), types.ErrorCodeInvalidApiType, types.ErrOptionWithSkipRetry())
	}
	adaptor.Init(info)

	if info.ApiType == constant.APITypeGemini {
		return geminiNativeRelay(c, info, adaptor, gemReq)
	}
	return geminiToOpenAIRelay(c, info, adaptor, gemReq)
}

func geminiNativeRelay(c gin.Context, info *relaycommon.RelayInfo, adaptor channelAdaptor, gemReq *dto.GeminiChatRequest) *types.StarAPIError {
	if err := helper.ModelMappedHelper(c, info, gemReq); err != nil {
		return types.NewError(err, types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())
	}

	var payload any = gemReq
	if gemReq.Action == "embedContent" && gemReq.Content != nil {
		payload = &dto.GeminiEmbeddingRequest{
			Model:                "models/" + info.UpstreamModelName,
			Content:              *gemReq.Content,
			TaskType:             gemReq.TaskType,
			OutputDimensionality: gemReq.OutputDimensionality,
		}
	}

	jsonData, err := common.Marshal(payload)
	if err != nil {
		return types.NewError(err, types.ErrorCodeJsonMarshalFailed, types.ErrOptionWithSkipRetry())
	}
	if len(info.ParamOverride) > 0 {
		jsonData, err = relaycommon.ApplyParamOverride(jsonData, info.ParamOverride, relaycommon.BuildParamOverrideContext(info))
		if err != nil {
			return types.NewError(err, types.ErrorCodeChannelParamOverrideInvalid, types.ErrOptionWithSkipRetry())
		}
	}
	logger.LogDebug(c, "gemini native request body: "+string(jsonData))

	resp, err := adaptor.DoRequest(c, info, bytes.NewBuffer(jsonData))
	if err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	}
	httpResp, ok := resp.(*http.Response)
	if !ok || httpResp == nil {
		return types.NewError(fmt.Errorf("upstream response is nil"), types.ErrorCodeBadResponse)
	}
	if httpResp.StatusCode != http.StatusOK {
		apiErr := relayerr.RelayErrorHandler(c.Request().Context(), httpResp, false)
		relayerr.ResetStatusCode(apiErr, c.GetString("status_code_mapping"))
		return apiErr
	}

	usage, apiErr := adaptor.DoResponse(c, httpResp, info)
	if apiErr != nil {
		relayerr.ResetStatusCode(apiErr, c.GetString("status_code_mapping"))
		return apiErr
	}
	if u, ok := usage.(*dto.Usage); ok {
		recordUsage(c, info, u, nil)
	} else {
		recordUsage(c, info, nil, nil)
	}
	return nil
}

func geminiToOpenAIRelay(c gin.Context, info *relaycommon.RelayInfo, adaptor channelAdaptor, gemReq *dto.GeminiChatRequest) *types.StarAPIError {
	if err := helper.ModelMappedHelper(c, info, gemReq); err != nil {
		return types.NewError(err, types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())
	}

	oaiReq := dto.GeminiRequestToOpenAIRequest(gemReq, info.UpstreamModelName, info.IsStream)
	converted, err := adaptor.ConvertOpenAIRequest(c, info, oaiReq)
	if err != nil {
		return convertRequestError(c, info, err)
	}
	relaycommon.AppendRequestConversionFromRequest(info, converted)

	jsonData, err := common.Marshal(converted)
	if err != nil {
		return types.NewError(err, types.ErrorCodeJsonMarshalFailed, types.ErrOptionWithSkipRetry())
	}
	jsonData, err = relaycommon.RemoveDisabledFields(jsonData, info.ChannelOtherSettings)
	if err != nil {
		return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}
	if len(info.ParamOverride) > 0 {
		jsonData, err = relaycommon.ApplyParamOverride(jsonData, info.ParamOverride, relaycommon.BuildParamOverrideContext(info))
		if err != nil {
			return types.NewError(err, types.ErrorCodeChannelParamOverrideInvalid, types.ErrOptionWithSkipRetry())
		}
	}

	// 将上游模式与路径切换为 ChatCompletions 以命中目标渠道的对话端点
	origMode := info.RelayMode
	origFormat := info.RelayFormat
	origPath := info.RequestURLPath
	info.RelayMode = 1 // RelayModeChatCompletions
	info.RelayFormat = types.RelayFormatOpenAI
	info.RequestURLPath = "/v1/chat/completions"
	defer func() {
		info.RelayMode = origMode
		info.RelayFormat = origFormat
		info.RequestURLPath = origPath
	}()

	resp, err := adaptor.DoRequest(c, info, bytes.NewBuffer(jsonData))
	if err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	}
	httpResp, ok := resp.(*http.Response)
	if !ok || httpResp == nil {
		return types.NewError(fmt.Errorf("upstream response is nil"), types.ErrorCodeBadResponse)
	}
	if httpResp.StatusCode != http.StatusOK {
		apiErr := relayerr.RelayErrorHandler(c.Request().Context(), httpResp, false)
		relayerr.ResetStatusCode(apiErr, c.GetString("status_code_mapping"))
		return apiErr
	}

	// 使用内部 Recorder 接收 adaptor 生成的标准 OpenAI 响应/SSE，再转换为 Gemini 格式写回客户端
	rec := httptest.NewRecorder()
	subCtx, _ := gin.CreateTestContext(rec)
	subCtx.SetRequest(c.Request())
	usageAny, apiErr := adaptor.DoResponse(subCtx, httpResp, info)
	if apiErr != nil {
		relayerr.ResetStatusCode(apiErr, c.GetString("status_code_mapping"))
		return apiErr
	}

	if info.IsStream {
		writeGeminiSSEFromOpenAISSE(c, rec.Body, info.OriginModelName, usageAny)
	} else {
		var oaiResp dto.OpenAITextResponse
		if unmarshalErr := common.Unmarshal(rec.Body.Bytes(), &oaiResp); unmarshalErr != nil {
			return types.NewError(unmarshalErr, types.ErrorCodeBadResponseBody)
		}
		gemResp := dto.OpenAIResponseToGeminiResponse(&oaiResp)
		c.JSON(http.StatusOK, gemResp)
	}

	if u, ok := usageAny.(*dto.Usage); ok {
		recordUsage(c, info, u, nil)
	} else {
		recordUsage(c, info, nil, nil)
	}
	return nil
}

func writeGeminiSSEFromOpenAISSE(c gin.Context, src io.Reader, modelName string, usageAny any) {
	w := c.Response()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	scanner := bufio.NewScanner(src)
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
		var chunk struct {
			Choices []dto.ChatCompletionsStreamResponseChoice `json:"choices"`
		}
		if common.Unmarshal([]byte(payload), &chunk) != nil {
			continue
		}
		for _, ch := range chunk.Choices {
			var parts []dto.GeminiPart
			if r := ch.Delta.GetReasoningContent(); r != "" {
				parts = append(parts, dto.GeminiPart{Text: r, Thought: true})
			}
			if txt := ch.Delta.GetContentString(); txt != "" {
				parts = append(parts, dto.GeminiPart{Text: txt})
			}
			finishReason := ""
			if ch.FinishReason != nil && *ch.FinishReason != "" {
				if *ch.FinishReason == "length" {
					finishReason = "MAX_TOKENS"
				} else {
					finishReason = "STOP"
				}
			}
			if len(parts) == 0 && finishReason == "" {
				continue
			}
			gemChunk := dto.GeminiChatResponse{
				ModelVersion: modelName,
				Candidates: []dto.GeminiChatCandidate{{
					Index:        ch.Index,
					Content:      dto.GeminiChatContent{Role: "model", Parts: parts},
					FinishReason: finishReason,
				}},
			}
			if finishReason != "" {
				if u, ok := usageAny.(*dto.Usage); ok && u != nil {
					gemChunk.UsageMetadata = &dto.GeminiUsageMetadata{
						PromptTokenCount:     u.PromptTokens,
						CandidatesTokenCount: u.CompletionTokens,
						TotalTokenCount:      u.TotalTokens,
					}
				}
			}
			b, _ := common.Marshal(gemChunk)
			_, _ = fmt.Fprintf(w, "data: %s\n\n", b)
			w.Flush()
		}
	}
}
