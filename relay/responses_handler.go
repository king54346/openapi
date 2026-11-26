package relay

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"openapi/common"
	appconstant "openapi/constant"
	"openapi/dto"
	"openapi/relay/channel"
	relaycommon "openapi/relay/common"
	"openapi/relay/compat"
	relayconstant "openapi/relay/constant"
	"openapi/relay/helper"
	relayerr "openapi/relay/relayerr"
	"openapi/setting"
	"openapi/types"

	gin "github.com/king54346/gin-tiny"
)

func ResponsesHelper(c gin.Context, info *relaycommon.RelayInfo) (apiError *types.StarAPIError) {
	info.InitChannelMeta(c)
	if info.RelayMode == relayconstant.RelayModeResponsesCompact {
		switch info.ApiType {
		case appconstant.APITypeOpenAI:
		default:
			return types.NewErrorWithStatusCode(
				fmt.Errorf("unsupported endpoint %q for api type %d", "/v1/responses/compact", info.ApiType),
				types.ErrorCodeInvalidRequest,
				http.StatusBadRequest,
				types.ErrOptionWithSkipRetry(),
			)
		}
	}

	var responsesReq *dto.OpenAIResponsesRequest
	switch req := info.Request.(type) {
	case *dto.OpenAIResponsesRequest:
		responsesReq = req
	case *dto.OpenAIResponsesCompactionRequest:
		responsesReq = &dto.OpenAIResponsesRequest{
			Model:              req.Model,
			Input:              req.Input,
			Instructions:       req.Instructions,
			PreviousResponseID: req.PreviousResponseID,
		}
	default:
		return types.NewErrorWithStatusCode(
			fmt.Errorf("invalid request type, expected dto.OpenAIResponsesRequest or dto.OpenAIResponsesCompactionRequest, got %T", info.Request),
			types.ErrorCodeInvalidRequest,
			http.StatusBadRequest,
			types.ErrOptionWithSkipRetry(),
		)
	}

	request, err := common.DeepCopy(responsesReq)
	if err != nil {
		return types.NewError(fmt.Errorf("failed to copy request to OpenAIResponsesRequest: %w", err), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}

	err = helper.ModelMappedHelper(c, info, request)
	if err != nil {
		return types.NewError(err, types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())
	}

	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return types.NewError(fmt.Errorf("invalid api type: %d", info.ApiType), types.ErrorCodeInvalidApiType, types.ErrOptionWithSkipRetry())
	}
	adaptor.Init(info)

	var requestBody io.Reader
	if setting.GetGlobalSettings().PassThroughRequestEnabled || info.ChannelSetting.PassThroughBodyEnabled {
		body, err := common.GetRequestBody(c)
		if err != nil {
			return types.NewError(err, types.ErrorCodeReadRequestBodyFailed, types.ErrOptionWithSkipRetry())
		}
		requestBody = bytes.NewBuffer(body)
	} else {
		convertedRequest, err := adaptor.ConvertOpenAIResponsesRequest(c, info, *request)
		if err != nil {
			if errors.Is(err, channel.ErrNotImplemented) && info.RelayMode != relayconstant.RelayModeResponsesCompact {
				return responsesViaChatCompletions(c, info, adaptor, request)
			}
			return convertRequestError(c, info, err)
		}
		relaycommon.AppendRequestConversionFromRequest(info, convertedRequest)
		jsonData, err := common.Marshal(convertedRequest)
		if err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
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

		if common.DebugEnabled {
			println("requestBody: ", string(jsonData))
		}
		requestBody = bytes.NewBuffer(jsonData)
	}

	var httpResp *http.Response
	resp, err := adaptor.DoRequest(c, info, requestBody)
	if err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	}

	statusCodeMappingStr := c.GetString("status_code_mapping")

	if resp != nil {
		httpResp = resp.(*http.Response)
		if httpResp.StatusCode == http.StatusNotFound && info.RelayMode == relayconstant.RelayModeResponses {
			_ = httpResp.Body.Close()
			return responsesViaChatCompletions(c, info, adaptor, request)
		}
		if httpResp.StatusCode != http.StatusOK {
			apiError = relayerr.RelayErrorHandler(c.Request().Context(), httpResp, false)
			relayerr.ResetStatusCode(apiError, statusCodeMappingStr)
			return apiError
		}
	}

	usage, apiError := adaptor.DoResponse(c, httpResp, info)
	if apiError != nil {
		relayerr.ResetStatusCode(apiError, statusCodeMappingStr)
		return apiError
	}

	recordUsage(c, info, usage.(*dto.Usage), nil)
	return nil
}

