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
	httpclient "openapi/service/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"openapi/common"
	"openapi/common/system"
	"openapi/database"
	"openapi/logger"
	"openapi/middleware"
	"openapi/model"
	"openapi/router"
	cronsvc "openapi/service/cron"
	tasksvc "openapi/service/task"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/joho/godotenv"
	gin "github.com/king54346/gin-tiny"
	"github.com/king54346/gin-tiny/middleware/sessions"
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
	defer common.StopClusterEventBus()

	common.SysLog("OpenAPI " + common.Version + " started")
	if os.Getenv("GIN_MODE") != "debug" {
		gin.SetMode(gin.ReleaseMode)
	}
	if common.DebugEnabled {
		common.SysLog("running in debug mode")
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

	httpclient.InitHttpClient()

	// Redis 优先初始化：连接成功后启用集群模式（Session、Pub/Sub 缓存同步、跨节点亲和性、分布式锁与限流）；未配置则降级单机内存
	if err := common.InitRedisClient(); err != nil {
		common.SysError("failed to init redis: " + err.Error() + ", falling back to local memory mode")
	}
	if common.RedisEnabled {
		common.StartClusterEventBus(context.Background())
	}

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
		common.StopClusterEventBus()
		database.Close(mainDB)
		database.Close(logsDB)
		return nil, nil, err
	}
	if err := model.InitDB(mainDB, logsDB); err != nil {
		return fail(fmt.Errorf("migrate database: %w", err))
	}

	// 系统配置：options 表的键值同步到运行时配置（公告、Turnstile、亲和性开关等）
	if err := model.LoadOptionsFromDB(); err != nil {
		return fail(fmt.Errorf("load options: %w", err))
	}

	// 渠道缓存：启动时加载，启用 Redis 时通过 Pub/Sub 实时同步变更并以定时同步兜底
	if err := model.InitChannelCache(); err != nil {
		return fail(fmt.Errorf("load channel cache: %w", err))
	}
	go model.SyncChannelCache(common.SyncFrequency)

	// 异步任务（视频等）后台轮询；多实例启用 Redis 时通过分布式锁自动防重
	tasksvc.StartTaskPolling()

	// 通用定时任务（如 channel_test 定时测试渠道）；多实例启用 Redis 时通过分布式锁自动防重
	cronsvc.RegisterCronExecutors()
	if common.GetEnvOrDefaultBool("CRON_ENABLED", true) {
		cronsvc.StartScheduler()
	} else {
		common.SysLog("cron scheduler disabled (CRON_ENABLED=false)")
	}

	// 启动系统监控（供 SystemPerformanceCheck 中间件使用）
	system.StartSystemMonitor()

	return mainDB, logsDB, nil
}

// newSessionStore 创建会话存储：启用 Redis 时使用服务端可控的 RedisSessionStore（支持踢下线），否则回退 CookieStore。
func newSessionStore() sessions.Store {
	secret := os.Getenv("SESSION_SECRET")
	if secret == "" {
		secret = rand.Text() + rand.Text()
		common.SysLog("SESSION_SECRET not set, using a random secret; sessions will not survive restarts")
	}
	return middleware.NewSessionStore([]byte(secret), sessions.Options{
		Path:     "/",
		MaxAge:   2592000, // 30 天
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteStrictMode,
	})
}
