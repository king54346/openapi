package controller_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"openapi/common"
	"openapi/constant"
	"openapi/model"
	"openapi/router"

	"github.com/glebarez/sqlite"
	gin "github.com/king54346/gin-tiny"
	"github.com/king54346/gin-tiny/middleware/sessions"
	"github.com/king54346/gin-tiny/middleware/sessions/cookie"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type apiResp struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type testServer struct {
	t      *testing.T
	engine *gin.Engine
}

// actor 发请求的身份：access token 或 session cookie，二选一。
type actor struct {
	id          int
	accessToken string
	cookie      string
}

func newTestServer(t *testing.T) *testServer {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	if err := model.InitDB(db, nil); err != nil {
		t.Fatal(err)
	}
	if err := model.InitChannelCache(); err != nil {
		t.Fatal(err)
	}
	common.SyncFrequency = 60
	model.ClearAuthCache()
	t.Cleanup(func() { _ = sqlDB.Close() })

	engine := gin.New()
	store := cookie.NewStore([]byte("test-secret"))
	engine.Use(sessions.Sessions("session", store))
	router.SetApiRouter(engine)
	return &testServer{t: t, engine: engine}
}

// createUser 直接在库里建用户并生成 access token。
func (s *testServer) createUser(username string, role int) actor {
	s.t.Helper()
	u := &model.User{Username: username, Role: role}
	if err := u.Insert("password123"); err != nil {
		s.t.Fatalf("create user %s: %v", username, err)
	}
	token, err := model.GenerateUserAccessToken(u.Id)
	if err != nil {
		s.t.Fatal(err)
	}
	return actor{id: u.Id, accessToken: token}
}

func (s *testServer) do(a actor, method, path string, body any) (*httptest.ResponseRecorder, apiResp) {
	s.t.Helper()
	var reader *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	if a.id > 0 {
		req.Header.Set("Open-Api-User", strconv.Itoa(a.id))
	}
	if a.accessToken != "" {
		req.Header.Set("Authorization", a.accessToken)
	}
	if a.cookie != "" {
		req.Header.Set("Cookie", a.cookie)
	}
	w := httptest.NewRecorder()
	s.engine.ServeHTTP(w, req)
	var resp apiResp
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w, resp
}

// ok 发请求并要求 success=true，返回 data。
func (s *testServer) ok(a actor, method, path string, body any) json.RawMessage {
	s.t.Helper()
	w, resp := s.do(a, method, path, body)
	if !resp.Success {
		s.t.Fatalf("%s %s: expected success, got %d %s", method, path, w.Code, w.Body.String())
	}
	return resp.Data
}

// fail 发请求并要求失败，返回 message。
func (s *testServer) fail(a actor, method, path string, body any) string {
	s.t.Helper()
	w, resp := s.do(a, method, path, body)
	if resp.Success {
		s.t.Fatalf("%s %s: expected failure, got %d %s", method, path, w.Code, w.Body.String())
	}
	return resp.Message
}

type page[T any] struct {
	Items []T   `json:"items"`
	Total int64 `json:"total"`
}

func decode[T any](t *testing.T, raw json.RawMessage) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	return v
}

