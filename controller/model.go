package controller

import (
	"errors"
	"openapi/common"
	"openapi/constant"
	"openapi/model"
	"openapi/relay"
	relaycommon "openapi/relay/common"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"

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
				// tokens.OpenAI 适配器按渠道类型返回不同清单（如 OpenRouter），需先 Init
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

// ChannelListModels 全部适配器内置的模型（tokens.OpenAI 格式），供新建/编辑渠道时选择模型。
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

// 模型元数据管理接口（需管理员），参照 new-api controller/model_meta.go。
// 附加信息（提供该模型的渠道、可用分组、规则匹配到的模型）来自渠道缓存；本项目不做计费，没有定价刷新。

// GetAllModelsMeta 分页列出模型元数据，附带各供应商的模型数量。
func GetAllModelsMeta(c gin.Context) {
	pageInfo := common.GetPageQuery(c)
	models, total, err := model.GetAllModels(pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		apiError(c, err)
		return
	}
	model.EnrichModels(models)
	vendorCounts, _ := model.GetVendorModelCounts()
	result := pageResult(pageInfo, models, total)
	result["vendor_counts"] = vendorCounts
	apiSuccess(c, result)
}

// GetModelTags 所有模型使用过的标签。
func GetModelTags(c gin.Context) {
	tags, err := model.GetAllModelTags()
	if err != nil {
		apiError(c, err)
		return
	}
	apiSuccess(c, tags)
}

// SearchModelsMeta 按 keyword（名称/描述/标签）、vendor（id 或名称）、tags（逗号分隔，需全部命中）搜索。
func SearchModelsMeta(c gin.Context) {
	var tags []string
	for _, t := range strings.Split(c.Query("tags"), ",") {
		if t = strings.TrimSpace(t); t != "" {
			tags = append(tags, t)
		}
	}
	pageInfo := common.GetPageQuery(c)
	models, total, err := model.SearchModels(c.Query("keyword"), c.Query("vendor"), tags, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		apiError(c, err)
		return
	}
	model.EnrichModels(models)
	apiSuccess(c, pageResult(pageInfo, models, total))
}

func modelMetaNotFoundOrError(c gin.Context, err error) {
	if errors.Is(err, model.ErrModelMetaNotFound) {
		apiErrorMsg(c, "model not found")
		return
	}
	apiError(c, err)
}

// GetModelMeta 单个模型元数据。
func GetModelMeta(c gin.Context) {
	id, ok := paramID(c)
	if !ok {
		return
	}
	m, err := model.GetModelMetaById(id)
	if err != nil {
		modelMetaNotFoundOrError(c, err)
		return
	}
	model.EnrichModels([]*model.Model{m})
	apiSuccess(c, m)
}

// checkModelName 校验名称非空且不与其他模型重名，失败时已写回错误。
func checkModelName(c gin.Context, id int, name string) bool {
	if strings.TrimSpace(name) == "" {
		apiErrorMsg(c, "model_name is required")
		return false
	}
	dup, err := model.IsModelNameDuplicated(id, name)
	if err != nil {
		apiError(c, err)
		return false
	}
	if dup {
		apiErrorMsg(c, "model_name already exists")
		return false
	}
	return true
}

// CreateModelMeta 新建模型元数据。
func CreateModelMeta(c gin.Context) {
	var m model.Model
	if err := c.ShouldBindJSON(&m); err != nil {
		apiErrorMsg(c, "invalid request: "+err.Error())
		return
	}
	m.Id = 0
	m.ModelName = strings.TrimSpace(m.ModelName)
	if !checkModelName(c, 0, m.ModelName) {
		return
	}
	if err := m.Insert(); err != nil {
		apiError(c, err)
		return
	}
	apiSuccess(c, &m)
}

// UpdateModelMeta 修改模型元数据；?status_only=true 时只修改状态。
func UpdateModelMeta(c gin.Context) {
	var m model.Model
	if err := c.ShouldBindJSON(&m); err != nil {
		apiErrorMsg(c, "invalid request: "+err.Error())
		return
	}
	if m.Id <= 0 {
		apiErrorMsg(c, "id is required")
		return
	}
	if _, err := model.GetModelMetaById(m.Id); err != nil {
		modelMetaNotFoundOrError(c, err)
		return
	}
	if c.Query("status_only") == "true" {
		if err := model.UpdateModelMetaStatus(m.Id, m.Status); err != nil {
			apiError(c, err)
			return
		}
	} else {
		m.ModelName = strings.TrimSpace(m.ModelName)
		if !checkModelName(c, m.Id, m.ModelName) {
			return
		}
		if err := m.Update(); err != nil {
			apiError(c, err)
			return
		}
	}
	updated, err := model.GetModelMetaById(m.Id)
	if err != nil {
		apiError(c, err)
		return
	}
	apiSuccess(c, updated)
}

// DeleteModelMeta 删除模型元数据（软删除）。
func DeleteModelMeta(c gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		apiErrorMsg(c, "invalid id")
		return
	}
	if err := model.DeleteModelMetaById(id); err != nil {
		apiError(c, err)
		return
	}
	apiOK(c)
}

