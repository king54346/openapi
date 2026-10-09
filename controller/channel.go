package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"openapi/common"
	"openapi/constant"
	"openapi/model"
	"openapi/service"

	gin "github.com/king54346/gin-tiny"
)

// 渠道管理接口（需管理员），参照 new-api controller/channel.go，只保留本项目支持的渠道类型。
// 渠道变更后立即刷新渠道缓存，转发马上使用新配置。
// 渠道 key 只通过 GetChannelKey（root）返回，其余接口一律不输出。

// parseStatusFilter status 参数：enabled/1 只看启用，disabled/0 只看禁用（含手动与自动），其余不过滤。
func parseStatusFilter(statusParam string) model.ChannelStatusFilter {
	switch strings.ToLower(statusParam) {
	case "enabled", "1":
		return model.ChannelStatusOnlyEnabled
	case "disabled", "0":
		return model.ChannelStatusOnlyDisabled
	default:
		return model.ChannelStatusAny
	}
}

// parseChannelFilter 解析渠道列表 / 搜索的筛选参数：keyword、group、model、status、type、id_sort。
func parseChannelFilter(c gin.Context) model.ChannelFilter {
	channelType, _ := strconv.Atoi(c.Query("type"))
	idSort, _ := strconv.ParseBool(c.Query("id_sort"))
	return model.ChannelFilter{
		Keyword: c.Query("keyword"),
		Group:   c.Query("group"),
		Model:   c.Query("model"),
		Status:  parseStatusFilter(c.Query("status")),
		Type:    channelType,
		IdSort:  idSort,
	}
}

// clearChannelInfo 列表与详情不输出多 key 的禁用原因与时间（可能含上游报错细节）。
func clearChannelInfo(channel *model.Channel) {
	channel.Key = ""
	if channel.ChannelInfo.IsMultiKey {
		channel.ChannelInfo.MultiKeyDisabledReason = nil
		channel.ChannelInfo.MultiKeyDisabledTime = nil
	}
}

// GetAllChannels 分页列出渠道，筛选、计数、分页都在数据库完成。
// 参数：p、page_size、keyword（id/名称/base_url/完整 key）、group、model、status（enabled/disabled）、type、id_sort；
// tag_mode=true 时按标签分页（total 为标签数），返回当页各标签下满足条件的全部渠道。
// 额外返回 type_counts：满足除 type 外其余条件的渠道按类型计数。
func GetAllChannels(c gin.Context) {
	page := common.GetPageQuery(c)
	filter := parseChannelFilter(c)
	tagMode, _ := strconv.ParseBool(c.Query("tag_mode"))

	var channels []*model.Channel
	var total int64
	var err error
	if tagMode {
		var tags []string
		if tags, total, err = model.ListChannelTags(filter, page.GetStartIdx(), page.GetPageSize()); err == nil {
			channels, err = model.ListChannelsByTags(filter, tags)
		}
	} else {
		channels, total, err = model.ListChannels(filter, page.GetStartIdx(), page.GetPageSize())
	}
	if err != nil {
		apiError(c, err)
		return
	}
	typeCounts, err := model.CountChannelsByType(filter)
	if err != nil {
		apiError(c, err)
		return
	}
	for _, ch := range channels {
		clearChannelInfo(ch)
	}
	result := pageResult(page, channels, total)
	result["type_counts"] = typeCounts
	apiSuccess(c, result)
}

// SearchChannels 搜索渠道，参数与返回同 GetAllChannels。
func SearchChannels(c gin.Context) {
	GetAllChannels(c)
}

// GetChannel 取单个渠道（不含 key）。
func GetChannel(c gin.Context) {
	id, ok := paramID(c)
	if !ok {
		return
	}
	channel, err := model.GetChannelById(id, false)
	if err != nil {
		apiErrorMsg(c, "channel not found")
		return
	}
	clearChannelInfo(channel)
	apiSuccess(c, channel)
}

// GetChannelKey 取渠道 key（路由已限制 root），并记一条系统日志。
func GetChannelKey(c gin.Context) {
	id, ok := paramID(c)
	if !ok {
		return
	}
	channel, err := model.GetChannelById(id, true)
	if err != nil {
		apiErrorMsg(c, "channel not found")
		return
	}
	model.RecordLog(c.GetInt("id"), model.LogTypeSystem, fmt.Sprintf("viewed key of channel #%d", id))
	apiSuccess(c, gin.H{"key": channel.Key})
}