// responsesViaChatCompletions 在渠道不支持原生 /v1/responses 时，自动降级为 Chat Completions 调用并包装回 Responses 格式。
func responsesViaChatCompletions(c gin.Context, info *relaycommon.RelayInfo, adaptor channel.Adaptor, req *dto.OpenAIResponsesRequest) *types.StarAPIError {
	chatReq, err := compat.ResponsesRequestToChatCompletionsRequest(req)
	if err != nil {
		return types.NewErrorWithStatusCode(err, types.ErrorCodeConvertRequestFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}

	origMode := info.RelayMode
	origFormat := info.RelayFormat
	origPath := info.RequestURLPath
	info.RelayMode = relayconstant.RelayModeChatCompletions
	info.RelayFormat = types.RelayFormatOpenAI
	info.RequestURLPath = "/v1/chat/completions"
	defer func() {
		info.RelayMode = origMode
		info.RelayFormat = origFormat
		info.RequestURLPath = origPath
	}()

	converted, err := adaptor.ConvertOpenAIRequest(c, info, chatReq)
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

	resp, err := adaptor.DoRequest(c, info, bytes.NewBuffer(jsonData))
	if err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	}
	httpResp, ok := resp.(*http.Response)
	if !ok || httpResp == nil {
		return types.NewError(fmt.Errorf("upstream response is nil"), types.ErrorCodeBadResponse)
	}
	statusCodeMappingStr := c.GetString("status_code_mapping")
	if httpResp.StatusCode != http.StatusOK {
		apiErr := relayerr.RelayErrorHandler(c.Request().Context(), httpResp, false)
		relayerr.ResetStatusCode(apiErr, statusCodeMappingStr)
		return apiErr
	}

	rec := httptest.NewRecorder()
	subCtx, _ := gin.CreateTestContext(rec)
	subCtx.SetRequest(c.Request())
	usageAny, apiErr := adaptor.DoResponse(subCtx, httpResp, info)
	if apiErr != nil {
		relayerr.ResetStatusCode(apiErr, statusCodeMappingStr)
		return apiErr
	}

	if info.IsStream {
		writeResponsesSSEFromChatSSE(c, rec.Body, info.OriginModelName, usageAny)
	} else {
		var chatResp dto.OpenAITextResponse
		if unmarshalErr := common.Unmarshal(rec.Body.Bytes(), &chatResp); unmarshalErr != nil {
			return types.NewError(unmarshalErr, types.ErrorCodeBadResponseBody)
		}
		outResp := compat.ChatCompletionsResponseToResponsesResponse(&chatResp, info.OriginModelName)
		c.JSON(http.StatusOK, outResp)
	}

	if u, ok := usageAny.(*dto.Usage); ok {
		recordUsage(c, info, u, nil)
	} else {
		recordUsage(c, info, nil, nil)
	}
	return nil
}

func writeResponsesSSEFromChatSSE(c gin.Context, src io.Reader, modelName string, usageAny any) {
	w := c.Response()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	respID := "resp_" + common.GetRandomString(20)
	msgID := "msg_" + common.GetRandomString(20)
	createdAt := int(time.Now().Unix())

	writeEvent := func(eventType string, payload any) {
		b, _ := common.Marshal(payload)
		_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, b)
		w.Flush()
	}

	writeEvent("response.created", map[string]any{
		"type": "response.created",
		"response": dto.OpenAIResponsesResponse{
			ID:        respID,
			Object:    "response",
			CreatedAt: createdAt,
			Status:    json.RawMessage(`"in_progress"`),
			Model:     modelName,
			Output:    []dto.ResponsesOutput{},
		},
	})

	var fullText strings.Builder
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
			if delta := ch.Delta.GetContentString(); delta != "" {
				fullText.WriteString(delta)
				writeEvent("response.output_text.delta", map[string]any{
					"type":          "response.output_text.delta",
					"item_id":       msgID,
					"output_index":  0,
					"content_index": 0,
					"delta":         delta,
				})
			}
		}
	}

	var usage *dto.Usage
	if u, ok := usageAny.(*dto.Usage); ok && u != nil {
		copyU := *u
		copyU.InputTokens = copyU.PromptTokens
		copyU.OutputTokens = copyU.CompletionTokens
		usage = &copyU
	}

	writeEvent("response.completed", map[string]any{
		"type": "response.completed",
		"response": dto.OpenAIResponsesResponse{
			ID:        respID,
			Object:    "response",
			CreatedAt: createdAt,
			Status:    json.RawMessage(`"completed"`),
			Model:     modelName,
			Output: []dto.ResponsesOutput{{
				Type:   "message",
				ID:     msgID,
				Status: "completed",
				Role:   "assistant",
				Content: []dto.ResponsesOutputContent{{
					Type:        "output_text",
					Text:        fullText.String(),
					Annotations: []any{},
				}},
			}},
			Usage: usage,
		},
	})
}
