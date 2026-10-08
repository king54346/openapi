package router

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"

	"openapi/common"

	gin "github.com/king54346/gin-tiny"
)

// frontendBaseURLEnv 外部前端地址的环境变量名
const frontendBaseURLEnv = "FRONTEND_BASE_URL"

// webFSCandidates buildFS 内前端产物的可能子目录（按优先级排序）
var webFSCandidates = []string{"dist", "web/dist", "build", "web/build", "frontend/dist", "public"}

// apiPathPrefixes 命中这些前缀的未匹配请求返回 JSON 404，不走 SPA 回退
var apiPathPrefixes = []string{"/api", "/v1", "/pg", "/kling", "/jimeng"}

// SetRouter 配置所有路由
// 这是路由配置的入口函数，负责注册所有模块的路由
// 参数:
//   - router: Gin 引擎实例
//   - buildFS: 嵌入的前端静态文件系统
//   - indexPage: 前端入口页面内容（SPA 回退页，可为空）
func SetRouter(router *gin.Engine, buildFS embed.FS, indexPage []byte) {
	// 注册 API 路由（管理接口、用户接口、系统接口等）
	SetApiRouter(router)

	// 注册中继路由（AI 模型请求转发）
	SetRelayRouter(router)

	// 注册视频生成路由（视频生成任务接口）
	SetVideoRouter(router)

	// 注册前端静态资源路由（嵌入前端或重定向到外部前端）
	setupFrontend(router, buildFS, indexPage)
}

// setupFrontend 根据 FRONTEND_BASE_URL 环境变量决定前端 serving 方式：
//   - 为空：使用编译进二进制的前端静态文件（含 SPA 回退）
//   - 非空：所有未匹配路由 302 重定向到外部前端地址（保留原始 RequestURI）
func setupFrontend(router *gin.Engine, buildFS embed.FS, indexPage []byte) {
	frontendBaseURL := strings.TrimSuffix(strings.TrimSpace(os.Getenv(frontendBaseURLEnv)), "/")
	if frontendBaseURL == "" {
		SetWebRouter(router, buildFS, indexPage)
		return
	}

	common.SysLog(fmt.Sprintf("using external frontend: %s", frontendBaseURL))
	router.NoRoute(func(c gin.Context) {
		// 将所有未匹配的请求重定向到外部前端
		c.Redirect(http.StatusFound, frontendBaseURL+c.Request().RequestURI)
	})
}

// SetWebRouter 注册嵌入式前端静态资源路由：
//
//   - 已存在的文件（js/css/图片等）由 StaticFS 直接 serving
//   - "/" 返回 indexPage（支持启动时注入配置后的入口页）
//   - 其他未匹配的非 API 路径回退到 indexPage（SPA 前端路由）
//   - 未匹配的 API 路径返回 JSON 404，避免把 HTML 当接口数据返回
func SetWebRouter(router *gin.Engine, buildFS embed.FS, indexPage []byte) {
	if webFS, err := resolveWebFS(buildFS); err != nil {
		common.SysLog(fmt.Sprintf("embedded frontend not found: %v", err))
	} else {
		router.StaticFS("/", http.FS(webFS))
	}

	if len(indexPage) > 0 {
		router.GET("/", func(c gin.Context) {
			serveIndexPage(c, indexPage)
		})
	}

	router.NoRoute(func(c gin.Context) {
		if isAPIPath(c.Request().URL.Path) {
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "not_found",
				"message": "route not found: " + c.Request().URL.Path,
			})
			return
		}
		if len(indexPage) > 0 {
			serveIndexPage(c, indexPage)
			return
		}
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "not_found",
			"message": "page not found",
		})
	})
}

// resolveWebFS 在 buildFS 中定位前端产物根目录，兼容多种嵌入布局
func resolveWebFS(buildFS embed.FS) (fs.FS, error) {
	if hasEmbedFile(buildFS, "index.html") {
		return buildFS, nil
	}
	for _, dir := range webFSCandidates {
		if hasEmbedFile(buildFS, path.Join(dir, "index.html")) {
			return fs.Sub(buildFS, dir)
		}
	}
	return nil, fmt.Errorf("index.html not found in embedded FS")
}

// hasEmbedFile 检查嵌入文件系统中是否存在指定文件
func hasEmbedFile(buildFS embed.FS, name string) bool {
	f, err := buildFS.Open(name)
	if err != nil {
		return false
	}
	defer f.Close()
	info, err := f.Stat()
	return err == nil && !info.IsDir()
}

// serveIndexPage 返回 SPA 入口页，禁用缓存以保证前端升级后即时生效
func serveIndexPage(c gin.Context, indexPage []byte) {
	c.Header("Cache-Control", "no-cache")
	c.Data(http.StatusOK, "text/html; charset=utf-8", indexPage)
}

// isAPIPath 判断是否为接口路径前缀，这类 404 必须返回 JSON 而不是 HTML
func isAPIPath(requestPath string) bool {
	for _, prefix := range apiPathPrefixes {
		if requestPath == prefix || strings.HasPrefix(requestPath, prefix+"/") {
			return true
		}
	}
	return false
}