// validateChannel 新增/修改渠道时的校验。
func validateChannel(ch *model.Channel, isAdd bool) error {
	if err := ch.ValidateSettings(); err != nil {
		return fmt.Errorf("invalid channel setting: %w", err)
	}
	if isAdd {
		if strings.TrimSpace(ch.Name) == "" {
			return errors.New("channel name is required")
		}
		if strings.TrimSpace(ch.Key) == "" {
			return errors.New("channel key is required")
		}
		if !constant.IsRelaySupportedChannelType(ch.Type) {
			return fmt.Errorf("channel type %d is not supported", ch.Type)
		}
		if len(ch.GetModels()) == 0 {
			return errors.New("channel models are required")
		}
	}
	for _, m := range ch.GetModels() {
		if len(m) > 255 {
			return fmt.Errorf("model name too long: %s", m)
		}
	}
	for name, v := range map[string]*string{"param_override": ch.ParamOverride, "header_override": ch.HeaderOverride} {
		if v != nil && strings.TrimSpace(*v) != "" && !json.Valid([]byte(*v)) {
			return fmt.Errorf("%s must be valid JSON", name)
		}
	}
	return nil
}

// splitKeys 按行拆分 key，去掉空行与首尾空白。
func splitKeys(raw string) []string {
	var keys []string
	for _, key := range strings.Split(raw, "\n") {
		if key = strings.TrimSpace(key); key != "" {
			keys = append(keys, key)
		}
	}
	return keys
}

// addChannelRequest 新增渠道请求。mode：
//   - single（默认）：整段 key 建一个渠道
//   - batch：按行拆分，每个 key 建一个渠道；batch_add_set_key_prefix_2_name=true 时渠道名追加 key 前 8 位
//   - multi_to_single：按行拆分后建一个多 key 渠道，multi_key_mode 为 random 或 polling
type addChannelRequest struct {
	Mode                      string                `json:"mode"`
	MultiKeyMode              constant.MultiKeyMode `json:"multi_key_mode"`
	BatchAddSetKeyPrefix2Name bool                  `json:"batch_add_set_key_prefix_2_name"`
	Channel                   *model.Channel        `json:"channel"`
}

// AddChannel 新增渠道。
func AddChannel(c gin.Context) {
	var req addChannelRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Channel == nil {
		apiErrorMsg(c, "invalid request")
		return
	}
	base := *req.Channel
	base.Id = 0
	base.CreatedTime = common.GetTimestamp()
	if base.Group == "" {
		base.Group = "default"
	}
	if base.Status == 0 {
		base.Status = common.ChannelStatusEnabled
	}
	if err := validateChannel(&base, true); err != nil {
		apiErrorMsg(c, err.Error())
		return
	}

	var keys []string
	switch req.Mode {
	case "", "single":
		keys = []string{base.Key}
	case "batch":
		keys = splitKeys(base.Key)
	case "multi_to_single":
		cleanKeys := splitKeys(base.Key)
		if len(cleanKeys) == 0 {
			apiErrorMsg(c, "channel key is required")
			return
		}
		mode := req.MultiKeyMode
		if mode == "" {
			mode = constant.MultiKeyModeRandom
		}
		if mode != constant.MultiKeyModeRandom && mode != constant.MultiKeyModePolling {
			apiErrorMsg(c, "multi_key_mode must be random or polling")
			return
		}
		base.ChannelInfo.IsMultiKey = true
		base.ChannelInfo.MultiKeyMode = mode
		base.ChannelInfo.MultiKeySize = len(cleanKeys)
		base.Key = strings.Join(cleanKeys, "\n")
		keys = []string{base.Key}
	default:
		apiErrorMsg(c, "unsupported mode: "+req.Mode)
		return
	}
	if len(keys) == 0 {
		apiErrorMsg(c, "channel key is required")
		return
	}

	channels := make([]model.Channel, 0, len(keys))
	for _, key := range keys {
		ch := base
		ch.Key = key
		if req.BatchAddSetKeyPrefix2Name && len(keys) > 1 {
			ch.Name = fmt.Sprintf("%s %s", base.Name, key[:min(8, len(key))])
		}
		channels = append(channels, ch)
	}
	if err := model.BatchInsertChannels(channels); err != nil {
		apiError(c, err)
		return
	}
	service.ResetProxyClientCache()
	model.RefreshChannelCache()
	ids := make([]int, 0, len(channels))
	for _, ch := range channels {
		ids = append(ids, ch.Id)
	}
	apiSuccess(c, gin.H{"count": len(channels), "ids": ids})
}

