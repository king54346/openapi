package channel

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"openapi/common"
	"openapi/constant"
	"openapi/model"
	"openapi/relay/channel/zhipu"
	httpclient "openapi/service/http"
)

const (
	fetchModelsTimeout     = 30 * time.Second
	maxModelsResponseBytes = 10 << 20
)

func upstreamModelsURL(channelType int, baseURL string) string {
	baseURL = strings.TrimRight(baseURL, "/")
	if plan, ok := constant.ChannelSpecialBases[baseURL]; ok && plan.OpenAIBaseURL != "" {
		openAIBase := strings.TrimRight(plan.OpenAIBaseURL, "/")
		return openAIBase + "/models"
	}
	switch channelType {
	case constant.ChannelTypeAli:
		if strings.HasSuffix(baseURL, "/compatible-mode/v1") {
			return baseURL + "/models"
		}
		return baseURL + "/compatible-mode/v1/models"
	case constant.ChannelTypeZhipu_v4:
		if strings.HasSuffix(baseURL, "/api/paas/v4") {
			return baseURL + "/models"
		}
		return baseURL + "/api/paas/v4/models"
	default:
		if strings.HasSuffix(baseURL, "/v1") {
			return baseURL + "/models"
		}
		return baseURL + "/v1/models"
	}
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

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
		return nil, fmt.Errorf("upstream returned status %d: %s", resp.StatusCode, truncateStr(string(body), 200))
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

func FetchUpstreamModels(id int) ([]string, error) {
	ch, err := model.GetChannelById(id, true)
	if err != nil {
		return nil, ErrChannelNotFound
	}
	if ch.Type == constant.ChannelTypeZhipu {
		return append([]string(nil), zhipu.ModelList...), nil
	}
	key, _, apiErr := ch.GetNextEnabledKey()
	if apiErr != nil {
		return nil, validationErr("no available key: " + apiErr.Error())
	}
	ids, err := fetchUpstreamModelIDs(upstreamModelsURL(ch.Type, ch.GetBaseURL()),
		strings.TrimSpace(key), ch.GetSetting().Proxy, ch.GetHeaderOverride())
	if err != nil {
		return nil, validationErr("failed to fetch models: " + err.Error())
	}
	return ids, nil
}

type FetchModelsInput struct {
	BaseURL string `json:"base_url"`
	Type    int    `json:"type"`
	Key     string `json:"key"`
}

func FetchModels(req FetchModelsInput) ([]string, error) {
	if req.Type == constant.ChannelTypeZhipu {
		return append([]string(nil), zhipu.ModelList...), nil
	}
	baseURL := req.BaseURL
	if baseURL == "" {
		baseURL = constant.ChannelBaseURLs[req.Type]
	}
	if baseURL == "" {
		return nil, validationErr("base_url is required")
	}
	key := strings.Split(strings.TrimSpace(req.Key), "\n")[0]
	ids, err := fetchUpstreamModelIDs(upstreamModelsURL(req.Type, baseURL), strings.TrimSpace(key), "", nil)
	if err != nil {
		return nil, validationErr("failed to fetch models: " + err.Error())
	}
	return ids, nil
}

func RemoveModelFromChannel(id int, modelName string) ([]string, error) {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return nil, validationErr("model is required")
	}
	ch, err := model.GetChannelById(id, true)
	if err != nil {
		return nil, ErrChannelNotFound
	}
	remaining := make([]string, 0, len(ch.GetModels()))
	removed := false
	for _, item := range ch.GetModels() {
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
		return nil, validationErr("model not found in channel")
	}
	if len(remaining) == 0 {
		return nil, validationErr("channel must keep at least one model")
	}
	ch.Models = strings.Join(remaining, ",")
	if err := ch.Save(); err != nil {
		return nil, err
	}
	model.RefreshChannelCache()
	return remaining, nil
}

func FixChannelsAbilities() (fixed int, total int, err error) {
	var channels []*model.Channel
	if err := model.DB.Find(&channels).Error; err != nil {
		return 0, 0, err
	}
	for _, ch := range channels {
		models := normalizeCommaList(ch.Models)
		newModels := strings.Join(models, ",")
		if newModels == ch.Models {
			continue
		}
		if err := model.DB.Model(&model.Channel{}).Where("id = ?", ch.Id).
			Updates(map[string]any{"models": newModels}).Error; err != nil {
			return 0, 0, err
		}
		fixed++
	}
	model.RefreshChannelCache()
	return fixed, len(channels), nil
}

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
