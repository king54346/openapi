package relay

import (
	"errors"
	"net/http"
	"strings"

	"openapi/constant"
	"openapi/dto"
	relaysvc "openapi/service/relay"
	"openapi/types"

	gin "github.com/king54346/gin-tiny"
)

func ListModels(c gin.Context, channelType int) {
	models := relaysvc.UsableModels(c)
	if channelType == constant.ChannelTypeGemini && strings.HasPrefix(c.Request().URL.Path, "/v1beta/models") {
		c.JSON(http.StatusOK, relaysvc.BuildGeminiModelList(models))
		return
	}
	data := make([]relaysvc.OpenAIModel, 0, len(models))
	for _, m := range models {
		data = append(data, relaysvc.ToOpenAIModel(m))
	}
	c.JSON(http.StatusOK, gin.H{"object": "list", "data": data})
}

func RetrieveModel(c gin.Context, channelType int) {
	name := strings.TrimPrefix(c.Param("model"), "/")
	models := relaysvc.UsableModels(c)
	if name == "" && channelType == constant.ChannelTypeGemini {
		// GET /v1/models 带 x-goog-api-key 时列出全部模型
		ListModels(c, constant.ChannelTypeOpenAI)
		return
	}
	for _, m := range models {
		if m == name {
			if channelType == constant.ChannelTypeGemini && strings.HasPrefix(c.Request().URL.Path, "/v1beta/models") {
				c.JSON(http.StatusOK, dto.GeminiModelInfo{
					Name:                       "models/" + m,
					DisplayName:                m,
					SupportedGenerationMethods: []string{"generateContent", "streamGenerateContent", "countTokens"},
				})
				return
			}
			c.JSON(http.StatusOK, relaysvc.ToOpenAIModel(m))
			return
		}
	}
	err := types.NewErrorWithStatusCode(errors.New("model "+name+" does not exist"), types.ErrorCodeModelNotFound, http.StatusNotFound)
	c.JSON(http.StatusNotFound, gin.H{"error": err.ToOpenAIError()})
}
