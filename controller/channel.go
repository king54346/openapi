package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"openapi/common"
	"openapi/constant"
	"openapi/middleware"
	"openapi/model"
	relaycommon "openapi/relay/common"
	"openapi/relay/helper"
	"openapi/service/channels"
	"openapi/types"

	gin "github.com/king54346/gin-tiny"
)

// 渠道管理接口（需管理员），参照 new-api controller/go，只保留本项目支持的渠道类型。
// 渠道变更后立即刷新渠道缓存，转发马上使用新配置。
// 渠道 key 只通过 GetChannelKey（root）返回，其余接口一律不输出。

// parseStatusFilter status 参数：enabled/1 只看启用，disabled/0 只看禁用（含手动与自动），其余不过滤。
func parseStatusFilter(statusParam string) ChannelStatusFilter {
	switch strings.ToLower(statusParam) {
	case "enabled", "1":
		return ChannelStatusOnlyEnabled
	case "disabled", "0":
		return ChannelStatusOnlyDisabled
	default:
		return ChannelStatusAny
	}
}

// parseChannelFilter 解析渠道列表 / 搜索的筛选参数：keyword、group、model、status、type、id_sort。
func parseChannelFilter(c gin.Context) ChannelFilter {
	channelType, _ := strconv.Atoi(c.Query("type"))
	idSort, _ := strconv.ParseBool(c.Query("id_sort"))
	return ChannelFilter{
		Keyword: c.Query("keyword"),
		Model:   c.Query("model"),
		Status:  parseStatusFilter(c.Query("status")),
		Type:    channelType,
		IdSort:  idSort,
	}
}

