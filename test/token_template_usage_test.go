package e2e_test

import (
	"fmt"
	"net/http"
	"testing"
)

// TestTokenTemplateUsageAndAnalyticsAPIs 覆盖令牌 CRUD、访问限制模板、用量查询与统计分析接口：
//   - POST   /api/token/
//   - GET    /api/token/
//   - GET    /api/token/search
//   - GET    /api/token/:id
//   - PUT    /api/token/
//   - PUT    /api/token/?status_only=true
//   - DELETE /api/token/:id
//   - POST   /api/token/batch
//   - GET    /api/token/analytics/:id
//   - GET    /api/usage/token/
//   - GET    /api/token_template/
//   - POST   /api/token_template/
//   - PUT    /api/token_template/
//   - DELETE /api/token_template/:id
func TestTokenTemplateUsageAndAnalyticsAPIs(t *testing.T) {
	admin := newLoggedInAdminClient(t)

	// 1. 令牌访问限制模板 CRUD
	tplName := uniqueName("e2e_tpl")
	tpl := mustDecode[struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}](t, admin.MustOK(http.MethodPost, "/api/token_template/", map[string]any{
		"name":                 tplName,
		"model_limits_enabled": true,
		"model_limits":         "deepseek-flash,Qwen/Qwen2.5-7B-Instruct",
		"model_limits_type":    0,
	}))
	t.Cleanup(func() {
		_, _, _ = admin.DoAPI(http.MethodDelete, fmt.Sprintf("/api/token_template/%d", tpl.ID), nil, nil)
	})

	tplList := mustDecode[PageData[map[string]any]](t, admin.MustOK(http.MethodGet, "/api/token_template/?p=1&page_size=20", nil))
	if tplList.Total < 1 {
		t.Fatalf("expected at least 1 token template, got %d", tplList.Total)
	}

	admin.MustOK(http.MethodPut, "/api/token_template/", map[string]any{
		"id":                   tpl.ID,
		"name":                 tplName + "_updated",
		"model_limits_enabled": true,
		"model_limits":         "deepseek-flash",
		"model_limits_type":    0,
	})
	admin.MustOK(http.MethodDelete, fmt.Sprintf("/api/token_template/%d", tpl.ID), nil)

	// 2. 新建两个测试令牌（一个用于单删与鉴权测试，一个用于批量删除测试）
	tk1Name := uniqueName("e2e_tk1")
	tk2Name := uniqueName("e2e_tk2")

	type tokenDTO struct {
		ID     int    `json:"id"`
		Key    string `json:"key"`
		Name   string `json:"name"`
		Status int    `json:"status"`
	}
	tk1 := mustDecode[tokenDTO](t, admin.MustOK(http.MethodPost, "/api/token/", map[string]any{
		"name":            tk1Name,
		"unlimited_quota": true,
		"expired_time":    -1,
	}))
	tk2 := mustDecode[tokenDTO](t, admin.MustOK(http.MethodPost, "/api/token/", map[string]any{
		"name":            tk2Name,
		"unlimited_quota": false,
		"remain_quota":    50000,
		"expired_time":    -1,
	}))
	t.Cleanup(func() {
		_, _, _ = admin.DoAPI(http.MethodDelete, fmt.Sprintf("/api/token/%d", tk1.ID), nil, nil)
		_, _, _ = admin.DoAPI(http.MethodPost, "/api/token/batch", map[string]any{"ids": []int{tk2.ID}}, nil)
	})

	// 3. 列表、搜索、详情
	_ = mustDecode[PageData[tokenDTO]](t, admin.MustOK(http.MethodGet, "/api/token/?p=1&page_size=10", nil))
	searched := mustDecode[PageData[tokenDTO]](t, admin.MustOK(http.MethodGet, "/api/token/search?keyword="+tk1Name, nil))
	if searched.Total != 1 || searched.Items[0].ID != tk1.ID {
		t.Fatalf("unexpected token search result: %+v", searched)
	}
	gotTk1 := mustDecode[tokenDTO](t, admin.MustOK(http.MethodGet, fmt.Sprintf("/api/token/%d", tk1.ID), nil))
	if gotTk1.Key == "" {
		t.Fatal("expected token key in detail response")
	}

	// 4. 修改令牌配置 + 启停状态立即生效检查
	admin.MustOK(http.MethodPut, "/api/token/", map[string]any{
		"id":                   tk1.ID,
		"name":                 tk1Name + "_renamed",
		"unlimited_quota":      true,
		"expired_time":         -1,
		"model_limits_enabled": true,
		"model_limits":         "deepseek-flash",
	})

	// 用 API Key 查询 /api/usage/token/
	anon := newLiveClient(t)
	code, usageResp, raw := anon.DoAPI(http.MethodGet, "/api/usage/token/", nil, map[string]string{
		"Authorization": "Bearer sk-" + tk1.Key,
	})
	if code != http.StatusOK || !usageResp.Success {
		t.Fatalf("GET /api/usage/token/ failed: HTTP %d %s", code, string(raw))
	}

	// 禁用令牌后再次请求应被拒绝 (401)
	admin.MustOK(http.MethodPut, "/api/token/?status_only=true", map[string]any{
		"id":     tk1.ID,
		"status": 2,
	})
	disabledCode, _, _ := anon.RawDo(http.MethodGet, "/v1/models", nil, map[string]string{
		"Authorization": "Bearer sk-" + tk1.Key,
	})
	if disabledCode != http.StatusUnauthorized {
		t.Fatalf("disabled token expected 401, got %d", disabledCode)
	}

	// 重新启用后恢复可用
	admin.MustOK(http.MethodPut, "/api/token/?status_only=true", map[string]any{
		"id":     tk1.ID,
		"status": 1,
	})
	enabledCode, _, _ := anon.RawDo(http.MethodGet, "/v1/models", nil, map[string]string{
		"Authorization": "Bearer sk-" + tk1.Key,
	})
	if enabledCode != http.StatusOK {
		t.Fatalf("re-enabled token expected 200, got %d", enabledCode)
	}

	// 5. 令牌统计分析接口 /api/token/analytics/:id
	analytics := mustDecode[map[string]any](t, admin.MustOK(http.MethodGet, fmt.Sprintf("/api/token/analytics/%d?days=7", tk1.ID), nil))
	if _, ok := analytics["summary"]; !ok {
		t.Fatalf("analytics missing summary: %v", analytics)
	}

	// 6. 单个删除与批量删除
	admin.MustOK(http.MethodDelete, fmt.Sprintf("/api/token/%d", tk1.ID), nil)
	deletedCount := mustDecode[int](t, admin.MustOK(http.MethodPost, "/api/token/batch", map[string]any{
		"ids": []int{tk2.ID},
	}))
	if deletedCount != 1 {
		t.Fatalf("expected batch delete count 1, got %d", deletedCount)
	}
}
