package middleware

import (
	"errors"
	"fmt"
	"net/http"
	"openapi/common"
	"openapi/constant"
	"openapi/dto"
	"openapi/model"
	relayconstant "openapi/relay/constant"
	"openapi/service/channels"
	"openapi/service/dispatch"
	"openapi/service/tasks"
	"openapi/types"
	"slices"
	"strconv"
	"strings"
	"time"

	gin "github.com/king54346/gin-tiny"
)

type ModelRequest struct {
	Model string `json:"model"`
	Group string `json:"group,omitempty"`
}

func Distribute() func(c gin.Context) {
	return func(c gin.Context) {
		var channel *model.Channel
		channelId, ok := common.GetContextKey(c, constant.ContextKeyTokenSpecificChannelId)
		modelRequest, shouldSelectChannel, err := getModelRequest(c)
		if err != nil {
			abortWithOpenAiMessage(c, http.StatusBadRequest, "Invalid request, "+err.Error())
			return
		}
		if ok {
			id, err := strconv.Atoi(channelId.(string))
			if err != nil {
				abortWithOpenAiMessage(c, http.StatusBadRequest, "无效的渠道 Id")
				return
			}
			channel, err = model.GetChannelById(id, true)
			if err != nil {
				abortWithOpenAiMessage(c, http.StatusBadRequest, "无效的渠道 Id")
				return
			}
			if channel.Status != common.ChannelStatusEnabled {
				abortWithOpenAiMessage(c, http.StatusForbidden, "该渠道已被禁用")
				return
			}
		} else {
			// Select a channel for the user
			// check token model mapping
			modelLimitEnable := common.GetContextKeyBool(c, constant.ContextKeyTokenModelLimitEnabled)
			// 任务查询类请求（如 GET /v1/video/generations/:task_id、mj/suno fetch）不携带模型名，
			// 不选择渠道也不计费，跳过令牌模型限制校验，否则会被误判为"无权访问模型 "（模型名为空）
			if modelLimitEnable && modelRequest.Model != "" {
				s, ok := common.GetContextKey(c, constant.ContextKeyTokenModelLimit)
				if !ok {
					// token model limit is empty, all models are not allowed (whitelist mode)
					abortWithOpenAiMessage(c, http.StatusForbidden, "该令牌无权访问任何模型")
					return
				}
				var tokenModelLimit map[string]bool
				tokenModelLimit, ok = s.(map[string]bool)
				if !ok {
					tokenModelLimit = map[string]bool{}
				}
				matchName := dispatch.FormatMatchingModelName(modelRequest.Model) // match gpts & thinking-*
				limitType, _ := c.Get("token_model_limit_type")
				limitTypeInt, _ := limitType.(int)
				if limitTypeInt == 1 {
					// blacklist mode: listed models are blocked
					if _, blocked := tokenModelLimit[matchName]; blocked {
						abortWithOpenAiMessage(c, http.StatusForbidden, "该令牌无权访问模型 "+modelRequest.Model)
						return
					}
				} else {
					// whitelist mode (default): only listed models are allowed
					if _, ok := tokenModelLimit[matchName]; !ok {
						abortWithOpenAiMessage(c, http.StatusForbidden, "该令牌无权访问模型 "+modelRequest.Model)
						return
					}
				}
			}

			if shouldSelectChannel {
				if modelRequest.Model == "" {
					abortWithOpenAiMessage(c, http.StatusBadRequest, "未指定模型名称，模型名称不能为空")
					return
				}

				// 亲和性优先：同会话复用上次成功的渠道
				if preferredChannelID, found := channels.GetPreferredChannelByAffinity(c, modelRequest.Model); found {
					if preferred, prefErr := model.CacheGetChannel(preferredChannelID); prefErr == nil && preferred != nil &&
						preferred.Status == common.ChannelStatusEnabled &&
						model.IsChannelEnabledForGroupModel("", modelRequest.Model, preferred.Id) {
						channel = preferred
						channels.MarkChannelAffinityUsed(c, preferred.Id)
					}
				}

				if channel == nil {
					channel, err = dispatch.CacheGetRandomSatisfiedChannel(&dispatch.RetryParam{
						Ctx:       c,
						ModelName: modelRequest.Model,
						Retry:     common.GetPointer(0),
					})
					if err != nil {
						abortWithOpenAiMessage(c, http.StatusServiceUnavailable,
							fmt.Sprintf("获取模型 %s 的可用渠道失败（distributor）: %s", modelRequest.Model, err.Error()), types.ErrorCodeModelNotFound)
						return
					}
					if channel == nil {
						abortWithOpenAiMessage(c, http.StatusServiceUnavailable,
							fmt.Sprintf("模型 %s 无可用渠道（distributor）", modelRequest.Model), types.ErrorCodeModelNotFound)
						return
					}
				}
			}
		}
		common.SetContextKey(c, constant.ContextKeyRequestStartTime, time.Now())
		SetupContextForSelectedChannel(c, channel, modelRequest.Model)
		c.Next()
		if channel != nil && c.Response() != nil && c.Response().Status() < http.StatusBadRequest {
			channels.RecordChannelAffinity(c, channel.Id)
		}
	}
}

