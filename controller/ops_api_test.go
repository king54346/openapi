package controller_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"openapi/common"
	"openapi/constant"
	"openapi/controller"
	"openapi/model"
)

// newChatUpstream 模拟上游：/ok 下对话与 embedding 成功，/fail 下返回 500。
func newChatUpstream(t *testing.T, calls *atomic.Int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasPrefix(r.URL.Path, "/fail/"):
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = io.WriteString(w, `{"error":{"message":"upstream down","type":"server_error"}}`)
		case strings.HasSuffix(r.URL.Path, "/v1/embeddings"):
			_, _ = io.WriteString(w, `{"object":"list","data":[{"object":"embedding","index":0,"embedding":[0.1]}],"model":"text-embedding-v1","usage":{"prompt_tokens":1,"total_tokens":1}}`)
		default:
			_, _ = io.WriteString(w, `{"id":"c1","object":"chat.completion","created":1,"model":"m","choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func insertTestChannel(t *testing.T, name, baseURL, models string, status int) *model.Channel {
	t.Helper()
	autoBan := 1
	ch := &model.Channel{Type: constant.ChannelTypeOpenAI, Key: "k-" + name, Name: name, Status: status,
		Group: "default", Models: models, BaseURL: &baseURL, AutoBan: &autoBan}
	if err := ch.Insert(); err != nil {
		t.Fatal(err)
	}
	return ch
}

func channelStatusOf(t *testing.T, id int) int {
	t.Helper()
	ch, err := model.GetChannelById(id, false)
	if err != nil {
		t.Fatal(err)
	}
	return ch.Status
}

func TestCronJobAPIAndChannelTest(t *testing.T) {
	s := newTestServer(t)
	controller.RegisterCronExecutors()
	admin := s.createUser("admin", common.RoleAdminUser)
	user := s.createUser("alice", common.RoleCommonUser)

	var calls atomic.Int32
	upstream := newChatUpstream(t, &calls)
	healthy := insertTestChannel(t, "healthy", upstream.URL+"/ok", "gpt-4o", common.ChannelStatusEnabled)
	broken := insertTestChannel(t, "broken", upstream.URL+"/fail", "gpt-4o", common.ChannelStatusEnabled)
	recovered := insertTestChannel(t, "recovered", upstream.URL+"/ok", "text-embedding-v1", common.ChannelStatusAutoDisabled)
	manual := insertTestChannel(t, "manual", upstream.URL+"/ok", "gpt-4o", common.ChannelStatusManuallyDisabled)
	image := insertTestChannel(t, "image", upstream.URL+"/ok", "doubao-seedream-4-5", common.ChannelStatusEnabled)
	if err := model.InitChannelCache(); err != nil {
		t.Fatal(err)
	}

	// 权限与校验
	s.fail(user, http.MethodGet, "/api/cron_job/", nil)
	if types := decode[[]string](t, s.ok(admin, http.MethodGet, "/api/cron_job/types", nil)); len(types) != 1 || types[0] != "channel_test" {
		t.Fatalf("types: %v", types)
	}
	for _, bad := range []map[string]any{
		{"name": "", "type": "channel_test", "cron": "* * * * *"},
		{"name": "x", "type": "account_test", "cron": "* * * * *"},
		{"name": "x", "type": "channel_test", "cron": "61 * * * *"},
	} {
		s.fail(admin, http.MethodPost, "/api/cron_job/", bad)
	}

	job := decode[model.CronJob](t, s.ok(admin, http.MethodPost, "/api/cron_job/", map[string]any{
		"name": "test channels", "type": "channel_test", "cron": "*/10 * * * *", "enabled": true,
		"payload": map[string]any{"auto_disable": true},
	}))
	s.ok(admin, http.MethodPut, fmt.Sprintf("/api/cron_job/%d", job.Id), map[string]any{
		"name": "test channels (renamed)", "type": "channel_test", "cron": "0 * * * *", "enabled": true,
		"payload": map[string]any{"auto_disable": true},
	})
	if jobs := decode[page[model.CronJob]](t, s.ok(admin, http.MethodGet, "/api/cron_job/?p=1&page_size=10", nil)); jobs.Total != 1 || jobs.Items[0].Id != job.Id {
		t.Fatalf("cron job list: %+v", jobs)
	}
	s.ok(admin, http.MethodPost, fmt.Sprintf("/api/cron_job/%d/enabled", job.Id), map[string]any{"enabled": false})
	if got := decode[model.CronJob](t, s.ok(admin, http.MethodGet, fmt.Sprintf("/api/cron_job/%d", job.Id), nil)); got.Enabled || got.Cron != "0 * * * *" {
		t.Fatalf("after update/toggle: %+v", got)
	}

	// 立即执行一次，等待运行结果
	s.ok(admin, http.MethodPost, fmt.Sprintf("/api/cron_job/%d/run", job.Id), nil)
	var ran *model.CronJob
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if ran, _ = model.GetCronJobById(job.Id); ran.RunCount > 0 {
			break
		}
	}
	if ran.RunCount != 1 || ran.LastStatus != model.CronJobStatusFailed {
		t.Fatalf("run result: %+v", ran)
	}
	if !strings.Contains(ran.LastMessage, "tested 3 channels: 2 passed, 1 failed, 1 skipped") || !strings.Contains(ran.LastMessage, "broken") {
		t.Fatalf("run message: %q", ran.LastMessage)
	}

	// 失败的渠道被自动禁用，自动禁用但测试通过的被恢复，手动禁用的不测不动
	if channelStatusOf(t, broken.Id) != common.ChannelStatusAutoDisabled {
		t.Error("broken channel should be auto disabled")
	}
	if channelStatusOf(t, recovered.Id) != common.ChannelStatusEnabled {
		t.Error("auto-disabled healthy channel should be recovered")
	}
	if channelStatusOf(t, healthy.Id) != common.ChannelStatusEnabled || channelStatusOf(t, manual.Id) != common.ChannelStatusManuallyDisabled ||
		channelStatusOf(t, image.Id) != common.ChannelStatusEnabled {
		t.Error("other channels must keep their status")
	}
	if ch, _ := model.GetRandomSatisfiedChannel("default", "text-embedding-v1", 0); ch == nil || ch.Id != recovered.Id {
		t.Error("recovered channel should be back in cache")
	}

	runs := decode[page[model.CronJobRun]](t, s.ok(admin, http.MethodGet, fmt.Sprintf("/api/cron_job/%d/runs?p=1&page_size=10", job.Id), nil))
	if runs.Total != 1 || len(runs.Items) != 1 || runs.Items[0].Success {
		t.Fatalf("runs: %+v", runs)
	}

	// 手动测试单个渠道
	type testResp struct {
		Success bool           `json:"success"`
		Message string         `json:"message"`
		Data    map[string]any `json:"data"`
	}
	var tr testResp
	_, resp := s.do(admin, http.MethodGet, fmt.Sprintf("/api/channel/test/%d", healthy.Id), nil)
	_ = json.Unmarshal(resp.Data, &tr.Data)
	if !resp.Success || tr.Data["path"] != "/v1/chat/completions" {
		t.Fatalf("test healthy: %+v %s", resp, resp.Data)
	}
	if msg := s.fail(admin, http.MethodGet, fmt.Sprintf("/api/channel/test/%d", image.Id), nil); !strings.HasPrefix(msg, "skipped") {
		t.Fatalf("test image channel: %q", msg)
	}
	if msg := s.fail(admin, http.MethodGet, fmt.Sprintf("/api/channel/test/%d", broken.Id), nil); !strings.Contains(msg, "500") {
		t.Fatalf("test broken channel: %q", msg)
	}

	s.ok(admin, http.MethodDelete, fmt.Sprintf("/api/cron_job/%d", job.Id), nil)
	s.fail(admin, http.MethodGet, fmt.Sprintf("/api/cron_job/%d", job.Id), nil)
}

func TestLogAPI(t *testing.T) {
	s := newTestServer(t)
	admin := s.createUser("admin", common.RoleAdminUser)
	alice := s.createUser("alice", common.RoleCommonUser)
	token := &model.Token{UserId: alice.id, Name: "t", ExpiredTime: -1, UnlimitedQuota: true}
	if err := token.Insert(); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	for i, l := range []model.Log{
		{UserId: alice.id, Username: "alice", Type: model.LogTypeConsume, ModelName: "gpt-4o", TokenId: token.Id, TokenName: "t", Quota: 10, RequestId: "r1"},
		{UserId: alice.id, Username: "alice", Type: model.LogTypeError, ModelName: "gpt-4o", TokenId: token.Id, TokenName: "t", RequestId: "r2"},
		{UserId: admin.id, Username: "admin", Type: model.LogTypeConsume, ModelName: "deepseek", Quota: 5},
	} {
		l.CreatedAt = now - int64(i)
		if err := model.LOG_DB.Create(&l).Error; err != nil {
			t.Fatal(err)
		}
	}

	all := decode[page[model.Log]](t, s.ok(admin, http.MethodGet, "/api/log/?p=1&page_size=2", nil))
	if all.Total != 3 || len(all.Items) != 2 {
		t.Fatalf("all logs: total=%d items=%d", all.Total, len(all.Items))
	}
	if consume := decode[page[model.Log]](t, s.ok(admin, http.MethodGet, fmt.Sprintf("/api/log/?type=%d", model.LogTypeConsume), nil)); consume.Total != 2 {
		t.Fatalf("consume logs: %d", consume.Total)
	}
	s.fail(alice, http.MethodGet, "/api/log/", nil) // 普通用户不能看全部日志
	if self := decode[page[model.Log]](t, s.ok(alice, http.MethodGet, "/api/log/self", nil)); self.Total != 2 {
		t.Fatalf("self logs: %d", self.Total)
	}
	stat := decode[map[string]int](t, s.ok(admin, http.MethodGet, fmt.Sprintf("/api/log/stat?type=%d", model.LogTypeConsume), nil))
	if stat["quota"] != 15 {
		t.Fatalf("stat: %v", stat)
	}

	// 凭令牌 key 查日志，无需登录
	byKey := decode[page[model.Log]](t, s.ok(actor{}, http.MethodGet, "/api/log/token?key=sk-"+token.Key+"&request_id=r2", nil))
	if byKey.Total != 1 || byKey.Items[0].RequestId != "r2" {
		t.Fatalf("logs by key: %+v", byKey)
	}
	if byKey = decode[page[model.Log]](t, s.ok(actor{}, http.MethodGet, "/api/log/token?key="+token.Key+"&p=2&page_size=1", nil)); byKey.Total != 2 || len(byKey.Items) != 1 || byKey.Items[0].RequestId != "r1" {
		t.Fatalf("logs by key page 2: %+v", byKey)
	}

	// 关键字搜索分页：管理员按类型搜全部，用户只搜自己的
	if r := decode[page[model.Log]](t, s.ok(admin, http.MethodGet, fmt.Sprintf("/api/log/search?keyword=%d&page_size=1", model.LogTypeConsume), nil)); r.Total != 2 || len(r.Items) != 1 {
		t.Fatalf("search all logs: %+v", r)
	}
	if r := decode[page[model.Log]](t, s.ok(alice, http.MethodGet, fmt.Sprintf("/api/log/self/search?keyword=%d", model.LogTypeConsume), nil)); r.Total != 1 || r.Items[0].Username != "alice" {
		t.Fatalf("search self logs: %+v", r)
	}
	s.fail(actor{}, http.MethodGet, "/api/log/token?key=sk-nope", nil)

	s.fail(admin, http.MethodDelete, "/api/log/", nil)
	if n := decode[int](t, s.ok(admin, http.MethodDelete, fmt.Sprintf("/api/log/?target_timestamp=%d", now), nil)); n != 2 {
		t.Fatalf("deleted logs: %d", n)
	}
}

func TestModelMetaAPI(t *testing.T) {
	s := newTestServer(t)
	admin := s.createUser("admin", common.RoleAdminUser)
	insertTestChannel(t, "ch", "http://127.0.0.1:1", "gpt-4o,gpt-4o-mini", common.ChannelStatusEnabled)
	if err := model.InitChannelCache(); err != nil {
		t.Fatal(err)
	}

	exact := decode[model.Model](t, s.ok(admin, http.MethodPost, "/api/models/", map[string]any{
		"model_name": "gpt-4o", "tags": "对话,工具", "vendor_id": 1}))
	s.fail(admin, http.MethodPost, "/api/models/", map[string]any{"model_name": "gpt-4o"}) // 重名
	s.fail(admin, http.MethodPost, "/api/models/", map[string]any{"model_name": " "})
	// 前缀规则：匹配所有以 gpt-4o 开头的已启用模型
	prefix := decode[model.Model](t, s.ok(admin, http.MethodPost, "/api/models/", map[string]any{
		"model_name": "gpt-", "name_rule": model.NameRulePrefix, "tags": "对话"}))

	got := decode[model.Model](t, s.ok(admin, http.MethodGet, fmt.Sprintf("/api/models/%d", exact.Id), nil))
	if len(got.BoundChannels) != 1 || got.BoundChannels[0].Name != "ch" || len(got.EnableGroups) != 1 || got.EnableGroups[0] != "default" {
		t.Fatalf("exact model enrichment: %+v", got)
	}
	got = decode[model.Model](t, s.ok(admin, http.MethodGet, fmt.Sprintf("/api/models/%d", prefix.Id), nil))
	if got.MatchedCount != 2 || got.MatchedModels[0] != "gpt-4o" || got.MatchedModels[1] != "gpt-4o-mini" {
		t.Fatalf("prefix model enrichment: %+v", got)
	}

	type metaList struct {
		Items        []model.Model    `json:"items"`
		Total        int64            `json:"total"`
		VendorCounts map[string]int64 `json:"vendor_counts"`
	}
	list := decode[metaList](t, s.ok(admin, http.MethodGet, "/api/models/?p=1&page_size=1", nil))
	if list.Total != 2 || len(list.Items) != 1 || list.VendorCounts["1"] != 1 || list.VendorCounts["0"] != 1 {
		t.Fatalf("list: %+v", list)
	}
	if tags := decode[[]string](t, s.ok(admin, http.MethodGet, "/api/models/tags", nil)); len(tags) != 2 || tags[0] != "对话" {
		t.Fatalf("tags: %v", tags)
	}
	// 标签需全部命中；按供应商 id 过滤
	if r := decode[page[model.Model]](t, s.ok(admin, http.MethodGet, "/api/models/search?tags=对话,工具", nil)); r.Total != 1 || r.Items[0].Id != exact.Id {
		t.Fatalf("search by tags: %+v", r)
	}
	if r := decode[page[model.Model]](t, s.ok(admin, http.MethodGet, "/api/models/search?vendor=1&keyword=gpt", nil)); r.Total != 1 {
		t.Fatalf("search by vendor: %+v", r)
	}

	// 修改：重名被拒；只改状态时其他字段不变
	s.fail(admin, http.MethodPut, "/api/models/", map[string]any{"id": prefix.Id, "model_name": "gpt-4o"})
	updated := decode[model.Model](t, s.ok(admin, http.MethodPut, "/api/models/", map[string]any{
		"id": exact.Id, "model_name": "gpt-4o", "description": "desc", "tags": "对话"}))
	if updated.Description != "desc" || updated.Tags != "对话" || updated.CreatedTime != exact.CreatedTime {
		t.Fatalf("update: %+v", updated)
	}
	updated = decode[model.Model](t, s.ok(admin, http.MethodPut, "/api/models/?status_only=true", map[string]any{"id": exact.Id, "status": 0}))
	if updated.Status != 0 || updated.Description != "desc" {
		t.Fatalf("status only: %+v", updated)
	}
	s.fail(admin, http.MethodPut, "/api/models/", map[string]any{"id": 999, "model_name": "x"})

	s.ok(admin, http.MethodDelete, fmt.Sprintf("/api/models/%d", exact.Id), nil)
	s.fail(admin, http.MethodGet, fmt.Sprintf("/api/models/%d", exact.Id), nil)
	// 软删除后可以重新创建同名模型
	s.ok(admin, http.MethodPost, "/api/models/", map[string]any{"model_name": "gpt-4o"})
}

func TestPerformanceAPI(t *testing.T) {
	s := newTestServer(t)
	root := s.createUser("root", common.RoleRootUser)
	admin := s.createUser("admin", common.RoleAdminUser)

	s.fail(admin, http.MethodGet, "/api/performance/stats", nil) // 需要 root
	stats := decode[map[string]json.RawMessage](t, s.ok(root, http.MethodGet, "/api/performance/stats", nil))
	for _, key := range []string{"cache_stats", "memory_stats", "disk_cache_info", "disk_space_info", "config"} {
		if _, ok := stats[key]; !ok {
			t.Errorf("stats missing %s", key)
		}
	}
	for _, path := range []string{"/api/performance/reset_stats", "/api/performance/gc"} {
		s.ok(root, http.MethodPost, path, nil)
	}
	s.ok(root, http.MethodDelete, "/api/performance/disk_cache", nil)
}
