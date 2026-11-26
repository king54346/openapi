package channel_test

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
	"openapi/controller/common/testhelper"
	"openapi/model"
)

func TestChannelAdminAPI(t *testing.T) {
	s := testhelper.NewTestServer(t)
	root := s.CreateUser("root", common.RoleRootUser)
	admin := s.CreateUser("admin", common.RoleAdminUser)
	user := s.CreateUser("alice", common.RoleCommonUser)

	newChannel := map[string]any{
		"mode": "single",
		"channel": map[string]any{
			"name": "openai-1", "type": constant.ChannelTypeOpenAI, "key": "sk-secret",
			"models": "gpt-4o,gpt-4o-mini",
		},
	}
	s.Fail(user, http.MethodPost, "/api/channel/", newChannel)
	s.OK(admin, http.MethodPost, "/api/channel/", newChannel)

	// Gemini 已支持接入，未对接渠道类型（如 Azure）与缺少 key 会被拒绝
	s.OK(admin, http.MethodPost, "/api/channel/", map[string]any{"channel": map[string]any{
		"name": "gemini", "type": constant.ChannelTypeGemini, "key": "gem-k", "models": "gemini-2.5-flash"}})
	s.Fail(admin, http.MethodPost, "/api/channel/", map[string]any{"channel": map[string]any{
		"name": "azure", "type": constant.ChannelTypeAzure, "key": "k", "models": "gpt-4o"}})
	s.Fail(admin, http.MethodPost, "/api/channel/", map[string]any{"channel": map[string]any{
		"name": "nokey", "type": constant.ChannelTypeOpenAI, "models": "gpt-4o"}})

	s.OK(admin, http.MethodPost, "/api/channel/", map[string]any{"mode": "batch", "channel": map[string]any{
		"name": "ds", "type": constant.ChannelTypeDeepSeek, "key": "k1\nk2\n\n", "models": "deepseek-chat"}})

	list := testhelper.Decode[testhelper.Page[model.Channel]](t, s.OK(admin, http.MethodGet, "/api/channel/?p=1&page_size=10&id_sort=true", nil))
	if list.Total != 4 || len(list.Items) != 4 {
		t.Fatalf("expected 4 channels, got %d", list.Total)
	}
	for _, ch := range list.Items {
		if ch.Key != "" {
			t.Fatal("channel list must not expose keys")
		}
	}
	if ch, _ := model.GetRandomSatisfiedChannel("", "gpt-4o-mini", 0); ch == nil {
		t.Fatal("new channel should be in cache immediately")
	}
	var openaiID int
	for _, ch := range list.Items {
		if ch.Name == "openai-1" {
			openaiID = ch.Id
		}
	}

	got := testhelper.Decode[model.Channel](t, s.OK(admin, http.MethodGet, fmt.Sprintf("/api/channel/%d", openaiID), nil))
	if got.Name != "openai-1" || got.Key != "" {
		t.Fatalf("unexpected channel: %+v", got)
	}

	s.OK(admin, http.MethodPut, "/api/channel/", map[string]any{"id": openaiID, "name": "openai-renamed"})
	if ch, _ := model.GetRandomSatisfiedChannel("", "gpt-4o-mini", 0); ch == nil {
		t.Fatal("renamed channel should stay in cache")
	}
	stored, _ := model.GetChannelById(openaiID, true)
	if stored.Name != "openai-renamed" || stored.Key != "sk-secret" {
		t.Fatalf("update should keep key when omitted: %+v", stored)
	}

	// 只有 root 能查看 key
	s.Fail(admin, http.MethodPost, fmt.Sprintf("/api/channel/%d/key", openaiID), nil)
	keyResp := testhelper.Decode[map[string]string](t, s.OK(root, http.MethodPost, fmt.Sprintf("/api/channel/%d/key", openaiID), nil))
	if keyResp["key"] != "sk-secret" {
		t.Fatalf("unexpected key: %v", keyResp)
	}

	// 移除模型、复制、标签、删除
	s.OK(admin, http.MethodDelete, fmt.Sprintf("/api/channel/%d/model?model=gpt-4o-mini", openaiID), nil)
	copied := testhelper.Decode[map[string]int](t, s.OK(admin, http.MethodPost, fmt.Sprintf("/api/channel/copy/%d", openaiID), nil))
	s.OK(admin, http.MethodPost, "/api/channel/batch/tag", map[string]any{"ids": []int{openaiID, copied["id"]}, "tag": "t1"})
	s.OK(admin, http.MethodPost, "/api/channel/tag/disabled", map[string]any{"tag": "t1"})
	if ch, _ := model.GetRandomSatisfiedChannel("", "gpt-4o", 0); ch != nil {
		t.Fatal("disabled tag channels should leave cache")
	}
	s.OK(admin, http.MethodPost, "/api/channel/tag/enabled", map[string]any{"tag": "t1"})
	if ch, _ := model.GetRandomSatisfiedChannel("", "gpt-4o", 0); ch == nil {
		t.Fatal("re-enabled tag channels should be back in cache")
	}
	s.OK(admin, http.MethodDelete, fmt.Sprintf("/api/channel/%d", copied["id"]), nil)
	s.OK(admin, http.MethodPost, "/api/channel/batch", map[string]any{"ids": []int{list.Items[0].Id}})

	// 渠道亲和性缓存管理接口
	s.OK(root, http.MethodGet, "/api/option/channel_affinity_cache", nil)
	s.OK(root, http.MethodDelete, "/api/option/channel_affinity_cache", nil)
	s.OK(admin, http.MethodGet, "/api/log/channel_affinity_usage_cache", nil)
}