// patchChannel 修改渠道请求：multi_key_mode 修改多 key 选择方式；
// key_mode=append 时把 key 追加到多 key 渠道（自动去重），否则整体替换。
type patchChannel struct {
	model.Channel
	MultiKeyMode *string `json:"multi_key_mode"`
	KeyMode      *string `json:"key_mode"`
}

// UpdateChannel 修改渠道；key 留空表示不修改，零值字段不更新。多 key 状态始终沿用库中的值。
func UpdateChannel(c gin.Context) {
	var req patchChannel
	if err := c.ShouldBindJSON(&req); err != nil {
		apiErrorMsg(c, "invalid request: "+err.Error())
		return
	}
	if req.Id <= 0 {
		apiErrorMsg(c, "invalid id")
		return
	}
	origin, err := model.GetChannelById(req.Id, true)
	if err != nil {
		apiErrorMsg(c, "channel not found")
		return
	}
	if req.Type != 0 && req.Type != origin.Type && !constant.IsRelaySupportedChannelType(req.Type) {
		apiErrorMsg(c, fmt.Sprintf("channel type %d is not supported", req.Type))
		return
	}
	if err := validateChannel(&req.Channel, false); err != nil {
		apiErrorMsg(c, err.Error())
		return
	}

	// 客户端通常不回传 channel_info，沿用库中的多 key 状态，避免被清空
	req.ChannelInfo = origin.ChannelInfo
	if req.MultiKeyMode != nil && *req.MultiKeyMode != "" {
		mode := constant.MultiKeyMode(*req.MultiKeyMode)
		if mode != constant.MultiKeyModeRandom && mode != constant.MultiKeyModePolling {
			apiErrorMsg(c, "multi_key_mode must be random or polling")
			return
		}
		req.ChannelInfo.MultiKeyMode = mode
	}
	if req.KeyMode != nil && *req.KeyMode == "append" && req.ChannelInfo.IsMultiKey && req.Key != "" {
		existing := origin.GetKeys()
		seen := make(map[string]struct{}, len(existing))
		for _, k := range existing {
			seen[strings.TrimSpace(k)] = struct{}{}
		}
		for _, k := range splitKeys(req.Key) {
			if _, dup := seen[k]; !dup {
				seen[k] = struct{}{}
				existing = append(existing, k)
			}
		}
		req.Key = strings.Join(existing, "\n")
	}

	channel := req.Channel
	if err := channel.Update(); err != nil {
		apiError(c, err)
		return
	}
	service.ResetProxyClientCache()
	model.RefreshChannelCache()
	clearChannelInfo(&channel)
	apiSuccess(c, &channel)
}

// DeleteChannel 删除单个渠道。
func DeleteChannel(c gin.Context) {
	id, ok := paramID(c)
	if !ok {
		return
	}
	if err := (&model.Channel{Id: id}).Delete(); err != nil {
		apiError(c, err)
		return
	}
	model.RefreshChannelCache()
	apiOK(c)
}

// DeleteChannelBatch 批量删除渠道，请求体 {"ids": [...]}。
func DeleteChannelBatch(c gin.Context) {
	var req idsRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Ids) == 0 {
		apiErrorMsg(c, "ids is required")
		return
	}
	if err := model.BatchDeleteChannels(req.Ids); err != nil {
		apiError(c, err)
		return
	}
	model.RefreshChannelCache()
	apiSuccess(c, len(req.Ids))
}

