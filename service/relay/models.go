package relay

import (
	"openapi/common"
	"openapi/constant"
	"openapi/dto"
	"openapi/model"

	gin "github.com/king54346/gin-tiny"
)

type OpenAIModel struct {
	Id      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

func UsableModels(c gin.Context) []string {
	models := model.GetEnabledModels()
	if !common.GetContextKeyBool(c, constant.ContextKeyTokenModelLimitEnabled) {
		return models
	}
	limits, _ := c.Get(string(constant.ContextKeyTokenModelLimit))
	limitMap, _ := limits.(map[string]bool)
	limitType, _ := c.Get("token_model_limit_type")
	blacklist := limitType == 1

	out := make([]string, 0, len(models))
	for _, m := range models {
		if limitMap[m] != blacklist {
			out = append(out, m)
		}
	}
	return out
}

func ToOpenAIModel(name string) OpenAIModel {
	return OpenAIModel{Id: name, Object: "model", Created: common.StartTime, OwnedBy: "openapi"}
}

func BuildGeminiModelList(models []string) dto.GeminiModelListResponse {
	items := make([]dto.GeminiModelInfo, 0, len(models))
	for _, m := range models {
		items = append(items, dto.GeminiModelInfo{
			Name:                       "models/" + m,
			DisplayName:                m,
			SupportedGenerationMethods: []string{"generateContent", "streamGenerateContent", "countTokens"},
		})
	}
	return dto.GeminiModelListResponse{Models: items}
}
