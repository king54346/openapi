package e2e_test

import (
	"fmt"
	"net/http"
	"testing"
)

// TestChannelFullLifecycleAndOpsAPIs 覆盖渠道模块全部管理、标签、多 Key、模型拉取、余额与测速记录接口：
//   - GET    /api/channel/（普通分页 + tag_mode=true）
//   - GET    /api/channel/search
//   - GET    /api/channel/models
//   - GET    /api/channel/models_enabled
//   - GET    /api/models
//   - POST   /api/channel/fetch_models
//   - POST   /api/channel/（single / batch / multi_to_single）
//   - GET    /api/channel/:id
//   - POST   /api/channel/:id/key
//   - GET    /api/channel/fetch_models/:id
//   - PUT    /api/channel/（常规修改 + key_mode=append）
//   - POST   /api/channel/copy/:id
//   - DELETE /api/channel/:id/model
//   - POST   /api/channel/fix
//   - POST   /api/channel/batch/tag
//   - GET    /api/channel/tag/models
//   - PUT    /api/channel/tag
//   - POST   /api/channel/tag/disabled
//   - POST   /api/channel/tag/enabled
//   - POST   /api/channel/multi_key/manage
//   - GET    /api/channel/update_balance/:id
//   - GET    /api/channel/test/:id
//   - GET    /api/channel/test/:id/all_models
//   - GET    /api/channel/test_records
//   - GET    /api/channel/test_records/model_tags
//   - POST   /api/channel/test_records/:id/retest
//   - POST   /api/channel/test_records/batch_retest
//   - POST   /api/channel/test_records/retest_filtered
//   - DELETE /api/channel/test_records（仅按临时 channel_id 清理，不触碰线上记录）
//   - DELETE /api/channel/:id & POST /api/channel/batch
func TestChannelFullLifecycleAndOpsAPIs(t *testing.T) {
	admin := newLoggedInAdminClient(t)

	// 1. 内置与启用模型目录接口
	builtinModels := mustDecode[[]map[string]any](t, admin.MustOK(http.MethodGet, "/api/channel/models", nil))
	if len(builtinModels) == 0 {
		t.Fatal("expected non-empty /api/channel/models")
	}
	_ = mustDecode[[]string](t, admin.MustOK(http.MethodGet, "/api/channel/models_enabled", nil))
	byType := mustDecode[map[string][]string](t, admin.MustOK(http.MethodGet, "/api/models", nil))
	if len(byType["43"]) == 0 || len(byType["24"]) == 0 {
		t.Fatalf("expected DeepSeek(43) and Gemini(24) in /api/models, got keys=%v", byType)
	}

	// 2. 新建渠道前拉取两个真实上游的模型列表
	dsKey := deepSeekTestKey(t)
	xzURL := customOpenAIBaseURL(t)
	xzKey := customOpenAITestKey(t)

	dsModels := mustDecode[[]string](t, admin.MustOK(http.MethodPost, "/api/channel/fetch_models", map[string]any{
		"type":     43, // ChannelTypeDeepSeek
		"base_url": "",
		"key":      dsKey,
	}))
	if len(dsModels) == 0 {
		t.Fatal("expected DeepSeek upstream models")
	}

	xzModels := mustDecode[[]string](t, admin.MustOK(http.MethodPost, "/api/channel/fetch_models", map[string]any{
		"type":     1, // ChannelTypeOpenAI
		"base_url": xzURL,
		"key":      xzKey,
	}))
	if len(xzModels) == 0 {
		t.Fatal("expected Custom OpenAI upstream models")
	}

	var cleanupIDs []int
	t.Cleanup(func() {
		if len(cleanupIDs) > 0 {
			_, _, _ = admin.DoAPI(http.MethodPost, "/api/channel/batch", map[string]any{"ids": cleanupIDs}, nil)
		}
	})

	// 3. 新建 single 渠道（DeepSeek，使用独立测试模型名 + model_mapping 避免接走线上流量）
	chName := uniqueName("e2e_ds_single")
	tagName := uniqueName("e2e_tag")
	singleModelA := uniqueName("e2e-ds-a")
	singleModelB := uniqueName("e2e-ds-b")
	mappingJSON := fmt.Sprintf(`{%q:%q,%q:%q}`, singleModelA, dsModels[0], singleModelB, dsModels[0])

	addSingle := mustDecode[struct {
		Count int   `json:"count"`
		IDs   []int `json:"ids"`
	}](t, admin.MustOK(http.MethodPost, "/api/channel/", map[string]any{
		"mode": "single",
		"channel": map[string]any{
			"name":          chName,
			"type":          43,
			"key":           dsKey,
			"models":        singleModelA + "," + singleModelB,
			"model_mapping": mappingJSON,
			"priority":      0,
			"auto_ban":      0,
		},
	}))
	singleID := addSingle.IDs[0]
	cleanupIDs = append(cleanupIDs, singleID)

	t.Cleanup(func() {
		_, _, _ = admin.DoAPI(http.MethodDelete, fmt.Sprintf("/api/channel/test_records?channel_id=%d", singleID), nil, nil)
	})

	// 4. 列表、搜索、详情（key 脱敏）、Root 查看真实 key、已保存渠道拉取模型
	_ = mustDecode[PageData[map[string]any]](t, admin.MustOK(http.MethodGet, "/api/channel/?p=1&page_size=10&id_sort=true", nil))
	_ = mustDecode[PageData[map[string]any]](t, admin.MustOK(http.MethodGet, "/api/channel/?tag_mode=true", nil))
	searchRes := mustDecode[PageData[map[string]any]](t, admin.MustOK(http.MethodGet, "/api/channel/search?keyword="+chName, nil))
	if searchRes.Total != 1 {
		t.Fatalf("expected search total=1 for %s, got %d", chName, searchRes.Total)
	}

	chDetail := mustDecode[map[string]any](t, admin.MustOK(http.MethodGet, fmt.Sprintf("/api/channel/%d", singleID), nil))
	if chDetail["key"] != "" {
		t.Fatalf("channel detail must mask key, got %v", chDetail["key"])
	}
	keyData := mustDecode[map[string]string](t, admin.MustOK(http.MethodPost, fmt.Sprintf("/api/channel/%d/key", singleID), nil))
	if keyData["key"] != dsKey {
		t.Fatalf("unexpected channel key from /:id/key")
	}
	savedModels := mustDecode[[]string](t, admin.MustOK(http.MethodGet, fmt.Sprintf("/api/channel/fetch_models/%d", singleID), nil))
	if len(savedModels) == 0 {
		t.Fatal("expected non-empty models from GET /api/channel/fetch_models/:id")
	}

	// 5. 更新渠道、移除单个模型、修复渠道能力、复制渠道
	admin.MustOK(http.MethodPut, "/api/channel/", map[string]any{
		"id":       singleID,
		"priority": 3,
		"weight":   2,
	})
	rmRes := mustDecode[map[string]any](t, admin.MustOK(http.MethodDelete, fmt.Sprintf("/api/channel/%d/model?model=%s", singleID, singleModelB), nil))
	if rmRes["removed"] != singleModelB {
		t.Fatalf("unexpected remove model result: %v", rmRes)
	}
	_ = mustDecode[map[string]any](t, admin.MustOK(http.MethodPost, "/api/channel/fix", nil))

	copyRes := mustDecode[map[string]int](t, admin.MustOK(http.MethodPost, fmt.Sprintf("/api/channel/copy/%d?suffix=_copy&reset_balance=true", singleID), nil))
	copiedID := copyRes["id"]
	cleanupIDs = append(cleanupIDs, copiedID)

	// 6. 标签批处理（绑定标签 -> 查标签模型 -> 编辑标签 -> 禁用标签 -> 启用标签）
	admin.MustOK(http.MethodPost, "/api/channel/batch/tag", map[string]any{
		"ids": []int{singleID, copiedID},
		"tag": tagName,
	})
	tagModels := mustDecode[string](t, admin.MustOK(http.MethodGet, "/api/channel/tag/models?tag="+tagName, nil))
	if tagModels != singleModelA {
		t.Fatalf("expected tag models %q, got %q", singleModelA, tagModels)
	}
	admin.MustOK(http.MethodPut, "/api/channel/tag", map[string]any{
		"tag":      tagName,
		"priority": 1,
	})
	admin.MustOK(http.MethodPost, "/api/channel/tag/disabled", map[string]any{"tag": tagName})
	admin.MustOK(http.MethodPost, "/api/channel/tag/enabled", map[string]any{"tag": tagName})

	// 7. batch 模式与 multi_to_single 多密钥管理
	batchAdd := mustDecode[struct {
		IDs []int `json:"ids"`
	}](t, admin.MustOK(http.MethodPost, "/api/channel/", map[string]any{
		"mode":                            "batch",
		"batch_add_set_key_prefix_2_name": true,
		"channel": map[string]any{
			"name":   uniqueName("e2e_batch"),
			"type":   1,
			"key":    "sk-batchkey111\nsk-batchkey222",
			"models": uniqueName("e2e-batch-m"),
			"status": 2, // 默认手动禁用，绝不参与线上分发
		},
	}))
	cleanupIDs = append(cleanupIDs, batchAdd.IDs...)

	multiAdd := mustDecode[struct {
		IDs []int `json:"ids"`
	}](t, admin.MustOK(http.MethodPost, "/api/channel/", map[string]any{
		"mode":           "multi_to_single",
		"multi_key_mode": "polling",
		"channel": map[string]any{
			"name":   uniqueName("e2e_multi"),
			"type":   1,
			"key":    "sk-multikey1\nsk-multikey2",
			"models": uniqueName("e2e-multi-m"),
			"status": 2,
		},
	}))
	multiID := multiAdd.IDs[0]
	cleanupIDs = append(cleanupIDs, multiID)

	// 多 key 追加与状态操作
	admin.MustOK(http.MethodPut, "/api/channel/", map[string]any{
		"id":       multiID,
		"key":      "sk-multikey2\nsk-multikey3",
		"key_mode": "append",
	})
	keyStatus := mustDecode[struct {
		Total        int `json:"total"`
		EnabledCount int `json:"enabled_count"`
	}](t, admin.MustOK(http.MethodPost, "/api/channel/multi_key/manage", map[string]any{
		"channel_id": multiID,
		"action":     "get_key_status",
	}))
	if keyStatus.Total != 3 || keyStatus.EnabledCount != 3 {
		t.Fatalf("expected 3 enabled multi-keys, got %+v", keyStatus)
	}
	for _, op := range []map[string]any{
		{"channel_id": multiID, "action": "disable_key", "key_index": 0},
		{"channel_id": multiID, "action": "enable_key", "key_index": 0},
		{"channel_id": multiID, "action": "disable_all_keys"},
		{"channel_id": multiID, "action": "enable_all_keys"},
		{"channel_id": multiID, "action": "delete_key", "key_index": 2},
	} {
		admin.MustOK(http.MethodPost, "/api/channel/multi_key/manage", op)
	}

	// 8. DeepSeek 余额查询、单模型测试、全模型测试与测速记录复测/清理
	bal := mustDecode[map[string]any](t, admin.MustOK(http.MethodGet, fmt.Sprintf("/api/channel/update_balance/%d", singleID), nil))
	if bal["channel_id"] != float64(singleID) {
		t.Fatalf("unexpected balance response: %v", bal)
	}

	testOne := mustDecode[map[string]any](t, admin.MustOK(http.MethodGet, fmt.Sprintf("/api/channel/test/%d?model=%s", singleID, singleModelA), nil))
	if testOne["model"] != singleModelA {
		t.Fatalf("unexpected single channel test response: %v", testOne)
	}

	allModelsTest := mustDecode[struct {
		Passed int `json:"passed"`
	}](t, admin.MustOK(http.MethodGet, fmt.Sprintf("/api/channel/test/%d/all_models", singleID), nil))
	if allModelsTest.Passed != 1 {
		t.Fatalf("expected 1 passed model in all_models test, got %+v", allModelsTest)
	}

	_ = mustDecode[[]string](t, admin.MustOK(http.MethodGet, "/api/channel/test_records/model_tags", nil))
	recordsPage := mustDecode[PageData[struct {
		ID int `json:"id"`
	}]](t, admin.MustOK(http.MethodGet, fmt.Sprintf("/api/channel/test_records?channel_id=%d&p=1&page_size=10", singleID), nil))
	if recordsPage.Total < 1 || len(recordsPage.Items) == 0 {
		t.Fatalf("expected test records for channel %d, got %+v", singleID, recordsPage)
	}
	recID := recordsPage.Items[0].ID

	// 单条复测、按 ID 批量复测、按筛选条件复测
	admin.MustOK(http.MethodPost, fmt.Sprintf("/api/channel/test_records/%d/retest", recID), nil)
	admin.MustOK(http.MethodPost, "/api/channel/test_records/batch_retest", map[string]any{"ids": []int{recID}})
	admin.MustOK(http.MethodPost, "/api/channel/test_records/retest_filtered", map[string]any{"channel_id": singleID})

	// 按临时 channel_id 删除测速记录
	deletedRecords := mustDecode[int64](t, admin.MustOK(http.MethodDelete, fmt.Sprintf("/api/channel/test_records?channel_id=%d", singleID), nil))
	if deletedRecords < 1 {
		t.Fatalf("expected at least 1 deleted test record, got %d", deletedRecords)
	}

	// 9. 单删与批量删除临时渠道
	admin.MustOK(http.MethodDelete, fmt.Sprintf("/api/channel/%d", singleID), nil)
	admin.MustOK(http.MethodPost, "/api/channel/batch", map[string]any{"ids": cleanupIDs[1:]})
	cleanupIDs = nil
}