// CopyChannel 复制渠道（含 key）。suffix 为新名称后缀（默认 "_复制"），
// reset_balance（默认 true）为 true 时清零余额与用量；测试时间与响应时间总是清零。
func CopyChannel(c gin.Context) {
	id, ok := paramID(c)
	if !ok {
		return
	}
	resetBalance := true
	if v, err := strconv.ParseBool(c.DefaultQuery("reset_balance", "true")); err == nil {
		resetBalance = v
	}
	origin, err := model.GetChannelById(id, true)
	if err != nil {
		apiErrorMsg(c, "channel not found")
		return
	}
	clone := *origin
	clone.Id = 0
	clone.Name += c.DefaultQuery("suffix", "_复制")
	clone.CreatedTime = common.GetTimestamp()
	clone.TestTime = 0
	clone.ResponseTime = 0
	if resetBalance {
		clone.Balance = 0
		clone.UsedQuota = 0
	}
	if err := clone.Insert(); err != nil {
		apiError(c, err)
		return
	}
	model.RefreshChannelCache()
	apiSuccess(c, gin.H{"id": clone.Id})
}

// DeleteDisabledChannel 删除所有已禁用（手动或自动）的渠道。
func DeleteDisabledChannel(c gin.Context) {
	rows, err := model.DeleteDisabledChannel()
	if err != nil {
		apiError(c, err)
		return
	}
	model.RefreshChannelCache()
	apiSuccess(c, rows)
}

type tagRequest struct {
	Tag            string  `json:"tag"`
	NewTag         *string `json:"new_tag"`
	ModelMapping   *string `json:"model_mapping"`
	Models         *string `json:"models"`
	Groups         *string `json:"groups"`
	Priority       *int64  `json:"priority"`
	Weight         *uint   `json:"weight"`
	ParamOverride  *string `json:"param_override"`
	HeaderOverride *string `json:"header_override"`
}

func bindTag(c gin.Context) (*tagRequest, bool) {
	var req tagRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Tag == "" {
		apiErrorMsg(c, "tag is required")
		return nil, false
	}
	return &req, true
}

// DisableTagChannels 禁用某标签下的所有渠道。
func DisableTagChannels(c gin.Context) {
	req, ok := bindTag(c)
	if !ok {
		return
	}
	if err := model.DisableChannelByTag(req.Tag); err != nil {
		apiError(c, err)
		return
	}
	model.RefreshChannelCache()
	apiOK(c)
}

// EnableTagChannels 启用某标签下的所有渠道。
func EnableTagChannels(c gin.Context) {
	req, ok := bindTag(c)
	if !ok {
		return
	}
	if err := model.EnableChannelByTag(req.Tag); err != nil {
		apiError(c, err)
		return
	}
	model.RefreshChannelCache()
	apiOK(c)
}

// trimJSONField 去掉首尾空白并校验 JSON，空串视为清空。
func trimJSONField(name string, v *string) (*string, error) {
	if v == nil {
		return nil, nil
	}
	trimmed := strings.TrimSpace(*v)
	if trimmed != "" && !json.Valid([]byte(trimmed)) {
		return nil, fmt.Errorf("%s must be valid JSON", name)
	}
	return &trimmed, nil
}

// EditTagChannels 批量修改某标签下渠道的公共字段（只修改请求中给出的字段）。
func EditTagChannels(c gin.Context) {
	req, ok := bindTag(c)
	if !ok {
		return
	}
	var err error
	if req.ParamOverride, err = trimJSONField("param_override", req.ParamOverride); err != nil {
		apiErrorMsg(c, err.Error())
		return
	}
	if req.HeaderOverride, err = trimJSONField("header_override", req.HeaderOverride); err != nil {
		apiErrorMsg(c, err.Error())
		return
	}
	if err := model.EditChannelByTag(req.Tag, req.NewTag, req.ModelMapping, req.Models, req.Groups,
		req.Priority, req.Weight, req.ParamOverride, req.HeaderOverride); err != nil {
		apiError(c, err)
		return
	}
	model.RefreshChannelCache()
	apiOK(c)
}

