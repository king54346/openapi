package common

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"

	"openapi/constant"

	gin "github.com/king54346/gin-tiny"
)

// 请求体在请求上下文中的统一入口：读取（首次读取后缓存为 BodyStorage）、大小限制、
// 重复读取、按 Content-Type 解析，以及请求结束时的清理（BodyStorageCleanup 中间件调用 CleanupBodyStorage）。

// KeyBodyStorage 请求上下文中存放 BodyStorage 的键
const KeyBodyStorage = "key_body_storage"

const keyOriginalMultipartContentType = "_original_multipart_ct"

// defaultMaxRequestBodyMB 未配置 MAX_REQUEST_BODY_MB 时的上限
const defaultMaxRequestBodyMB = 128

// MaxRequestBodyBytes 请求体大小上限（MAX_REQUEST_BODY_MB，默认 128MB）。
func MaxRequestBodyBytes() int64 {
	maxMB := constant.MaxRequestBodyMB
	if maxMB <= 0 {
		maxMB = defaultMaxRequestBodyMB
	}
	return int64(maxMB) << 20
}

// IsRequestBodyTooLargeError 是否为请求体超限错误（含 http.MaxBytesReader 的错误）。
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

// GetBodyStorage 当前请求的请求体存储。首次调用时读取原始 Body 并缓存，之后直接复用；
// 每次调用都会把 c.Request().Body 重置为从头读取，直接读 Body 的代码也能拿到完整数据。
func GetBodyStorage(c gin.Context) (BodyStorage, error) {
	if v, ok := c.Get(KeyBodyStorage); ok && v != nil {
		if storage, ok := v.(BodyStorage); ok {
			if err := resetRequestBody(c, storage); err != nil {
				return nil, err
			}
			return storage, nil
		}
	}

	req := c.Request()
	if req.Body == nil || req.Body == http.NoBody {
		req.Body = http.NoBody
	}
	maxBytes := MaxRequestBodyBytes()
	storage, err := NewBodyStorage(req.Body, req.ContentLength, maxBytes)
	_ = req.Body.Close()
	if err != nil {
		if IsRequestBodyTooLargeError(err) {
			return nil, fmt.Errorf("%w: request body exceeds %d MB", ErrRequestBodyTooLarge, maxBytes>>20)
		}
		return nil, err
	}
	c.Set(KeyBodyStorage, storage)
	if err := resetRequestBody(c, storage); err != nil {
		return nil, err
	}
	return storage, nil
}

// GetRequestBody 完整请求体，同一请求内可多次调用；返回的切片只读，不要修改。
func GetRequestBody(c gin.Context) ([]byte, error) {
	storage, err := GetBodyStorage(c)
	if err != nil {
		return nil, err
	}
	return storage.Bytes()
}

// resetRequestBody 让 c.Request().Body 从头读取存储中的数据。
func resetRequestBody(c gin.Context, storage BodyStorage) error {
	reader, err := storage.NewReader()
	if err != nil {
		return err
	}
	c.Request().Body = io.NopCloser(reader)
	return nil
}

// CleanupBodyStorage 释放请求体存储（磁盘存储会删除临时文件），请求结束时调用，可重复调用。
func CleanupBodyStorage(c gin.Context) {
	v, ok := c.Get(KeyBodyStorage)
	if !ok || v == nil {
		return
	}
	if storage, ok := v.(BodyStorage); ok {
		if err := storage.Close(); err != nil {
			SysError("failed to cleanup request body storage: " + err.Error())
		}
	}
	c.Set(KeyBodyStorage, nil)
}

// UnmarshalBodyReusable 按 Content-Type 把请求体解析到 v（JSON / 表单 / multipart），不消耗请求体。
func UnmarshalBodyReusable(c gin.Context, v any) error {
	contentType := c.Request().Header.Get("Content-Type")
	if strings.Contains(contentType, gin.MIMEMultipartPOSTForm) {
		return parseMultipartFormData(c, v)
	}
	requestBody, err := GetRequestBody(c)
	if err != nil {
		return err
	}
	if strings.Contains(contentType, gin.MIMEPOSTForm) {
		return parseFormData(requestBody, v)
	}
	// application/json、+json、未声明类型等都按 JSON 解析
	return Unmarshal(requestBody, v)
}

// originalMultipartContentType 首次解析时的 Content-Type，
// 避免调用方重建 multipart 后改写了请求头导致 boundary 对不上。
func originalMultipartContentType(c gin.Context) string {
	if saved, ok := c.Get(keyOriginalMultipartContentType); ok {
		return saved.(string)
	}
	contentType := c.Request().Header.Get("Content-Type")
	c.Set(keyOriginalMultipartContentType, contentType)
	return contentType
}

// ParseMultipartFormReusable 解析 multipart 表单，不消耗请求体；文件部分超过内存上限时由标准库写入临时文件，
// 调用方用完后应调用 form.RemoveAll()。
func ParseMultipartFormReusable(c gin.Context) (*multipart.Form, error) {
	boundary, err := parseBoundary(originalMultipartContentType(c))
	if err != nil {
		return nil, err
	}
	return readMultipartForm(c, boundary)
}

// readMultipartForm 从请求体存储流式解析 multipart，磁盘存储时不会把整份数据读进内存。
func readMultipartForm(c gin.Context, boundary string) (*multipart.Form, error) {
	storage, err := GetBodyStorage(c)
	if err != nil {
		return nil, err
	}
	reader, err := storage.NewReader()
	if err != nil {
		return nil, err
	}
	return multipart.NewReader(reader, boundary).ReadForm(multipartMemoryLimit())
}

func processFormMap(formMap map[string]any, v any) error {
	jsonData, err := Marshal(formMap)
	if err != nil {
		return err
	}
	return Unmarshal(jsonData, v)
}

// formValuesToMap 单值字段取字符串，多值字段取切片。
func formValuesToMap(values map[string][]string) map[string]any {
	formMap := make(map[string]any, len(values))
	for key, vals := range values {
		if len(vals) == 1 {
			formMap[key] = vals[0]
		} else {
			formMap[key] = vals
		}
	}
	return formMap
}

func parseFormData(data []byte, v any) error {
	values, err := url.ParseQuery(string(data))
	if err != nil {
		return err
	}
	return processFormMap(formValuesToMap(values), v)
}

func parseMultipartFormData(c gin.Context, v any) error {
	boundary, err := parseBoundary(originalMultipartContentType(c))
	if err != nil {
		if errors.Is(err, errBoundaryNotFound) {
			// 声明了 multipart 却没有 boundary 时按 JSON 解析
			requestBody, err := GetRequestBody(c)
			if err != nil {
				return err
			}
			return Unmarshal(requestBody, v)
		}
		return err
	}
	form, err := readMultipartForm(c, boundary)
	if err != nil {
		return err
	}
	defer form.RemoveAll()
	return processFormMap(formValuesToMap(form.Value), v)
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

// multipartMemoryLimit multipart 解析时允许使用的内存上限（字节），超出部分由标准库写入临时文件
func multipartMemoryLimit() int64 {
	limitMB := constant.MaxFileDownloadMB
	if limitMB <= 0 {
		limitMB = 32
	}
	return int64(limitMB) << 20
}
