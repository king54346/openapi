package common

import (
	"time"

	"openapi/constant"

	gin "github.com/king54346/gin-tiny"
)

// 请求上下文读写的类型化封装。请求体相关见 request_body.go。

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
