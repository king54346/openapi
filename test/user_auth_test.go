package e2e_test

import (
	"fmt"
	"net/http"
	"strconv"
	"testing"
)

// TestUserAuthAndAdminManagementAPIs 覆盖用户登录、个人信息、AccessToken 及管理员用户 CRUD 接口：
//   - POST   /api/user/login
//   - GET    /api/user/self
//   - GET    /api/user/models
//   - GET    /api/user/token（仅在临时测试用户上生成，不影响线上管理员 token）
//   - GET    /api/user/logout
//   - GET    /api/user/
//   - GET    /api/user/search
//   - GET    /api/user/:id
//   - POST   /api/user/
//   - PUT    /api/user/
//   - POST   /api/user/manage（disable / enable / promote / demote）
//   - DELETE /api/user/:id
func TestUserAuthAndAdminManagementAPIs(t *testing.T) {
	admin := newLoggedInAdminClient(t)

	// 1. 当前管理员个人信息与可用模型列表
	self := mustDecode[map[string]any](t, admin.MustOK(http.MethodGet, "/api/user/self", nil))
	if self["username"] != adminUsername() {
		t.Fatalf("unexpected self username: %v", self)
	}
	_ = mustDecode[[]string](t, admin.MustOK(http.MethodGet, "/api/user/models", nil))

	// 2. 创建临时测试用户，注册自动清理
	username := uniqueName("e2e_user")
	created := mustDecode[struct {
		ID       int    `json:"id"`
		Username string `json:"username"`
		Role     int    `json:"role"`
		Status   int    `json:"status"`
	}](t, admin.MustOK(http.MethodPost, "/api/user/", map[string]any{
		"username":     username,
		"password":     "Pass123456!",
		"display_name": "E2E 临时测试用户",
	}))
	if created.ID <= 0 {
		t.Fatalf("invalid created user id: %+v", created)
	}
	t.Cleanup(func() {
		_, _, _ = admin.DoAPI(http.MethodDelete, fmt.Sprintf("/api/user/%d", created.ID), nil, nil)
	})

	// 3. 列表、搜索、详情与更新
	userPage := mustDecode[PageData[map[string]any]](t, admin.MustOK(http.MethodGet, "/api/user/?p=1&page_size=10", nil))
	if userPage.Total < 2 {
		t.Fatalf("expected at least 2 users, got %d", userPage.Total)
	}
	searchPage := mustDecode[PageData[map[string]any]](t, admin.MustOK(http.MethodGet, "/api/user/search?keyword="+username, nil))
	if searchPage.Total != 1 {
		t.Fatalf("expected search to match 1 user, got %d", searchPage.Total)
	}
	detail := mustDecode[map[string]any](t, admin.MustOK(http.MethodGet, fmt.Sprintf("/api/user/%d", created.ID), nil))
	if detail["username"] != username {
		t.Fatalf("unexpected user detail: %v", detail)
	}

	admin.MustOK(http.MethodPut, "/api/user/", map[string]any{
		"id":           created.ID,
		"display_name": "E2E 已更新用户",
		"password":     "NewPass123456!",
		"remark":       "e2e remark",
	})

	// 4. 用户状态与角色管理（promote -> demote -> disable -> enable）
	for _, action := range []string{"promote", "demote", "disable", "enable"} {
		admin.MustOK(http.MethodPost, "/api/user/manage", map[string]any{
			"id":     created.ID,
			"action": action,
		})
	}

	// 5. 使用临时用户测试错误密码拦截、正常登录、Access Token 鉴权与登出
	tempClient := newLiveClient(t)
	tempClient.MustFail(http.MethodPost, "/api/user/login", map[string]string{
		"username": username,
		"password": "wrong-password",
	})
	tempClient.Login(username, "NewPass123456!")

	accessToken := mustDecode[string](t, tempClient.MustOK(http.MethodGet, "/api/user/token", nil))
	if len(accessToken) == 0 {
		t.Fatal("expected non-empty access token")
	}

	// 无 Cookie 客户端仅凭 Open-Api-User + Authorization(AccessToken) 访问 /api/user/self
	tokenOnlyClient := newLiveClient(t)
	code, tokenResp, raw := tokenOnlyClient.DoAPI(http.MethodGet, "/api/user/self", nil, map[string]string{
		"Open-Api-User": strconv.Itoa(created.ID),
		"Authorization": "Bearer " + accessToken,
	})
	if code != http.StatusOK || !tokenResp.Success {
		t.Fatalf("access token auth failed: HTTP %d %s", code, string(raw))
	}

	// 登出后原 Session 立即失效
	tempClient.MustOK(http.MethodGet, "/api/user/logout", nil)
	tempClient.MustFail(http.MethodGet, "/api/user/self", nil)

	// 6. 删除临时用户并验证不可再查到
	admin.MustOK(http.MethodDelete, fmt.Sprintf("/api/user/%d", created.ID), nil)
	admin.MustFail(http.MethodGet, fmt.Sprintf("/api/user/%d", created.ID), nil)
}