// getModelFromRequest 从请求中读取模型信息
// 根据 Content-Type 自动处理：
// - application/json
// - application/x-www-form-urlencoded
// - multipart/form-data
func getModelFromRequest(c gin.Context) (*ModelRequest, error) {
	var modelRequest ModelRequest
	err := common.UnmarshalBodyReusable(c, &modelRequest)
	if err != nil {
		return nil, errors.New("无效的请求, " + err.Error())
	}
	return &modelRequest, nil
}

func getModelRequest(c gin.Context) (*ModelRequest, bool, error) {
	var modelRequest ModelRequest
	shouldSelectChannel := true
	var err error
	if strings.Contains(c.Request().URL.Path, "/mj/") {
		relayMode := relayconstant.Path2RelayModeMidjourney(c.Request().URL.Path)
		if relayMode == relayconstant.RelayModeMidjourneyTaskFetch ||
			relayMode == relayconstant.RelayModeMidjourneyTaskFetchByCondition ||
			relayMode == relayconstant.RelayModeMidjourneyNotify ||
			relayMode == relayconstant.RelayModeMidjourneyTaskImageSeed {
			shouldSelectChannel = false
		} else {
			midjourneyRequest := dto.MidjourneyRequest{}
			err = common.UnmarshalBodyReusable(c, &midjourneyRequest)
			if err != nil {
				return nil, false, errors.New("无效的midjourney请求, " + err.Error())
			}
			midjourneyModel, mjErr, success := dispatch.GetMjRequestModel(relayMode, &midjourneyRequest)
			if mjErr != nil {
				return nil, false, fmt.Errorf("%s", mjErr.Description)
			}
			if midjourneyModel == "" {
				if !success {
					return nil, false, fmt.Errorf("无效的请求, 无法解析模型")
				} else {
					// task fetch, task fetch by condition, notify
					shouldSelectChannel = false
				}
			}
			modelRequest.Model = midjourneyModel
		}
		c.Set("relay_mode", relayMode)
	} else if strings.Contains(c.Request().URL.Path, "/suno/") {
		relayMode := relayconstant.Path2RelaySuno(c.Request().Method, c.Request().URL.Path)
		if relayMode == relayconstant.RelayModeSunoFetch ||
			relayMode == relayconstant.RelayModeSunoFetchByID {
			shouldSelectChannel = false
		} else {
			modelName := tasks.CoverTaskActionToModelName(constant.TaskPlatformSuno, c.Param("action"))
			modelRequest.Model = modelName
		}
		c.Set("platform", string(constant.TaskPlatformSuno))
		c.Set("relay_mode", relayMode)
	} else if strings.Contains(c.Request().URL.Path, "/v1/videos/") && strings.HasSuffix(c.Request().URL.Path, "/remix") {
		relayMode := relayconstant.RelayModeVideoSubmit
		c.Set("relay_mode", relayMode)
		shouldSelectChannel = false
	} else if strings.Contains(c.Request().URL.Path, "/v1/videos") {
		//curl https://api.openai.com/v1/videos \
		//  -H "Authorization: Bearer $OPENAI_API_KEY" \
		//  -F "model=sora-2" \
		//  -F "prompt=A calico cat playing a piano on stage"
		//	-F input_reference="@image.jpg"
		relayMode := relayconstant.RelayModeUnknown
		if c.Request().Method == http.MethodPost {
			relayMode = relayconstant.RelayModeVideoSubmit
			req, err := getModelFromRequest(c)
			if err != nil {
				return nil, false, err
			}
			if req != nil {
				modelRequest.Model = req.Model
			}
		} else if c.Request().Method == http.MethodGet {
			relayMode = relayconstant.RelayModeVideoFetchByID
			shouldSelectChannel = false
		}
		c.Set("relay_mode", relayMode)
	} else if strings.Contains(c.Request().URL.Path, "/v1/video/generations") ||
		strings.Contains(c.Request().URL.Path, "/pg/video/generations") {
		relayMode := relayconstant.RelayModeUnknown
		if c.Request().Method == http.MethodPost {
			req, err := getModelFromRequest(c)
			if err != nil {
				return nil, false, err
			}
			modelRequest.Model = req.Model
			relayMode = relayconstant.RelayModeVideoSubmit
		} else if c.Request().Method == http.MethodGet {
			relayMode = relayconstant.RelayModeVideoFetchByID
			shouldSelectChannel = false
		}
		if _, ok := c.Get("relay_mode"); !ok {
			c.Set("relay_mode", relayMode)
		}
	} else if strings.Contains(c.Request().URL.Path, "/pg/video/tasks/") {
		// Playground 视频任务查询
		if c.Request().Method == http.MethodGet {
			c.Set("relay_mode", relayconstant.RelayModeVideoFetchByID)
			shouldSelectChannel = false
		}
	} else if strings.HasPrefix(c.Request().URL.Path, "/v1beta/models/") || strings.HasPrefix(c.Request().URL.Path, "/v1/models/") {
		// tokens.Gemini API 路径处理: /v1beta/models/gemini-2.0-flash:generateContent
		relayMode := relayconstant.RelayModeGemini
		modelName := extractModelNameFromGeminiPath(c.Request().URL.Path)
		if modelName != "" {
			modelRequest.Model = modelName
		}
		c.Set("relay_mode", relayMode)
	} else if strings.HasPrefix(c.Request().URL.Path, "/kling/v1/images/") {
		// Kling 图像 API 路径处理
		if c.Request().Method == http.MethodGet {
			// GET 请求用于查询任务状态，不需要选择渠道
			shouldSelectChannel = false
			c.Set("relay_mode", relayconstant.RelayModeVideoFetchByID)
		} else {
			req, err := getModelFromRequest(c)
			if err != nil {
				return nil, false, err
			}
			modelRequest.Model = req.Model
		}
	} else if strings.HasPrefix(c.Request().URL.Path, "/pg/audio/transcriptions") {
		// Playground 语音转文本：multipart form，通过可复用解析读取 model 字段（避免消耗 body）
		if form, err2 := common.ParseMultipartFormReusable(c); err2 == nil && form != nil {
			if vals, ok := form.Value["model"]; ok && len(vals) > 0 {
				modelRequest.Model = vals[0]
			}
		}
	} else if !strings.HasPrefix(c.Request().URL.Path, "/v1/audio/transcriptions") && !strings.Contains(c.Request().Header.Get("Content-Type"), "multipart/form-data") {
		req, err := getModelFromRequest(c)
		if err != nil {
			return nil, false, err
		}
		modelRequest.Model = req.Model
	}
	if strings.HasPrefix(c.Request().URL.Path, "/v1/realtime") {
		//wss://api.openai.com/v1/realtime?model=gpt-4o-realtime-preview-2024-10-01
		modelRequest.Model = c.Query("model")
	}
	if strings.HasPrefix(c.Request().URL.Path, "/v1/moderations") {
		if modelRequest.Model == "" {
			modelRequest.Model = "text-moderation-stable"
		}
	}
	if strings.HasSuffix(c.Request().URL.Path, "embeddings") {
		if modelRequest.Model == "" {
			modelRequest.Model = c.Param("model")
		}
	}
	if strings.HasPrefix(c.Request().URL.Path, "/v1/images/generations") ||
		strings.HasPrefix(c.Request().URL.Path, "/pg/images/generations") {
		modelRequest.Model = common.GetStringIfEmpty(modelRequest.Model, "dall-e")
	} else if strings.HasPrefix(c.Request().URL.Path, "/v1/images/edits") {
		//modelRequest.Model = common.GetStringIfEmpty(c.PostForm("model"), "gpt-image-1")
		contentType := c.ContentType()
		if slices.Contains([]string{gin.MIMEPOSTForm, gin.MIMEMultipartPOSTForm}, contentType) {
			req, err := getModelFromRequest(c)
			if err == nil && req.Model != "" {
				modelRequest.Model = req.Model
			}
		}
	}
	if strings.HasPrefix(c.Request().URL.Path, "/v1/audio") {
		relayMode := relayconstant.RelayModeAudioSpeech
		if strings.HasPrefix(c.Request().URL.Path, "/v1/audio/speech") {

			modelRequest.Model = common.GetStringIfEmpty(modelRequest.Model, "tts-1")
		} else if strings.HasPrefix(c.Request().URL.Path, "/v1/audio/translations") {
			// 先尝试从请求读取
			if req, err := getModelFromRequest(c); err == nil && req.Model != "" {
				modelRequest.Model = req.Model
			}
			modelRequest.Model = common.GetStringIfEmpty(modelRequest.Model, "whisper-1")
			relayMode = relayconstant.RelayModeAudioTranslation
		} else if strings.HasPrefix(c.Request().URL.Path, "/v1/audio/transcriptions") {
			// 先尝试从请求读取
			if req, err := getModelFromRequest(c); err == nil && req.Model != "" {
				modelRequest.Model = req.Model
			}
			modelRequest.Model = common.GetStringIfEmpty(modelRequest.Model, "whisper-1")
			relayMode = relayconstant.RelayModeAudioTranscription
		}
		c.Set("relay_mode", relayMode)
	}
	if strings.HasPrefix(c.Request().URL.Path, "/pg/chat/completions") {
		// playground chat completions
		req, err := getModelFromRequest(c)
		if err != nil {
			return nil, false, err
		}
		modelRequest.Model = req.Model
	}

	if strings.HasPrefix(c.Request().URL.Path, "/v1/responses/compact") && modelRequest.Model != "" {
		modelRequest.Model = dispatch.WithCompactModelSuffix(modelRequest.Model)
	}
	return &modelRequest, shouldSelectChannel, nil
}