// BatchSetChannelTag 给一批渠道设置标签，请求体 {"ids": [...], "tag": "..."}，tag 为 null 时清除。
func BatchSetChannelTag(c gin.Context) {
	var req struct {
		Ids []int   `json:"ids"`
		Tag *string `json:"tag"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Ids) == 0 {
		apiErrorMsg(c, "ids is required")
		return
	}
	if err := model.BatchSetChannelTag(req.Ids, req.Tag); err != nil {
		apiError(c, err)
		return
	}
	model.RefreshChannelCache()
	apiSuccess(c, len(req.Ids))
}

// GetTagModels 返回某标签下模型最多的那个渠道的模型列表（逗号分隔），供前端编辑标签时预填。
func GetTagModels(c gin.Context) {
	tag := c.Query("tag")
	if tag == "" {
		apiErrorMsg(c, "tag is required")
		return
	}
	channels, err := model.GetChannelsByTag(tag, false, false)
	if err != nil {
		apiError(c, err)
		return
	}
	longest, maxCount := "", 0
	for _, ch := range channels {
		if n := len(ch.GetModels()); n > maxCount {
			maxCount, longest = n, ch.Models
		}
	}
	apiSuccess(c, longest)
}

// ---------- 上游模型列表 ----------

// fetchModelsTimeout 拉取上游模型列表的超时时间
const fetchModelsTimeout = 30 * time.Second

// maxModelsResponseBytes 上游模型列表响应体上限
const maxModelsResponseBytes = 10 << 20

// upstreamModelsURL 各渠道类型的模型列表地址。
func upstreamModelsURL(channelType int, baseURL string) string {
	baseURL = strings.TrimSuffix(baseURL, "/")
	if channelType == constant.ChannelTypeAli {
		return baseURL + "/compatible-mode/v1/models"
	}
	return baseURL + "/v1/models"
}

// fetchUpstreamModelIDs 请求上游 /models，返回模型 ID 列表。
// headerOverride 中的 {api_key} 会被替换为实际 key。
func fetchUpstreamModelIDs(url, key, proxy string, headerOverride map[string]any) ([]string, error) {
	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("create http client: %w", err)
	}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	for k, v := range headerOverride {
		str, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("invalid header override for %s", k)
		}
		req.Header.Set(k, strings.ReplaceAll(str, "{api_key}", key))
	}
	ctx, cancel := context.WithTimeout(context.Background(), fetchModelsTimeout)
	defer cancel()
	resp, err := client.Do(req.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("request upstream: %s", common.MaskSensitiveInfo(err.Error()))
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxModelsResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("read upstream response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream returned status %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := common.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse upstream response: %w", err)
	}
	ids := make([]string, 0, len(result.Data))
	for _, m := range result.Data {
		ids = append(ids, m.ID)
	}
	return ids, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// FetchUpstreamModels 用已保存渠道的配置（多 key 渠道取一个启用的 key）拉取上游模型列表。
func FetchUpstreamModels(c gin.Context) {
	id, ok := paramID(c)
	if !ok {
		return
	}
	channel, err := model.GetChannelById(id, true)
	if err != nil {
		apiErrorMsg(c, "channel not found")
		return
	}
	key, _, apiErr := channel.GetNextEnabledKey()
	if apiErr != nil {
		apiErrorMsg(c, "no available key: "+apiErr.Error())
		return
	}
	ids, err := fetchUpstreamModelIDs(upstreamModelsURL(channel.Type, channel.GetBaseURL()),
		strings.TrimSpace(key), channel.GetSetting().Proxy, channel.GetHeaderOverride())
	if err != nil {
		apiErrorMsg(c, "failed to fetch models: "+err.Error())
		return
	}
	apiSuccess(c, ids)
}

// FetchModels 新建渠道前用表单里的 base_url / type / key 拉取上游模型列表（多行 key 只取第一行）。
func FetchModels(c gin.Context) {
	var req struct {
		BaseURL string `json:"base_url"`
		Type    int    `json:"type"`
		Key     string `json:"key"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		apiErrorMsg(c, "invalid request")
		return
	}
	baseURL := req.BaseURL
	if baseURL == "" {
		baseURL = constant.ChannelBaseURLs[req.Type]
	}
	if baseURL == "" {
		apiErrorMsg(c, "base_url is required")
		return
	}
	key := strings.Split(strings.TrimSpace(req.Key), "\n")[0]
	ids, err := fetchUpstreamModelIDs(upstreamModelsURL(req.Type, baseURL), strings.TrimSpace(key), "", nil)
	if err != nil {
		apiErrorMsg(c, "failed to fetch models: "+err.Error())
		return
	}
	apiSuccess(c, ids)
}

// ---------- 多 key 管理 ----------

const (
	keyStatusEnabled        = common.ChannelStatusEnabled
	keyStatusManualDisabled = common.ChannelStatusManuallyDisabled
	keyStatusAutoDisabled   = common.ChannelStatusAutoDisabled
)