// 模型元数据与渠道模型列表的同步：
// 找出已被渠道提供但还没登记到 models 表的模型，并批量补录入库。

// missingModels 返回提供方有、models 表里没有的模型名（已排序）。
func missingModels(candidate []string) ([]string, error) {
	if len(candidate) == 0 {
		return []string{}, nil
	}
	var existing []string
	if err := model.DB.Model(&model.Model{}).Where("model_name IN ?", candidate).Pluck("model_name", &existing).Error; err != nil {
		return nil, err
	}
	known := make(map[string]bool, len(existing))
	for _, name := range existing {
		known[name] = true
	}
	missing := make([]string, 0, len(candidate))
	for _, name := range candidate {
		if name != "" && !known[name] {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	return missing, nil
}

// GetMissingModels GET /api/models/missing：已启用渠道提供但未登记的模型。
func GetMissingModels(c gin.Context) {
	missing, err := missingModels(model.GetEnabledModels())
	if err != nil {
		apiError(c, err)
		return
	}
	apiSuccess(c, gin.H{"models": missing, "total": len(missing)})
}

// channelsForSync 解析要同步的渠道：channel_ids 为空时取所有启用渠道。
func channelsForSync(channelIds []int) ([]*model.Channel, error) {
	if len(channelIds) > 0 {
		return model.GetChannelsByIds(channelIds)
	}
	var channels []*model.Channel
	err := model.DB.Where("status = ?", common.ChannelStatusEnabled).Order("id").Find(&channels).Error
	return channels, err
}

// SyncUpstreamPreview GET /api/models/sync_upstream/preview：预览要新建与已存在的模型。
func SyncUpstreamPreview(c gin.Context) {
	channelIds := make([]int, 0)
	for _, raw := range strings.Split(c.Query("channel_ids"), ",") {
		if id := common.String2Int(strings.TrimSpace(raw)); id > 0 {
			channelIds = append(channelIds, id)
		}
	}
	channels, err := channelsForSync(channelIds)
	if err != nil {
		apiError(c, err)
		return
	}

	// 汇总渠道提供的全部模型，并记录每个模型出现的分组
	seen := make(map[string]bool)
	provided := make([]string, 0)
	for _, channel := range channels {
		for _, modelName := range channel.GetModels() {
			if modelName = strings.TrimSpace(modelName); modelName != "" && !seen[modelName] {
				seen[modelName] = true
				provided = append(provided, modelName)
			}
		}
	}
	sort.Strings(provided)

	missing, err := missingModels(provided)
	if err != nil {
		apiError(c, err)
		return
	}
	missingSet := make(map[string]bool, len(missing))
	for _, name := range missing {
		missingSet[name] = true
	}
	existing := make([]string, 0, len(provided)-len(missing))
	for _, name := range provided {
		if !missingSet[name] {
			existing = append(existing, name)
		}
	}

	apiSuccess(c, gin.H{
		"create": missing,
		"update": existing,
		"total":  len(provided),
	})
}

// SyncUpstreamModels POST /api/models/sync_upstream：把缺失模型批量登记进 models 表。
func SyncUpstreamModels(c gin.Context) {
	var req struct {
		ChannelIds []int `json:"channel_ids"`
		DryRun     bool  `json:"dry_run"`
	}
	_ = c.ShouldBindJSON(&req)

	channels, err := channelsForSync(req.ChannelIds)
	if err != nil {
		apiError(c, err)
		return
	}

	seen := make(map[string]bool)
	provided := make([]string, 0)
	for _, channel := range channels {
		for _, modelName := range channel.GetModels() {
			if modelName = strings.TrimSpace(modelName); modelName != "" && !seen[modelName] {
				seen[modelName] = true
				provided = append(provided, modelName)
			}
		}
	}

	missing, err := missingModels(provided)
	if err != nil {
		apiError(c, err)
		return
	}
	if req.DryRun {
		apiSuccess(c, gin.H{"created": 0, "skipped": len(missing), "dry_run": true, "models": missing})
		return
	}

	created := 0
	for _, modelName := range missing {
		meta := &model.Model{ModelName: modelName, Status: 1, SyncOfficial: 1}
		if err := meta.Insert(); err != nil {
			// 并发或唯一索引冲突时跳过该模型，不影响其它模型入库
			common.SysError("failed to create model metadata " + modelName + ": " + err.Error())
			continue
		}
		created++
	}
	apiSuccess(c, gin.H{"created": created, "skipped": len(provided) - created})
}

// 可用模型查询。分组概念已移除：所有已启用渠道提供的模型对所有用户可见。

// GetUserModels 当前用户可调用的模型列表（已排序去重）。
func GetUserModels(c gin.Context) {
	apiSuccess(c, sortedModels(model.GetEnabledModels()))
}

// sortedModels 去空、排序，保证返回稳定。
func sortedModels(models []string) []string {
	out := make([]string, 0, len(models))
	for _, modelName := range models {
		if modelName != "" {
			out = append(out, modelName)
		}
	}
	sort.Strings(out)
	return out
}
