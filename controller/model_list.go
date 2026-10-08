package controller

import (
	"slices"
	"strconv"
	"sync"

	"openapi/common"
	"openapi/constant"
	"openapi/model"
	"openapi/relay"
	relaycommon "openapi/relay/common"

	gin "github.com/king54346/gin-tiny"
)

// 管理端的模型列表接口，参照 new-api controller/model.go，只包含本项目支持的渠道类型。
// 面向调用方的 /v1/models 见 relay.go 的 ListModels / RetrieveModel。

// builtinModelCatalog 各适配器内置的模型清单（普通接口 + 视频任务），进程内不变。
type builtinModelCatalog struct {
	byChannelType map[int][]string // 渠道类型 -> 内置模型
	models        []openAIModel    // 全部内置模型（按 id 去重，owned_by 为首个提供它的渠道）
}

// getBuiltinModels 首次调用时从适配器收集内置模型。
var getBuiltinModels = sync.OnceValue(func() builtinModelCatalog {
	catalog := builtinModelCatalog{byChannelType: make(map[int][]string)}

	channelTypes := make([]int, 0, len(constant.ChannelTypeNames))
	for t := range constant.ChannelTypeNames {
		if constant.IsRelaySupportedChannelType(t) {
			channelTypes = append(channelTypes, t)
		}
	}
	slices.Sort(channelTypes)

	seen := make(map[string]bool)
	for _, channelType := range channelTypes {
		var names []string
		owner := constant.GetChannelTypeName(channelType)
		if apiType, ok := constant.ChannelType2APIType(channelType); ok {
			if adaptor := relay.GetAdaptor(apiType); adaptor != nil {
				// OpenAI 适配器按渠道类型返回不同清单（如 OpenRouter），需先 Init
				adaptor.Init(&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: channelType}})
				names = append(names, adaptor.GetModelList()...)
				owner = adaptor.GetChannelName()
			}
		}
		if taskAdaptor := relay.GetTaskAdaptor(constant.TaskPlatform(strconv.Itoa(channelType))); taskAdaptor != nil {
			names = append(names, taskAdaptor.GetModelList()...)
		}
		names = uniqueStrings(names)
		catalog.byChannelType[channelType] = names
		for _, name := range names {
			if !seen[name] {
				seen[name] = true
				catalog.models = append(catalog.models, openAIModel{Id: name, Object: "model", Created: common.StartTime, OwnedBy: owner})
			}
		}
	}
	return catalog
})

// uniqueStrings 去掉空串与重复项，保持原有顺序。
func uniqueStrings(items []string) []string {
	seen := make(map[string]bool, len(items))
	out := make([]string, 0, len(items))
	for _, s := range items {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// ChannelListModels 全部适配器内置的模型（OpenAI 格式），供新建/编辑渠道时选择模型。
func ChannelListModels(c gin.Context) {
	apiSuccess(c, getBuiltinModels().models)
}

// DashboardListModels 渠道类型 -> 该类型内置的模型列表，供前端按渠道类型预填模型。
func DashboardListModels(c gin.Context) {
	apiSuccess(c, getBuiltinModels().byChannelType)
}

// EnabledListModels 当前已启用渠道实际提供的模型（去重、排序）。
func EnabledListModels(c gin.Context) {
	models := model.GetEnabledModels()
	if models == nil {
		models = []string{}
	}
	apiSuccess(c, models)
}
