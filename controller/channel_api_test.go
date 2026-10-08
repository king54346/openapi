package controller_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"openapi/common"
	"openapi/constant"
	"openapi/model"
)

func TestChannelMultiKeyAPI(t *testing.T) {
	s := newTestServer(t)
	admin := s.createUser("admin", common.RoleAdminUser)

	// batch 返回每个新渠道的 id
	created := decode[struct {
		Count int   `json:"count"`
		Ids   []int `json:"ids"`
	}](t, s.ok(admin, http.MethodPost, "/api/channel/", map[string]any{
		"mode": "batch", "channel": map[string]any{"name": "b", "type": constant.ChannelTypeOpenAI, "key": "k1\nk2", "models": "m"}}))
	if created.Count != 2 || len(created.Ids) != 2 || created.Ids[0] == 0 || created.Ids[0] == created.Ids[1] {
		t.Fatalf("batch add result: %+v", created)
	}
	s.ok(admin, http.MethodPost, "/api/channel/batch", map[string]any{"ids": created.Ids})

	// multi_to_single：多行 key 合成一个多 key 渠道
	s.ok(admin, http.MethodPost, "/api/channel/", map[string]any{
		"mode": "multi_to_single", "multi_key_mode": "polling",
		"channel": map[string]any{"name": "multi", "type": constant.ChannelTypeOpenAI,
			"key": "key-a\n\nkey-b\nkey-c\n", "models": "gpt-4o"}})
	s.fail(admin, http.MethodPost, "/api/channel/", map[string]any{
		"mode": "multi_to_single", "multi_key_mode": "weird",
		"channel": map[string]any{"name": "x", "type": constant.ChannelTypeOpenAI, "key": "k", "models": "m"}})
	list := decode[page[model.Channel]](t, s.ok(admin, http.MethodGet, "/api/channel/", nil))
	id := list.Items[0].Id
	ch, _ := model.GetChannelById(id, true)
	if !ch.ChannelInfo.IsMultiKey || ch.ChannelInfo.MultiKeySize != 3 || ch.ChannelInfo.MultiKeyMode != constant.MultiKeyModePolling {
		t.Fatalf("multi key channel: %+v", ch.ChannelInfo)
	}

	// 追加 key（去重），且不传 channel_info 时多 key 状态不被清空
	s.ok(admin, http.MethodPut, "/api/channel/", map[string]any{"id": id, "key": "key-b\nkey-d", "key_mode": "append"})
	ch, _ = model.GetChannelById(id, true)
	if ch.Key != "key-a\nkey-b\nkey-c\nkey-d" || ch.ChannelInfo.MultiKeySize != 4 || !ch.ChannelInfo.IsMultiKey {
		t.Fatalf("after append: key=%q info=%+v", ch.Key, ch.ChannelInfo)
	}

	manage := func(action string, extra map[string]any) json.RawMessage {
		body := map[string]any{"channel_id": id, "action": action}
		for k, v := range extra {
			body[k] = v
		}
		return s.ok(admin, http.MethodPost, "/api/channel/multi_key/manage", body)
	}
	type keyStatusResp struct {
		Keys []struct {
			Index      int    `json:"index"`
			Status     int    `json:"status"`
			KeyPreview string `json:"key_preview"`
		} `json:"keys"`
		Total               int `json:"total"`
		EnabledCount        int `json:"enabled_count"`
		ManualDisabledCount int `json:"manual_disabled_count"`
		AutoDisabledCount   int `json:"auto_disabled_count"`
	}

	manage("disable_key", map[string]any{"key_index": 1})
	// 模拟 key-c 被自动禁用
	model.UpdateChannelStatus(id, "key-c", common.ChannelStatusAutoDisabled, "bad key")
	st := decode[keyStatusResp](t, manage("get_key_status", nil))
	if st.EnabledCount != 2 || st.ManualDisabledCount != 1 || st.AutoDisabledCount != 1 {
		t.Fatalf("key status: %+v", st)
	}
	st = decode[keyStatusResp](t, manage("get_key_status", map[string]any{"status": common.ChannelStatusManuallyDisabled}))
	if st.Total != 1 || st.Keys[0].Index != 1 || st.Keys[0].KeyPreview != "key-b" {
		t.Fatalf("filtered key status: %+v", st)
	}

	// 删除自动禁用的 key 后，其余 key 的状态按新下标保留
	manage("delete_disabled_keys", nil)
	ch, _ = model.GetChannelById(id, true)
	if ch.Key != "key-a\nkey-b\nkey-d" || ch.ChannelInfo.MultiKeySize != 3 || ch.ChannelInfo.MultiKeyStatusList[1] != common.ChannelStatusManuallyDisabled {
		t.Fatalf("after delete_disabled_keys: key=%q info=%+v", ch.Key, ch.ChannelInfo)
	}
	manage("enable_key", map[string]any{"key_index": 1})
	manage("delete_key", map[string]any{"key_index": 0})
	ch, _ = model.GetChannelById(id, true)
	if ch.Key != "key-b\nkey-d" || len(ch.ChannelInfo.MultiKeyStatusList) != 0 {
		t.Fatalf("after enable+delete: key=%q info=%+v", ch.Key, ch.ChannelInfo)
	}
	manage("disable_all_keys", nil)
	if st := decode[keyStatusResp](t, manage("get_key_status", nil)); st.EnabledCount != 0 {
		t.Fatalf("all keys should be disabled: %+v", st)
	}
	manage("enable_all_keys", nil)
	if st := decode[keyStatusResp](t, manage("get_key_status", nil)); st.EnabledCount != 2 {
		t.Fatalf("all keys should be enabled: %+v", st)
	}
	s.fail(admin, http.MethodPost, "/api/channel/multi_key/manage", map[string]any{"channel_id": id, "action": "delete_key", "key_index": 9})

	// 详情不输出 key 和多 key 禁用原因
	got := decode[model.Channel](t, s.ok(admin, http.MethodGet, fmt.Sprintf("/api/channel/%d", id), nil))
	if got.Key != "" || got.ChannelInfo.MultiKeyDisabledReason != nil {
		t.Fatalf("channel detail leaks data: %+v", got)
	}
}

