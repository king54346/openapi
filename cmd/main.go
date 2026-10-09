package main

import (
	"context"
	"crypto/rand"
	"embed"
	"fmt"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"openapi/common"
	"openapi/common/system"
	"openapi/controller"
	"openapi/database"
	"openapi/logger"
	"openapi/middleware"
	"openapi/model"
	"openapi/router"
	"openapi/service"
	"openapi/service/cron"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/joho/godotenv"
	gin "github.com/king54346/gin-tiny"
	"github.com/king54346/gin-tiny/middleware/sessions"
	"github.com/king54346/gin-tiny/middleware/sessions/cookie"
	"gorm.io/gorm"
)

// 暂无内置前端：buildFS 为空、indexPage 为 nil 时，
// 未匹配的 API 路径返回 JSON 404；也可通过 FRONTEND_BASE_URL 重定向到外部前端。
var (
	buildFS   embed.FS
	indexPage []byte
)

func main() {
	startTime := time.Now()

	db, logDB, err := InitResources()
	if err != nil {
		common.FatalLog("failed to initialize resources: " + err.Error())
	}
	defer database.Close(db)
	defer database.Close(logDB)

	common.SysLog("OpenAPI " + common.Version + " started")
	if os.Getenv("GIN_MODE") != "debug" {
		gin.SetMode(gin.ReleaseMode)
	}
	if common.DebugEnabled {
		common.SysLog("running in debug mode")
	}
	if !common.IsMasterNode {
		common.SysLog("running as slave node: migrations, task polling and cron are disabled by default")
	}

	if common.GetEnvOrDefaultBool("ENABLE_PPROF", false) {
		pprofAddr := "0.0.0.0:" + common.GetEnvOrDefaultString("PPROF_PORT", "8005")
		gopool.Go(func() {
			log.Println(http.ListenAndServe(pprofAddr, nil))
		})
		go system.Monitor()
		common.SysLog("pprof enabled on " + pprofAddr)
	}

	if err := system.StartPyroScope(); err != nil {
		common.SysError(fmt.Sprintf("start pyroscope error: %v", err))
	}

	server := newServer()

	port := common.GetEnvOrDefaultString("PORT", "3000")
	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		common.SysError("failed to listen on :" + port + ": " + err.Error())
		return
	}
	common.SysLog(fmt.Sprintf("server listening on :%s (startup took %s)", port, time.Since(startTime).Round(time.Millisecond)))

	// 收到 Ctrl+C / SIGTERM 后优雅关闭：停止接收新连接，等待进行中的请求完成
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := runServer(ctx, server, listener); err != nil {
		common.SysError("HTTP server error: " + err.Error())
		return
	}
	common.SysLog("server stopped")
}

// newServer 创建 HTTP 服务并注册中间件与全部路由。
// 需在 InitResources（logger.SetupLogger）之后调用，请求日志才会写入文件。
func newServer() *gin.Engine {
	server := gin.New()
	server.Use(gin.CustomRecovery(func(c gin.Context, err any) {
		common.SysError(fmt.Sprintf("panic detected: %v", err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"message": fmt.Sprintf("panic detected, error: %v", err),
				"type":    "openapi_panic",
			},
		})
	}))
	server.Use(gin.Logger())
	server.Use(middleware.RequestId())
	// 管理接口鉴权（UserAuth/AdminAuth）依赖 session，必须注册
	server.Use(sessions.Sessions("session", newSessionStore()))

	router.SetRouter(server, buildFS, indexPage)
	return server
}

// runServer 在 listener 上提供服务，ctx 取消后优雅关闭并返回 nil。
func runServer(ctx context.Context, server *gin.Engine, listener net.Listener) error {
	return server.RunListenerContext(ctx, listener)
}

// InitResources 加载配置并初始化日志、HTTP 客户端、数据库、Redis 与系统监控。
// 返回业务库与日志库连接；未配置 LOG_SQL_DSN 时 logDB 为 nil（与业务库共用）。
func InitResources() (*gorm.DB, *gorm.DB, error) {
	if err := godotenv.Load(".env"); err != nil && common.DebugEnabled {
		common.SysLog("no .env file found, using environment variables")
	}

	// 加载环境变量
	common.InitEnv()

	if err := logger.SetupLogger(os.Getenv("LOG_DIR")); err != nil {
		return nil, nil, fmt.Errorf("setup logger: %w", err)
	}

	service.InitHttpClient()

	// 请求体磁盘缓存：读取配置，并清理上次异常退出残留的缓存文件
	system.LoadDiskCacheConfigFromEnv()
	if err := system.CleanupOldDiskCacheFiles(5 * time.Minute); err != nil {
		common.SysError("failed to cleanup old disk cache files: " + err.Error())
	}

	// 数据库：SQL_DSN 选择 SQLite / MySQL / PostgreSQL，LOG_SQL_DSN 可把日志单独存库
	mainDB, logsDB, err := database.Open(database.ConfigFromEnv())
	if err != nil {
		return nil, nil, err
	}
	fail := func(err error) (*gorm.DB, *gorm.DB, error) {
		database.Close(mainDB)
		database.Close(logsDB)
		return nil, nil, err
	}
	if err := model.InitDB(mainDB, logsDB); err != nil {
		return fail(fmt.Errorf("migrate database: %w", err))
	}

	// 渠道缓存：启动时加载，之后每 SYNC_FREQUENCY 秒从数据库重建
	if err := model.InitChannelCache(); err != nil {
		return fail(fmt.Errorf("load channel cache: %w", err))
	}
	go model.SyncChannelCache(common.SyncFrequency)

	// 异步任务（视频等）后台轮询；UPDATE_TASK 默认只在 master 节点开启
	controller.StartTaskPolling()

	// 通用定时任务（如 channel_test 定时测试渠道）；CRON_ENABLED 默认只在 master 节点开启
	controller.RegisterCronExecutors()
	if common.GetEnvOrDefaultBool("CRON_ENABLED", common.IsMasterNode) {
		cron.StartScheduler()
	} else {
		common.SysLog("cron scheduler disabled (CRON_ENABLED=false or slave node)")
	}

	// Redis：REDIS_CONN_STRING 或 REDIS_ADDR 都未配置时保持禁用，走内存限流
	if err := common.InitRedisClient(); err != nil {
		common.SysError("failed to init redis: " + err.Error() + ", falling back to in-memory rate limiter")
	}

	// 启动系统监控（供 SystemPerformanceCheck 中间件使用）
	system.StartSystemMonitor()

	return mainDB, logsDB, nil
}

// newSessionStore 创建 cookie session 存储。未设置 SESSION_SECRET 时使用随机密钥，
// 重启后已登录的 session 全部失效，多实例部署时必须显式配置同一个密钥。
func newSessionStore() sessions.Store {
	secret := os.Getenv("SESSION_SECRET")
	if secret == "" {
		secret = rand.Text() + rand.Text()
		common.SysLog("SESSION_SECRET not set, using a random secret; sessions will not survive restarts")
	}
	store := cookie.NewStore([]byte(secret))
	store.Options(sessions.Options{
		Path:     "/",
		MaxAge:   2592000, // 30 天
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteStrictMode,
	})
	return store
}
