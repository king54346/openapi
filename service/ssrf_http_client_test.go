package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"openapi/common"
)

func setFetchSetting(t *testing.T, mutate func(s *common.FetchSetting)) {
	t.Helper()
	orig := common.GetFetchSetting()
	t.Cleanup(func() { _ = common.UpdateFetchSetting(orig) })
	s := common.DefaultFetchSetting()
	s.AllowedPorts = nil // httptest 使用随机端口
	mutate(&s)
	if err := common.UpdateFetchSetting(s); err != nil {
		t.Fatal(err)
	}
}

func TestSSRFSafeGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, strings.Replace("http://"+r.Host+"/ok", "127.0.0.1", "localhost", 1), http.StatusFound)
			return
		}
		_, _ = io.WriteString(w, "ok")
	}))
	defer srv.Close()
	port := srv.URL[strings.LastIndex(srv.URL, ":"):]

	// 默认禁止内网：IP 字面量在请求前拦截
	setFetchSetting(t, func(s *common.FetchSetting) {})
	if _, err := DoSSRFSafeGet(context.Background(), srv.URL); !errors.Is(err, ErrSSRFBlocked) {
		t.Fatalf("loopback literal should be blocked, got %v", err)
	}
	// 域名解析到回环：请求前不解析，由拨号时的 IP 校验拦截（防 DNS 重绑定）
	if _, err := DoSSRFSafeGet(context.Background(), "http://localhost"+port); err == nil || !strings.Contains(err.Error(), ErrSSRFBlocked.Error()) {
		t.Fatalf("localhost should be blocked at dial time, got %v", err)
	}

	// 允许内网后可访问；重定向到黑名单域名被拦截
	setFetchSetting(t, func(s *common.FetchSetting) {
		s.AllowPrivateIp = true
		s.DomainList = []string{"localhost"}
	})
	resp, err := DoSSRFSafeGet(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("private ip allowed, got %v", err)
	}
	CloseResponseBodyGracefully(resp)
	if _, err := DoSSRFSafeGet(context.Background(), srv.URL+"/redirect"); err == nil || !strings.Contains(err.Error(), "domain in blacklist") {
		t.Fatalf("redirect to blacklisted domain should be blocked, got %v", err)
	}

	// 关闭防护后不做任何限制
	setFetchSetting(t, func(s *common.FetchSetting) { s.EnableSSRFProtection = false })
	resp, err = DoSSRFSafeGet(context.Background(), "http://localhost"+port+"/redirect")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("protection disabled, got %v", err)
	}
	CloseResponseBodyGracefully(resp)

	// GetImageFromUrl 走同一防护
	setFetchSetting(t, func(s *common.FetchSetting) {})
	if _, _, err := GetImageFromUrl(srv.URL + "/a.png"); !errors.Is(err, ErrSSRFBlocked) {
		t.Fatalf("image download should be blocked, got %v", err)
	}
}