func SetupContextForSelectedChannel(c gin.Context, channel *model.Channel, modelName string) *types.StarAPIError {
	c.Set("original_model", modelName) // for retry
	if channel == nil {
		return types.NewError(errors.New("channel is nil"), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
	}
	common.SetContextKey(c, constant.ContextKeyChannelId, channel.Id)
	common.SetContextKey(c, constant.ContextKeyChannelName, channel.Name)
	common.SetContextKey(c, constant.ContextKeyChannelType, channel.Type)
	common.SetContextKey(c, constant.ContextKeyChannelCreateTime, channel.CreatedTime)
	common.SetContextKey(c, constant.ContextKeyChannelSetting, channel.GetSetting())
	common.SetContextKey(c, constant.ContextKeyChannelOtherSetting, channel.GetOtherSettings())
	common.SetContextKey(c, constant.ContextKeyChannelParamOverride, channel.GetParamOverride())
	common.SetContextKey(c, constant.ContextKeyChannelHeaderOverride, channel.GetHeaderOverride())
	if nil != channel.OpenAIOrganization && *channel.OpenAIOrganization != "" {
		common.SetContextKey(c, constant.ContextKeyChannelOrganization, *channel.OpenAIOrganization)
	}
	common.SetContextKey(c, constant.ContextKeyChannelAutoBan, channel.GetAutoBan())
	common.SetContextKey(c, constant.ContextKeyChannelModelMapping, channel.GetModelMapping())
	common.SetContextKey(c, constant.ContextKeyChannelStatusCodeMapping, channel.GetStatusCodeMapping())

	key, index, apiErr := channel.GetNextEnabledKey()
	if apiErr != nil {
		return apiErr
	}
	if channel.ChannelInfo.IsMultiKey {
		common.SetContextKey(c, constant.ContextKeyChannelIsMultiKey, true)
		common.SetContextKey(c, constant.ContextKeyChannelMultiKeyIndex, index)
	} else {
		// 必须设置为 false，否则在重试到单个 key 的时候会导致日志显示错误
		common.SetContextKey(c, constant.ContextKeyChannelIsMultiKey, false)
	}
	// c.Request().Header.Set("Authorization", fmt.Sprintf("Bearer %s", key))
	common.SetContextKey(c, constant.ContextKeyChannelKey, key)
	common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, channel.GetBaseURL())

	common.SetContextKey(c, constant.ContextKeySystemPromptOverride, false)

	// 渠道的 Other 列在不同渠道类型下含义不同，统一在这里写入上下文
	SetupChannelOtherContext(c, channel)
	return nil
}

