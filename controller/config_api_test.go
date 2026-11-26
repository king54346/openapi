package controller_test

import (
	"net/http"
	"net/http/httptest"
	"openapi/common"
	"openapi/constant"
	"openapi/controller"
	"openapi/model"
	"openapi/setting"
	"strconv"
	"strings"
	"testing"

	gin "github.com/king54346/gin-tiny"
)

// videoTestPlatform 任务平台按渠道类型字符串存储，这里用 Sora 渠道类型。
var videoTestPlatform = constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeSora))

// newUpstreamServer 启动一个按路径分发的测试上游。
func newUpstreamServer(t *testing.T, handlers map[string]http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if handler, ok := handlers[r.URL.Path]; ok {
			handler(w, r)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// newRequestWithAuth 构造带身份头的请求。
func newRequestWithAuth(method, path string, userID int, tokenKey string) *http.Request {
	req := httptest.NewRequest(method, path, nil)
	if userID > 0 {
		req.Header.Set("Open-Api-User", strconv.Itoa(userID))
	}
	if tokenKey != "" {
		req.Header.Set("Authorization", tokenKey)
	}
	return req
}

// serve 直接在测试引擎上执行一次请求。
func serve(engine *gin.Engine, req *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

// resetGlobalSettings 恢复运行时配置，避免用例之间互相影响。
func resetGlobalSettings(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		setting.UpdateGlobalSettings(func(s *setting.GlobalSettings) {
			s.Notice, s.About, s.HomePageContent, s.UserAgreement, s.PrivacyPolicy, s.DocsLink = "", "", "", "", "", ""
			s.TurnstileCheckEnabled, s.TurnstileSiteKey, s.TurnstileSecretKey = false, "", ""
		})
	})
}

func TestOptionAPI(t *testing.T) {
	s := newTestServer(t)
	resetGlobalSettings(t)
	root := s.createUser("root", common.RoleRootUser)
	admin := s.createUser("admin", common.RoleAdminUser)

	// 只有 root 能读写配置
	s.fail(admin, http.MethodGet, "/api/option/", nil)
	s.fail(admin, http.MethodPut, "/api/option/", map[string]any{"key": "Notice", "value": "x"})
	if options := decode[map[string]string](t, s.ok(root, http.MethodGet, "/api/option/", nil)); len(options) != 0 {
		t.Fatalf("new database should have no options: %v", options)
	}
	s.fail(root, http.MethodPut, "/api/option/", map[string]any{"key": "  ", "value": "x"})

	// 写入后立即生效：公开接口能读到
	s.ok(root, http.MethodPut, "/api/option/", map[string]any{"key": "Notice", "value": "维护公告"})
	if got := decode[string](t, s.ok(actor{}, http.MethodGet, "/api/notice", nil)); got != "维护公告" {
		t.Fatalf("notice = %q", got)
	}
	for path, want := range map[string]string{
		"/api/about":             "关于内容",
		"/api/home_page_content": "首页内容",
		"/api/user-agreement":    "协议内容",
		"/api/privacy-policy":    "隐私内容",
	} {
		key := map[string]string{
			"/api/about":             "About",
			"/api/home_page_content": "HomePageContent",
			"/api/user-agreement":    "UserAgreement",
			"/api/privacy-policy":    "PrivacyPolicy",
		}[path]
		s.ok(root, http.MethodPut, "/api/option/", map[string]any{"key": key, "value": want})
		if got := decode[string](t, s.ok(actor{}, http.MethodGet, path, nil)); got != want {
			t.Fatalf("%s = %q, want %q", path, got, want)
		}
	}

	// 敏感项读取时脱敏，回写占位符不会覆盖真实值
	s.ok(root, http.MethodPut, "/api/option/", map[string]any{"key": "TurnstileSecretKey", "value": "real-secret"})
	options := decode[map[string]string](t, s.ok(root, http.MethodGet, "/api/option/", nil))
	if options["TurnstileSecretKey"] != "******" {
		t.Fatalf("secret should be masked: %v", options["TurnstileSecretKey"])
	}
	s.ok(root, http.MethodPut, "/api/option/", map[string]any{"key": "TurnstileSecretKey", "value": "******"})
	if value, _ := model.GetOptionValue("TurnstileSecretKey"); value != "real-secret" {
		t.Fatalf("masked value must not overwrite the stored secret: %q", value)
	}

	// 旧版控制台设置迁移是幂等的
	s.ok(root, http.MethodPut, "/api/option/", map[string]any{"key": "console_setting.api_info", "value": `[{"id":1}]`})
	migrated := decode[map[string][]string](t, s.ok(root, http.MethodPost, "/api/option/migrate_console_setting", nil))
	if len(migrated["migrated"]) != 1 {
		t.Fatalf("first migration: %v", migrated)
	}
	migrated = decode[map[string][]string](t, s.ok(root, http.MethodPost, "/api/option/migrate_console_setting", nil))
	if len(migrated["migrated"]) != 0 || len(migrated["skipped"]) == 0 {
		t.Fatalf("second migration should be a no-op: %v", migrated)
	}
	if value, _ := model.GetOptionValue("ApiInfo"); value != `[{"id":1}]` {
		t.Fatalf("migrated value: %q", value)
	}
}

func TestSetupAndStatusAPI(t *testing.T) {
	s := newTestServer(t)
	resetGlobalSettings(t)

	setup := decode[map[string]any](t, s.ok(actor{}, http.MethodGet, "/api/setup", nil))
	if setup["initialized"] != false || setup["root_user"] != false {
		t.Fatalf("fresh database should be uninitialized: %v", setup)
	}

	// 校验失败：短密码、用户名不匹配确认、已初始化
	s.fail(actor{}, http.MethodPost, "/api/setup", map[string]any{"username": "root", "password": "short"})
	s.fail(actor{}, http.MethodPost, "/api/setup", map[string]any{"username": "root", "password": "password123", "confirm_password": "password124"})
	s.ok(actor{}, http.MethodPost, "/api/setup", map[string]any{"username": "root", "password": "password123"})
	setup = decode[map[string]any](t, s.ok(actor{}, http.MethodGet, "/api/setup", nil))
	if setup["initialized"] != true {
		t.Fatalf("should be initialized: %v", setup)
	}
	s.fail(actor{}, http.MethodPost, "/api/setup", map[string]any{"username": "root2", "password": "password123"})
	if _, err := model.ValidateUserLogin("root", "password123"); err != nil {
		t.Fatalf("root login should work: %v", err)
	}

	// 状态接口暴露版本与公开配置，但不泄露密钥
	status := decode[map[string]any](t, s.ok(actor{}, http.MethodGet, "/api/status", nil))
	if status["version"] != common.Version || status["setup"] != true {
		t.Fatalf("status: %v", status)
	}
	if _, ok := status["turnstile_site_key"]; !ok {
		t.Fatalf("status should expose turnstile_site_key: %v", status)
	}
}

func TestStatusSelfCheck(t *testing.T) {
	s := newTestServer(t)
	user := s.createUser("alice", common.RoleCommonUser)
	admin := s.createUser("admin", common.RoleAdminUser)

	// 普通用户不能做连通性自检（接口要求管理员）
	s.fail(user, http.MethodGet, "/api/status/test", nil)
	result := decode[map[string]any](t, s.ok(admin, http.MethodGet, "/api/status/test", nil))
	if result["success"] != true {
		t.Fatalf("self check should pass: %v", result)
	}
	items, _ := result["items"].([]any)
	if len(items) == 0 {
		t.Fatalf("self check items missing: %v", result)
	}

	// 未配置 Uptime Kuma 时返回空对象
	if uptime := decode[map[string]any](t, s.ok(actor{}, http.MethodGet, "/api/uptime/status", nil)); len(uptime) != 0 {
		t.Fatalf("uptime status should be empty when disabled: %v", uptime)
	}
}

func TestGroupsAndUserModelsAPI(t *testing.T) {
	s := newTestServer(t)
	resetGlobalSettings(t)
	admin := s.createUser("admin", common.RoleAdminUser)
	alice := s.createUser("alice", common.RoleCommonUser)

	insertTestChannel(t, "ch", "http://127.0.0.1:1", "gpt-4o,gpt-4o-mini", common.ChannelStatusEnabled)
	insertTestChannel(t, "second", "http://127.0.0.1:1", "gpt-4o-premium", common.ChannelStatusEnabled)
	if err := model.InitChannelCache(); err != nil {
		t.Fatal(err)
	}

	// 分组已移除：所有已启用渠道的模型对所有用户可见
	models := decode[[]string](t, s.ok(alice, http.MethodGet, "/api/user/models", nil))
	for _, want := range []string{"gpt-4o", "gpt-4o-mini", "gpt-4o-premium"} {
		if !contains(models, want) {
			t.Fatalf("missing model %s in %v", want, models)
		}
	}

	// 分组相关接口已下线
	s.do(alice, http.MethodGet, "/api/group/", nil)
	s.do(actor{}, http.MethodGet, "/api/user/groups", nil)
	s.do(admin, http.MethodGet, "/api/user/self/groups", nil)
}

// contains 判断字符串切片是否包含目标值。
func contains(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func TestVideoProxyAPI(t *testing.T) {
	newTestServer(t) // 初始化内存数据库
	alice := mustCreateUser(t, "alice", common.RoleCommonUser)
	bob := mustCreateUser(t, "bob", common.RoleCommonUser)

	videoBytes := strings.Repeat("v", 2048)
	upstream := newUpstreamServer(t, map[string]http.HandlerFunc{
		"/video.mp4": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "video/mp4")
			if r.Header.Get("Range") == "bytes=0-9" {
				w.Header().Set("Content-Range", "bytes 0-9/2048")
				w.WriteHeader(http.StatusPartialContent)
				_, _ = w.Write([]byte(videoBytes[:10]))
				return
			}
			_, _ = w.Write([]byte(videoBytes))
		},
	})

	channel := insertTestChannel(t, "ch", upstream.URL, "gpt-4o", common.ChannelStatusEnabled)
	task := &model.Task{
		TaskID: "task_ok", UserId: alice.id, Platform: videoTestPlatform,
		Status: model.TaskStatusSuccess, ChannelId: channel.Id,
		PrivateData: model.TaskPrivateData{ResultURL: upstream.URL + "/video.mp4"},
	}
	if err := task.Insert(); err != nil {
		t.Fatal(err)
	}
	pending := &model.Task{
		TaskID: "task_pending", UserId: alice.id, Platform: videoTestPlatform,
		Status: model.TaskStatusInProgress, ChannelId: channel.Id,
	}
	if err := pending.Insert(); err != nil {
		t.Fatal(err)
	}

	// VideoProxy 挂在 SetVideoRouter 上，这里单独组一个引擎，用中间件注入令牌所属用户
	engine := gin.New()
	engine.GET("/v1/videos/:task_id/content", func(c gin.Context) {
		c.Set("id", alice.id)
		controller.VideoProxy(c)
	})

	// 正常下载
	w := serve(engine, newRequestWithAuth(http.MethodGet, "/v1/videos/task_ok/content", 0, ""))
	if w.Code != http.StatusOK || w.Body.String() != videoBytes {
		t.Fatalf("video proxy: %d, %d bytes", w.Code, w.Body.Len())
	}

	// Range 透传 206 与 Content-Range
	req := newRequestWithAuth(http.MethodGet, "/v1/videos/task_ok/content", 0, "")
	req.Header.Set("Range", "bytes=0-9")
	w = serve(engine, req)
	if w.Code != http.StatusPartialContent || w.Header().Get("Content-Range") == "" {
		t.Fatalf("range request: %d %v", w.Code, w.Header())
	}

	// 未完成 / 不存在 / 他人任务
	aliceEngine := gin.New()
	aliceEngine.GET("/v1/videos/:task_id/content", func(c gin.Context) {
		c.Set("id", alice.id)
		controller.VideoProxy(c)
	})
	bobEngine := gin.New()
	bobEngine.GET("/v1/videos/:task_id/content", func(c gin.Context) {
		c.Set("id", bob.id)
		controller.VideoProxy(c)
	})
	for _, tc := range []struct {
		engine *gin.Engine
		path   string
		want   int
	}{
		{aliceEngine, "/v1/videos/task_pending/content", http.StatusBadRequest},
		{aliceEngine, "/v1/videos/no_such_task/content", http.StatusNotFound},
		{bobEngine, "/v1/videos/task_ok/content", http.StatusNotFound},
	} {
		if w := serve(tc.engine, newRequestWithAuth(http.MethodGet, tc.path, 0, "")); w.Code != tc.want {
			t.Fatalf("%s: got %d, want %d", tc.path, w.Code, tc.want)
		}
	}
}

