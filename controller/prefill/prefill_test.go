package prefill_test

import (
	"fmt"
	"net/http"
	"testing"

	"openapi/common"
	"openapi/controller/common/testhelper"
	"openapi/model"
)

func TestPrefillGroupAPI(t *testing.T) {
	s := testhelper.NewTestServer(t)
	admin := s.CreateUser("admin", common.RoleAdminUser)
	user := s.CreateUser("alice", common.RoleCommonUser)

	s.Fail(user, http.MethodGet, "/api/prefill_group/", nil)
	pg := testhelper.Decode[model.PrefillGroup](t, s.OK(admin, http.MethodPost, "/api/prefill_group/", map[string]any{
		"name":  "chat-models",
		"type":  "model",
		"items": []string{"gpt-4o", "claude-sonnet-4-6"},
	}))
	if pg.Name != "chat-models" {
		t.Fatalf("unexpected prefill group: %+v", pg)
	}

	list := testhelper.Decode[testhelper.Page[model.PrefillGroup]](t, s.OK(admin, http.MethodGet, "/api/prefill_group/?type=model", nil))
	if list.Total != 1 || list.Items[0].Id != pg.Id {
		t.Fatalf("prefill group list: %+v", list)
	}

	updated := testhelper.Decode[model.PrefillGroup](t, s.OK(admin, http.MethodPut, "/api/prefill_group/", map[string]any{
		"id": pg.Id, "name": "chat-models-v2", "type": "model", "items": []string{"gpt-4o"},
	}))
	if updated.Name != "chat-models-v2" {
		t.Fatalf("prefill update: %+v", updated)
	}

	s.OK(admin, http.MethodDelete, fmt.Sprintf("/api/prefill_group/%d", pg.Id), nil)
	list = testhelper.Decode[testhelper.Page[model.PrefillGroup]](t, s.OK(admin, http.MethodGet, "/api/prefill_group/", nil))
	if list.Total != 0 {
		t.Fatalf("expected 0 prefill groups after delete, got %d", list.Total)
	}
}