// multiKeyManageRequest action：get_key_status、disable_key、enable_key、enable_all_keys、
// disable_all_keys、delete_key、delete_disabled_keys（只删自动禁用的）。
type multiKeyManageRequest struct {
	ChannelId int    `json:"channel_id"`
	Action    string `json:"action"`
	KeyIndex  *int   `json:"key_index,omitempty"`
	Page      int    `json:"page,omitempty"`
	PageSize  int    `json:"page_size,omitempty"`
	Status    *int   `json:"status,omitempty"` // get_key_status 过滤：1 启用、2 手动禁用、3 自动禁用
}

type keyStatus struct {
	Index        int    `json:"index"`
	Status       int    `json:"status"`
	DisabledTime int64  `json:"disabled_time,omitempty"`
	Reason       string `json:"reason,omitempty"`
	KeyPreview   string `json:"key_preview"` // key 前 10 位，用于辨认
}

type multiKeyStatusResponse struct {
	Keys                []keyStatus `json:"keys"`
	Total               int         `json:"total"`
	Page                int         `json:"page"`
	PageSize            int         `json:"page_size"`
	TotalPages          int         `json:"total_pages"`
	EnabledCount        int         `json:"enabled_count"`
	ManualDisabledCount int         `json:"manual_disabled_count"`
	AutoDisabledCount   int         `json:"auto_disabled_count"`
}

// keyStatusOf 多 key 渠道中第 i 个 key 的状态（未记录即启用）。
func keyStatusOf(info *model.ChannelInfo, i int) int {
	if s, ok := info.MultiKeyStatusList[i]; ok {
		return s
	}
	return keyStatusEnabled
}

func ensureKeyStatusMaps(info *model.ChannelInfo) {
	if info.MultiKeyStatusList == nil {
		info.MultiKeyStatusList = make(map[int]int)
	}
	if info.MultiKeyDisabledTime == nil {
		info.MultiKeyDisabledTime = make(map[int]int64)
	}
	if info.MultiKeyDisabledReason == nil {
		info.MultiKeyDisabledReason = make(map[int]string)
	}
}

// removeKeys 删除 shouldDelete 为 true 的 key，其余 key 的状态按新下标重新编号，返回删除数量。
func removeKeys(channel *model.Channel, shouldDelete func(i, status int) bool) int {
	info := &channel.ChannelInfo
	var remaining []string
	statusList := make(map[int]int)
	disabledTime := make(map[int]int64)
	disabledReason := make(map[int]string)
	deleted := 0
	for i, key := range channel.GetKeys() {
		status := keyStatusOf(info, i)
		if shouldDelete(i, status) {
			deleted++
			continue
		}
		idx := len(remaining)
		remaining = append(remaining, key)
		if status != keyStatusEnabled {
			statusList[idx] = status
			if t, ok := info.MultiKeyDisabledTime[i]; ok {
				disabledTime[idx] = t
			}
			if r, ok := info.MultiKeyDisabledReason[i]; ok {
				disabledReason[idx] = r
			}
		}
	}
	if deleted == 0 || len(remaining) == 0 {
		return 0
	}
	channel.Key = strings.Join(remaining, "\n")
	channel.Keys = nil
	info.MultiKeySize = len(remaining)
	info.MultiKeyStatusList = statusList
	info.MultiKeyDisabledTime = disabledTime
	info.MultiKeyDisabledReason = disabledReason
	return deleted
}