func TestChannelListModesAndFetchModels(t *testing.T) {
	s := newTestServer(t)
	admin := s.createUser("admin", common.RoleAdminUser)

	var gotPath, gotAuth, gotExtra string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth, gotExtra = r.URL.Path, r.Header.Get("Authorization"), r.Header.Get("X-Extra")
		if r.Header.Get("Authorization") == "Bearer bad" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"invalid key"}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"m1"},{"id":"m2"}]}`))
	}))
	defer upstream.Close()

	ids := map[string]int{}
	add := func(name string, typ int, tag string, status int) {
		s.ok(admin, http.MethodPost, "/api/channel/", map[string]any{"channel": map[string]any{
			"name": name, "type": typ, "key": "sk-" + name, "models": "a,b", "tag": tag, "status": status,
			"base_url": upstream.URL, "header_override": `{"X-Extra":"x-{api_key}"}`}})
		var ch model.Channel
		model.DB.Where("name = ?", name).First(&ch)
		ids[name] = ch.Id
	}
	add("o1", constant.ChannelTypeOpenAI, "t1", common.ChannelStatusEnabled)
	add("o2", constant.ChannelTypeOpenAI, "t1", common.ChannelStatusManuallyDisabled)
	add("d1", constant.ChannelTypeDeepSeek, "t2", common.ChannelStatusEnabled)
	add("ali", constant.ChannelTypeAli, "", common.ChannelStatusEnabled)
	s.fail(admin, http.MethodPost, "/api/channel/", map[string]any{"channel": map[string]any{
		"name": "bad", "type": constant.ChannelTypeOpenAI, "key": "k", "models": "a", "param_override": "{not json"}})

	type listResp struct {
		Items      []model.Channel  `json:"items"`
		Total      int64            `json:"total"`
		TypeCounts map[string]int64 `json:"type_counts"`
	}
	all := decode[listResp](t, s.ok(admin, http.MethodGet, "/api/channel/?status=enabled", nil))
	if all.Total != 3 || all.TypeCounts[strconv.Itoa(constant.ChannelTypeOpenAI)] != 1 {
		t.Fatalf("enabled list: total=%d counts=%v", all.Total, all.TypeCounts)
	}
	tagged := decode[listResp](t, s.ok(admin, http.MethodGet, "/api/channel/?tag_mode=true", nil))
	if tagged.Total != 2 || len(tagged.Items) != 3 {
		t.Fatalf("tag mode: tags=%d channels=%d", tagged.Total, len(tagged.Items))
	}
	search := decode[listResp](t, s.ok(admin, http.MethodGet, "/api/channel/search?keyword=o&status=disabled", nil))
	if search.Total != 1 || search.Items[0].Name != "o2" {
		t.Fatalf("search disabled: %+v", search)
	}
	search = decode[listResp](t, s.ok(admin, http.MethodGet,
		fmt.Sprintf("/api/channel/search?type=%d&p=1&page_size=1", constant.ChannelTypeOpenAI), nil))
	if search.Total != 2 || len(search.Items) != 1 || search.TypeCounts[strconv.Itoa(constant.ChannelTypeDeepSeek)] != 1 {
		t.Fatalf("search by type: %+v", search)
	}
	if models := decode[string](t, s.ok(admin, http.MethodGet, "/api/channel/tag/models?tag=t1", nil)); models != "a,b" {
		t.Fatalf("tag models: %q", models)
	}

	// 拉取已保存渠道的上游模型：带 key 与 header 覆盖（{api_key} 替换）
	models := decode[[]string](t, s.ok(admin, http.MethodGet, fmt.Sprintf("/api/channel/fetch_models/%d", ids["o1"]), nil))
	if len(models) != 2 || gotPath != "/v1/models" || gotAuth != "Bearer sk-o1" || gotExtra != "x-sk-o1" {
		t.Fatalf("fetch saved: models=%v path=%s auth=%s extra=%s", models, gotPath, gotAuth, gotExtra)
	}
	s.ok(admin, http.MethodGet, fmt.Sprintf("/api/channel/fetch_models/%d", ids["ali"]), nil)
	if gotPath != "/compatible-mode/v1/models" {
		t.Fatalf("ali models path: %s", gotPath)
	}

	// 新建渠道前用表单参数拉取，多行 key 只取第一行；上游报错时返回失败
	models = decode[[]string](t, s.ok(admin, http.MethodPost, "/api/channel/fetch_models", map[string]any{
		"base_url": upstream.URL, "type": constant.ChannelTypeOpenAI, "key": "first\nsecond"}))
	if len(models) != 2 || gotAuth != "Bearer first" {
		t.Fatalf("fetch form: models=%v auth=%s", models, gotAuth)
	}
	if msg := s.fail(admin, http.MethodPost, "/api/channel/fetch_models", map[string]any{
		"base_url": upstream.URL, "type": constant.ChannelTypeOpenAI, "key": "bad"}); !strings.Contains(msg, "401") {
		t.Fatalf("upstream error message: %q", msg)
	}
}
