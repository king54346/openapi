package token_test

import (
	"fmt"
	"net/http"
	"strconv"
	"testing"
	"time"

	"openapi/common"
	"openapi/controller/common/testhelper"
	"openapi/model"
)

func TestTokenCRUDAndTemplateAPI(t *testing.T) {
	s := testhelper.NewTestServer(t)
	alice := s.CreateUser("alice", common.RoleCommonUser)
	bob := s.CreateUser("bob", common.RoleCommonUser)

	tk := testhelper.Decode[model.Token](t, s.OK(alice, http.MethodPost, "/api/token/", map[string]any{
		"name": "t1", "unlimited_quota": true, "expired_time": -1}))
	if len(tk.Key) != 48 || tk.Status != common.TokenStatusEnabled {
		t.Fatalf("unexpected token: %+v", tk)
	}
	s.Fail(alice, http.MethodPost, "/api/token/", map[string]any{"name": "t3", "expired_time": time.Now().Add(-time.Hour).Unix()})

	if _, err := model.ValidateUserToken(tk.Key); err != nil {
		t.Fatalf("new token should be valid: %v", err)
	}
	s.OK(alice, http.MethodPut, "/api/token/?status_only=true", map[string]any{"id": tk.Id, "status": common.TokenStatusDisabled})
	if _, err := model.ValidateUserToken(tk.Key); err == nil {
		t.Fatal("disabled token should be rejected immediately")
	}
	s.OK(alice, http.MethodPut, "/api/token/", map[string]any{
		"id": tk.Id, "name": "renamed", "unlimited_quota": false, "remain_quota": 100, "expired_time": -1,
		"model_limits_enabled": true, "model_limits": "gpt-4o"})
	s.OK(alice, http.MethodPut, "/api/token/?status_only=true", map[string]any{"id": tk.Id, "status": common.TokenStatusEnabled})
	validated, err := model.ValidateUserToken(tk.Key)
	if err != nil || validated.Name != "renamed" || validated.RemainQuota != 100 || !validated.GetModelLimitsMap()["gpt-4o"] {
		t.Fatalf("updated token: %+v, %v", validated, err)
	}

	s.Fail(bob, http.MethodGet, fmt.Sprintf("/api/token/%d", tk.Id), nil)
	s.Fail(bob, http.MethodDelete, fmt.Sprintf("/api/token/%d", tk.Id), nil)
	if list := testhelper.Decode[testhelper.Page[model.Token]](t, s.OK(bob, http.MethodGet, "/api/token/", nil)); list.Total != 0 {
		t.Fatal("bob should see no tokens")
	}
	if list := testhelper.Decode[testhelper.Page[model.Token]](t, s.OK(alice, http.MethodGet, "/api/token/search?keyword=ren", nil)); list.Total != 1 || len(list.Items) != 1 {
		t.Fatal("search by name should find the token")
	}

	// Token access template CRUD
	tpl := testhelper.Decode[model.TokenAccessTemplate](t, s.OK(alice, http.MethodPost, "/api/token_template/", map[string]any{
		"name": "tpl-1", "model_limits_enabled": true, "model_limits": "gpt-4o",
	}))
	if tpl.Name != "tpl-1" {
		t.Fatalf("unexpected template: %+v", tpl)
	}
	tplList := testhelper.Decode[testhelper.Page[model.TokenAccessTemplate]](t, s.OK(alice, http.MethodGet, "/api/token_template/", nil))
	if tplList.Total != 1 {
		t.Fatalf("expected 1 template, got %d", tplList.Total)
	}
	s.OK(alice, http.MethodDelete, fmt.Sprintf("/api/token_template/%d", tpl.Id), nil)

	s.OK(alice, http.MethodDelete, fmt.Sprintf("/api/token/%d", tk.Id), nil)
	if _, err := model.ValidateUserToken(tk.Key); err == nil {
		t.Fatal("deleted token should be rejected immediately")
	}
}

func TestTokenUsageAndAnalyticsAPI(t *testing.T) {
	s := testhelper.NewTestServer(t)
	alice := s.CreateUser("alice", common.RoleCommonUser)
	bob := s.CreateUser("bob", common.RoleCommonUser)

	token := &model.Token{UserId: alice.ID, Name: "usage", ExpiredTime: -1, UnlimitedQuota: true}
	if err := token.Insert(); err != nil {
		t.Fatal(err)
	}
	now := common.GetTimestamp()
	for i := range 3 {
		log := model.Log{
			UserId: alice.ID, Username: "alice", Type: model.LogTypeConsume, ModelName: "gpt-4o",
			TokenId: token.Id, TokenName: token.Name, PromptTokens: 10, CompletionTokens: 5, Quota: 1,
			CreatedAt: now - int64(i)*86400,
		}
		if err := model.LOG_DB.Create(&log).Error; err != nil {
			t.Fatal(err)
		}
	}

	usage := testhelper.Decode[map[string]any](t, s.OK(testhelper.Actor{ID: alice.ID, AccessToken: token.Key}, http.MethodGet, "/api/usage/token/", nil))
	if usage["total_requests"] != float64(3) || usage["prompt_tokens"] != float64(30) || usage["completion_tokens"] != float64(15) {
		t.Fatalf("token usage: %v", usage)
	}

	tokenID := strconv.Itoa(token.Id)
	analytics := testhelper.Decode[map[string]any](t, s.OK(alice, http.MethodGet, "/api/token/analytics/"+tokenID+"?days=7", nil))
	summary, _ := analytics["summary"].(map[string]any)
	if summary["total_calls"] != float64(3) {
		t.Fatalf("analytics summary: %v", analytics)
	}
	stats, _ := analytics["model_stats"].([]any)
	if len(stats) != 1 {
		t.Fatalf("analytics model stats: %v", stats)
	}

	s.Fail(bob, http.MethodGet, "/api/token/analytics/"+tokenID, nil)
	s.Fail(alice, http.MethodGet, "/api/token/analytics/99999", nil)
	s.Fail(alice, http.MethodGet, "/api/token/analytics/"+tokenID+"?start_time=100&end_time=10", nil)
}