// clearChannelInfo 列表与详情不输出多 key 的禁用原因与时间（可能含上游报错细节）。
func clearChannelInfo(channel *Channel) {
	channel.Key = ""
	if ChannelInfo.IsMultiKey {
		ChannelInfo.MultiKeyDisabledReason = nil
		ChannelInfo.MultiKeyDisabledTime = nil
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

	var channels []*Channel
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
func validateChannel(ch *Channel, isAdd bool) error {
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
	Mode                      string       `json:"mode"`
	MultiKeyMode              MultiKeyMode `json:"multi_key_mode"`
	BatchAddSetKeyPrefix2Name bool         `json:"batch_add_set_key_prefix_2_name"`
	Channel                   *Channel     `json:"channel"`
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
	if base.Status == 0 {
		base.Status = ChannelStatusEnabled
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
			mode = MultiKeyModeRandom
		}
		if mode != MultiKeyModeRandom && mode != MultiKeyModePolling {
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

	channels := make([]Channel, 0, len(keys))
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
	httpclient.ResetProxyClientCache()
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
	Channel
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
		mode := MultiKeyMode(*req.MultiKeyMode)
		if mode != MultiKeyModeRandom && mode != MultiKeyModePolling {
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
	httpclient.ResetProxyClientCache()
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
	if err := (&Channel{Id: id}).Delete(); err != nil {
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
	rows, err := DeleteDisabledChannel()
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
	if err := model.EditChannelByTag(req.Tag, req.NewTag, req.ModelMapping, req.Models,
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
	if err := BatchSetChannelTag(req.Ids, req.Tag); err != nil {
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
	if channelType == ChannelTypeAli {
		return baseURL + "/compatible-mode/v1/models"
	}
	return baseURL + "/v1/models"
}

// fetchUpstreamModelIDs 请求上游 /models，返回模型 ID 列表。
// headerOverride 中的 {api_key} 会被替换为实际 key。
func fetchUpstreamModelIDs(url, key, proxy string, headerOverride map[string]any) ([]string, error) {
	client, err := httpclient.GetHttpClientWithProxy(proxy)
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
		baseURL = ChannelBaseURLs[req.Type]
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
	keyStatusEnabled        = ChannelStatusEnabled
	keyStatusManualDisabled = ChannelStatusManuallyDisabled
	keyStatusAutoDisabled   = ChannelStatusAutoDisabled
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
func keyStatusOf(info *ChannelInfo, i int) int {
	if s, ok := info.MultiKeyStatusList[i]; ok {
		return s
	}
	return keyStatusEnabled
}

func ensureKeyStatusMaps(info *ChannelInfo) {
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
func removeKeys(channel *Channel, shouldDelete func(i, status int) bool) int {
	info := &ChannelInfo
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
	Keys = nil
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
	if !ChannelInfo.IsMultiKey {
		apiErrorMsg(c, "channel is not in multi-key mode")
		return
	}

	// 与转发时的 key 轮询共用一把锁，避免并发修改状态
	lock := model.GetChannelPollingLock(channel.Id)
	lock.Lock()
	defer lock.Unlock()

	info := &ChannelInfo
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
func buildKeyStatus(channel *Channel, req multiKeyManageRequest) multiKeyStatusResponse {
	info := &ChannelInfo
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

// 渠道测试：用最小请求走一遍真实的转发流程（模型映射、参数覆盖、渠道适配器都会生效），
// 以此判断渠道是否可用。参照 new-api controller/channel-test.go。

// channelTestTimeout 单个渠道的测试超时
const channelTestTimeout = 60 * time.Second

// channelTestResult 一次渠道测试的结果
type channelTestResult struct {
	Model      string
	Path       string
	Latency    time.Duration
	Err        *types.StarAPIError // 为 nil 且未跳过表示测试通过
	Skipped    bool                // 不支持测试（如视频、图片模型），不计入成功或失败
	SkipReason string
	UsingKey   string // 本次使用的 key（多 key 渠道自动禁用时只禁用该 key）
}

func (r channelTestResult) ok() bool { return !r.Skipped && r.Err == nil }

// 这些模型按名称识别为图片 / 视频生成，测试成本高，不做自动测试
var expensiveModelMarkers = []string{"seedream", "seedance", "dall-e", "gpt-image", "sora", "wan2", "kling", "vidu", "hailuo", "image"}

// resolveTestModel 测试模型：优先使用指定模型，其次渠道的 test_model，最后取渠道第一个模型。
func resolveTestModel(channel *Channel, testModel string) string {
	if testModel = strings.TrimSpace(testModel); testModel != "" {
		return testModel
	}
	if channel.TestModel != nil && strings.TrimSpace(*channel.TestModel) != "" {
		return strings.TrimSpace(*channel.TestModel)
	}
	for _, m := range channel.GetModels() {
		if m = strings.TrimSpace(m); m != "" {
			return m
		}
	}
	return ""
}

// testRequestFor 按模型名决定测试接口与请求体；返回空 path 表示不支持测试。
func testRequestFor(modelName string) (path string, format types.RelayFormat, body map[string]any, skipReason string) {
	lower := strings.ToLower(modelName)
	for _, marker := range expensiveModelMarkers {
		if strings.Contains(lower, marker) {
			return "", "", nil, "image/video model is not tested automatically: " + modelName
		}
	}
	switch {
	case strings.Contains(lower, "rerank"):
		return "/v1/rerank", types.RelayFormatRerank, map[string]any{"model": modelName, "query": "hi", "documents": []string{"hi"}}, ""
	case strings.Contains(lower, "embed") || strings.Contains(lower, "bge-") || strings.HasPrefix(lower, "m3e"):
		return "/v1/embeddings", types.RelayFormatEmbedding, map[string]any{"model": modelName, "input": "hi"}, ""
	}
	return "/v1/chat/completions", types.RelayFormatOpenAI, map[string]any{
		"model":      modelName,
		"max_tokens": 16,
		"messages":   []map[string]string{{"role": "user", "content": "hi"}},
	}, ""
}

// testChannel 测试单个渠道，超过 channelTestTimeout 视为失败。
func testChannel(channel *Channel, testModel string) channelTestResult {
	done := make(chan channelTestResult, 1)
	go func() { done <- runChannelTest(channel, testModel) }()
	select {
	case res := <-done:
		return res
	case <-time.After(channelTestTimeout):
		return channelTestResult{
			Model:   resolveTestModel(channel, testModel),
			Latency: channelTestTimeout,
			Err: types.NewErrorWithStatusCode(fmt.Errorf("channel test timed out after %s", channelTestTimeout),
				ErrorCodeDoRequestFailed, http.StatusGatewayTimeout),
		}
	}
}

func runChannelTest(channel *Channel, testModel string) (res channelTestResult) {
	res.Model = resolveTestModel(channel, testModel)
	if !constant.IsRelaySupportedChannelType(channel.Type) {
		res.Skipped, res.SkipReason = true, fmt.Sprintf("channel type %d has no relay implementation", channel.Type)
		return res
	}
	if _, ok := ChannelType2APIType(channel.Type); !ok {
		res.Skipped, res.SkipReason = true, "video-only channel is not tested automatically"
		return res
	}
	if res.Model == "" {
		res.Err = types.NewErrorWithStatusCode(errors.New("channel has no model to test"), ErrorCodeInvalidRequest,
			http.StatusBadRequest, ErrOptionWithSkipRetry())
		return res
	}
	path, format, body, skipReason := testRequestFor(res.Model)
	if path == "" {
		res.Skipped, res.SkipReason = true, skipReason
		return res
	}
	res.Path = path

	// 构造一个内部请求：用户 id 为 0，日志中令牌名记为 channel-test
	payload, _ := common.Marshal(body)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(payload)))
	req.Header.Set("Content-Type", "application/json")
	c.SetRequest(req)
	// 内部请求不经过 BodyStorageCleanup 中间件，需自行释放请求体存储
	defer common.CleanupBodyStorage(c)
	c.Set("token_name", "channel-test")
	c.Set("username", "system")

	start := time.Now()
	defer func() { res.Latency = time.Since(start) }()

	if apiErr := middleware.SetupContextForSelectedChannel(c, channel, res.Model); apiErr != nil {
		res.Err = apiErr
		return res
	}
	res.UsingKey = common.GetContextKeyString(c, constant.ContextKeyChannelKey)
	common.SetContextKey(c, constant.ContextKeyRequestStartTime, start)

	request, err := helper.GetAndValidateRequest(c, format)
	if err != nil {
		res.Err = requestBodyError(err)
		return res
	}
	info, err := relaycommon.GenRelayInfo(c, format, request, nil)
	if err != nil {
		res.Err = types.NewError(err, ErrorCodeGenRelayInfoFailed, ErrOptionWithSkipRetry())
		return res
	}
	info.IsChannelTest = true
	res.Err = relayOnce(c, info, format)
	return res
}

// shouldDisableAfterTest 测试失败是否应禁用渠道：只看渠道侧问题，429 限流属暂时性问题不禁用。
func shouldDisableAfterTest(apiErr *types.StarAPIError) bool {
	if apiErr == nil || types.IsSkipRetryError(apiErr) || apiErr.StatusCode == http.StatusTooManyRequests {
		return false
	}
	code := apiErr.StatusCode
	return types.IsChannelError(apiErr) || code == http.StatusUnauthorized || code == http.StatusForbidden ||
		code >= http.StatusInternalServerError
}

// TestChannel 手动测试单个渠道：GET /api/channel/test/:id?model=xxx。
// 只测试并更新响应时间，不自动启停渠道。
func TestChannel(c gin.Context) {
	id, ok := paramID(c)
	if !ok {
		return
	}
	channel, err := model.GetChannelById(id, true)
	if err != nil {
		apiErrorMsg(c, "channel not found")
		return
	}
	res := testChannel(channel, c.Query("model"))
	data := gin.H{"model": res.Model, "path": res.Path, "time": res.Latency.Seconds()}
	switch {
	case res.Skipped:
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "skipped: " + res.SkipReason, "data": data})
	case res.Err != nil:
		channel.UpdateResponseTime(res.Latency.Milliseconds())
		c.JSON(http.StatusOK, gin.H{"success": false, "message": res.Err.MaskSensitiveErrorWithStatusCode(), "data": data})
	default:
		channel.UpdateResponseTime(res.Latency.Milliseconds())
		apiSuccess(c, data)
	}
}

// 渠道运维：整渠道测速、测速记录维护、模型清理与上游余额查询。

const (
	// channelTestConcurrency 整渠道并发测试的并发上限
	channelTestConcurrency = 4
	// balanceQueryTimeout 余额查询超时
	balanceQueryTimeout = 30 * time.Second
	// maxBalanceResponseBytes 余额响应体读取上限
	maxBalanceResponseBytes = 1 << 20
	// channelTestTypeAllModels 逐模型测试的记录类型
	channelTestTypeAllModels = "all_models"
)

// channelTestAllRunning 避免重复触发整渠道测试
var channelTestAllRunning atomic.Bool

// recordChannelTestResult 把一次模型测试结果写入 channel_test_records。
func recordChannelTestResult(channel *Channel, modelName, testType string, res channelTestResult) error {
	record := &ChannelTestRecord{
		CreatedAt:    common.GetTimestamp(),
		ChannelId:    channel.Id,
		ChannelName:  channel.Name,
		ChannelTag:   channel.GetTag(),
		ModelName:    modelName,
		Status:       ChannelTestStatusSuccess,
		ResponseTime: int(res.Latency.Milliseconds()),
		TestType:     testType,
	}
	switch {
	case res.Skipped:
		record.Status = ChannelTestStatusFail
		record.ErrorMessage = "skipped: " + res.SkipReason
	case res.Err != nil:
		record.Status = ChannelTestStatusFail
		record.ErrorMessage = res.Err.MaskSensitiveErrorWithStatusCode()
	}
	return record.Insert()
}

// testRecordResult 一次按模型测试的结论，供接口返回。
type testRecordResult struct {
	Model     string `json:"model"`
	Success   bool   `json:"success"`
	Skipped   bool   `json:"skipped"`
	LatencyMS int    `json:"latency_ms"`
	Error     string `json:"error,omitempty"`
}

// testChannelModels 逐个模型测试渠道并把结果落库，返回汇总与明细。
func testChannelModels(channel *Channel, models []string) (items []testRecordResult, passed, failed, skipped int) {
	items = make([]testRecordResult, 0, len(models))
	for _, modelName := range models {
		modelName = strings.TrimSpace(modelName)
		if modelName == "" {
			continue
		}
		res := testChannel(channel, modelName)
		item := testRecordResult{
			Model:     modelName,
			Success:   res.ok(),
			LatencyMS: int(res.Latency.Milliseconds()),
		}
		switch {
		case res.Skipped:
			item.Skipped = true
			item.Error = res.SkipReason
			skipped++
		case res.Err != nil:
			item.Error = res.Err.MaskSensitiveErrorWithStatusCode()
			failed++
		default:
			passed++
		}
		items = append(items, item)
		if err := recordChannelTestResult(channel, modelName, channelTestTypeAllModels, res); err != nil {
			common.SysError("failed to record channel test result: " + err.Error())
		}
	}
	return items, passed, failed, skipped
}

// TestChannelAllModels GET /api/channel/test/:id/all_models：测试渠道的全部模型并记录结果。
func TestChannelAllModels(c gin.Context) {
	id, ok := paramID(c)
	if !ok {
		return
	}
	channel, err := model.GetChannelById(id, true)
	if err != nil {
		apiErrorMsg(c, "channel not found")
		return
	}
	items, passed, failed, skipped := testChannelModels(channel, channel.GetModels())
	apiSuccess(c, gin.H{
		"items":   items,
		"passed":  passed,
		"failed":  failed,
		"skipped": skipped,
	})
}

// TestAllChannels GET /api/channel/test：后台并发测试所有已启用渠道，立即返回。
func TestAllChannels(c gin.Context) {
	if !channelTestAllRunning.CompareAndSwap(false, true) {
		apiErrorMsg(c, "channel test is already running")
		return
	}

	channels, err := enabledChannels()
	if err != nil {
		channelTestAllRunning.Store(false)
		apiError(c, err)
		return
	}

	go func() {
		defer channelTestAllRunning.Store(false)
		sem := make(chan struct{}, channelTestConcurrency)
		var wg sync.WaitGroup
		for _, channel := range channels {
			wg.Add(1)
			sem <- struct{}{}
			go func(channel *Channel) {
				defer func() {
					<-sem
					wg.Done()
				}()
				res := testChannel(channel, "")
				if err := recordChannelTestResult(channel, res.Model, "all_channels", res); err != nil {
					common.SysError("failed to record channel test result: " + err.Error())
				}
			}(channel)
		}
		wg.Wait()
		common.SysLog(fmt.Sprintf("all-channel test finished: %d channels", len(channels)))
	}()

	apiSuccess(c, gin.H{"started": true, "channels": len(channels)})
}

// enabledChannels 取出所有已启用渠道。
func enabledChannels() ([]*Channel, error) {
	var channels []*Channel
	err := model.DB.Where("status = ?", ChannelStatusEnabled).Order("id").Find(&channels).Error
	return channels, err
}

// RemoveModelFromChannel DELETE /api/channel/:id/model?model=xxx：从渠道模型列表移除指定模型。
func RemoveModelFromChannel(c gin.Context) {
	id, ok := paramID(c)
	if !ok {
		return
	}
	modelName := strings.TrimSpace(c.Query("model"))
	if modelName == "" {
		apiErrorMsg(c, "model is required")
		return
	}
	channel, err := model.GetChannelById(id, true)
	if err != nil {
		apiErrorMsg(c, "channel not found")
		return
	}

	remaining := make([]string, 0, len(channel.GetModels()))
	removed := false
	for _, item := range channel.GetModels() {
		if item = strings.TrimSpace(item); item == "" {
			continue
		}
		if item == modelName {
			removed = true
			continue
		}
		remaining = append(remaining, item)
	}
	if !removed {
		apiErrorMsg(c, "model not found in channel")
		return
	}
	if len(remaining) == 0 {
		apiErrorMsg(c, "channel must keep at least one model")
		return
	}

	Models = strings.Join(remaining, ",")
	if err := channel.Save(); err != nil {
		apiError(c, err)
		return
	}
	model.RefreshChannelCache()
	apiSuccess(c, gin.H{"models": remaining, "removed": modelName})
}

// FixChannelsAbilities POST /api/channel/fix：清理渠道模型与分组的脏数据并重建缓存。
func FixChannelsAbilities(c gin.Context) {
	var channels []*Channel
	if err := model.DB.Find(&channels).Error; err != nil {
		apiError(c, err)
		return
	}

	fixed := 0
	for _, channel := range channels {
		models := normalizeCommaList(Models)
		newModels := strings.Join(models, ",")
		if newModels == Models {
			continue
		}
		if err := model.DB.Model(&Channel{}).Where("id = ?", channel.Id).
			Updates(map[string]any{"models": newModels}).Error; err != nil {
			apiError(c, err)
			return
		}
		fixed++
	}
	model.RefreshChannelCache()
	apiSuccess(c, gin.H{"fixed": fixed, "total": len(channels)})
}

// normalizeCommaList 去掉空项与重复项；group 为空时补默认分组。
func normalizeCommaList(raw string) []string {
	seen := make(map[string]bool)
	items := make([]string, 0)
	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		items = append(items, item)
	}
	return items
}

// ---------- 渠道余额 ----------

// balanceQueryUnsupported 该渠道类型不支持查询余额。
type balanceQueryUnsupported struct{ channelType int }

func (e balanceQueryUnsupported) Error() string {
	return fmt.Sprintf("balance query is not supported for channel type %s",
		constant.GetChannelTypeName(e.channelType))
}

// queryChannelBalance 查询上游余额（美元）：只支持 OpenRouter / DeepSeek / Moonshot。
func queryChannelBalance(channel *Channel) (float64, error) {
	baseURL := strings.TrimSuffix(channel.GetBaseURL(), "/")
	key, _, apiErr := channel.GetNextEnabledKey()
	if apiErr != nil {
		return 0, fmt.Errorf("no available key: %s", apiErr.Error())
	}
	client, err := httpclient.GetHttpClientWithProxy(channel.GetSetting().Proxy)
	if err != nil {
		return 0, err
	}

	var endpoint string
	switch channel.Type {
	case ChannelTypeOpenRouter:
		endpoint = baseURL + "/v1/credits"
	case ChannelTypeDeepSeek:
		endpoint = baseURL + "/user/balance"
	case ChannelTypeMoonshot:
		endpoint = baseURL + "/v1/users/me/balance"
	default:
		return 0, balanceQueryUnsupported{channelType: channel.Type}
	}

	body, err := fetchJSON(client, endpoint, strings.TrimSpace(key))
	if err != nil {
		return 0, err
	}

	switch channel.Type {
	case ChannelTypeOpenRouter:
		var resp struct {
			Data struct {
				TotalCredits float64 `json:"total_credits"`
				TotalUsage   float64 `json:"total_usage"`
			} `json:"data"`
		}
		if err := common.Unmarshal(body, &resp); err != nil {
			return 0, err
		}
		return resp.Data.TotalCredits - resp.Data.TotalUsage, nil
	case ChannelTypeDeepSeek:
		var resp struct {
			BalanceInfos []struct {
				Currency       string `json:"currency"`
				TotalBalance   string `json:"total_balance"`
				GrantedBalance string `json:"granted_balance"`
			} `json:"balance_infos"`
		}
		if err := common.Unmarshal(body, &resp); err != nil {
			return 0, err
		}
		for _, info := range resp.BalanceInfos {
			if info.Currency != "" && info.Currency != "USD" {
				// 非美元余额不与美元混算，直接跳过
				continue
			}
			return parseFloatString(info.TotalBalance)
		}
		return 0, fmt.Errorf("no usd balance info in upstream response")
	default: // Moonshot
		var resp struct {
			Data struct {
				AvailableBalance float64 `json:"available_balance"`
			} `json:"data"`
		}
		if err := common.Unmarshal(body, &resp); err != nil {
			return 0, err
		}
		return resp.Data.AvailableBalance, nil
	}
}

// fetchJSON 带超时地请求上游并返回响应体。
func fetchJSON(client *http.Client, endpoint, key string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), balanceQueryTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request upstream: %s", common.MaskSensitiveInfo(err.Error()))
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBalanceResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("read upstream response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream returned status %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	return body, nil
}

// parseFloatString 解析上游返回的字符串金额。
func parseFloatString(value string) (float64, error) {
	var result float64
	if _, err := fmt.Sscanf(strings.TrimSpace(value), "%f", &result); err != nil {
		return 0, fmt.Errorf("invalid balance value %q", value)
	}
	return result, nil
}

// UpdateChannelBalance GET /api/channel/update_balance/:id：查询并保存单个渠道余额。
func UpdateChannelBalance(c gin.Context) {
	id, ok := paramID(c)
	if !ok {
		return
	}
	channel, err := model.GetChannelById(id, true)
	if err != nil {
		apiErrorMsg(c, "channel not found")
		return
	}
	balance, err := queryChannelBalance(channel)
	if err != nil {
		var unsupported balanceQueryUnsupported
		if errors.As(err, &unsupported) {
			apiErrorMsg(c, err.Error())
			return
		}
		apiErrorMsg(c, "failed to query balance: "+err.Error())
		return
	}
	channel.UpdateBalance(balance)
	apiSuccess(c, gin.H{"channel_id": channel.Id, "balance": balance})
}

// UpdateAllChannelsBalance GET /api/channel/update_balance：并发刷新所有启用渠道余额。
func UpdateAllChannelsBalance(c gin.Context) {
	channels, err := enabledChannels()
	if err != nil {
		apiError(c, err)
		return
	}

	var (
		mu                       sync.Mutex
		updated, skipped, failed int
		details                  []gin.H
	)
	var wg sync.WaitGroup
	sem := make(chan struct{}, channelTestConcurrency)
	for _, channel := range channels {
		wg.Add(1)
		sem <- struct{}{}
		go func(channel *Channel) {
			defer func() {
				<-sem
				wg.Done()
			}()
			balance, queryErr := queryChannelBalance(channel)
			mu.Lock()
			defer mu.Unlock()
			detail := gin.H{"channel_id": channel.Id, "name": channel.Name}
			switch {
			case queryErr == nil:
				channel.UpdateBalance(balance)
				updated++
				detail["balance"] = balance
			default:
				var unsupported balanceQueryUnsupported
				if errors.As(queryErr, &unsupported) {
					skipped++
					detail["skipped"] = unsupported.Error()
				} else {
					failed++
					detail["error"] = queryErr.Error()
				}
			}
			details = append(details, detail)
		}(channel)
	}
	wg.Wait()

	apiSuccess(c, gin.H{"updated": updated, "skipped": skipped, "failed": failed, "details": details})
}

// ---------- 测速记录 ----------

// testRecordFilterFromQuery 从查询参数解析测速记录筛选条件。
func testRecordFilterFromQuery(c gin.Context) ChannelTestRecordFilter {
	filter := ChannelTestRecordFilter{
		ChannelId:  common.String2Int(c.Query("channel_id")),
		ModelName:  strings.TrimSpace(c.Query("model_name")),
		TestType:   strings.TrimSpace(c.Query("test_type")),
		ChannelTag: strings.TrimSpace(c.Query("channel_tag")),
		StartTime:  int64(common.String2Int(c.Query("start_time"))),
		EndTime:    int64(common.String2Int(c.Query("end_time"))),
	}
	if status := c.Query("status"); status != "" {
		parsed := common.String2Int(status)
		filter.Status = &parsed
	}
	return filter
}

// GetChannelTestRecords GET /api/channel/test_records：多条件分页查询测速记录。
func GetChannelTestRecords(c gin.Context) {
	page := common.GetPageQuery(c)
	records, total, err := model.ListChannelTestRecords(testRecordFilterFromQuery(c), page.GetStartIdx(), page.GetPageSize())
	if err != nil {
		apiError(c, err)
		return
	}
	apiSuccess(c, pageResult(page, records, total))
}

// DeleteChannelTestRecords DELETE /api/channel/test_records：按条件删除测速记录。
func DeleteChannelTestRecords(c gin.Context) {
	filter := testRecordFilterFromQuery(c)
	if filter.Empty() {
		apiErrorMsg(c, "at least one filter condition is required")
		return
	}
	deleted, err := DeleteChannelTestRecords(filter)
	if err != nil {
		apiError(c, err)
		return
	}
	apiSuccess(c, deleted)
}

// ClearChannelTestRecords DELETE /api/channel/test_records/all：清空全部测速记录。
func ClearChannelTestRecords(c gin.Context) {
	deleted, err := ClearChannelTestRecords()
	if err != nil {
		apiError(c, err)
		return
	}
	apiSuccess(c, deleted)
}

// retestRecord 复测一条记录并写回结果。
func retestRecord(record *ChannelTestRecord) (channelTestResult, error) {
	channel, err := model.GetChannelById(record.ChannelId, true)
	if err != nil {
		return channelTestResult{Model: record.ModelName}, fmt.Errorf("channel not found")
	}
	res := testChannel(channel, record.ModelName)
	status := ChannelTestStatusSuccess
	message := ""
	switch {
	case res.Skipped:
		status, message = ChannelTestStatusFail, "skipped: "+res.SkipReason
	case res.Err != nil:
		status, message = ChannelTestStatusFail, res.Err.MaskSensitiveErrorWithStatusCode()
	}
	if err := record.SaveTestResult(status, int(res.Latency.Milliseconds()), message, common.GetTimestamp()); err != nil {
		return res, err
	}
	record.Status = status
	record.ErrorMessage = message
	record.ResponseTime = int(res.Latency.Milliseconds())
	return res, nil
}

// RetestChannelModel POST /api/channel/test_records/:id/retest：复测单条记录。
func RetestChannelModel(c gin.Context) {
	id, ok := paramID(c)
	if !ok {
		return
	}
	record, err := model.GetChannelTestRecordById(id)
	if err != nil {
		apiErrorMsg(c, "test record not found")
		return
	}
	if _, err := retestRecord(record); err != nil {
		apiErrorMsg(c, err.Error())
		return
	}
	apiSuccess(c, record)
}

// BatchRetestChannelModels POST /api/channel/test_records/batch_retest：按 id 批量复测。
func BatchRetestChannelModels(c gin.Context) {
	var req idsRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Ids) == 0 {
		apiErrorMsg(c, "ids is required")
		return
	}
	passed, failed := 0, 0
	for _, id := range req.Ids {
		record, err := model.GetChannelTestRecordById(id)
		if err != nil {
			failed++
			continue
		}
		res, err := retestRecord(record)
		if err != nil || !res.ok() {
			failed++
			continue
		}
		passed++
	}
	apiSuccess(c, gin.H{"total": len(req.Ids), "passed": passed, "failed": failed})
}

// RetestFilteredRecords POST /api/channel/test_records/retest_filtered：按筛选条件全部复测。
func RetestFilteredRecords(c gin.Context) {
	var req struct {
		ChannelId  int    `json:"channel_id"`
		ModelName  string `json:"model_name"`
		Status     *int   `json:"status"`
		TestType   string `json:"test_type"`
		ChannelTag string `json:"channel_tag"`
	}
	_ = c.ShouldBindJSON(&req)
	filter := ChannelTestRecordFilter{
		ChannelId:  req.ChannelId,
		ModelName:  strings.TrimSpace(req.ModelName),
		Status:     req.Status,
		TestType:   strings.TrimSpace(req.TestType),
		ChannelTag: strings.TrimSpace(req.ChannelTag),
	}
	if filter.Empty() {
		apiErrorMsg(c, "at least one filter condition is required")
		return
	}
	records, err := model.FindChannelTestRecords(filter)
	if err != nil {
		apiError(c, err)
		return
	}
	passed, failed := 0, 0
	for _, record := range records {
		res, err := retestRecord(record)
		if err != nil || !res.ok() {
			failed++
			continue
		}
		passed++
	}
	apiSuccess(c, gin.H{"total": len(records), "passed": passed, "failed": failed})
}

// 渠道亲和性缓存的管理接口（Root）。

// GetChannelAffinityCacheStats 亲和性缓存统计：条目数、命中率、TTL 与容量上限。
func GetChannelAffinityCacheStats(c gin.Context) {
	apiSuccess(c, channels.GetAffinityCacheStats())
}

// ClearChannelAffinityCache 清空亲和性缓存与统计。
func ClearChannelAffinityCache(c gin.Context) {
	channels.ClearAffinityCache()
	apiOK(c)
}

// GetChannelAffinityUsageCacheStats 各渠道被亲和性选中的次数（管理员在日志页查看）。
func GetChannelAffinityUsageCacheStats(c gin.Context) {
	apiSuccess(c, gin.H{
		"items": channels.GetAffinityUsageStats(),
		"stats": channels.GetAffinityCacheStats(),
	})
}
