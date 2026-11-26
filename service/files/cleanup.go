package files

import (
	"openapi/common"
	"openapi/constant"
	"openapi/logger"
	"os"
	"strings"
	"sync"

	gin "github.com/king54346/gin-tiny"
)

// 请求期间落盘的临时文件（如从 URL 下载的音频/图片）在请求结束时统一清理，
// 避免异常退出或提前返回时残留。

// fileSourceRegistry 一次请求登记的待清理文件

type fileSourceRegistry struct {
	mu    sync.Mutex
	paths []string
}

// registryInitMu 保护上下文里的登记表懒加载，避免并发创建多份

var registryInitMu sync.Mutex

// fileSourceRegistryOf 取（必要时创建）当前请求的登记表。

func fileSourceRegistryOf(c gin.Context) *fileSourceRegistry {
	if c == nil {
		return nil
	}
	registryInitMu.Lock()
	defer registryInitMu.Unlock()
	if value, ok := common.GetContextKey(c, constant.ContextKeyFileSourcesToCleanup); ok {
		if registry, ok := value.(*fileSourceRegistry); ok {
			return registry
		}
	}
	registry := &fileSourceRegistry{}
	common.SetContextKey(c, constant.ContextKeyFileSourcesToCleanup, registry)
	return registry
}

// RegisterTempFile 登记一个请求结束后需要删除的本地文件。
// path 为空或已登记过时忽略；文件不存在不影响后续清理。

func RegisterTempFile(c gin.Context, path string) {
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}
	registry := fileSourceRegistryOf(c)
	if registry == nil {
		return
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	for _, existing := range registry.paths {
		if existing == path {
			return
		}
	}
	registry.paths = append(registry.paths, path)
}

// CleanupFileSources 删除本次请求登记过的临时文件。可重复调用。

func CleanupFileSources(c gin.Context) {
	if c == nil {
		return
	}
	value, ok := common.GetContextKey(c, constant.ContextKeyFileSourcesToCleanup)
	if !ok {
		return
	}
	registry, ok := value.(*fileSourceRegistry)
	if !ok || registry == nil {
		return
	}
	// 先把上下文里的登记表置空，重复调用不会重复删除
	c.Set(string(constant.ContextKeyFileSourcesToCleanup), nil)

	registry.mu.Lock()
	paths := registry.paths
	registry.paths = nil
	registry.mu.Unlock()

	for _, path := range paths {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			logger.LogError(c, "failed to remove temp file "+path+": "+err.Error())
		}
	}
}
