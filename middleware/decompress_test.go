package middleware

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/andybalholm/brotli"
	gin "github.com/king54346/gin-tiny"
	"github.com/klauspost/compress/zstd"
)

func runDecompress(t *testing.T, body []byte, encoding string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body == nil {
		req = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	} else {
		req = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	}
	if encoding != "" {
		req.Header.Set("Content-Encoding", encoding)
	}
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.SetRequest(req)

	var downstream []byte
	DecompressRequestMiddleware()(c)
	if c.IsAborted() {
		return w
	}
	downstream, _ = io.ReadAll(c.Request().Body)
	w.Write(downstream)
	return w
}

func gzipBytes(t *testing.T, raw []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func zlibBytes(t *testing.T, raw []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	if _, err := w.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func brotliBytes(t *testing.T, raw []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := brotli.NewWriter(&buf)
	if _, err := w.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func zstdBytes(t *testing.T, raw []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w, err := zstd.NewWriter(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestDecompressPassthroughWithoutEncoding(t *testing.T) {
	raw := `{"model":"gpt-4o"}`
	w := runDecompress(t, []byte(raw), "")
	if w.Body.String() != raw {
		t.Fatalf("expected passthrough, got %q", w.Body.String())
	}
}

func TestDecompressGzip(t *testing.T) {
	raw := `{"model":"gpt-4o"}`
	w := runDecompress(t, gzipBytes(t, []byte(raw)), "gzip")
	if w.Body.String() != raw {
		t.Fatalf("gzip decode failed, got %q", w.Body.String())
	}
}

func TestDecompressDeflate(t *testing.T) {
	raw := `{"model":"gpt-4o"}`
	w := runDecompress(t, zlibBytes(t, []byte(raw)), "deflate")
	if w.Body.String() != raw {
		t.Fatalf("deflate decode failed, got %q", w.Body.String())
	}
}

func TestDecompressBrotli(t *testing.T) {
	raw := `{"model":"gpt-4o"}`
	w := runDecompress(t, brotliBytes(t, []byte(raw)), "br")
	if w.Body.String() != raw {
		t.Fatalf("br decode failed, got %q", w.Body.String())
	}
}

func TestDecompressZstd(t *testing.T) {
	raw := `{"model":"gpt-4o"}`
	w := runDecompress(t, zstdBytes(t, []byte(raw)), "zstd")
	if w.Body.String() != raw {
		t.Fatalf("zstd decode failed, got %q", w.Body.String())
	}
}

func TestDecompressUnsupportedEncoding(t *testing.T) {
	w := runDecompress(t, []byte("data"), "compress")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestDecompressCorruptBody(t *testing.T) {
	w := runDecompress(t, []byte("not-gzip-data"), "gzip")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}