// SetupChannelOtherContext 把渠道 Other 列解析出的附加信息写入上下文：
// 版本号类渠道写 api_version（渠道未配置时回退到请求 Query，tokens.Gemini 再回退到默认版本），
// Vertex AI 写 region，阿里写 plugin，Coze 写 bot_id。
func SetupChannelOtherContext(c gin.Context, channel *model.Channel) {
	if channel == nil {
		return
	}
	other := strings.TrimSpace(channel.Other)
	switch channel.Type {
	case constant.ChannelTypeAzure, constant.ChannelTypeXunfei,
		constant.ChannelCloudflare, constant.ChannelTypeMokaAI, constant.ChannelTypeGemini:
		version := other
		if version == "" {
			version = strings.TrimSpace(c.Query("api_version"))
		}
		if version == "" && channel.Type == constant.ChannelTypeGemini {
			version = "v1beta"
		}
		if version != "" {
			c.Set("api_version", version)
		}
	case constant.ChannelTypeVertexAi:
		if other != "" {
			c.Set("region", other)
		}
	case constant.ChannelTypeAli:
		if other != "" {
			c.Set("plugin", other)
		}
	case constant.ChannelTypeCoze:
		if other != "" {
			c.Set("bot_id", other)
		}
	}
}

// extractModelNameFromGeminiPath 从 tokens.Gemini API URL 路径中提取模型名
// 输入格式: /v1beta/models/gemini-2.0-flash:generateContent
// 输出: gemini-2.0-flash
func extractModelNameFromGeminiPath(path string) string {
	// 查找 "/models/" 的位置
	modelsPrefix := "/models/"
	modelsIndex := strings.Index(path, modelsPrefix)
	if modelsIndex == -1 {
		return ""
	}

	// 从 "/models/" 之后开始提取
	startIndex := modelsIndex + len(modelsPrefix)
	if startIndex >= len(path) {
		return ""
	}

	// 查找 ":" 的位置，模型名在 ":" 之前
	colonIndex := strings.Index(path[startIndex:], ":")
	if colonIndex == -1 {
		// 如果没有找到 ":"，返回从 "/models/" 到路径结尾的部分
		return path[startIndex:]
	}

	// 返回模型名部分
	return path[startIndex : startIndex+colonIndex]
}
