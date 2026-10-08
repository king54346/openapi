package common_handler

import (
	"io"
	"net/http"

	"openapi/common"
	"openapi/dto"
	relaycommon "openapi/relay/common"
	"openapi/service"
	"openapi/types"

	gin "github.com/king54346/gin-tiny"
)

// RerankHandler 处理 Jina 格式的 rerank 响应
func RerankHandler(c gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.StarAPIError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	service.CloseResponseBodyGracefully(resp)
	if common.DebugEnabled {
		println("reranker response body: ", string(responseBody))
	}
	var jinaResp dto.RerankResponse
	if err = common.Unmarshal(responseBody, &jinaResp); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	jinaResp.Usage.PromptTokens = jinaResp.Usage.TotalTokens

	c.Response().Header().Set("Content-Type", "application/json")
	c.JSON(http.StatusOK, jinaResp)
	return &jinaResp.Usage, nil
}
