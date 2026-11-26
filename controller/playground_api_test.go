package controller_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"openapi/common"
	"openapi/controller"
	"openapi/middleware"
	"openapi/model"
	"strconv"
	"strings"
	"testing"

	gin "github.com/king54346/gin-tiny"
	"github.com/king54346/gin-tiny/middleware/sessions"
	"github.com/king54346/gin-tiny/middleware/sessions/cookie"
)

// jsonBody 把请求体序列化为 JSON 字符串。
func jsonBody(t *testing.T, body any) string {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	return string(data)
}

// TestPlaygroundAPI 体验中心：用登录用户身份直接转发，不需要先创建 API 令牌。
// /pg 路由挂在 SetRelayRouter 上，这里按同样的中间件顺序单独组一个引擎。
func TestPlaygroundAPI(t *testing.T) {
	newTestServer(t) // 初始化内存数据库
	resetGlobalSettings(t)
	alice := mustCreateUser(t, "alice", common.RoleCommonUser)

	upstream := newUpstreamServer(t, map[string]http.HandlerFunc{
		"/v1/chat/completions": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"id":"c1","object":"chat.completion","created":1,"model":"gpt-4o",`+
				`"choices":[{"index":0,"message":{"role":"assistant","content":"playground hello"},"finish_reason":"stop"}],`+
				`"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
		},
	})
	insertTestChannel(t, "ch", upstream.URL, "gpt-4o", common.ChannelStatusEnabled)
	if err := model.InitChannelCache(); err != nil {
		t.Fatal(err)
	}

	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("test-secret"))))
	engine.POST("/login", controller.Login)
	engine.POST("/pg/chat/completions", middleware.UserAuth(), middleware.Distribute(), controller.Playground)

	body := map[string]any{
		"model":    "gpt-4o",
		"group":    "default",
		"messages": []map[string]string{{"role": "user", "content": "hi"}},
	}

	// 未登录被拒绝
	req := newJSONRequest(t, http.MethodPost, "/pg/chat/completions", body)
	if w := serve(engine, req); w.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated playground: %d %s", w.Code, w.Body.String())
	}

	// 登录后可以直接对话
	loginReq := newJSONRequest(t, http.MethodPost, "/login", map[string]any{"username": "alice", "password": "password123"})
	loginResp := serve(engine, loginReq)
	if loginResp.Code != http.StatusOK {
		t.Fatalf("login failed: %d %s", loginResp.Code, loginResp.Body.String())
	}
	sessionCookie := loginResp.Header().Get("Set-Cookie")
	if sessionCookie == "" {
		t.Fatal("login should set session cookie")
	}

	req = newJSONRequest(t, http.MethodPost, "/pg/chat/completions", body)
	req.Header.Set("Cookie", sessionCookie)
	req.Header.Set("Open-Api-User", strconv.Itoa(alice.id))
	w := serve(engine, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "playground hello") {
		t.Fatalf("playground response: %d %s", w.Code, w.Body.String())
	}

	// 分组已移除：请求体里的 group 字段被忽略，仍然正常转发
	body["group"] = "vip"
	req = newJSONRequest(t, http.MethodPost, "/pg/chat/completions", body)
	req.Header.Set("Cookie", sessionCookie)
	req.Header.Set("Open-Api-User", strconv.Itoa(alice.id))
	if w := serve(engine, req); w.Code != http.StatusOK {
		t.Fatalf("playground should ignore group field: %d %s", w.Code, w.Body.String())
	}
}

// newJSONRequest 构造带 JSON 请求体的请求。
func newJSONRequest(t *testing.T, method, path string, body any) *http.Request {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(jsonBody(t, body)))
	req.Header.Set("Content-Type", "application/json")
	return req
}
