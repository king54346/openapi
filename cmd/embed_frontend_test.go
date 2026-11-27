package main

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"openapi/frontend"
)

func TestEmbeddedFrontendSPAServing(t *testing.T) {
	if len(frontend.IndexPage()) == 0 {
		t.Skip("frontend/dist/index.html not built yet; skipping embedded SPA test")
	}
	t.Setenv("FRONTEND_BASE_URL", "")
	srv := newServer()

	// 1. 根路径 "/" 返回内嵌 index.html
	wRoot := httptest.NewRecorder()
	srv.ServeHTTP(wRoot, httptest.NewRequest(http.MethodGet, "/", nil))
	if wRoot.Code != http.StatusOK || !strings.Contains(wRoot.Body.String(), `<div id="root"></div>`) {
		t.Fatalf("GET / expected 200 SPA index.html, got %d body=%s", wRoot.Code, wRoot.Body.String())
	}

	// 2. SPA 前端路由（如 /console/channel）回退返回 index.html
	wSPA := httptest.NewRecorder()
	srv.ServeHTTP(wSPA, httptest.NewRequest(http.MethodGet, "/console/channel", nil))
	if wSPA.Code != http.StatusOK || !strings.Contains(wSPA.Body.String(), `<div id="root"></div>`) {
		t.Fatalf("GET /console/channel expected 200 SPA fallback, got %d", wSPA.Code)
	}

	// 3. 静态产物（/assets/*.js）正常返回 JS 且携带长期缓存头
	entries, err := fs.ReadDir(frontend.BuildFS, "dist/assets")
	if err != nil || len(entries) == 0 {
		t.Fatalf("expected built assets in frontend/dist/assets: %v", err)
	}
	assetPath := "/assets/" + entries[0].Name()
	wAsset := httptest.NewRecorder()
	srv.ServeHTTP(wAsset, httptest.NewRequest(http.MethodGet, assetPath, nil))
	if wAsset.Code != http.StatusOK || wAsset.Body.Len() == 0 {
		t.Fatalf("GET %s expected 200 non-empty asset, got %d", assetPath, wAsset.Code)
	}
	if !strings.Contains(wAsset.Header().Get("Cache-Control"), "immutable") {
		t.Fatalf("expected immutable Cache-Control on %s, got %q", assetPath, wAsset.Header().Get("Cache-Control"))
	}

	// 4. 未匹配的 /api/* 依然返回 JSON 404，绝不误回退为 HTML
	wAPI := httptest.NewRecorder()
	srv.ServeHTTP(wAPI, httptest.NewRequest(http.MethodGet, "/api/nonexistent_route", nil))
	if wAPI.Code != http.StatusNotFound || !strings.Contains(wAPI.Body.String(), `"not_found"`) {
		t.Fatalf("GET /api/nonexistent_route expected 404 JSON, got %d %s", wAPI.Code, wAPI.Body.String())
	}
}
