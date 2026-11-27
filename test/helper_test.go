package e2e_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/joho/godotenv"
)

func init() {
	// 自动加载项目根目录或当前目录的本地 .env（.env 已被 .gitignore 忽略，不在源码中硬编码任何密钥）
	_ = godotenv.Load("../.env", ".env")
}

var seqCounter atomic.Int64

func envOrDefault(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func requireEnvOrSkip(t *testing.T, key string) string {
	t.Helper()
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	if os.Getenv("LIVE_API_TEST") == "1" {
		t.Fatalf("required environment variable %s is not set", key)
	}
	t.Skipf("skipping live test: environment variable %s is not configured", key)
	return ""
}

func baseURL() string {
	return strings.TrimRight(envOrDefault("TEST_BASE_URL", "http://localhost:3000"), "/")
}

func adminUsername() string {
	return envOrDefault("TEST_ADMIN_USER", "admin")
}

func adminPassword(t *testing.T) string {
	return requireEnvOrSkip(t, "TEST_ADMIN_PASS")
}

func deepSeekTestKey(t *testing.T) string {
	return requireEnvOrSkip(t, "TEST_DEEPSEEK_KEY")
}

func customOpenAIBaseURL(t *testing.T) string {
	return requireEnvOrSkip(t, "TEST_CUSTOM_OPENAI_URL")
}

func customOpenAITestKey(t *testing.T) string {
	return requireEnvOrSkip(t, "TEST_CUSTOM_OPENAI_KEY")
}

func uniqueName(prefix string) string {
	n := seqCounter.Add(1)
	return fmt.Sprintf("%s_%d_%d", prefix, time.Now().UnixMilli()%1000000, n)
}

type APIResponse struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type PageData[T any] struct {
	Items    []T   `json:"items"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
}

type LiveClient struct {
	t      *testing.T
	base   string
	http   *http.Client
	userID int
}

// newLiveClient 创建指向已启动后端的 HTTP 测试客户端；若后端未启动且未强制开启 LIVE_API_TEST，则自动跳过。
func newLiveClient(t *testing.T) *LiveClient {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("create cookiejar: %v", err)
	}
	c := &LiveClient{
		t:    t,
		base: baseURL(),
		http: &http.Client{
			Jar:     jar,
			Timeout: 60 * time.Second,
		},
	}

	probeClient := &http.Client{Timeout: 2 * time.Second}
	resp, err := probeClient.Get(c.base + "/api/status")
	if err != nil {
		if os.Getenv("LIVE_API_TEST") == "1" {
			t.Fatalf("backend %s is not reachable: %v", c.base, err)
		}
		t.Skipf("skipping live API test: backend %s is not running (%v)", c.base, err)
	}
	_ = resp.Body.Close()
	return c
}

// newLoggedInAdminClient 登录管理员账号并返回带 Session Cookie 与 Open-Api-User 头的客户端。
func newLoggedInAdminClient(t *testing.T) *LiveClient {
	t.Helper()
	c := newLiveClient(t)
	c.Login(adminUsername(), adminPassword(t))
	return c
}

func (c *LiveClient) Login(username, password string) map[string]any {
	c.t.Helper()
	code, resp, raw := c.DoAPI(http.MethodPost, "/api/user/login", map[string]string{
		"username": username,
		"password": password,
	}, nil)
	if code != http.StatusOK || !resp.Success {
		c.t.Fatalf("login %q failed: HTTP %d body=%s", username, code, string(raw))
	}
	var user map[string]any
	if err := json.Unmarshal(resp.Data, &user); err != nil {
		c.t.Fatalf("decode login user: %v", err)
	}
	if idFloat, ok := user["id"].(float64); ok {
		c.userID = int(idFloat)
	}
	return user
}

// RawDo 发送任意 HTTP 请求并返回状态码、响应头与原始字节。
func (c *LiveClient) RawDo(method, path string, body any, headers map[string]string) (int, http.Header, []byte) {
	c.t.Helper()
	var reader io.Reader
	if body != nil {
		switch v := body.(type) {
		case []byte:
			reader = bytes.NewReader(v)
		case string:
			reader = strings.NewReader(v)
		default:
			b, err := json.Marshal(body)
			if err != nil {
				c.t.Fatalf("marshal body for %s %s: %v", method, path, err)
			}
			reader = bytes.NewReader(b)
		}
	}
	req, err := http.NewRequest(method, c.base+path, reader)
	if err != nil {
		c.t.Fatalf("build request %s %s: %v", method, path, err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.userID > 0 {
		req.Header.Set("Open-Api-User", strconv.Itoa(c.userID))
	}
	for k, v := range headers {
		if v == "" {
			req.Header.Del(k)
		} else {
			req.Header.Set(k, v)
		}
	}
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatalf("request %s %s failed: %v", method, path, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		c.t.Fatalf("read response %s %s: %v", method, path, err)
	}
	return resp.StatusCode, resp.Header, data
}

// DoAPI 调用 /api/* 管理接口并解析标准 {success, message, data} 结构。
func (c *LiveClient) DoAPI(method, path string, body any, headers map[string]string) (int, APIResponse, []byte) {
	c.t.Helper()
	code, _, raw := c.RawDo(method, path, body, headers)
	var apiResp APIResponse
	_ = json.Unmarshal(raw, &apiResp)
	return code, apiResp, raw
}

// MustOK 断言管理接口返回 HTTP 200 且 success=true。
func (c *LiveClient) MustOK(method, path string, body any) json.RawMessage {
	c.t.Helper()
	code, resp, raw := c.DoAPI(method, path, body, nil)
	if code != http.StatusOK || !resp.Success {
		c.t.Fatalf("%s %s expected success=true, got HTTP %d: %s", method, path, code, string(raw))
	}
	return resp.Data
}

// MustFail 断言管理接口返回业务失败（success=false 或 HTTP >= 400）。
func (c *LiveClient) MustFail(method, path string, body any) (int, string) {
	c.t.Helper()
	code, resp, raw := c.DoAPI(method, path, body, nil)
	if code == http.StatusOK && resp.Success {
		c.t.Fatalf("%s %s expected failure, but succeeded: %s", method, path, string(raw))
	}
	return code, resp.Message
}

func mustDecode[T any](t *testing.T, raw json.RawMessage) T {
	t.Helper()
	var out T
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode JSON %s: %v", string(raw), err)
	}
	return out
}
