package e2e_test

import (
	"fmt"
	"net/http"
	"testing"
)

// TestCronJobLogAndTaskAPIs 覆盖定时任务管理、日志查询与统计、异步任务及视频异常路径接口：
//   - GET/POST/PUT/DELETE /api/cron_job/*（含 /types、/:id/enabled、/:id/runs）
//   - GET/DELETE          /api/log/*（含 /stat、/self、/self/stat、/search、/self/search、/channel_affinity_usage_cache、/token）
//   - GET                 /api/task/ 与 /api/task/self
//   - GET                 /v1/videos/:task_id/content、/v1/videos/:task_id、/v1/video/generations/:task_id（404 校验）
func TestCronJobLogAndTaskAPIs(t *testing.T) {
	admin := newLoggedInAdminClient(t)

	// ==================== 1. 定时任务 (/api/cron_job) ====================
	jobTypes := mustDecode[[]string](t, admin.MustOK(http.MethodGet, "/api/cron_job/types", nil))
	if len(jobTypes) == 0 {
		t.Fatal("expected registered cron job types")
	}

	jobName := uniqueName("e2e_cron")
	job := mustDecode[struct {
		ID      int64  `json:"id"`
		Name    string `json:"name"`
		Enabled bool   `json:"enabled"`
	}](t, admin.MustOK(http.MethodPost, "/api/cron_job/", map[string]any{
		"name":    jobName,
		"type":    "channel_test",
		"cron":    "0 4 * * *",
		"enabled": false, // 禁用状态创建，避免触碰线上渠道状态
		"payload": map[string]any{"auto_disable": false},
	}))
	t.Cleanup(func() {
		_, _, _ = admin.DoAPI(http.MethodDelete, fmt.Sprintf("/api/cron_job/%d", job.ID), nil, nil)
	})

	_ = mustDecode[PageData[map[string]any]](t, admin.MustOK(http.MethodGet, "/api/cron_job/?p=1&page_size=10", nil))
	_ = mustDecode[map[string]any](t, admin.MustOK(http.MethodGet, fmt.Sprintf("/api/cron_job/%d", job.ID), nil))
	admin.MustOK(http.MethodPut, fmt.Sprintf("/api/cron_job/%d", job.ID), map[string]any{
		"name":    jobName + "_updated",
		"type":    "channel_test",
		"cron":    "30 4 * * *",
		"enabled": false,
		"payload": map[string]any{"auto_disable": false},
	})
	admin.MustOK(http.MethodPost, fmt.Sprintf("/api/cron_job/%d/enabled", job.ID), map[string]any{"enabled": true})
	admin.MustOK(http.MethodPost, fmt.Sprintf("/api/cron_job/%d/enabled", job.ID), map[string]any{"enabled": false})
	_ = mustDecode[PageData[map[string]any]](t, admin.MustOK(http.MethodGet, fmt.Sprintf("/api/cron_job/%d/runs?p=1&page_size=10", job.ID), nil))
	admin.MustOK(http.MethodDelete, fmt.Sprintf("/api/cron_job/%d", job.ID), nil)

	// ==================== 2. 日志查询与统计 (/api/log) ====================
	_ = mustDecode[PageData[map[string]any]](t, admin.MustOK(http.MethodGet, "/api/log/?p=1&page_size=10", nil))
	_ = mustDecode[map[string]any](t, admin.MustOK(http.MethodGet, "/api/log/stat", nil))
	_ = mustDecode[PageData[map[string]any]](t, admin.MustOK(http.MethodGet, "/api/log/self?p=1&page_size=10", nil))
	_ = mustDecode[map[string]any](t, admin.MustOK(http.MethodGet, "/api/log/self/stat", nil))
	_ = mustDecode[PageData[map[string]any]](t, admin.MustOK(http.MethodGet, "/api/log/search?keyword=2", nil))
	_ = mustDecode[PageData[map[string]any]](t, admin.MustOK(http.MethodGet, "/api/log/self/search?keyword=2", nil))
	_ = mustDecode[map[string]any](t, admin.MustOK(http.MethodGet, "/api/log/channel_affinity_usage_cache", nil))

	// target_timestamp 缺失时报错；target_timestamp=1（1970-01-01）安全校验删除接口且不删真实日志
	admin.MustFail(http.MethodDelete, "/api/log/", nil)
	_ = mustDecode[int64](t, admin.MustOK(http.MethodDelete, "/api/log/?target_timestamp=1", nil))

	// 新建临时令牌测试免登录 /api/log/token?key=sk-... 以及异步视频不存在任务的 404 响应
	tk := mustDecode[struct {
		ID  int    `json:"id"`
		Key string `json:"key"`
	}](t, admin.MustOK(http.MethodPost, "/api/token/", map[string]any{
		"name":            uniqueName("e2e_log_tk"),
		"unlimited_quota": true,
		"expired_time":    -1,
	}))
	t.Cleanup(func() {
		_, _, _ = admin.DoAPI(http.MethodDelete, fmt.Sprintf("/api/token/%d", tk.ID), nil, nil)
	})

	anon := newLiveClient(t)
	_ = mustDecode[PageData[map[string]any]](t, anon.MustOK(http.MethodGet, "/api/log/token?key=sk-"+tk.Key, nil))

	// ==================== 3. 异步任务与视频接口 (/api/task, /v1/videos) ====================
	_ = mustDecode[PageData[map[string]any]](t, admin.MustOK(http.MethodGet, "/api/task/?p=1&page_size=10", nil))
	_ = mustDecode[PageData[map[string]any]](t, admin.MustOK(http.MethodGet, "/api/task/self?p=1&page_size=10", nil))

	authHdr := map[string]string{"Authorization": "Bearer sk-" + tk.Key}
	for _, videoPath := range []string{
		"/v1/videos/e2e_nonexistent_task/content",
		"/v1/videos/e2e_nonexistent_task",
		"/v1/video/generations/e2e_nonexistent_task",
	} {
		code, _, body := anon.RawDo(http.MethodGet, videoPath, nil, authHdr)
		if code != http.StatusNotFound && code != http.StatusBadRequest {
			t.Fatalf("GET %s expected 404/400 for nonexistent task, got %d: %s", videoPath, code, string(body))
		}
	}
}
