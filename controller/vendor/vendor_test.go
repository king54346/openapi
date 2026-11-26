package vendor_test

import (
	"fmt"
	"net/http"
	"testing"

	"openapi/common"
	"openapi/controller/common/testhelper"
	"openapi/model"
)

func TestVendorAPI(t *testing.T) {
	s := testhelper.NewTestServer(t)
	admin := s.CreateUser("admin", common.RoleAdminUser)
	user := s.CreateUser("alice", common.RoleCommonUser)

	s.Fail(user, http.MethodGet, "/api/vendors/", nil)
	v := testhelper.Decode[model.Vendor](t, s.OK(admin, http.MethodPost, "/api/vendors/", map[string]any{
		"name": "OpenAI", "description": "AI research lab", "icon": "openai",
	}))
	if v.Name != "OpenAI" || v.Status != 1 {
		t.Fatalf("unexpected vendor: %+v", v)
	}
	s.Fail(admin, http.MethodPost, "/api/vendors/", map[string]any{"name": "OpenAI"})

	search := testhelper.Decode[testhelper.Page[model.Vendor]](t, s.OK(admin, http.MethodGet, "/api/vendors/search?keyword=Open", nil))
	if search.Total != 1 || search.Items[0].Id != v.Id {
		t.Fatalf("vendor search: %+v", search)
	}

	updated := testhelper.Decode[model.Vendor](t, s.OK(admin, http.MethodPut, "/api/vendors/", map[string]any{
		"id": v.Id, "description": "Updated lab",
	}))
	if updated.Description != "Updated lab" {
		t.Fatalf("vendor update: %+v", updated)
	}

	s.OK(admin, http.MethodDelete, fmt.Sprintf("/api/vendors/%d", v.Id), nil)
	s.Fail(admin, http.MethodGet, fmt.Sprintf("/api/vendors/%d", v.Id), nil)
}
