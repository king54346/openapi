package e2e_test

import (
	"fmt"
	"net/http"
	"testing"
)

// TestVendorModelMetaAndPrefillGroupAPIs 覆盖供应商元数据、模型元数据与预填充组管理接口：
//   - GET/POST/PUT/DELETE /api/vendors/*
//   - GET/POST/PUT/DELETE /api/models/*（含 missing、tags、sync_upstream/preview、sync_upstream dry_run）
//   - GET/POST/PUT/DELETE /api/prefill_group/*
func TestVendorModelMetaAndPrefillGroupAPIs(t *testing.T) {
	admin := newLoggedInAdminClient(t)

	// ==================== 1. 供应商元数据 (/api/vendors) ====================
	vendorName := uniqueName("E2E_Vendor")
	vendor := mustDecode[struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}](t, admin.MustOK(http.MethodPost, "/api/vendors/", map[string]any{
		"name":        vendorName,
		"description": "E2E automated vendor test",
		"icon":        "openai",
		"status":      1,
	}))
	t.Cleanup(func() {
		_, _, _ = admin.DoAPI(http.MethodDelete, fmt.Sprintf("/api/vendors/%d", vendor.ID), nil, nil)
	})

	_ = mustDecode[PageData[map[string]any]](t, admin.MustOK(http.MethodGet, "/api/vendors/?p=1&page_size=10", nil))
	vSearch := mustDecode[PageData[map[string]any]](t, admin.MustOK(http.MethodGet, "/api/vendors/search?keyword="+vendorName, nil))
	if vSearch.Total != 1 {
		t.Fatalf("expected vendor search total=1, got %d", vSearch.Total)
	}
	vDetail := mustDecode[map[string]any](t, admin.MustOK(http.MethodGet, fmt.Sprintf("/api/vendors/%d", vendor.ID), nil))
	if vDetail["name"] != vendorName {
		t.Fatalf("unexpected vendor detail: %v", vDetail)
	}
	admin.MustOK(http.MethodPut, "/api/vendors/", map[string]any{
		"id":          vendor.ID,
		"description": "E2E vendor updated",
	})

	// ==================== 2. 模型元数据 (/api/models) ====================
	modelName := uniqueName("e2e-meta-model")
	meta := mustDecode[struct {
		ID        int    `json:"id"`
		ModelName string `json:"model_name"`
	}](t, admin.MustOK(http.MethodPost, "/api/models/", map[string]any{
		"model_name":  modelName,
		"description": "E2E model metadata",
		"tags":        "E2E,对话",
		"vendor_id":   vendor.ID,
		"name_rule":   0,
	}))
	t.Cleanup(func() {
		_, _, _ = admin.DoAPI(http.MethodDelete, fmt.Sprintf("/api/models/%d", meta.ID), nil, nil)
	})

	_ = mustDecode[map[string]any](t, admin.MustOK(http.MethodGet, "/api/models/?p=1&page_size=10", nil))
	mSearch := mustDecode[PageData[map[string]any]](t, admin.MustOK(http.MethodGet, "/api/models/search?keyword="+modelName+"&tags=E2E", nil))
	if mSearch.Total != 1 {
		t.Fatalf("expected model meta search total=1, got %d", mSearch.Total)
	}
	_ = mustDecode[[]string](t, admin.MustOK(http.MethodGet, "/api/models/tags", nil))
	_ = mustDecode[map[string]any](t, admin.MustOK(http.MethodGet, "/api/models/missing", nil))
	_ = mustDecode[map[string]any](t, admin.MustOK(http.MethodGet, "/api/models/sync_upstream/preview", nil))
	// dry_run=true 只预览不落库，安全验证同步接口
	_ = mustDecode[map[string]any](t, admin.MustOK(http.MethodPost, "/api/models/sync_upstream", map[string]any{
		"dry_run": true,
	}))

	mDetail := mustDecode[map[string]any](t, admin.MustOK(http.MethodGet, fmt.Sprintf("/api/models/%d", meta.ID), nil))
	if mDetail["model_name"] != modelName {
		t.Fatalf("unexpected model meta detail: %v", mDetail)
	}
	admin.MustOK(http.MethodPut, "/api/models/", map[string]any{
		"id":          meta.ID,
		"model_name":  modelName,
		"description": "E2E model metadata updated",
		"tags":        "E2E",
		"vendor_id":   vendor.ID,
	})
	admin.MustOK(http.MethodPut, "/api/models/?status_only=true", map[string]any{
		"id":     meta.ID,
		"status": 0,
	})
	admin.MustOK(http.MethodDelete, fmt.Sprintf("/api/models/%d", meta.ID), nil)
	admin.MustOK(http.MethodDelete, fmt.Sprintf("/api/vendors/%d", vendor.ID), nil)

	// ==================== 3. 预填充组 (/api/prefill_group) ====================
	pgName := uniqueName("e2e_prefill")
	pg := mustDecode[struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}](t, admin.MustOK(http.MethodPost, "/api/prefill_group/", map[string]any{
		"name":  pgName,
		"type":  "model",
		"items": []string{"deepseek-flash", "Qwen/Qwen2.5-7B-Instruct"},
	}))
	t.Cleanup(func() {
		_, _, _ = admin.DoAPI(http.MethodDelete, fmt.Sprintf("/api/prefill_group/%d", pg.ID), nil, nil)
	})

	_ = mustDecode[PageData[map[string]any]](t, admin.MustOK(http.MethodGet, "/api/prefill_group/?type=model", nil))
	admin.MustOK(http.MethodPut, "/api/prefill_group/", map[string]any{
		"id":    pg.ID,
		"name":  pgName + "_v2",
		"type":  "model",
		"items": []string{"deepseek-flash"},
	})
	admin.MustOK(http.MethodDelete, fmt.Sprintf("/api/prefill_group/%d", pg.ID), nil)
}
