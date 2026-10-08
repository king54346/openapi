package middleware

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/andybalholm/brotli"
	gin "github.com/king54346/gin-tiny"
	"github.com/klauspost/compress/zstd"
)

func serveCompress(t *testing.T, handler gin.HandlerFunc, acceptEncoding string, path string, accept string) *httptest.ResponseRecorder {
	t.Helper()
	r := gin.New()
	r.Use(handler)
	r.GET("/api/status", func(c gin.Context) {
		c.Header("Content-Type", "application/json")
		c.String(http.StatusOK, "%s", strings.Repeat("hello world, ", 200))
	})
	r.GET("/assets/logo.png", func(c gin.Context) {
		c.String(http.StatusOK, "fake-png")
	})
	r.GET("/healthz", func(c gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, path, nil)
	if acceptEncoding != "" {
		req.Header.Set("Accept-Encoding", acceptEncoding)
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func readGzipBody(t *testing.T, raw []byte) string {
	t.Helper()
	gr, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("response is not gzip: %v", err)
	}
	defer gr.Close()
	decoded, err := io.ReadAll(gr)
	if err != nil {
		t.Fatalf("gunzip failed: %v", err)
	}
	return string(decoded)
}

func readBrotliBody(t *testing.T, raw []byte) string {
	t.Helper()
	decoded, err := io.ReadAll(brotli.NewReader(bytes.NewReader(raw)))
	if err != nil {
		t.Fatalf("br decode failed: %v", err)
	}
	return string(decoded)
}

func readZstdBody(t *testing.T, raw []byte) string {
	t.Helper()
	dec, err := zstd.NewReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("response is not zstd: %v", err)
	}
	defer dec.Close()
	decoded, err := io.ReadAll(dec)
	if err != nil {
		t.Fatalf("zstd decode failed: %v", err)
	}
	return string(decoded)
}

func TestCompressGzipResponse(t *testing.T) {
	raw := strings.Repeat("hello world, ", 200)
	w := serveCompress(t, Compress(), "gzip", "/api/status", "")
	if w.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("expected gzip encoding, headers: %v", w.Header())
	}
	if got := readGzipBody(t, w.Body.Bytes()); got != raw {
		t.Fatalf("roundtrip mismatch: got %d bytes, want %d", len(got), len(raw))
	}
}

func TestCompressBrotliResponse(t *testing.T) {
	raw := strings.Repeat("hello world, ", 200)
	w := serveCompress(t, Compress(), "br", "/api/status", "")
	if w.Header().Get("Content-Encoding") != "br" {
		t.Fatalf("expected br encoding, headers: %v", w.Header())
	}
	if got := readBrotliBody(t, w.Body.Bytes()); got != raw {
		t.Fatalf("roundtrip mismatch: got %d bytes, want %d", len(got), len(raw))
	}
}

func TestCompressZstdResponse(t *testing.T) {
	raw := strings.Repeat("hello world, ", 200)
	w := serveCompress(t, Compress(), "zstd", "/api/status", "")
	if w.Header().Get("Content-Encoding") != "zstd" {
		t.Fatalf("expected zstd encoding, headers: %v", w.Header())
	}
	if got := readZstdBody(t, w.Body.Bytes()); got != raw {
		t.Fatalf("roundtrip mismatch: got %d bytes, want %d", len(got), len(raw))
	}
}

func TestCompressPrefersZstdOverBrotliAndGzip(t *testing.T) {
	w := serveCompress(t, Compress(), "gzip, br, zstd", "/api/status", "")
	if w.Header().Get("Content-Encoding") != "zstd" {
		t.Fatalf("expected zstd to win, got %q", w.Header().Get("Content-Encoding"))
	}
}

func TestCompressPrefersBrotliOverGzip(t *testing.T) {
	w := serveCompress(t, Compress(), "gzip, br", "/api/status", "")
	if w.Header().Get("Content-Encoding") != "br" {
		t.Fatalf("expected br to win over gzip, got %q", w.Header().Get("Content-Encoding"))
	}
}

func TestCompressRespectsQValuePriority(t *testing.T) {
	// br q 值更高时胜出，即使 zstd 也在列表中。
	w := serveCompress(t, Compress(), "zstd;q=0.5, br;q=0.9, gzip;q=0.1", "/api/status", "")
	if w.Header().Get("Content-Encoding") != "br" {
		t.Fatalf("expected br by q value, got %q", w.Header().Get("Content-Encoding"))
	}
}

func TestCompressSkippedWithoutAcceptEncoding(t *testing.T) {
	w := serveCompress(t, Compress(), "", "/api/status", "")
	if enc := w.Header().Get("Content-Encoding"); enc != "" {
		t.Fatalf("should not compress, got encoding %q", enc)
	}
	if !strings.Contains(w.Body.String(), "hello world") {
		t.Fatalf("expected passthrough, got %q", w.Body.String())
	}
}

func TestCompressSkippedForSSE(t *testing.T) {
	w := serveCompress(t, Compress(), "gzip", "/api/status", "text/event-stream")
	if enc := w.Header().Get("Content-Encoding"); enc != "" {
		t.Fatalf("SSE must not be compressed, got %q", enc)
	}
}

func TestCompressSkippedForImages(t *testing.T) {
	w := serveCompress(t, Compress(), "gzip", "/assets/logo.png", "")
	if enc := w.Header().Get("Content-Encoding"); enc != "" {
		t.Fatalf("images must not be compressed, got %q", enc)
	}
}

func TestCompressRespectsQZero(t *testing.T) {
	w := serveCompress(t, Compress(), "gzip;q=0", "/api/status", "")
	if enc := w.Header().Get("Content-Encoding"); enc != "" {
		t.Fatalf("q=0 must opt out, got %q", enc)
	}
}

func TestCompressCustomLevelAndExcludedPath(t *testing.T) {
	r := gin.New()
	r.Use(CompressWithLevel(gzip.BestSpeed, WithCompressExcludedPaths("/healthz")))
	r.GET("/healthz", func(c gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if enc := w.Header().Get("Content-Encoding"); enc != "" {
		t.Fatalf("excluded path must not be compressed, got %q", enc)
	}
	if w.Body.String() != "ok" {
		t.Fatalf("expected passthrough body, got %q", w.Body.String())
	}
}
