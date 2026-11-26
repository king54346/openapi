package playground_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"openapi/common"
	"openapi/controller/common/testhelper"
	pgctrl "openapi/controller/playground"
	userctrl "openapi/controller/user"
	"openapi/middleware"
	"openapi/model"

	gin "github.com/king54346/gin-tiny"
	"github.com/king54346/gin-tiny/middleware/sessions"
	"github.com/king54346/gin-tiny/middleware/sessions/cookie"
)

func newJSONRequest(t *testing.T, method, path string, body any) *http.Request {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req := httptest.NewRequest(method, path, strings.NewReader(string(data)))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func serve(engine *gin.Engine, req *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

func TestPlaygroundAPI(t *testing.T) {
	s := testhelper.NewTestServer(t)
	testhelper.ResetGlobalSettings(t)
	alice := s.CreateUser("alice", common.RoleCommonUser)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"c1","object":"chat.completion","created":1,"model":"gpt-4o",`+
			`"choices":[{"index":0,"message":{"role":"assistant","content":"playground hello"},"finish_reason":"stop"}],`+
			`"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	}))
	t.Cleanup(upstream.Close)

	testhelper.InsertTestChannel(t, "ch", upstream.URL, "gpt-4o", common.ChannelStatusEnabled)
	if err := model.InitChannelCache(); err != nil {
		t.Fatal(err)
	}

	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("test-secret"))))
	engine.POST("/login", userctrl.Login)
	engine.POST("/pg/chat/completions", middleware.UserAuth(), middleware.Distribute(), pgctrl.Playground)

	body := map[string]any{
		"model":    "gpt-4o",
		"group":    "default",
		"messages": []map[string]string{{"role": "user", "content": "hi"}},
	}

	if w := serve(engine, newJSONRequest(t, http.MethodPost, "/pg/chat/completions", body)); w.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated playground: %d %s", w.Code, w.Body.String())
	}

	loginResp := serve(engine, newJSONRequest(t, http.MethodPost, "/login", map[string]any{"username": "alice", "password": "password123"}))
	if loginResp.Code != http.StatusOK {
		t.Fatalf("login failed: %d %s", loginResp.Code, loginResp.Body.String())
	}
	sessionCookie := loginResp.Header().Get("Set-Cookie")
	if sessionCookie == "" {
		t.Fatal("login should set session cookie")
	}

	req := newJSONRequest(t, http.MethodPost, "/pg/chat/completions", body)
	req.Header.Set("Cookie", sessionCookie)
	req.Header.Set("Open-Api-User", strconv.Itoa(alice.ID))
	w := serve(engine, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "playground hello") {
		t.Fatalf("playground response: %d %s", w.Code, w.Body.String())
	}

	body["group"] = "vip"
	req = newJSONRequest(t, http.MethodPost, "/pg/chat/completions", body)
	req.Header.Set("Cookie", sessionCookie)
	req.Header.Set("Open-Api-User", strconv.Itoa(alice.ID))
	if w := serve(engine, req); w.Code != http.StatusOK {
		t.Fatalf("playground should ignore group field: %d %s", w.Code, w.Body.String())
	}
}
