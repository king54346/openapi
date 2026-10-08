package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"openapi/common"
	"openapi/constant"
	"openapi/model"

	gin "github.com/king54346/gin-tiny"
)

// mockUpstream 模拟上游服务商，按路径前缀区分渠道：
//   - /fail：所有请求返回 503
//   - /ok：OpenAI 兼容的对话（支持流式）与模型列表
//   - /sora：视频任务提交与查询
type mockUpstream struct {
	*httptest.Server
	mu    sync.Mutex
	calls []string // "METHOD /path"
}

func newMockUpstream(t *testing.T) *mockUpstream {
	m := &mockUpstream{}
	m.Server = httptest.NewServer(http.HandlerFunc(m.serve))
	t.Cleanup(m.Close)
	return m
}

func (m *mockUpstream) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	m.mu.Lock()
	m.calls = append(m.calls, r.Method+" "+r.URL.Path)
	m.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	switch {
	case strings.HasPrefix(r.URL.Path, "/fail/"):
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, `{"error":{"message":"upstream overloaded","type":"server_error"}}`)

	case r.URL.Path == "/ok/v1/chat/completions":
		if r.Header.Get("Authorization") != "Bearer ok-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if bytes.Contains(body, []byte(`"stream":true`)) {
			w.Header().Set("Content-Type", "text/event-stream")
			for _, part := range []string{"hel", "lo"} {
				fmt.Fprintf(w, "data: {\"id\":\"c1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{\"content\":%q}}]}\n\n", part)
			}
			_, _ = io.WriteString(w, "data: {\"id\":\"c1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":7,\"completion_tokens\":2,\"total_tokens\":9}}\n\ndata: [DONE]\n\n")
			return
		}
		_, _ = io.WriteString(w, `{"id":"c1","object":"chat.completion","created":1,"model":"gpt-4o","choices":[{"index":0,"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":1,"total_tokens":6}}`)

	case r.URL.Path == "/ok/v1/models":
		_, _ = io.WriteString(w, `{"data":[{"id":"gpt-4o"},{"id":"gpt-4o-mini"}]}`)

	case r.Method == http.MethodPost && r.URL.Path == "/sora/v1/videos":
		_, _ = io.WriteString(w, `{"id":"vid_1","object":"video","model":"sora-2","status":"queued","progress":0}`)

	case r.Method == http.MethodGet && r.URL.Path == "/sora/v1/videos/vid_1":
		_, _ = io.WriteString(w, `{"id":"vid_1","object":"video","model":"sora-2","status":"completed","progress":100}`)

	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// takeCalls 返回并清空已记录的上游调用。
func (m *mockUpstream) takeCalls() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	calls := m.calls
	m.calls = nil
	return calls
}

// client 访问被测服务：管理接口用 session（cookie），转发接口用令牌。
type client struct {
	t       *testing.T
	base    string
	http    *http.Client
	adminID int
}

func (c *client) do(method, path string, body any, headers map[string]string) (int, []byte) {
	c.t.Helper()
	var reader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.base+path, reader)
	if err != nil {
		c.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, data
}