func TestChannelAdminAPI(t *testing.T) {
	s := newTestServer(t)
	admin := s.createUser("admin", common.RoleAdminUser)
	user := s.createUser("alice", common.RoleCommonUser)

	newChannel := map[string]any{
		"mode": "single",
		"channel": map[string]any{
			"name": "openai-1", "type": constant.ChannelTypeOpenAI, "key": "sk-secret",
			"models": "gpt-4o,gpt-4o-mini", "group": "default,vip",
		},
	}
	// 普通用户无权管理渠道
	s.fail(user, http.MethodPost, "/api/channel/", newChannel)
	s.ok(admin, http.MethodPost, "/api/channel/", newChannel)

	// 不支持的渠道类型、缺少 key 都会被拒绝
	s.fail(admin, http.MethodPost, "/api/channel/", map[string]any{"channel": map[string]any{
		"name": "gemini", "type": constant.ChannelTypeGemini, "key": "k", "models": "gemini-pro"}})
	s.fail(admin, http.MethodPost, "/api/channel/", map[string]any{"channel": map[string]any{
		"name": "nokey", "type": constant.ChannelTypeOpenAI, "models": "gpt-4o"}})

	// batch 模式按行拆成多个渠道
	s.ok(admin, http.MethodPost, "/api/channel/", map[string]any{"mode": "batch", "channel": map[string]any{
		"name": "ds", "type": constant.ChannelTypeDeepSeek, "key": "k1\nk2\n\n", "models": "deepseek-chat"}})

	list := decode[page[model.Channel]](t, s.ok(admin, http.MethodGet, "/api/channel/?p=1&page_size=10&id_sort=true", nil))
	if list.Total != 3 || len(list.Items) != 3 {
		t.Fatalf("expected 3 channels, got %d", list.Total)
	}
	for _, ch := range list.Items {
		if ch.Key != "" {
			t.Fatal("channel list must not expose keys")
		}
	}
	// 新增后立即进入渠道缓存
	if ch, _ := model.GetRandomSatisfiedChannel("vip", "gpt-4o-mini", 0); ch == nil {
		t.Fatal("new channel should be in cache immediately")
	}
	openaiID := list.Items[2].Id

	got := decode[model.Channel](t, s.ok(admin, http.MethodGet, fmt.Sprintf("/api/channel/%d", openaiID), nil))
	if got.Name != "openai-1" || got.Key != "" {
		t.Fatalf("unexpected channel: %+v", got)
	}

	// 修改：去掉 vip 分组，缓存随之更新
	s.ok(admin, http.MethodPut, "/api/channel/", map[string]any{"id": openaiID, "group": "default", "name": "openai-renamed"})
	if ch, _ := model.GetRandomSatisfiedChannel("vip", "gpt-4o-mini", 0); ch != nil {
		t.Fatal("channel removed from vip should leave cache")
	}
	stored, _ := model.GetChannelById(openaiID, true)
	if stored.Name != "openai-renamed" || stored.Key != "sk-secret" {
		t.Fatalf("update should keep key when omitted: %+v", stored)
	}

	// 只有 root 能看 key
	s.fail(admin, http.MethodPost, fmt.Sprintf("/api/channel/%d/key", openaiID), nil)

	// 复制、标签、删除
	copied := decode[map[string]int](t, s.ok(admin, http.MethodPost, fmt.Sprintf("/api/channel/copy/%d", openaiID), nil))
	s.ok(admin, http.MethodPost, "/api/channel/batch/tag", map[string]any{"ids": []int{openaiID, copied["id"]}, "tag": "t1"})
	s.ok(admin, http.MethodPost, "/api/channel/tag/disabled", map[string]any{"tag": "t1"})
	if ch, _ := model.GetRandomSatisfiedChannel("default", "gpt-4o", 0); ch != nil {
		t.Fatal("disabled tag channels should leave cache")
	}
	s.ok(admin, http.MethodPost, "/api/channel/tag/enabled", map[string]any{"tag": "t1"})
	if ch, _ := model.GetRandomSatisfiedChannel("default", "gpt-4o", 0); ch == nil {
		t.Fatal("re-enabled tag channels should be back in cache")
	}
	s.ok(admin, http.MethodDelete, fmt.Sprintf("/api/channel/%d", copied["id"]), nil)
	s.ok(admin, http.MethodPost, "/api/channel/batch", map[string]any{"ids": []int{list.Items[0].Id}})
	list = decode[page[model.Channel]](t, s.ok(admin, http.MethodGet, "/api/channel/", nil))
	if list.Total != 2 {
		t.Fatalf("expected 2 channels after deletes, got %d", list.Total)
	}
	search := decode[page[model.Channel]](t, s.ok(admin, http.MethodGet, "/api/channel/search?keyword=renamed", nil))
	if len(search.Items) != 1 || search.Items[0].Key != "" {
		t.Fatalf("search result: %+v", search.Items)
	}
}