func TestChannelMultiKeyAPI(t *testing.T) {
	s := testhelper.NewTestServer(t)
	admin := s.CreateUser("admin", common.RoleAdminUser)

	created := testhelper.Decode[struct {
		Count int   `json:"count"`
		Ids   []int `json:"ids"`
	}](t, s.OK(admin, http.MethodPost, "/api/channel/", map[string]any{
		"mode": "batch", "channel": map[string]any{"name": "b", "type": constant.ChannelTypeOpenAI, "key": "k1\nk2", "models": "m"}}))
	if created.Count != 2 || len(created.Ids) != 2 || created.Ids[0] == 0 || created.Ids[0] == created.Ids[1] {
		t.Fatalf("batch add result: %+v", created)
	}
	s.OK(admin, http.MethodPost, "/api/channel/batch", map[string]any{"ids": created.Ids})

	s.OK(admin, http.MethodPost, "/api/channel/", map[string]any{
		"mode": "multi_to_single", "multi_key_mode": "polling",
		"channel": map[string]any{"name": "multi", "type": constant.ChannelTypeOpenAI,
			"key": "key-a\n\nkey-b\nkey-c\n", "models": "gpt-4o"}})
	s.Fail(admin, http.MethodPost, "/api/channel/", map[string]any{
		"mode": "multi_to_single", "multi_key_mode": "weird",
		"channel": map[string]any{"name": "x", "type": constant.ChannelTypeOpenAI, "key": "k", "models": "m"}})
	list := testhelper.Decode[testhelper.Page[model.Channel]](t, s.OK(admin, http.MethodGet, "/api/channel/", nil))
	id := list.Items[0].Id
	ch, _ := model.GetChannelById(id, true)
	if !ch.ChannelInfo.IsMultiKey || ch.ChannelInfo.MultiKeySize != 3 || ch.ChannelInfo.MultiKeyMode != constant.MultiKeyModePolling {
		t.Fatalf("multi key channel: %+v", ch.ChannelInfo)
	}

	s.OK(admin, http.MethodPut, "/api/channel/", map[string]any{"id": id, "key": "key-b\nkey-d", "key_mode": "append"})
	ch, _ = model.GetChannelById(id, true)
	if ch.Key != "key-a\nkey-b\nkey-c\nkey-d" || ch.ChannelInfo.MultiKeySize != 4 || !ch.ChannelInfo.IsMultiKey {
		t.Fatalf("after append: key=%q info=%+v", ch.Key, ch.ChannelInfo)
	}

	manage := func(action string, extra map[string]any) json.RawMessage {
		body := map[string]any{"channel_id": id, "action": action}
		for k, v := range extra {
			body[k] = v
		}
		return s.OK(admin, http.MethodPost, "/api/channel/multi_key/manage", body)
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
	model.UpdateChannelStatus(id, "key-c", common.ChannelStatusAutoDisabled, "bad key")
	st := testhelper.Decode[keyStatusResp](t, manage("get_key_status", nil))
	if st.EnabledCount != 2 || st.ManualDisabledCount != 1 || st.AutoDisabledCount != 1 {
		t.Fatalf("key status: %+v", st)
	}
	st = testhelper.Decode[keyStatusResp](t, manage("get_key_status", map[string]any{"status": common.ChannelStatusManuallyDisabled}))
	if st.Total != 1 || st.Keys[0].Index != 1 || st.Keys[0].KeyPreview != "key-b" {
		t.Fatalf("filtered key status: %+v", st)
	}

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
	if st := testhelper.Decode[keyStatusResp](t, manage("get_key_status", nil)); st.EnabledCount != 0 {
		t.Fatalf("all keys should be disabled: %+v", st)
	}
	manage("enable_all_keys", nil)
	if st := testhelper.Decode[keyStatusResp](t, manage("get_key_status", nil)); st.EnabledCount != 2 {
		t.Fatalf("all keys should be enabled: %+v", st)
	}
	s.Fail(admin, http.MethodPost, "/api/channel/multi_key/manage", map[string]any{"channel_id": id, "action": "delete_key", "key_index": 9})

	got := testhelper.Decode[model.Channel](t, s.OK(admin, http.MethodGet, fmt.Sprintf("/api/channel/%d", id), nil))
	if got.Key != "" || got.ChannelInfo.MultiKeyDisabledReason != nil {
		t.Fatalf("channel detail leaks data: %+v", got)
	}
}

func TestChannelListModesFetchModelsAndBalance(t *testing.T) {
	s := testhelper.NewTestServer(t)
	admin := s.CreateUser("admin", common.RoleAdminUser)

	var gotPath, gotAuth, gotExtra string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth, gotExtra = r.URL.Path, r.Header.Get("Authorization"), r.Header.Get("X-Extra")
		if r.URL.Path == "/user/balance" {
			_, _ = w.Write([]byte(`{"balance_infos":[{"currency":"USD","total_balance":"42.50"}]}`))
			return
		}
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
		s.OK(admin, http.MethodPost, "/api/channel/", map[string]any{"channel": map[string]any{
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

	type listResp struct {
		Items      []model.Channel  `json:"items"`
		Total      int64            `json:"total"`
		TypeCounts map[string]int64 `json:"type_counts"`
	}
	all := testhelper.Decode[listResp](t, s.OK(admin, http.MethodGet, "/api/channel/?status=enabled", nil))
	if all.Total != 3 || all.TypeCounts[strconv.Itoa(constant.ChannelTypeOpenAI)] != 1 {
		t.Fatalf("enabled list: total=%d counts=%v", all.Total, all.TypeCounts)
	}
	tagged := testhelper.Decode[listResp](t, s.OK(admin, http.MethodGet, "/api/channel/?tag_mode=true", nil))
	if tagged.Total != 2 || len(tagged.Items) != 3 {
		t.Fatalf("tag mode: tags=%d channels=%d", tagged.Total, len(tagged.Items))
	}

	models := testhelper.Decode[[]string](t, s.OK(admin, http.MethodGet, fmt.Sprintf("/api/channel/fetch_models/%d", ids["o1"]), nil))
	if len(models) != 2 || gotPath != "/v1/models" || gotAuth != "Bearer sk-o1" || gotExtra != "x-sk-o1" {
		t.Fatalf("fetch saved: models=%v path=%s auth=%s extra=%s", models, gotPath, gotAuth, gotExtra)
	}
	s.OK(admin, http.MethodGet, fmt.Sprintf("/api/channel/fetch_models/%d", ids["ali"]), nil)
	if gotPath != "/compatible-mode/v1/models" {
		t.Fatalf("ali models path: %s", gotPath)
	}

	models = testhelper.Decode[[]string](t, s.OK(admin, http.MethodPost, "/api/channel/fetch_models", map[string]any{
		"base_url": upstream.URL, "type": constant.ChannelTypeOpenAI, "key": "first\nsecond"}))
	if len(models) != 2 || gotAuth != "Bearer first" {
		t.Fatalf("fetch form: models=%v auth=%s", models, gotAuth)
	}
	if msg := s.Fail(admin, http.MethodPost, "/api/channel/fetch_models", map[string]any{
		"base_url": upstream.URL, "type": constant.ChannelTypeOpenAI, "key": "bad"}); !strings.Contains(msg, "401") {
		t.Fatalf("upstream error message: %q", msg)
	}

	// Balance query for DeepSeek channel
	bal := testhelper.Decode[map[string]any](t, s.OK(admin, http.MethodGet, fmt.Sprintf("/api/channel/update_balance/%d", ids["d1"]), nil))
	if bal["balance"] != 42.5 {
		t.Fatalf("expected balance 42.5, got %v", bal)
	}
	s.Fail(admin, http.MethodGet, fmt.Sprintf("/api/channel/update_balance/%d", ids["o1"]), nil)
}