// mustCreateUser 在已初始化的测试库里建用户并返回身份。
func mustCreateUser(t *testing.T, username string, role int) actor {
	t.Helper()
	u := &model.User{Username: username, Role: role}
	if err := u.Insert("password123"); err != nil {
		t.Fatal(err)
	}
	token, err := model.GenerateUserAccessToken(u.Id)
	if err != nil {
		t.Fatal(err)
	}
	return actor{id: u.Id, accessToken: token}
}

func TestTokenUsageAndAnalyticsAPI(t *testing.T) {
	s := newTestServer(t)
	alice := s.createUser("alice", common.RoleCommonUser)
	bob := s.createUser("bob", common.RoleCommonUser)

	token := &model.Token{UserId: alice.id, Name: "usage", ExpiredTime: -1, UnlimitedQuota: true}
	if err := token.Insert(); err != nil {
		t.Fatal(err)
	}
	now := common.GetTimestamp()
	for i := range 3 {
		log := model.Log{
			UserId: alice.id, Username: "alice", Type: model.LogTypeConsume, ModelName: "gpt-4o",
			TokenId: token.Id, TokenName: token.Name, PromptTokens: 10, CompletionTokens: 5, Quota: 1,
			CreatedAt: now - int64(i)*86400,
		}
		if err := model.LOG_DB.Create(&log).Error; err != nil {
			t.Fatal(err)
		}
	}

	usage := decode[map[string]any](t, s.ok(actor{id: alice.id, accessToken: token.Key}, http.MethodGet, "/api/usage/token/", nil))
	if usage["total_requests"] != float64(3) || usage["prompt_tokens"] != float64(30) || usage["completion_tokens"] != float64(15) {
		t.Fatalf("token usage: %v", usage)
	}

	tokenID := strconv.Itoa(token.Id)
	analytics := decode[map[string]any](t, s.ok(alice, http.MethodGet, "/api/token/analytics/"+tokenID+"?days=7", nil))
	summary, _ := analytics["summary"].(map[string]any)
	if summary["total_calls"] != float64(3) {
		t.Fatalf("analytics summary: %v", analytics)
	}
	stats, _ := analytics["model_stats"].([]any)
	if len(stats) != 1 {
		t.Fatalf("analytics model stats: %v", stats)
	}

	// 只能看自己的令牌
	s.fail(bob, http.MethodGet, "/api/token/analytics/"+tokenID, nil)
	s.fail(alice, http.MethodGet, "/api/token/analytics/99999", nil)
	s.fail(alice, http.MethodGet, "/api/token/analytics/"+tokenID+"?start_time=100&end_time=10", nil)
}