// ManageMultiKeys 管理多 key 渠道中的单个或全部 key。
func ManageMultiKeys(c gin.Context) {
	var req multiKeyManageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiErrorMsg(c, "invalid request")
		return
	}
	channel, err := model.GetChannelById(req.ChannelId, true)
	if err != nil {
		apiErrorMsg(c, "channel not found")
		return
	}
	if !channel.ChannelInfo.IsMultiKey {
		apiErrorMsg(c, "channel is not in multi-key mode")
		return
	}

	// 与转发时的 key 轮询共用一把锁，避免并发修改状态
	lock := model.GetChannelPollingLock(channel.Id)
	lock.Lock()
	defer lock.Unlock()

	info := &channel.ChannelInfo
	checkIndex := func() (int, bool) {
		if req.KeyIndex == nil {
			apiErrorMsg(c, "key_index is required")
			return 0, false
		}
		if *req.KeyIndex < 0 || *req.KeyIndex >= info.MultiKeySize {
			apiErrorMsg(c, "key_index out of range")
			return 0, false
		}
		return *req.KeyIndex, true
	}
	save := func(message string, data any) {
		if err := channel.Update(); err != nil {
			apiError(c, err)
			return
		}
		model.RefreshChannelCache()
		c.JSON(http.StatusOK, gin.H{"success": true, "message": message, "data": data})
	}

	switch req.Action {
	case "get_key_status":
		apiSuccess(c, buildKeyStatus(channel, req))

	case "disable_key":
		idx, ok := checkIndex()
		if !ok {
			return
		}
		ensureKeyStatusMaps(info)
		info.MultiKeyStatusList[idx] = keyStatusManualDisabled
		info.MultiKeyDisabledTime[idx] = common.GetTimestamp()
		save("key disabled", nil)

	case "enable_key":
		idx, ok := checkIndex()
		if !ok {
			return
		}
		delete(info.MultiKeyStatusList, idx)
		delete(info.MultiKeyDisabledTime, idx)
		delete(info.MultiKeyDisabledReason, idx)
		save("key enabled", nil)

	case "enable_all_keys":
		count := len(info.MultiKeyStatusList)
		info.MultiKeyStatusList = make(map[int]int)
		info.MultiKeyDisabledTime = make(map[int]int64)
		info.MultiKeyDisabledReason = make(map[int]string)
		save(fmt.Sprintf("enabled %d keys", count), count)

	case "disable_all_keys":
		ensureKeyStatusMaps(info)
		count := 0
		for i := 0; i < info.MultiKeySize; i++ {
			if keyStatusOf(info, i) == keyStatusEnabled {
				info.MultiKeyStatusList[i] = keyStatusManualDisabled
				info.MultiKeyDisabledTime[i] = common.GetTimestamp()
				count++
			}
		}
		if count == 0 {
			apiErrorMsg(c, "no enabled keys to disable")
			return
		}
		save(fmt.Sprintf("disabled %d keys", count), count)

	case "delete_key":
		idx, ok := checkIndex()
		if !ok {
			return
		}
		if removeKeys(channel, func(i, _ int) bool { return i == idx }) == 0 {
			apiErrorMsg(c, "cannot delete the last key")
			return
		}
		save("key deleted", nil)

	case "delete_disabled_keys":
		count := removeKeys(channel, func(_, status int) bool { return status == keyStatusAutoDisabled })
		if count == 0 {
			apiErrorMsg(c, "no auto-disabled keys to delete (or all keys are auto-disabled)")
			return
		}
		save(fmt.Sprintf("deleted %d auto-disabled keys", count), count)

	default:
		apiErrorMsg(c, "unsupported action: "+req.Action)
	}
}

// buildKeyStatus get_key_status：统计全部 key，按 status 过滤后分页（默认每页 50）。
func buildKeyStatus(channel *model.Channel, req multiKeyManageRequest) multiKeyStatusResponse {
	info := &channel.ChannelInfo
	resp := multiKeyStatusResponse{Page: max(req.Page, 1), PageSize: req.PageSize}
	if resp.PageSize <= 0 {
		resp.PageSize = 50
	}
	var filtered []keyStatus
	for i, key := range channel.GetKeys() {
		status := keyStatusOf(info, i)
		switch status {
		case keyStatusEnabled:
			resp.EnabledCount++
		case keyStatusManualDisabled:
			resp.ManualDisabledCount++
		case keyStatusAutoDisabled:
			resp.AutoDisabledCount++
		}
		if req.Status != nil && *req.Status != status {
			continue
		}
		ks := keyStatus{Index: i, Status: status, KeyPreview: key}
		if len(key) > 10 {
			ks.KeyPreview = key[:10] + "..."
		}
		if status != keyStatusEnabled {
			ks.DisabledTime = info.MultiKeyDisabledTime[i]
			ks.Reason = info.MultiKeyDisabledReason[i]
		}
		filtered = append(filtered, ks)
	}
	resp.Total = len(filtered)
	resp.TotalPages = max((resp.Total+resp.PageSize-1)/resp.PageSize, 1)
	resp.Page = min(resp.Page, resp.TotalPages)
	start := min((resp.Page-1)*resp.PageSize, resp.Total)
	end := min(start+resp.PageSize, resp.Total)
	resp.Keys = filtered[start:end]
	return resp
}
