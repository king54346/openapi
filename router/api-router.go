package router

import (
	channelctrl "openapi/controller/channel"
	cronctrl "openapi/controller/cron"
	logctrl "openapi/controller/log"
	metactrl "openapi/controller/modelmeta"
	perfctrl "openapi/controller/performance"
	prefillctrl "openapi/controller/prefill"
	systemctrl "openapi/controller/system"
	taskctrl "openapi/controller/task"
	tokenctrl "openapi/controller/token"
	unimplctrl "openapi/controller/unimplemented"
	userctrl "openapi/controller/user"
	vendorctrl "openapi/controller/vendor"
	"openapi/middleware"

	GinTiny "github.com/king54346/gin-tiny"
	"github.com/king54346/gin-tiny/middleware/cors"
)

// SetApiRouter 设置 API 路由
// 这是管理后台和系统管理的核心路由，包括：
//   - 用户管理：注册、登录、个人信息、权限管理
//   - 渠道管理：渠道 CRUD、测试、余额查询
//   - 令牌管理：API Key 的创建、删除、统计
//   - 日志系统：请求日志、消费记录
//   - 系统设置：参数配置、性能监控
func SetApiRouter(router *GinTiny.Engine) {
	// ==================== 主 API 路由组 ====================
	apiRouter := router.Group("/api")
	apiRouter.Use(middleware.Compress())
	apiRouter.Use(middleware.BodyStorageCleanup())
	apiRouter.Use(middleware.GlobalAPIRateLimit())
	{
		// -------------------- 系统状态与配置接口 --------------------
		apiRouter.GET("/setup", systemctrl.GetSetup)
		apiRouter.POST("/setup", systemctrl.PostSetup)
		apiRouter.GET("/status", systemctrl.GetStatus)
		apiRouter.GET("/uptime/status", systemctrl.GetUptimeKumaStatus)
		apiRouter.GET("/models", middleware.UserAuth(), metactrl.DashboardListModels)
		apiRouter.GET("/status/test", middleware.AdminAuth(), systemctrl.TestStatus)

		// -------------------- 公开信息接口 --------------------
		apiRouter.GET("/notice", systemctrl.GetNotice)
		apiRouter.GET("/user-agreement", systemctrl.GetUserAgreement)
		apiRouter.GET("/privacy-policy", systemctrl.GetPrivacyPolicy)
		apiRouter.GET("/about", systemctrl.GetAbout)
		apiRouter.GET("/home_page_content", systemctrl.GetHomePageContent)
		apiRouter.GET("/pricing", middleware.TryUserAuth(), unimplctrl.GetPricing)

		// -------------------- 倍率配置（暂不实现） --------------------
		apiRouter.GET("/ratio_config", middleware.CriticalRateLimit(), unimplctrl.GetRatioConfig)

		// -------------------- 安全验证接口（暂不实现） --------------------
		apiRouter.POST("/verify", middleware.UserAuth(), middleware.CriticalRateLimit(), unimplctrl.UniversalVerify)
		apiRouter.GET("/verify/status", middleware.UserAuth(), unimplctrl.GetVerificationStatus)

		// ==================== 用户路由组 ====================
		userRoute := apiRouter.Group("/user")
		{
			userRoute.POST("/login", middleware.CriticalRateLimit(), middleware.TurnstileCheck(), userctrl.Login)
			userRoute.POST("/login/2fa", middleware.CriticalRateLimit(), unimplctrl.Verify2FALogin)
			userRoute.POST("/passkey/login/begin", middleware.CriticalRateLimit(), unimplctrl.PasskeyLoginBegin)
			userRoute.POST("/passkey/login/finish", middleware.CriticalRateLimit(), unimplctrl.PasskeyLoginFinish)
			userRoute.GET("/logout", userctrl.Logout)

			selfRoute := userRoute.Group("/")
			selfRoute.Use(middleware.UserAuth())
			{
				selfRoute.GET("/self", userctrl.GetSelf)
				selfRoute.GET("/models", userctrl.GetUserModels)
				selfRoute.PUT("/self", unimplctrl.UpdateSelf)
				selfRoute.DELETE("/self", unimplctrl.DeleteSelf)
				selfRoute.GET("/token", userctrl.GenerateAccessToken)
			}

			adminRoute := userRoute.Group("/")
			adminRoute.Use(middleware.AdminAuth())
			{
				adminRoute.GET("/", userctrl.GetAllUsers)
				adminRoute.GET("/topup", unimplctrl.GetAllTopUps)
				adminRoute.POST("/topup/complete", unimplctrl.AdminCompleteTopUp)
				adminRoute.GET("/search", userctrl.SearchUsers)
				adminRoute.GET("/:id", userctrl.GetUser)
				adminRoute.POST("/", userctrl.CreateUser)
				adminRoute.POST("/manage", userctrl.ManageUser)
				adminRoute.PUT("/", userctrl.UpdateUser)
				adminRoute.DELETE("/:id", userctrl.DeleteUser)
				adminRoute.DELETE("/:id/reset_passkey", unimplctrl.AdminResetPasskey)
				adminRoute.GET("/2fa/stats", unimplctrl.Admin2FAStats)
				adminRoute.DELETE("/:id/2fa", unimplctrl.AdminDisable2FA)
			}
		}

		// ==================== 订阅计费路由组（暂不实现） ====================
		subscriptionRoute := apiRouter.Group("/subscription")
		subscriptionRoute.Use(middleware.UserAuth())
		{
			subscriptionRoute.GET("/plans", unimplctrl.GetSubscriptionPlans)
			subscriptionRoute.GET("/self", unimplctrl.GetSubscriptionSelf)
			subscriptionRoute.PUT("/self/preference", unimplctrl.UpdateSubscriptionPreference)
		}

		// ==================== 系统配置路由组 ====================
		optionRoute := apiRouter.Group("/option")
		optionRoute.Use(middleware.RootAuth())
		{
			optionRoute.GET("/", systemctrl.GetOptions)
			optionRoute.PUT("/", systemctrl.UpdateOption)
			optionRoute.GET("/channel_affinity_cache", channelctrl.GetChannelAffinityCacheStats)
			optionRoute.DELETE("/channel_affinity_cache", channelctrl.ClearChannelAffinityCache)
			optionRoute.POST("/rest_model_ratio", unimplctrl.ResetModelRatio)
			optionRoute.GET("/peak_valley_time", unimplctrl.GetPeakValleyTimeInfo)
			optionRoute.POST("/migrate_console_setting", systemctrl.MigrateConsoleSetting)
		}

		// ==================== 性能监控路由组 ====================
		performanceRoute := apiRouter.Group("/performance")
		performanceRoute.Use(middleware.RootAuth())
		{
			performanceRoute.GET("/stats", perfctrl.GetPerformanceStats)
			performanceRoute.DELETE("/disk_cache", perfctrl.ClearDiskCache)
			performanceRoute.POST("/reset_stats", perfctrl.ResetPerformanceStats)
			performanceRoute.POST("/gc", perfctrl.ForceGC)
		}

		// ==================== 倍率同步路由组（暂不实现） ====================
		ratioSyncRoute := apiRouter.Group("/ratio_sync")
		ratioSyncRoute.Use(middleware.RootAuth())
		{
			ratioSyncRoute.GET("/channels", unimplctrl.GetSyncableChannels)
			ratioSyncRoute.POST("/fetch", unimplctrl.FetchUpstreamRatios)
		}

		// ==================== 渠道管理路由组 ====================
		channelRoute := apiRouter.Group("/channel")
		channelRoute.Use(middleware.AdminAuth())
		{
			channelRoute.GET("/", channelctrl.GetAllChannels)
			channelRoute.GET("/search", channelctrl.SearchChannels)
			channelRoute.GET("/models", metactrl.ChannelListModels)
			channelRoute.GET("/models_enabled", metactrl.EnabledListModels)
			channelRoute.GET("/:id", channelctrl.GetChannel)
			channelRoute.POST("/:id/key", middleware.RootAuth(), middleware.CriticalRateLimit(), middleware.DisableCache(), middleware.SecureVerificationRequired(), channelctrl.GetChannelKey)
			channelRoute.POST("/", channelctrl.AddChannel)
			channelRoute.PUT("/", channelctrl.UpdateChannel)
			channelRoute.DELETE("/:id", channelctrl.DeleteChannel)
			channelRoute.POST("/batch", channelctrl.DeleteChannelBatch)
			channelRoute.POST("/copy/:id", channelctrl.CopyChannel)

			channelRoute.GET("/test", channelctrl.TestAllChannels)
			channelRoute.GET("/test/:id", channelctrl.TestChannel)
			channelRoute.GET("/test/:id/all_models", channelctrl.TestChannelAllModels)
			channelRoute.GET("/update_balance", channelctrl.UpdateAllChannelsBalance)
			channelRoute.GET("/update_balance/:id", channelctrl.UpdateChannelBalance)

			channelRoute.GET("/test_records", channelctrl.GetChannelTestRecords)
			channelRoute.DELETE("/test_records", channelctrl.DeleteChannelTestRecords)
			channelRoute.DELETE("/test_records/all", channelctrl.ClearChannelTestRecords)
			channelRoute.POST("/test_records/:id/retest", channelctrl.RetestChannelModel)
			channelRoute.POST("/test_records/batch_retest", channelctrl.BatchRetestChannelModels)
			channelRoute.POST("/test_records/retest_filtered", channelctrl.RetestFilteredRecords)
			channelRoute.GET("/test_records/model_tags", metactrl.GetModelTags)
			channelRoute.DELETE("/:id/model", channelctrl.RemoveModelFromChannel)
			channelRoute.POST("/fix", channelctrl.FixChannelsAbilities)

			channelRoute.DELETE("/disabled", channelctrl.DeleteDisabledChannel)
			channelRoute.POST("/tag/disabled", channelctrl.DisableTagChannels)
			channelRoute.POST("/tag/enabled", channelctrl.EnableTagChannels)
			channelRoute.PUT("/tag", channelctrl.EditTagChannels)
			channelRoute.POST("/batch/tag", channelctrl.BatchSetChannelTag)
			channelRoute.GET("/tag/models", channelctrl.GetTagModels)

			channelRoute.GET("/fetch_models/:id", channelctrl.FetchUpstreamModels)
			channelRoute.POST("/fetch_models", channelctrl.FetchModels)
			channelRoute.POST("/multi_key/manage", channelctrl.ManageMultiKeys)
		}

		// ==================== 定时任务管理路由组 ====================
		cronJobRoute := apiRouter.Group("/cron_job")
		cronJobRoute.Use(middleware.AdminAuth())
		{
			cronJobRoute.GET("/", cronctrl.GetCronJobs)
			cronJobRoute.GET("/types", cronctrl.GetCronJobTypes)
			cronJobRoute.GET("/:id", cronctrl.GetCronJob)
			cronJobRoute.POST("/", cronctrl.CreateCronJob)
			cronJobRoute.PUT("/:id", cronctrl.UpdateCronJob)
			cronJobRoute.DELETE("/:id", cronctrl.DeleteCronJob)
			cronJobRoute.POST("/:id/enabled", cronctrl.ToggleCronJob)
			cronJobRoute.POST("/:id/run", cronctrl.RunCronJobNow)
			cronJobRoute.GET("/:id/runs", cronctrl.GetCronJobRuns)
		}

		// ==================== 令牌管理路由组 ====================
		tokenRoute := apiRouter.Group("/token")
		tokenRoute.Use(middleware.UserAuth())
		{
			tokenRoute.GET("/", tokenctrl.GetAllTokens)
			tokenRoute.GET("/search", tokenctrl.SearchTokens)
			tokenRoute.GET("/:id", tokenctrl.GetToken)
			tokenRoute.GET("/analytics/:id", tokenctrl.GetTokenAnalytics)
			tokenRoute.GET("/webstar_package", unimplctrl.DownloadWebStarPackage)
			tokenRoute.POST("/", tokenctrl.AddToken)
			tokenRoute.PUT("/", tokenctrl.UpdateToken)
			tokenRoute.DELETE("/:id", tokenctrl.DeleteToken)
			tokenRoute.POST("/batch", tokenctrl.DeleteTokenBatch)
		}

		// ==================== 令牌访问限制模板路由组 ====================
		tokenTemplateRoute := apiRouter.Group("/token_template")
		tokenTemplateRoute.Use(middleware.UserAuth())
		{
			tokenTemplateRoute.GET("/", tokenctrl.GetUserTokenAccessTemplates)
			tokenTemplateRoute.POST("/", tokenctrl.AddTokenAccessTemplate)
			tokenTemplateRoute.PUT("/", tokenctrl.UpdateTokenAccessTemplate)
			tokenTemplateRoute.DELETE("/:id", tokenctrl.DeleteTokenAccessTemplate)
		}

		// ==================== 使用情况查询路由组 ====================
		usageRoute := apiRouter.Group("/usage")
		usageRoute.Use(middleware.CriticalRateLimit())
		{
			tokenUsageRoute := usageRoute.Group("/token")
			tokenUsageRoute.Use(middleware.TokenAuth())
			{
				tokenUsageRoute.GET("/", tokenctrl.GetTokenUsage)
			}
		}

		// ==================== 日志系统路由组 ====================
		logRoute := apiRouter.Group("/log")
		logRoute.GET("/", middleware.AdminAuth(), logctrl.GetAllLogs)
		logRoute.DELETE("/", middleware.AdminAuth(), logctrl.DeleteHistoryLogs)
		logRoute.GET("/stat", middleware.AdminAuth(), logctrl.GetLogsStat)
		logRoute.GET("/self/stat", middleware.UserAuth(), logctrl.GetLogsSelfStat)
		logRoute.GET("/channel_affinity_usage_cache", middleware.AdminAuth(), channelctrl.GetChannelAffinityUsageCacheStats)
		logRoute.GET("/search", middleware.AdminAuth(), logctrl.SearchAllLogs)
		logRoute.GET("/self", middleware.UserAuth(), logctrl.GetUserLogs)
		logRoute.GET("/self/search", middleware.UserAuth(), logctrl.SearchUserLogs)

		logRoute.Use(cors.Default())
		{
			logRoute.GET("/token", logctrl.GetLogByKey)
		}

		// ==================== 预填充组管理路由 ====================
		prefillGroupRoute := apiRouter.Group("/prefill_group")
		prefillGroupRoute.Use(middleware.AdminAuth())
		{
			prefillGroupRoute.GET("/", prefillctrl.GetPrefillGroups)
			prefillGroupRoute.POST("/", prefillctrl.CreatePrefillGroup)
			prefillGroupRoute.PUT("/", prefillctrl.UpdatePrefillGroup)
			prefillGroupRoute.DELETE("/:id", prefillctrl.DeletePrefillGroup)
		}

		// ==================== 任务系统路由 ====================
		taskRoute := apiRouter.Group("/task")
		{
			taskRoute.GET("/self", middleware.UserAuth(), taskctrl.GetUserTask)
			taskRoute.GET("/", middleware.AdminAuth(), taskctrl.GetAllTask)
		}

		// ==================== 供应商元数据管理路由 ====================
		vendorRoute := apiRouter.Group("/vendors")
		vendorRoute.Use(middleware.AdminAuth())
		{
			vendorRoute.GET("/", vendorctrl.GetAllVendors)
			vendorRoute.GET("/search", vendorctrl.SearchVendors)
			vendorRoute.GET("/:id", vendorctrl.GetVendorMeta)
			vendorRoute.POST("/", vendorctrl.CreateVendorMeta)
			vendorRoute.PUT("/", vendorctrl.UpdateVendorMeta)
			vendorRoute.DELETE("/:id", vendorctrl.DeleteVendorMeta)
		}

		// ==================== 模型元数据管理路由 ====================
		modelsRoute := apiRouter.Group("/models")
		modelsRoute.Use(middleware.AdminAuth())
		{
			modelsRoute.GET("/sync_upstream/preview", metactrl.SyncUpstreamPreview)
			modelsRoute.POST("/sync_upstream", metactrl.SyncUpstreamModels)
			modelsRoute.GET("/missing", metactrl.GetMissingModels)
			modelsRoute.GET("/tags", metactrl.GetModelTags)

			modelsRoute.GET("/", metactrl.GetAllModelsMeta)
			modelsRoute.GET("/search", metactrl.SearchModelsMeta)
			modelsRoute.GET("/:id", metactrl.GetModelMeta)
			modelsRoute.POST("/", metactrl.CreateModelMeta)
			modelsRoute.PUT("/", metactrl.UpdateModelMeta)
			modelsRoute.DELETE("/:id", metactrl.DeleteModelMeta)
		}
	}
}
