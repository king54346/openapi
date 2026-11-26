package modelmeta

import (
	"slices"
	"strconv"
	"sync"

	"openapi/common"
	"openapi/constant"
	"openapi/model"
	"openapi/relay"
	relaycommon "openapi/relay/common"
)

type OpenAIModelItem struct {
	Id      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

type builtinModelCatalog struct {
	byChannelType map[int][]string
	models        []OpenAIModelItem
}

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
				catalog.models = append(catalog.models, OpenAIModelItem{
					Id:      name,
					Object:  "model",
					Created: common.StartTime,
					OwnedBy: owner,
				})
			}
		}
	}
	return catalog
})

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

func ChannelListModels() []OpenAIModelItem {
	return getBuiltinModels().models
}

func DashboardListModels() map[int][]string {
	return getBuiltinModels().byChannelType
}

func EnabledListModels() []string {
	models := model.GetEnabledModels()
	if models == nil {
		return []string{}
	}
	return models
}
