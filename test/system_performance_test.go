package e2e_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

// TestSystemPublicAndStatusAPIs 覆盖系统初始化状态、公开内容与连通性自检接口：
//   - GET  /api/setup
//   - POST /api/setup（已初始化时拒绝重复初始化）
//   - GET  /api/status
//   - GET  /api/status/test
//   - GET  /api/notice
//   - GET  /api/about
//   - GET  /api/home_page_content
//   - GET  /api/user-agreement
//   - GET  /api/privacy-policy
func TestSystemPublicAndStatusAPIs(t *testing.T) {
	anon := newLiveClient(t)
	admin := newLoggedInAdminClient(t)

	setupData := mustDecode[map[string]any](t, anon.MustOK(http.MethodGet, "/api/setup", nil))
	if setupData["initialized"] != true {
		t.Fatalf("expected initialized=true on running backend, got %v", setupData)
	}

	// 已初始化的系统再次 POST /api/setup 必须拒绝
	anon.MustFail(http.MethodPost, "/api/setup", map[string]string{
		"username": "another_root",
		"password": "password123",
	})

	statusData := mustDecode[map[string]any](t, anon.MustOK(http.MethodGet, "/api/status", nil))
	if statusData["setup"] != true {
		t.Fatalf("expected status.setup=true, got %v", statusData)
	}

	// 公开文案接口
	for _, path := range []string{
		"/api/notice",
		"/api/about",
		"/api/home_page_content",
		"/api/user-agreement",
		"/api/privacy-policy",
	} {
		_ = mustDecode[string](t, anon.MustOK(http.MethodGet, path, nil))
	}

	// 连通性自检：未登录拒绝，管理员成功
	anon.MustFail(http.MethodGet, "/api/status/test", nil)
	selfCheck := mustDecode[map[string]any](t, admin.MustOK(http.MethodGet, "/api/status/test", nil))
	if selfCheck["success"] != true {
		t.Fatalf("GET /api/status/test expected success=true, got %v", selfCheck)
	}
}

// TestOptionAndAffinityCacheAPIs 覆盖系统配置与渠道亲和性缓存管理接口（无损回滚）：
//   - GET    /api/option/
//   - PUT    /api/option/
//   - POST   /api/option/migrate_console_setting
//   - GET    /api/option/channel_affinity_cache
//   - DELETE /api/option/channel_affinity_cache
func TestOptionAndAffinityCacheAPIs(t *testing.T) {
	admin := newLoggedInAdminClient(t)

	origOptions := mustDecode[map[string]string](t, admin.MustOK(http.MethodGet, "/api/option/", nil))
	origAffinity, hadAffinity := origOptions["ChannelAffinityEnabled"]

	// 开启亲和性开关并在结束后原样恢复
	t.Cleanup(func() {
		restoreVal := "false"
		if hadAffinity {
			restoreVal = origAffinity
		}
		_, _, _ = admin.DoAPI(http.MethodPut, "/api/option/", map[string]string{
			"key":   "ChannelAffinityEnabled",
			"value": restoreVal,
		}, nil)
	})

	admin.MustOK(http.MethodPut, "/api/option/", map[string]string{
		"key":   "ChannelAffinityEnabled",
		"value": "true",
	})

	// 空 key 校验应失败
	admin.MustFail(http.MethodPut, "/api/option/", map[string]string{
		"key":   "   ",
		"value": "x",
	})

	// 控制台旧配置迁移接口（幂等无副作用）
	mig := mustDecode[map[string]any](t, admin.MustOK(http.MethodPost, "/api/option/migrate_console_setting", map[string]any{}))
	if _, ok := mig["skipped"]; !ok {
		t.Fatalf("unexpected migrate_console_setting response: %v", mig)
	}

	// 渠道亲和性缓存统计与清理
	affStats := mustDecode[map[string]any](t, admin.MustOK(http.MethodGet, "/api/option/channel_affinity_cache", nil))
	if affStats["enabled"] != true {
		t.Fatalf("expected channel affinity enabled=true, got %v", affStats)
	}
	admin.MustOK(http.MethodDelete, "/api/option/channel_affinity_cache", nil)
}

// TestPerformanceAPIs 覆盖性能监控与运维接口（Root）：
//   - GET    /api/performance/stats
//   - POST   /api/performance/reset_stats
//   - POST   /api/performance/gc
//   - DELETE /api/performance/disk_cache
func TestPerformanceAPIs(t *testing.T) {
	admin := newLoggedInAdminClient(t)

	stats := mustDecode[map[string]json.RawMessage](t, admin.MustOK(http.MethodGet, "/api/performance/stats", nil))
	for _, field := range []string{"cache_stats", "memory_stats", "disk_cache_info", "disk_space_info", "config"} {
		if _, ok := stats[field]; !ok {
			t.Errorf("GET /api/performance/stats missing field %q", field)
		}
	}

	admin.MustOK(http.MethodPost, "/api/performance/reset_stats", nil)
	admin.MustOK(http.MethodPost, "/api/performance/gc", nil)
	admin.MustOK(http.MethodDelete, "/api/performance/disk_cache", nil)
}