func TestUserAdminAPI(t *testing.T) {
	s := newTestServer(t)
	root := s.createUser("root", common.RoleRootUser)
	admin := s.createUser("admin", common.RoleAdminUser)

	created := decode[model.User](t, s.ok(admin, http.MethodPost, "/api/user/", map[string]any{
		"username": "bob", "password": "password123", "display_name": "Bob"}))
	if created.Role != common.RoleCommonUser || created.Group != "default" {
		t.Fatalf("unexpected created user: %+v", created)
	}
	s.fail(admin, http.MethodPost, "/api/user/", map[string]any{"username": "bob", "password": "password123"}) // 重名
	s.fail(admin, http.MethodPost, "/api/user/", map[string]any{"username": "c", "password": "short"})         // 密码太短
	s.fail(admin, http.MethodPost, "/api/user/", map[string]any{"username": "d", "password": "password123", "role": common.RoleAdminUser})

	// 管理员不能管理 root 或同级管理员
	s.fail(admin, http.MethodGet, fmt.Sprintf("/api/user/%d", root.id), nil)
	s.fail(admin, http.MethodPost, "/api/user/manage", map[string]any{"id": root.id, "action": "disable"})
	// 只有 root 能提升管理员
	s.fail(admin, http.MethodPost, "/api/user/manage", map[string]any{"id": created.Id, "action": "promote"})
	s.ok(root, http.MethodPost, "/api/user/manage", map[string]any{"id": created.Id, "action": "promote"})
	s.ok(root, http.MethodPost, "/api/user/manage", map[string]any{"id": created.Id, "action": "demote"})

	// 修改资料并重置密码，新密码可登录
	s.ok(admin, http.MethodPut, "/api/user/", map[string]any{"id": created.Id, "display_name": "Bobby", "group": "vip", "password": "newpassword1"})
	if _, err := model.ValidateUserLogin("bob", "newpassword1"); err != nil {
		t.Fatalf("login with new password: %v", err)
	}

	list := decode[page[model.User]](t, s.ok(admin, http.MethodGet, "/api/user/?p=1&page_size=2", nil))
	if list.Total != 3 || len(list.Items) != 2 {
		t.Fatalf("expected total 3 / page 2, got %d / %d", list.Total, len(list.Items))
	}
	search := decode[page[model.User]](t, s.ok(admin, http.MethodGet, "/api/user/search?keyword=bob&group=vip", nil))
	if search.Total != 1 || search.Items[0].DisplayName != "Bobby" {
		t.Fatalf("search: %+v", search)
	}
	s.ok(admin, http.MethodPost, "/api/user/batch/group", map[string]any{"ids": []int{created.Id}, "group": "svip"})
	if u, _ := model.GetUserCache(created.Id); u.Group != "svip" {
		t.Fatalf("group change should invalidate cache, got %s", u.Group)
	}

	s.ok(admin, http.MethodDelete, fmt.Sprintf("/api/user/%d", created.Id), nil)
	if _, err := model.GetUserCache(created.Id); err == nil {
		t.Fatal("deleted user should not be found")
	}
}

