package modelmeta_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"testing"

	"openapi/common"
	"openapi/constant"
	"openapi/controller/common/testhelper"
	metactrl "openapi/controller/modelmeta"
	"openapi/model"
	relaysvc "openapi/service/relay"

	gin "github.com/king54346/gin-tiny"
)

func callAdmin[T any](t *testing.T, handler gin.HandlerFunc) T {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	handler(c)
	var resp struct {
		Success bool `json:"success"`
		Data    T    `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil || !resp.Success {
		t.Fatalf("response: %s (%v)", w.Body.String(), err)
	}
	return resp.Data
}

func TestChannelAndDashboardListModels(t *testing.T) {
	models := callAdmin[[]relaysvc.OpenAIModel](t, metactrl.ChannelListModels)
	owners := make(map[string]string, len(models))
	for _, m := range models {
		if _, dup := owners[m.Id]; dup {
			t.Fatalf("duplicate model %s", m.Id)
		}
		owners[m.Id] = m.OwnedBy
	}
	for name, owner := range map[string]string{
		"gpt-4o":           "openai",
		"deepseek-chat":    "deepseek",
		"gemini-2.5-flash": "Google Gemini",
	} {
		if owners[name] != owner {
			t.Errorf("%s: owned_by=%q, want %q", name, owners[name], owner)
		}
	}
	if _, ok := owners["sora-2"]; !ok {
		t.Error("video task models should be included")
	}

	byType := callAdmin[map[string][]string](t, metactrl.DashboardListModels)
	if !slices.Contains(byType[strconv.Itoa(constant.ChannelTypeDeepSeek)], "deepseek-chat") {
		t.Errorf("deepseek models: %v", byType[strconv.Itoa(constant.ChannelTypeDeepSeek)])
	}
	if !slices.Contains(byType[strconv.Itoa(constant.ChannelTypeGemini)], "gemini-2.5-flash") {
		t.Errorf("gemini models: %v", byType[strconv.Itoa(constant.ChannelTypeGemini)])
	}
	if !slices.Contains(byType[strconv.Itoa(constant.ChannelTypeSora)], "sora-2") {
		t.Errorf("sora models: %v", byType[strconv.Itoa(constant.ChannelTypeSora)])
	}
	if _, ok := byType[strconv.Itoa(constant.ChannelTypeAzure)]; ok {
		t.Error("unsupported channel type Azure must not be listed")
	}
}

func TestEnabledListModelsAndMetaCRUD(t *testing.T) {
	s := testhelper.NewTestServer(t)
	admin := s.CreateUser("admin", common.RoleAdminUser)
	testhelper.InsertTestChannel(t, "ch", "http://127.0.0.1:1", "gpt-4o,gpt-4o-mini", common.ChannelStatusEnabled)
	testhelper.InsertTestChannel(t, "disabled", "http://127.0.0.1:1", "secret-model", common.ChannelStatusManuallyDisabled)
	if err := model.InitChannelCache(); err != nil {
		t.Fatal(err)
	}

	if got := callAdmin[[]string](t, metactrl.EnabledListModels); !slices.Equal(got, []string{"gpt-4o", "gpt-4o-mini"}) {
		t.Fatalf("enabled models = %v", got)
	}

	// Missing & SyncUpstream
	missingResp := testhelper.Decode[struct {
		Models []string `json:"models"`
		Total  int      `json:"total"`
	}](t, s.OK(admin, http.MethodGet, "/api/models/missing", nil))
	if missingResp.Total != 2 {
		t.Fatalf("expected 2 missing models, got %+v", missingResp)
	}

	exact := testhelper.Decode[model.Model](t, s.OK(admin, http.MethodPost, "/api/models/", map[string]any{
		"model_name": "gpt-4o", "tags": "对话,工具", "vendor_id": 1,
	}))
	s.Fail(admin, http.MethodPost, "/api/models/", map[string]any{"model_name": "gpt-4o"})
	s.Fail(admin, http.MethodPost, "/api/models/", map[string]any{"model_name": " "})

	prefix := testhelper.Decode[model.Model](t, s.OK(admin, http.MethodPost, "/api/models/", map[string]any{
		"model_name": "gpt-", "name_rule": model.NameRulePrefix, "tags": "对话",
	}))

	got := testhelper.Decode[model.Model](t, s.OK(admin, http.MethodGet, fmt.Sprintf("/api/models/%d", exact.Id), nil))
	if len(got.BoundChannels) != 1 || got.BoundChannels[0].Name != "ch" {
		t.Fatalf("exact model enrichment: %+v", got)
	}
	got = testhelper.Decode[model.Model](t, s.OK(admin, http.MethodGet, fmt.Sprintf("/api/models/%d", prefix.Id), nil))
	if got.MatchedCount != 2 || got.MatchedModels[0] != "gpt-4o" || got.MatchedModels[1] != "gpt-4o-mini" {
		t.Fatalf("prefix model enrichment: %+v", got)
	}

	type metaList struct {
		Items        []model.Model    `json:"items"`
		Total        int64            `json:"total"`
		VendorCounts map[string]int64 `json:"vendor_counts"`
	}
	list := testhelper.Decode[metaList](t, s.OK(admin, http.MethodGet, "/api/models/?p=1&page_size=1", nil))
	if list.Total != 2 || len(list.Items) != 1 || list.VendorCounts["1"] != 1 || list.VendorCounts["0"] != 1 {
		t.Fatalf("list: %+v", list)
	}
	if tags := testhelper.Decode[[]string](t, s.OK(admin, http.MethodGet, "/api/models/tags", nil)); len(tags) != 2 || tags[0] != "对话" {
		t.Fatalf("tags: %v", tags)
	}
	if r := testhelper.Decode[testhelper.Page[model.Model]](t, s.OK(admin, http.MethodGet, "/api/models/search?tags=对话,工具", nil)); r.Total != 1 || r.Items[0].Id != exact.Id {
		t.Fatalf("search by tags: %+v", r)
	}

	s.Fail(admin, http.MethodPut, "/api/models/", map[string]any{"id": prefix.Id, "model_name": "gpt-4o"})
	updated := testhelper.Decode[model.Model](t, s.OK(admin, http.MethodPut, "/api/models/", map[string]any{
		"id": exact.Id, "model_name": "gpt-4o", "description": "desc", "tags": "对话",
	}))
	if updated.Description != "desc" || updated.Tags != "对话" || updated.CreatedTime != exact.CreatedTime {
		t.Fatalf("update: %+v", updated)
	}
	updated = testhelper.Decode[model.Model](t, s.OK(admin, http.MethodPut, "/api/models/?status_only=true", map[string]any{"id": exact.Id, "status": 0}))
	if updated.Status != 0 || updated.Description != "desc" {
		t.Fatalf("status only: %+v", updated)
	}

	s.OK(admin, http.MethodDelete, fmt.Sprintf("/api/models/%d", exact.Id), nil)
	s.Fail(admin, http.MethodGet, fmt.Sprintf("/api/models/%d", exact.Id), nil)
	s.OK(admin, http.MethodPost, "/api/models/", map[string]any{"model_name": "gpt-4o"})
}
