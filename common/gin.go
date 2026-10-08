package common

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"

	"openapi/constant"

	gin "github.com/king54346/gin-tiny"
)

const KeyRequestBody = "key_request_body"

const keyOriginalMultipartContentType = "_original_multipart_ct"

var ErrRequestBodyTooLarge = errors.New("request body too large")

// CleanupBodyStorage 清理请求体磁盘/内存缓存（桩实现：无操作）。
func CleanupBodyStorage(c gin.Context) {}

func IsRequestBodyTooLargeError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrRequestBodyTooLarge) {
		return true
	}
	var mbe *http.MaxBytesError
	return errors.As(err, &mbe)
}

// getRequestBodyBytes 读取并缓存请求体，同一请求内多次调用返回同一份数据
func getRequestBodyBytes(c gin.Context) ([]byte, error) {
	if cached, ok := c.Get(KeyRequestBody); ok && cached != nil {
		if b, ok := cached.([]byte); ok {
			return b, nil
		}
	}

	maxMB := constant.MaxRequestBodyMB
	if maxMB <= 0 {
		maxMB = 128
	}
	maxBytes := int64(maxMB) << 20

	req := c.Request()
	body, err := io.ReadAll(io.LimitReader(req.Body, maxBytes+1))
	_ = req.Body.Close()
	if err != nil {
		if IsRequestBodyTooLargeError(err) {
			return nil, fmt.Errorf("%w: request body exceeds %d MB", ErrRequestBodyTooLarge, maxMB)
		}
		return nil, err
	}
	if int64(len(body)) > maxBytes {
		return nil, fmt.Errorf("%w: request body exceeds %d MB", ErrRequestBodyTooLarge, maxMB)
	}
	c.Set(KeyRequestBody, body)
	return body, nil
}

// resetRequestBody 让后续读取 c.Request().Body 的代码仍能读到完整请求体
func resetRequestBody(c gin.Context, body []byte) {
	c.Request().Body = io.NopCloser(bytes.NewReader(body))
}

// GetRequestBody 返回缓存的请求体，同一请求内可多次调用；调用方不要修改返回的切片
func GetRequestBody(c gin.Context) ([]byte, error) {
	return getRequestBodyBytes(c)
}

func UnmarshalBodyReusable(c gin.Context, v any) error {
	requestBody, err := getRequestBodyBytes(c)
	if err != nil {
		return err
	}
	contentType := c.Request().Header.Get("Content-Type")
	switch {
	case contentType == "",
		strings.HasPrefix(contentType, "application/json"),
		strings.HasSuffix(strings.Split(contentType, ";")[0], "+json"):
		err = Unmarshal(requestBody, v)
	case strings.Contains(contentType, gin.MIMEPOSTForm):
		err = parseFormData(requestBody, v)
	case strings.Contains(contentType, gin.MIMEMultipartPOSTForm):
		err = parseMultipartFormData(c, requestBody, v)
	default:
		err = Unmarshal(requestBody, v)
	}
	if err != nil {
		return err
	}
	resetRequestBody(c, requestBody)
	return nil
}

func SetContextKey(c gin.Context, key constant.ContextKey, value any) {
	c.Set(string(key), value)
}

func GetContextKey(c gin.Context, key constant.ContextKey) (any, bool) {
	return c.Get(string(key))
}

func GetContextKeyString(c gin.Context, key constant.ContextKey) string {
	return c.GetString(string(key))
}

func GetContextKeyInt(c gin.Context, key constant.ContextKey) int {
	return c.GetInt(string(key))
}

func GetContextKeyBool(c gin.Context, key constant.ContextKey) bool {
	return c.GetBool(string(key))
}

func GetContextKeyStringSlice(c gin.Context, key constant.ContextKey) []string {
	return c.GetStringSlice(string(key))
}

func GetContextKeyStringMap(c gin.Context, key constant.ContextKey) map[string]any {
	return c.GetStringMap(string(key))
}

func GetContextKeyTime(c gin.Context, key constant.ContextKey) time.Time {
	return c.GetTime(string(key))
}

func GetContextKeyType[T any](c gin.Context, key constant.ContextKey) (T, bool) {
	if value, ok := c.Get(string(key)); ok {
		if v, ok := value.(T); ok {
			return v, true
		}
	}
	var t T
	return t, false
}

// originalMultipartContentType 返回首次解析时的 Content-Type，
// 避免调用方重建 multipart 后改写了请求头导致 boundary 对不上
func originalMultipartContentType(c gin.Context) string {
	if saved, ok := c.Get(keyOriginalMultipartContentType); ok {
		return saved.(string)
	}
	contentType := c.Request().Header.Get("Content-Type")
	c.Set(keyOriginalMultipartContentType, contentType)
	return contentType
}

func ParseMultipartFormReusable(c gin.Context) (*multipart.Form, error) {
	requestBody, err := getRequestBodyBytes(c)
	if err != nil {
		return nil, err
	}
	boundary, err := parseBoundary(originalMultipartContentType(c))
	if err != nil {
		return nil, err
	}

	reader := multipart.NewReader(bytes.NewReader(requestBody), boundary)
	form, err := reader.ReadForm(multipartMemoryLimit())
	if err != nil {
		return nil, err
	}
	resetRequestBody(c, requestBody)
	return form, nil
}

func processFormMap(formMap map[string]any, v any) error {
	jsonData, err := Marshal(formMap)
	if err != nil {
		return err
	}
	return Unmarshal(jsonData, v)
}

func parseFormData(data []byte, v any) error {
	values, err := url.ParseQuery(string(data))
	if err != nil {
		return err
	}
	formMap := make(map[string]any)
	for key, vals := range values {
		if len(vals) == 1 {
			formMap[key] = vals[0]
		} else {
			formMap[key] = vals
		}
	}
	return processFormMap(formMap, v)
}

func parseMultipartFormData(c gin.Context, data []byte, v any) error {
	boundary, err := parseBoundary(originalMultipartContentType(c))
	if err != nil {
		if errors.Is(err, errBoundaryNotFound) {
			return Unmarshal(data, v) // 没有 boundary 时按 JSON 解析
		}
		return err
	}

	reader := multipart.NewReader(bytes.NewReader(data), boundary)
	form, err := reader.ReadForm(multipartMemoryLimit())
	if err != nil {
		return err
	}
	defer form.RemoveAll()
	formMap := make(map[string]any)
	for key, vals := range form.Value {
		if len(vals) == 1 {
			formMap[key] = vals[0]
		} else {
			formMap[key] = vals
		}
	}
	return processFormMap(formMap, v)
}

var errBoundaryNotFound = errors.New("multipart boundary not found")

// parseBoundary 从 Content-Type 中解析 multipart boundary
func parseBoundary(contentType string) (string, error) {
	if contentType == "" {
		return "", errBoundaryNotFound
	}
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return "", err
	}
	boundary, ok := params["boundary"]
	if !ok || boundary == "" {
		return "", errBoundaryNotFound
	}
	return boundary, nil
}

// multipartMemoryLimit 返回 multipart 解析时允许使用的内存上限（字节）
func multipartMemoryLimit() int64 {
	limitMB := constant.MaxFileDownloadMB
	if limitMB <= 0 {
		limitMB = 32
	}
	return int64(limitMB) << 20
}
