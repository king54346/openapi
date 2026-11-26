package cron_test

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
	"openapi/controller/common/testhelper"
	"openapi/model"
	cronsvc "openapi/service/cron"
)

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

func channelStatusOf(t *testing.T, id int) int {
	t.Helper()
	ch, err := model.GetChannelById(id, false)
	if err != nil {
		t.Fatal(err)
	}
	return ch.Status
}

func TestCronJobAPIAndChannelTest(t *testing.T) {
	s := testhelper.NewTestServer(t)
	cronsvc.RegisterCronExecutors()
	admin := s.CreateUser("admin", common.RoleAdminUser)
	user := s.CreateUser("alice", common.RoleCommonUser)

	var calls atomic.Int32
	upstream := newChatUpstream(t, &calls)
	healthy := testhelper.InsertTestChannel(t, "healthy", upstream.URL+"/ok", "gpt-4o", common.ChannelStatusEnabled)
	broken := testhelper.InsertTestChannel(t, "broken", upstream.URL+"/fail", "gpt-4o", common.ChannelStatusEnabled)
	recovered := testhelper.InsertTestChannel(t, "recovered", upstream.URL+"/ok", "text-embedding-v1", common.ChannelStatusAutoDisabled)
	manual := testhelper.InsertTestChannel(t, "manual", upstream.URL+"/ok", "gpt-4o", common.ChannelStatusManuallyDisabled)
	image := testhelper.InsertTestChannel(t, "image", upstream.URL+"/ok", "doubao-seedream-4-5", common.ChannelStatusEnabled)
	if err := model.InitChannelCache(); err != nil {
		t.Fatal(err)
	}

	s.Fail(user, http.MethodGet, "/api/cron_job/", nil)
	if types := testhelper.Decode[[]string](t, s.OK(admin, http.MethodGet, "/api/cron_job/types", nil)); len(types) != 1 || types[0] != "channel_test" {
		t.Fatalf("types: %v", types)
	}
	for _, bad := range []map[string]any{
		{"name": "", "type": "channel_test", "cron": "* * * * *"},
		{"name": "x", "type": "account_test", "cron": "* * * * *"},
		{"name": "x", "type": "channel_test", "cron": "61 * * * *"},
	} {
		s.Fail(admin, http.MethodPost, "/api/cron_job/", bad)
	}

	job := testhelper.Decode[model.CronJob](t, s.OK(admin, http.MethodPost, "/api/cron_job/", map[string]any{
		"name": "test channels", "type": "channel_test", "cron": "*/10 * * * *", "enabled": true,
		"payload": map[string]any{"auto_disable": true},
	}))
	s.OK(admin, http.MethodPut, fmt.Sprintf("/api/cron_job/%d", job.Id), map[string]any{
		"name": "test channels (renamed)", "type": "channel_test", "cron": "0 * * * *", "enabled": true,
		"payload": map[string]any{"auto_disable": true},
	})
	if jobs := testhelper.Decode[testhelper.Page[model.CronJob]](t, s.OK(admin, http.MethodGet, "/api/cron_job/?p=1&page_size=10", nil)); jobs.Total != 1 || jobs.Items[0].Id != job.Id {
		t.Fatalf("cron job list: %+v", jobs)
	}
	s.OK(admin, http.MethodPost, fmt.Sprintf("/api/cron_job/%d/enabled", job.Id), map[string]any{"enabled": false})
	if got := testhelper.Decode[model.CronJob](t, s.OK(admin, http.MethodGet, fmt.Sprintf("/api/cron_job/%d", job.Id), nil)); got.Enabled || got.Cron != "0 * * * *" {
		t.Fatalf("after update/toggle: %+v", got)
	}

	s.OK(admin, http.MethodPost, fmt.Sprintf("/api/cron_job/%d/run", job.Id), nil)
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
	if ch, _ := model.GetRandomSatisfiedChannel("", "text-embedding-v1", 0); ch == nil || ch.Id != recovered.Id {
		t.Error("recovered channel should be back in cache")
	}

	runs := testhelper.Decode[testhelper.Page[model.CronJobRun]](t, s.OK(admin, http.MethodGet, fmt.Sprintf("/api/cron_job/%d/runs?p=1&page_size=10", job.Id), nil))
	if runs.Total != 1 || len(runs.Items) != 1 || runs.Items[0].Success {
		t.Fatalf("runs: %+v", runs)
	}

	var trData map[string]any
	_, resp := s.Do(admin, http.MethodGet, fmt.Sprintf("/api/channel/test/%d", healthy.Id), nil)
	_ = json.Unmarshal(resp.Data, &trData)
	if !resp.Success || trData["path"] != "/v1/chat/completions" {
		t.Fatalf("test healthy: %+v %s", resp, resp.Data)
	}
	if msg := s.Fail(admin, http.MethodGet, fmt.Sprintf("/api/channel/test/%d", image.Id), nil); !strings.HasPrefix(msg, "skipped") {
		t.Fatalf("test image channel: %q", msg)
	}
	if msg := s.Fail(admin, http.MethodGet, fmt.Sprintf("/api/channel/test/%d", broken.Id), nil); !strings.Contains(msg, "500") {
		t.Fatalf("test broken channel: %q", msg)
	}

	s.OK(admin, http.MethodDelete, fmt.Sprintf("/api/cron_job/%d", job.Id), nil)
	s.Fail(admin, http.MethodGet, fmt.Sprintf("/api/cron_job/%d", job.Id), nil)
}
