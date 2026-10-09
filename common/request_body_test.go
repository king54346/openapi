package common

import (
	"bytes"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"openapi/constant"

	gin "github.com/king54346/gin-tiny"
)

func newBodyContext(t *testing.T, body io.Reader, contentType string) gin.Context {
	t.Helper()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	req := httptest.NewRequest(http.MethodPost, "/v1/test", body)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	c.SetRequest(req)
	t.Cleanup(func() { CleanupBodyStorage(c) })
	return c
}

func TestGetRequestBodyReusable(t *testing.T) {
	c := newBodyContext(t, strings.NewReader(`{"model":"gpt-4o","n":1}`), "application/json")

	first, err := GetRequestBody(c)
	if err != nil {
		t.Fatal(err)
	}
	second, _ := GetRequestBody(c)
	if string(first) != `{"model":"gpt-4o","n":1}` || &first[0] != &second[0] {
		t.Fatal("repeated reads should return the same cached data")
	}

	// 每次读取后 c.Request().Body 都被重置，直接读 Body 也能拿到完整数据
	for i := 0; i < 2; i++ {
		raw, _ := io.ReadAll(c.Request().Body)
		if string(raw) != string(first) {
			t.Fatalf("raw body read %d: %q", i, raw)
		}
		_, _ = GetBodyStorage(c)
	}

	var req struct {
		Model string `json:"model"`
		N     int    `json:"n"`
	}
	for i := 0; i < 2; i++ {
		if err := UnmarshalBodyReusable(c, &req); err != nil || req.Model != "gpt-4o" || req.N != 1 {
			t.Fatalf("unmarshal %d: %+v %v", i, req, err)
		}
	}

	CleanupBodyStorage(c)
	CleanupBodyStorage(c) // 可重复调用
	if v, _ := c.Get(KeyBodyStorage); v != nil {
		t.Fatal("storage should be removed from context")
	}
}

func TestGetRequestBodyTooLarge(t *testing.T) {
	old := constant.MaxRequestBodyMB
	constant.MaxRequestBodyMB = 1
	t.Cleanup(func() { constant.MaxRequestBodyMB = old })

	c := newBodyContext(t, bytes.NewReader(make([]byte, 1<<20+1)), "application/json")
	_, err := GetRequestBody(c)
	if !IsRequestBodyTooLargeError(err) || !strings.Contains(err.Error(), "1 MB") {
		t.Fatalf("too large: %v", err)
	}
	if MaxRequestBodyBytes() != 1<<20 {
		t.Fatalf("max bytes: %d", MaxRequestBodyBytes())
	}
	constant.MaxRequestBodyMB = 0
	if MaxRequestBodyBytes() != defaultMaxRequestBodyMB<<20 {
		t.Fatal("default limit should apply when not configured")
	}
}

func TestEmptyBody(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Body = nil
	c.SetRequest(req)
	body, err := GetRequestBody(c)
	if err != nil || len(body) != 0 {
		t.Fatalf("empty body: %q %v", body, err)
	}
	CleanupBodyStorage(c)
}

func TestUnmarshalFormBody(t *testing.T) {
	c := newBodyContext(t, strings.NewReader("model=whisper-1&tags=a&tags=b"), gin.MIMEPOSTForm)
	var req struct {
		Model string   `json:"model"`
		Tags  []string `json:"tags"`
	}
	if err := UnmarshalBodyReusable(c, &req); err != nil || req.Model != "whisper-1" || len(req.Tags) != 2 {
		t.Fatalf("form: %+v %v", req, err)
	}
}

func multipartBody(t *testing.T) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("model", "whisper-1")
	part, _ := w.CreateFormFile("file", "a.mp3")
	_, _ = part.Write([]byte("audio-bytes"))
	_ = w.Close()
	return &buf, w.FormDataContentType()
}

func TestMultipartBodyReusable(t *testing.T) {
	for _, disk := range []bool{false, true} {
		if disk {
			useFakeDiskCache(t, 1) // 全部落盘，验证从磁盘流式解析
		}
		body, contentType := multipartBody(t)
		raw := body.String()
		c := newBodyContext(t, body, contentType)

		var req struct {
			Model string `json:"model"`
		}
		if err := UnmarshalBodyReusable(c, &req); err != nil || req.Model != "whisper-1" {
			t.Fatalf("disk=%v unmarshal multipart: %+v %v", disk, req, err)
		}
		form, err := ParseMultipartFormReusable(c)
		if err != nil {
			t.Fatalf("disk=%v parse form: %v", disk, err)
		}
		fh := form.File["file"][0]
		f, _ := fh.Open()
		content, _ := io.ReadAll(f)
		_ = f.Close()
		_ = form.RemoveAll()
		if string(content) != "audio-bytes" {
			t.Fatalf("disk=%v file content: %q", disk, content)
		}
		// 解析后原始请求体仍然完整可读
		if got, _ := GetRequestBody(c); string(got) != raw {
			t.Fatalf("disk=%v body changed after parsing", disk)
		}
		storage, _ := GetBodyStorage(c)
		if storage.IsDisk() != disk {
			t.Fatalf("disk=%v storage.IsDisk=%v", disk, storage.IsDisk())
		}
	}
}

func TestMultipartWithoutBoundaryFallsBackToJSON(t *testing.T) {
	c := newBodyContext(t, strings.NewReader(`{"model":"x"}`), "multipart/form-data")
	var req struct {
		Model string `json:"model"`
	}
	if err := UnmarshalBodyReusable(c, &req); err != nil || req.Model != "x" {
		t.Fatalf("fallback json: %+v %v", req, err)
	}
	if _, err := ParseMultipartFormReusable(c); !errors.Is(err, errBoundaryNotFound) {
		t.Fatalf("parse form without boundary: %v", err)
	}
}