func TestLoginSessionAndTokenAPI(t *testing.T) {
	s := newTestServer(t)
	admin := s.createUser("admin", common.RoleAdminUser)
	alice := s.createUser("alice", common.RoleCommonUser)
	bob := s.createUser("bob", common.RoleCommonUser)

	// 错误密码 / 正确密码登录
	s.fail(actor{}, http.MethodPost, "/api/user/login", map[string]any{"username": "alice", "password": "wrong-pass"})
	w, resp := s.do(actor{}, http.MethodPost, "/api/user/login", map[string]any{"username": "alice", "password": "password123"})
	if !resp.Success {
		t.Fatalf("login failed: %s", w.Body.String())
	}
	cookieHeader := w.Header().Get("Set-Cookie")
	if cookieHeader == "" {
		t.Fatal("login should set session cookie")
	}
	aliceSession := actor{id: alice.id, cookie: cookieHeader}
	self := decode[model.User](t, s.ok(aliceSession, http.MethodGet, "/api/user/self", nil))
	if self.Username != "alice" {
		t.Fatalf("self: %+v", self)
	}

	// 令牌增删改查（session 身份）
	tk := decode[model.Token](t, s.ok(aliceSession, http.MethodPost, "/api/token/", map[string]any{
		"name": "t1", "unlimited_quota": true, "expired_time": -1}))
	if len(tk.Key) != 48 || tk.Status != common.TokenStatusEnabled {
		t.Fatalf("unexpected token: %+v", tk)
	}
	s.fail(aliceSession, http.MethodPost, "/api/token/", map[string]any{"name": "t2", "group": "vip"}) // 无权使用 vip 分组
	s.fail(aliceSession, http.MethodPost, "/api/token/", map[string]any{"name": "t3", "expired_time": time.Now().Add(-time.Hour).Unix()})

	if _, err := model.ValidateUserToken(tk.Key); err != nil {
		t.Fatalf("new token should be valid: %v", err)
	}
	// 禁用后缓存立即失效
	s.ok(aliceSession, http.MethodPut, "/api/token/?status_only=true", map[string]any{"id": tk.Id, "status": common.TokenStatusDisabled})
	if _, err := model.ValidateUserToken(tk.Key); err == nil {
		t.Fatal("disabled token should be rejected immediately")
	}
	s.ok(aliceSession, http.MethodPut, "/api/token/", map[string]any{
		"id": tk.Id, "name": "renamed", "unlimited_quota": false, "remain_quota": 100, "expired_time": -1,
		"model_limits_enabled": true, "model_limits": "gpt-4o"})
	s.ok(aliceSession, http.MethodPut, "/api/token/?status_only=true", map[string]any{"id": tk.Id, "status": common.TokenStatusEnabled})
	validated, err := model.ValidateUserToken(tk.Key)
	if err != nil || validated.Name != "renamed" || validated.RemainQuota != 100 || !validated.GetModelLimitsMap()["gpt-4o"] {
		t.Fatalf("updated token: %+v, %v", validated, err)
	}

	// 其他用户看不到、改不了、删不了
	s.fail(bob, http.MethodGet, fmt.Sprintf("/api/token/%d", tk.Id), nil)
	s.fail(bob, http.MethodDelete, fmt.Sprintf("/api/token/%d", tk.Id), nil)
	if list := decode[page[model.Token]](t, s.ok(bob, http.MethodGet, "/api/token/", nil)); list.Total != 0 {
		t.Fatal("bob should see no tokens")
	}
	if list := decode[page[model.Token]](t, s.ok(aliceSession, http.MethodGet, "/api/token/search?keyword=ren", nil)); len(list.Items) != 1 {
		t.Fatal("search by name should find the token")
	}

	// 管理员禁用 alice 后，她已登录的 session 立即失效
	s.ok(admin, http.MethodPost, "/api/user/manage", map[string]any{"id": alice.id, "action": "disable"})
	if msg := s.fail(aliceSession, http.MethodGet, "/api/user/self", nil); msg != "user is disabled" {
		t.Fatalf("disabled user session: %q", msg)
	}
	s.ok(admin, http.MethodPost, "/api/user/manage", map[string]any{"id": alice.id, "action": "enable"})

	s.ok(aliceSession, http.MethodDelete, fmt.Sprintf("/api/token/%d", tk.Id), nil)
	if _, err := model.ValidateUserToken(tk.Key); err == nil {
		t.Fatal("deleted token should be rejected immediately")
	}

	// 登出后 session 失效
	w, _ = s.do(aliceSession, http.MethodGet, "/api/user/logout", nil)
	loggedOut := actor{id: alice.id, cookie: w.Header().Get("Set-Cookie")}
	s.fail(loggedOut, http.MethodGet, "/api/user/self", nil)
}