// admin 以已登录管理员身份调用管理接口，要求 success=true，返回 data。
func (c *client) admin(method, path string, body any) json.RawMessage {
	c.t.Helper()
	_, data := c.do(method, path, body, map[string]string{"Open-Api-User": strconv.Itoa(c.adminID)})
	var resp struct {
		Success bool            `json:"success"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil || !resp.Success {
		c.t.Fatalf("%s %s: %s", method, path, data)
	}
	return resp.Data
}

// relay 以 API 令牌调用转发接口。
func (c *client) relay(method, path, key string, body any) (int, []byte) {
	c.t.Helper()
	return c.do(method, path, body, map[string]string{"Authorization": "Bearer sk-" + key})
}

func chatBody(stream bool) map[string]any {
	return map[string]any{
		"model":    "gpt-4o",
		"stream":   stream,
		"messages": []map[string]string{{"role": "user", "content": "hi"}},
	}
}

func channelStatus(t *testing.T, name string) int {
	t.Helper()
	var ch model.Channel
	if err := model.DB.Where("name = ?", name).First(&ch).Error; err != nil {
		t.Fatal(err)
	}
	return ch.Status
}

func countLogs(t *testing.T, logType int) int64 {
	t.Helper()
	var n int64
	model.LOG_DB.Model(&model.Log{}).Where("type = ?", logType).Count(&n)
	return n
}

// waitForLogs 等待某类日志达到 want 条（最多 2 秒），返回最终条数。
func waitForLogs(t *testing.T, logType int, want int64) int64 {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		n := countLogs(t, logType)
		if n >= want || time.Now().After(deadline) {
			return n
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestMainIntegration 用 main 的真实启动流程（InitResources + newServer + runServer）
// 在临时 SQLite 上启动完整服务，走一遍管理、转发、重试、自动禁用、视频任务与优雅关闭。
func TestMainIntegration(t *testing.T) {
	upstream := newMockUpstream(t)

	dbPath := filepath.ToSlash(filepath.Join(t.TempDir(), "integration.db"))
	t.Setenv("SQLITE_PATH", dbPath+"?_pragma=busy_timeout(5000)")
	t.Setenv("SESSION_SECRET", "integration-test-secret")
	t.Setenv("RETRY_TIMES", "1")
	t.Setenv("CHANNEL_AUTO_DISABLE_THRESHOLD", "2")
	t.Setenv("UPDATE_TASK", "false") // 视频任务在查询时同步，不依赖后台轮询的时机
	t.Setenv("LOG_DIR", "")
	gin.SetMode(gin.TestMode)

	// ---------- 启动 ----------
	db, err := InitResources()
	if err != nil {
		t.Fatalf("InitResources: %v", err)
	}
	t.Cleanup(func() { closeDB(db) })
	if common.RetryTimes != 1 || common.ChannelAutoDisableThreshold != 2 {
		t.Fatalf("env not loaded: retry=%d threshold=%d", common.RetryTimes, common.ChannelAutoDisableThreshold)
	}
	for _, table := range []any{&model.Channel{}, &model.Task{}, &model.Log{}, &model.User{}, &model.Token{}} {
		if !model.DB.Migrator().HasTable(table) {
			t.Fatalf("table for %T not created on fresh database", table)
		}
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	serverDone := make(chan error, 1)
	go func() { serverDone <- runServer(ctx, newServer(), listener) }()

	root := &model.User{Username: "root", Role: common.RoleRootUser}
	if err := root.Insert("root-password"); err != nil {
		t.Fatal(err)
	}
	jar, _ := cookiejar.New(nil)
	c := &client{t: t, base: "http://" + listener.Addr().String(), http: &http.Client{Jar: jar, Timeout: 10 * time.Second}, adminID: root.Id}

	t.Run("unauthenticated", func(t *testing.T) {
		if code, _ := c.relay(http.MethodPost, "/v1/chat/completions", "nope", chatBody(false)); code != http.StatusUnauthorized {
			t.Errorf("invalid token: %d", code)
		}
		if code, body := c.do(http.MethodGet, "/api/channel/", nil, nil); code != http.StatusUnauthorized {
			t.Errorf("admin api without login: %d %s", code, body)
		}
		if code, body := c.do(http.MethodGet, "/api/nope", nil, nil); code != http.StatusNotFound || !bytes.Contains(body, []byte("not_found")) {
			t.Errorf("unknown api route: %d %s", code, body)
		}
	})

	// ---------- 登录与配置 ----------
	var tokenKey string
	t.Run("login and setup", func(t *testing.T) {
		code, body := c.do(http.MethodPost, "/api/user/login", map[string]string{"username": "root", "password": "wrong-password"}, nil)
		if code != http.StatusOK || !bytes.Contains(body, []byte(`"success":false`)) {
			t.Fatalf("wrong password should fail: %d %s", code, body)
		}
		code, body = c.do(http.MethodPost, "/api/user/login", map[string]string{"username": "root", "password": "root-password"}, nil)
		if code != http.StatusOK || !bytes.Contains(body, []byte(`"success":true`)) {
			t.Fatalf("login: %d %s", code, body)
		}

		addChannel := func(name string, channelType int, prefix, key, models string, priority int) {
			c.admin(http.MethodPost, "/api/channel/", map[string]any{"channel": map[string]any{
				"name": name, "type": channelType, "key": key, "models": models,
				"base_url": upstream.URL + prefix, "priority": priority, "auto_ban": 1,
			}})
		}
		addChannel("primary", constant.ChannelTypeOpenAI, "/fail", "fail-key", "gpt-4o", 10)
		addChannel("backup", constant.ChannelTypeOpenAI, "/ok", "ok-key", "gpt-4o,gpt-4o-mini", 0)
		addChannel("sora", constant.ChannelTypeSora, "/sora", "sora-key", "sora-2", 0)

		var channels struct {
			Total int64 `json:"total"`
		}
		_ = json.Unmarshal(c.admin(http.MethodGet, "/api/channel/", nil), &channels)
		if channels.Total != 3 {
			t.Fatalf("expected 3 channels, got %d", channels.Total)
		}

		var token model.Token
		_ = json.Unmarshal(c.admin(http.MethodPost, "/api/token/", map[string]any{
			"name": "integration", "unlimited_quota": true, "expired_time": -1}), &token)
		if len(token.Key) != 48 {
			t.Fatalf("token key: %q", token.Key)
		}
		tokenKey = token.Key
	})
	if tokenKey == "" {
		t.FailNow()
	}

	t.Run("models", func(t *testing.T) {
		code, body := c.relay(http.MethodGet, "/v1/models", tokenKey, nil)
		if code != http.StatusOK {
			t.Fatalf("list models: %d %s", code, body)
		}
		for _, m := range []string{"gpt-4o", "gpt-4o-mini", "sora-2"} {
			if !bytes.Contains(body, []byte(`"id":"`+m+`"`)) {
				t.Errorf("model %s missing: %s", m, body)
			}
		}
		if code, _ := c.relay(http.MethodGet, "/v1/models/nope", tokenKey, nil); code != http.StatusNotFound {
			t.Errorf("unknown model: %d", code)
		}

		// 管理端模型列表
		if enabled := c.admin(http.MethodGet, "/api/channel/models_enabled", nil); string(enabled) != `["gpt-4o","gpt-4o-mini","sora-2"]` {
			t.Errorf("enabled models: %s", enabled)
		}
		if builtin := c.admin(http.MethodGet, "/api/channel/models", nil); !bytes.Contains(builtin, []byte(`"id":"deepseek-chat"`)) {
			t.Errorf("builtin models missing deepseek-chat")
		}
		byType := c.admin(http.MethodGet, "/api/models", nil)
		if !bytes.Contains(byType, []byte(`"`+strconv.Itoa(constant.ChannelTypeSora)+`":[`)) {
			t.Errorf("dashboard models missing sora: %s", byType)
		}
	})

	// ---------- 转发、重试与自动禁用 ----------
	t.Run("relay with retry", func(t *testing.T) {
		upstream.takeCalls()
		code, body := c.relay(http.MethodPost, "/v1/chat/completions", tokenKey, chatBody(false))
		if code != http.StatusOK || !bytes.Contains(body, []byte(`"content":"hello"`)) {
			t.Fatalf("chat: %d %s", code, body)
		}
		calls := upstream.takeCalls()
		if len(calls) != 2 || !strings.HasPrefix(calls[0], "POST /fail/") || !strings.HasPrefix(calls[1], "POST /ok/") {
			t.Fatalf("expected primary then backup, got %v", calls)
		}
		if channelStatus(t, "primary") != common.ChannelStatusEnabled {
			t.Fatal("one failure must not disable the channel")
		}
	})

	t.Run("stream and auto disable", func(t *testing.T) {
		code, body := c.relay(http.MethodPost, "/v1/chat/completions", tokenKey, chatBody(true))
		if code != http.StatusOK || !bytes.Contains(body, []byte(`"content":"hel"`)) || !bytes.Contains(body, []byte("[DONE]")) {
			t.Fatalf("stream: %d %s", code, body)
		}
		if channelStatus(t, "primary") != common.ChannelStatusAutoDisabled {
			t.Fatal("second consecutive failure should auto disable the primary channel")
		}
		upstream.takeCalls()
		if code, _ := c.relay(http.MethodPost, "/v1/chat/completions", tokenKey, chatBody(false)); code != http.StatusOK {
			t.Fatalf("chat after disable: %d", code)
		}
		if calls := upstream.takeCalls(); len(calls) != 1 || !strings.HasPrefix(calls[0], "POST /ok/") {
			t.Fatalf("disabled channel must be skipped, got %v", calls)
		}
	})

	t.Run("logs", func(t *testing.T) {
		if n := countLogs(t, model.LogTypeError); n != 2 {
			t.Errorf("error logs: %d", n)
		}
		// 用量日志在响应写回客户端之后才记录，最后一个请求的日志可能稍晚落库
		if n := waitForLogs(t, model.LogTypeConsume, 3); n != 3 {
			t.Errorf("consume logs: %d", n)
		}
		var last model.Log
		model.LOG_DB.Where("type = ?", model.LogTypeConsume).Order("id desc").First(&last)
		if last.TokenName != "integration" || last.Username != "root" || last.PromptTokens != 5 || last.CompletionTokens != 1 {
			t.Errorf("consume log: %+v", last)
		}
	})

	// ---------- 视频任务 ----------
	t.Run("video task", func(t *testing.T) {
		code, body := c.relay(http.MethodPost, "/v1/videos", tokenKey, map[string]any{"model": "sora-2", "prompt": "a cat playing piano"})
		if code != http.StatusOK || !bytes.Contains(body, []byte(`"id":"vid_1"`)) {
			t.Fatalf("submit video: %d %s", code, body)
		}
		var task model.Task
		if err := model.DB.Where("task_id = ?", "vid_1").First(&task).Error; err != nil {
			t.Fatalf("task not recorded: %v", err)
		}
		if task.UserId != root.Id || task.Status.IsFinished() {
			t.Fatalf("submitted task: %+v", task)
		}

		code, body = c.relay(http.MethodGet, "/v1/videos/vid_1", tokenKey, nil)
		if code != http.StatusOK || !bytes.Contains(body, []byte(`"status":"completed"`)) {
			t.Fatalf("fetch video: %d %s", code, body)
		}
		model.DB.Where("task_id = ?", "vid_1").First(&task)
		if task.Status != model.TaskStatusSuccess || task.Progress != "100%" {
			t.Fatalf("task after fetch: status=%s progress=%s", task.Status, task.Progress)
		}

		var tasks struct {
			Total int64 `json:"total"`
		}
		_ = json.Unmarshal(c.admin(http.MethodGet, "/api/task/?status=SUCCESS", nil), &tasks)
		if tasks.Total != 1 {
			t.Fatalf("admin task list: %d", tasks.Total)
		}
	})

	// ---------- 令牌禁用即时生效 ----------
	t.Run("disable token", func(t *testing.T) {
		var token model.Token
		model.DB.Where(map[string]any{"name": "integration"}).First(&token)
		c.admin(http.MethodPut, "/api/token/?status_only=true", map[string]any{"id": token.Id, "status": common.TokenStatusDisabled})
		if code, body := c.relay(http.MethodPost, "/v1/chat/completions", tokenKey, chatBody(false)); code != http.StatusUnauthorized {
			t.Fatalf("disabled token: %d %s", code, body)
		}
	})

	// ---------- 优雅关闭 ----------
	t.Run("graceful shutdown", func(t *testing.T) {
		cancel()
		select {
		case err := <-serverDone:
			if err != nil {
				t.Fatalf("runServer returned error: %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("server did not stop within 10s")
		}
		if _, err := http.Get(c.base + "/api/nope"); err == nil {
			t.Fatal("server should not accept connections after shutdown")
		}
	})
}
