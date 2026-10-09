package helper

import (
	"errors"
	"fmt"
	"strings"

	"openapi/common"
	"openapi/dto"
	relaycommon "openapi/relay/common"
	relayconstant "openapi/relay/constant"

	gin "github.com/king54346/gin-tiny"
)

// CompactModelSuffix 标记 responses compact 请求的模型名后缀，用于在日志和统计中区分普通请求
const CompactModelSuffix = "-openai-compact"

func withCompactModelSuffix(modelName string) string {
	if strings.HasSuffix(modelName, CompactModelSuffix) {
		return modelName
	}
	return modelName + CompactModelSuffix
}

func ModelMappedHelper(c gin.Context, info *relaycommon.RelayInfo, request dto.Request) error {
	if info.ChannelMeta == nil {
		info.ChannelMeta = &relaycommon.ChannelMeta{}
	}

	isResponsesCompact := info.RelayMode == relayconstant.RelayModeResponsesCompact

	// map model name
	modelMapping := c.GetString("model_mapping")
	upstreamModel, mapped, err := ResolveModelMapping(info.OriginModelName, modelMapping, isResponsesCompact)
	if err != nil {
		return err
	}
	info.IsModelMapped = mapped
	if isResponsesCompact {
		info.UpstreamModelName = upstreamModel
		info.OriginModelName = withCompactModelSuffix(upstreamModel)
	} else if mapped {
		info.UpstreamModelName = upstreamModel
	}
	if request != nil {
		request.SetModelName(info.UpstreamModelName)
	}
	return nil
}

// ResolveModelMapping interprets model_mapping JSON and follows chained redirects.
// It returns the final upstream model name and whether a mapping was applied.
func ResolveModelMapping(originModelName string, modelMapping string, isResponsesCompact bool) (string, bool, error) {
	mappingModelName := originModelName
	if isResponsesCompact && strings.HasSuffix(originModelName, CompactModelSuffix) {
		mappingModelName = strings.TrimSuffix(originModelName, CompactModelSuffix)
	}

	if modelMapping == "" || modelMapping == "{}" {
		return mappingModelName, false, nil
	}
	modelMap := make(map[string]string)
	if err := common.Unmarshal([]byte(modelMapping), &modelMap); err != nil {
		return "", false, fmt.Errorf("unmarshal_model_mapping_failed")
	}

	// 支持链式模型重定向，最终使用链尾的模型
	currentModel := mappingModelName
	isMapped := false
	visitedModels := map[string]bool{
		currentModel: true,
	}
	const maxRedirectHops = 32
	for hops := 0; hops < maxRedirectHops; hops++ {
		mappedModel, exists := modelMap[currentModel]
		if !exists || mappedModel == "" {
			return currentModel, isMapped, nil
		}
		// 模型重定向循环检测，避免无限循环
		if visitedModels[mappedModel] {
			if mappedModel == currentModel {
				if currentModel == originModelName {
					return currentModel, false, nil
				}
				return currentModel, true, nil
			}
			return "", false, errors.New("model_mapping_contains_cycle")
		}
		visitedModels[mappedModel] = true
		currentModel = mappedModel
		isMapped = true
	}
	return "", false, errors.New("model_mapping_too_deep")
}
