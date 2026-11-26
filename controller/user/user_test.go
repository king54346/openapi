package user_test

import (
	"fmt"
	"net/http"
	"slices"
	"testing"

	"openapi/common"
	"openapi/controller/common/testhelper"
	"openapi/model"
)

func TestUserAdminAPI(t *testing.T) {
	s := testhelper.NewTestServer(t)
	root := s.CreateUser("root", common.RoleRootUser)
	admin := s.CreateUser("admin", common.RoleAdminUser)

	created := testhelper.Decode[model.User](t, s.OK(admin, http.MethodPost, "/api/user/", map[string]any{
		"username": "bob", "password": "password123", "display_name": "Bob"}))
	if created.Role != common.RoleCommonUser {
		t.Fatalf("unexpected created user: %+v", created)
	}
	s.Fail(admin, http.MethodPost, "/api/user/", map[string]any{"username": "bob", "password": "password123"})
	s.Fail(admin, http.MethodPost, "/api/user/", map[string]any{"username": "c", "password": "short"})
	s.Fail(admin, http.MethodPost, "/api/user/", map[string]any{"username": "d", "password": "password123", "role": common.RoleAdminUser})

	s.Fail(admin, http.MethodGet, fmt.Sprintf("/api/user/%d", root.ID), nil)
	s.Fail(admin, http.MethodPost, "/api/user/manage", map[string]any{"id": root.ID, "action": "disable"})
	s.Fail(admin, http.MethodPost, "/api/user/manage", map[string]any{"id": created.Id, "action": "promote"})
	s.OK(root, http.MethodPost, "/api/user/manage", map[string]any{"id": created.Id, "action": "promote"})
	s.OK(root, http.MethodPost, "/api/user/manage", map[string]any{"id": created.Id, "action": "demote"})

	s.OK(admin, http.MethodPut, "/api/user/", map[string]any{"id": created.Id, "display_name": "Bobby", "password": "newpassword1"})
	if _, err := model.ValidateUserLogin("bob", "newpassword1"); err != nil {
		t.Fatalf("login with new password: %v", err)
	}

	list := testhelper.Decode[testhelper.Page[model.User]](t, s.OK(admin, http.MethodGet, "/api/user/?p=1&page_size=2", nil))
	if list.Total != 3 || len(list.Items) != 2 {
		t.Fatalf("expected total 3 / page 2, got %d / %d", list.Total, len(list.Items))
	}
	search := testhelper.Decode[testhelper.Page[model.User]](t, s.OK(admin, http.MethodGet, "/api/user/search?keyword=bob", nil))
	if search.Total != 1 || search.Items[0].DisplayName != "Bobby" {
		t.Fatalf("search: %+v", search)
	}

	s.OK(admin, http.MethodDelete, fmt.Sprintf("/api/user/%d", created.Id), nil)
	if _, err := model.GetUserCache(created.Id); err == nil {
		t.Fatal("deleted user should not be found")
	}
}

func TestLoginSessionAndUserModelsAPI(t *testing.T) {
	s := testhelper.NewTestServer(t)
	testhelper.ResetGlobalSettings(t)
	admin := s.CreateUser("admin", common.RoleAdminUser)
	alice := s.CreateUser("alice", common.RoleCommonUser)

	testhelper.InsertTestChannel(t, "ch", "http://127.0.0.1:1", "gpt-4o,gpt-4o-mini", common.ChannelStatusEnabled)
	testhelper.InsertTestChannel(t, "second", "http://127.0.0.1:1", "gpt-4o-premium", common.ChannelStatusEnabled)
	if err := model.InitChannelCache(); err != nil {
		t.Fatal(err)
	}

	models := testhelper.Decode[[]string](t, s.OK(alice, http.MethodGet, "/api/user/models", nil))
	for _, want := range []string{"gpt-4o", "gpt-4o-mini", "gpt-4o-premium"} {
		if !slices.Contains(models, want) {
			t.Fatalf("missing model %s in %v", want, models)
		}
	}

	// 错误密码 / 正确密码登录
	s.Fail(testhelper.Actor{}, http.MethodPost, "/api/user/login", map[string]any{"username": "alice", "password": "wrong-pass"})
	w, resp := s.Do(testhelper.Actor{}, http.MethodPost, "/api/user/login", map[string]any{"username": "alice", "password": "password123"})
	if !resp.Success {
		t.Fatalf("login failed: %s", w.Body.String())
	}
	cookieHeader := w.Header().Get("Set-Cookie")
	if cookieHeader == "" {
		t.Fatal("login should set session cookie")
	}
	aliceSession := testhelper.Actor{ID: alice.ID, Cookie: cookieHeader}
	self := testhelper.Decode[model.User](t, s.OK(aliceSession, http.MethodGet, "/api/user/self", nil))
	if self.Username != "alice" {
		t.Fatalf("self: %+v", self)
	}

	// 禁用用户后已登录 session 立即失效
	s.OK(admin, http.MethodPost, "/api/user/manage", map[string]any{"id": alice.ID, "action": "disable"})
	if msg := s.Fail(aliceSession, http.MethodGet, "/api/user/self", nil); msg != "user is disabled" {
		t.Fatalf("disabled user session: %q", msg)
	}
	s.OK(admin, http.MethodPost, "/api/user/manage", map[string]any{"id": alice.ID, "action": "enable"})

	// 登出后 session 失效
	w, _ = s.Do(aliceSession, http.MethodGet, "/api/user/logout", nil)
	loggedOut := testhelper.Actor{ID: alice.ID, Cookie: w.Header().Get("Set-Cookie")}
	s.Fail(loggedOut, http.MethodGet, "/api/user/self", nil)
}
