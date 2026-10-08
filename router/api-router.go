package router

import (
	"openapi/controller"
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
	// 路径前缀: /api
	apiRouter := router.Group("/api")
	apiRouter.Use(middleware.Compress())           // Gzip 响应压缩（与请求解压见 decompress.go）
	apiRouter.Use(middleware.BodyStorageCleanup()) // 清理请求体存储
	apiRouter.Use(middleware.GlobalAPIRateLimit()) // 全局 API 速率限制
	{
		// -------------------- 系统状态与配置接口 --------------------
		// GET /api/setup - 获取系统初始化状态
		apiRouter.GET("/setup", controller.GetSetup)
		// POST /api/setup - 执行系统初始化设置
		apiRouter.POST("/setup", controller.PostSetup)
		// GET /api/status - 获取系统状态信息（版本、配置等）
		apiRouter.GET("/status", controller.GetStatus)
		// GET /api/uptime/status - 获取 Uptime Kuma 监控状态
		apiRouter.GET("/uptime/status", controller.GetUptimeKumaStatus)
		// GET /api/models - 获取用户可用模型列表（需要登录）
		apiRouter.GET("/models", middleware.UserAuth(), controller.DashboardListModels)
		// GET /api/status/test - 测试系统状态（管理员专用）
		apiRouter.GET("/status/test", middleware.AdminAuth(), controller.TestStatus)

		// -------------------- 公开信息接口 --------------------
		// GET /api/notice - 获取系统公告
		apiRouter.GET("/notice", controller.GetNotice)
		// GET /api/user-agreement - 获取用户协议
		apiRouter.GET("/user-agreement", controller.GetUserAgreement)
		// GET /api/privacy-policy - 获取隐私政策
		apiRouter.GET("/privacy-policy", controller.GetPrivacyPolicy)
		// GET /api/about - 获取关于页面内容
		apiRouter.GET("/about", controller.GetAbout)
		// GET /api/home_page_content - 获取首页内容
		apiRouter.GET("/home_page_content", controller.GetHomePageContent)
		// GET /api/pricing - 获取价格信息（模型列表、倍率等）
		apiRouter.GET("/pricing", middleware.TryUserAuth(), controller.GetPricing)

		// -------------------- 倍率配置 --------------------
		// GET /api/ratio_config - 获取倍率配置（暴露接口）
		apiRouter.GET("/ratio_config", middleware.CriticalRateLimit(), controller.GetRatioConfig)

		// -------------------- 安全验证接口 --------------------
		// POST /api/verify - 通用安全验证（需要登录）
		apiRouter.POST("/verify", middleware.UserAuth(), middleware.CriticalRateLimit(), controller.UniversalVerify)
		// GET /api/verify/status - 获取验证状态
		apiRouter.GET("/verify/status", middleware.UserAuth(), controller.GetVerificationStatus)

		// ==================== 用户路由组 ====================
		// 路径前缀: /api/user
		userRoute := apiRouter.Group("/user")
		{
			// -------------------- 公开的用户接口（无需登录）--------------------
			// POST /api/user/login - 用户登录（普通密码登录）
			userRoute.POST("/login", middleware.CriticalRateLimit(), middleware.TurnstileCheck(), controller.Login)
			// POST /api/user/login/2fa - 双因素认证登录（输入验证码）
			userRoute.POST("/login/2fa", middleware.CriticalRateLimit(), controller.Verify2FALogin)
			// POST /api/user/passkey/login/begin - Passkey 登录开始（WebAuthn）
			userRoute.POST("/passkey/login/begin", middleware.CriticalRateLimit(), controller.PasskeyLoginBegin)
			// POST /api/user/passkey/login/finish - Passkey 登录完成
			userRoute.POST("/passkey/login/finish", middleware.CriticalRateLimit(), controller.PasskeyLoginFinish)
			//userRoute.POST("/tokenlog", middleware.CriticalRateLimit(), controller.TokenLog)
			// GET /api/user/logout - 用户登出
			userRoute.GET("/logout", controller.Logout)
			// GET /api/user/groups - 获取用户组列表（公开）
			userRoute.GET("/groups", controller.GetUserGroups)

			// -------------------- 用户个人信息路由组（需要登录）--------------------
			// 路径前缀: /api/user
			selfRoute := userRoute.Group("/")
			selfRoute.Use(middleware.UserAuth()) // 需要用户登录
			{
				// GET /api/user/self/groups - 获取当前用户的用户组
				selfRoute.GET("/self/groups", controller.GetUserGroups)
				// GET /api/user/self - 获取当前用户信息
				selfRoute.GET("/self", controller.GetSelf)
				// GET /api/user/models - 获取用户可用模型列表
				selfRoute.GET("/models", controller.GetUserModels)
				// PUT /api/user/self - 更新当前用户信息（昵称、邮箱等）
				selfRoute.PUT("/self", controller.UpdateSelf)
				// DELETE /api/user/self - 注销账号
				selfRoute.DELETE("/self", controller.DeleteSelf)
				// GET /api/user/token - 生成访问令牌（用于 API 调用）
				selfRoute.GET("/token", controller.GenerateAccessToken)
			}

			// -------------------- 用户管理路由组（管理员专用）--------------------
			// 路径前缀: /api/user
			adminRoute := userRoute.Group("/")
			adminRoute.Use(middleware.AdminAuth()) // 需要管理员权限
			{
				// GET /api/user/ - 获取所有用户列表
				adminRoute.GET("/", controller.GetAllUsers)
				// GET /api/user/topup - 获取所有充值记录
				adminRoute.GET("/topup", controller.GetAllTopUps)
				// POST /api/user/topup/complete - 管理员完成充值
				adminRoute.POST("/topup/complete", controller.AdminCompleteTopUp)
				// GET /api/user/search - 搜索用户
				adminRoute.GET("/search", controller.SearchUsers)
				// GET /api/user/:id - 获取指定用户信息
				adminRoute.GET("/:id", controller.GetUser)
				// POST /api/user/ - 创建新用户
				adminRoute.POST("/", controller.CreateUser)
				// POST /api/user/manage - 管理用户（启用/禁用/修改配额等）
				adminRoute.POST("/manage", controller.ManageUser)
				// POST /api/user/batch/group - 批量设置用户分组
				adminRoute.POST("/batch/group", controller.BatchUpdateUserGroup)
				// PUT /api/user/ - 更新用户信息
				adminRoute.PUT("/", controller.UpdateUser)
				// DELETE /api/user/:id - 删除用户
				adminRoute.DELETE("/:id", controller.DeleteUser)
				// DELETE /api/user/:id/reset_passkey - 重置用户的 Passkey
				adminRoute.DELETE("/:id/reset_passkey", controller.AdminResetPasskey)

				// 管理员 2FA 管理接口
				// GET /api/user/2fa/stats - 获取 2FA 使用统计
				adminRoute.GET("/2fa/stats", controller.Admin2FAStats)
				// DELETE /api/user/:id/2fa - 管理员禁用指定用户的 2FA
				adminRoute.DELETE("/:id/2fa", controller.AdminDisable2FA)
			}
		}

		// ==================== 订阅计费路由组 ====================
		// 路径前缀: /api/subscription
		// 功能：订阅计划、购买、管理员管理
		subscriptionRoute := apiRouter.Group("/subscription")
		subscriptionRoute.Use(middleware.UserAuth()) // 需要用户登录
		{
			// GET /api/subscription/plans - 获取可用的订阅计划列表
			subscriptionRoute.GET("/plans", controller.GetSubscriptionPlans)
			// GET /api/subscription/self - 获取当前用户的订阅信息
			subscriptionRoute.GET("/self", controller.GetSubscriptionSelf)
			// PUT /api/subscription/self/preference - 更新订阅偏好设置
			subscriptionRoute.PUT("/self/preference", controller.UpdateSubscriptionPreference)
		}

		// ==================== 系统配置路由组 ====================
		// 路径前缀: /api/option
		// 权限：超级管理员（Root）
		optionRoute := apiRouter.Group("/option")
		optionRoute.Use(middleware.RootAuth()) // 需要超级管理员权限
		{
			// GET /api/option/ - 获取所有系统配置参数
			optionRoute.GET("/", controller.GetOptions)
			// PUT /api/option/ - 更新系统配置参数
			optionRoute.PUT("/", controller.UpdateOption)
			// GET /api/option/channel_affinity_cache - 获取渠道亲和性缓存统计
			optionRoute.GET("/channel_affinity_cache", controller.GetChannelAffinityCacheStats)
			// DELETE /api/option/channel_affinity_cache - 清除渠道亲和性缓存
			optionRoute.DELETE("/channel_affinity_cache", controller.ClearChannelAffinityCache)
			// POST /api/option/rest_model_ratio - 重置模型倍率配置
			optionRoute.POST("/rest_model_ratio", controller.ResetModelRatio)
			// GET /api/option/peak_valley_time - 峰谷计费时区/服务器时间校对
			optionRoute.GET("/peak_valley_time", controller.GetPeakValleyTimeInfo)
			// POST /api/option/migrate_console_setting - 迁移旧控制台设置（下版本删除）
			optionRoute.POST("/migrate_console_setting", controller.MigrateConsoleSetting) // 用于迁移检测的旧键，下个版本会删除
		}
		// ==================== 性能监控路由组 ====================
		// 路径前缀: /api/performance
		// 权限：超级管理员（Root）
		performanceRoute := apiRouter.Group("/performance")
		performanceRoute.Use(middleware.RootAuth()) // 需要超级管理员权限
		{
			// GET /api/performance/stats - 获取系统性能统计（CPU、内存、磁盘等）
			performanceRoute.GET("/stats", controller.GetPerformanceStats)
			// DELETE /api/performance/disk_cache - 清除磁盘缓存
			performanceRoute.DELETE("/disk_cache", controller.ClearDiskCache)
			// POST /api/performance/reset_stats - 重置性能统计数据
			performanceRoute.POST("/reset_stats", controller.ResetPerformanceStats)
			// POST /api/performance/gc - 强制执行垃圾回收（GC）
			performanceRoute.POST("/gc", controller.ForceGC)
		}
		// ==================== 倍率同步路由组 ====================
		// 路径前缀: /api/ratio_sync
		// 权限：超级管理员（Root）
		// 功能：从上游渠道同步模型倍率配置
		ratioSyncRoute := apiRouter.Group("/ratio_sync")
		ratioSyncRoute.Use(middleware.RootAuth()) // 需要超级管理员权限
		{
			// GET /api/ratio_sync/channels - 获取可同步倍率的渠道列表
			ratioSyncRoute.GET("/channels", controller.GetSyncableChannels)
			// POST /api/ratio_sync/fetch - 从上游获取最新倍率配置
			ratioSyncRoute.POST("/fetch", controller.FetchUpstreamRatios)
		}
		// ==================== 渠道管理路由组 ====================
		// 路径前缀: /api/channel
		// 权限：管理员
		// 功能：API 渠道的 CRUD、测试、余额查询、模型同步等
		channelRoute := apiRouter.Group("/channel")
		channelRoute.Use(middleware.AdminAuth()) // 需要管理员权限
		{
			// -------------------- 渠道基本 CRUD --------------------
			// GET /api/channel/ - 获取所有渠道列表
			channelRoute.GET("/", controller.GetAllChannels)
			// GET /api/channel/search - 搜索渠道
			channelRoute.GET("/search", controller.SearchChannels)
			// GET /api/channel/models - 获取渠道支持的模型列表
			channelRoute.GET("/models", controller.ChannelListModels)
			// GET /api/channel/models_enabled - 获取已启用的模型列表
			channelRoute.GET("/models_enabled", controller.EnabledListModels)
			// GET /api/channel/:id - 获取指定渠道详情
			channelRoute.GET("/:id", controller.GetChannel)
			// POST /api/channel/:id/key - 获取渠道密钥（需要安全验证）
			channelRoute.POST("/:id/key", middleware.RootAuth(), middleware.CriticalRateLimit(), middleware.DisableCache(), middleware.SecureVerificationRequired(), controller.GetChannelKey)
			// POST /api/channel/ - 添加新渠道
			channelRoute.POST("/", controller.AddChannel)
			// PUT /api/channel/ - 更新渠道
			channelRoute.PUT("/", controller.UpdateChannel)
			// DELETE /api/channel/:id - 删除指定渠道
			channelRoute.DELETE("/:id", controller.DeleteChannel)
			// POST /api/channel/batch - 批量删除渠道
			channelRoute.POST("/batch", controller.DeleteChannelBatch)
			// POST /api/channel/copy/:id - 复制渠道
			channelRoute.POST("/copy/:id", controller.CopyChannel)

			// -------------------- 渠道测试与状态 --------------------
			// GET /api/channel/test - 测试所有渠道
			channelRoute.GET("/test", controller.TestAllChannels)
			// GET /api/channel/test/:id - 测试指定渠道
			channelRoute.GET("/test/:id", controller.TestChannel)
			// GET /api/channel/test/:id/all_models - 测试渠道所有模型并记录
			channelRoute.GET("/test/:id/all_models", controller.TestChannelAllModels)
			// GET /api/channel/update_balance - 更新所有渠道余额
			channelRoute.GET("/update_balance", controller.UpdateAllChannelsBalance)
			// GET /api/channel/update_balance/:id - 更新指定渠道余额
			channelRoute.GET("/update_balance/:id", controller.UpdateChannelBalance)

			// -------------------- 模型测试记录 --------------------
			// GET /api/channel/test_records - 查询测试记录（支持多条件筛选）
			channelRoute.GET("/test_records", controller.GetChannelTestRecords)
			// DELETE /api/channel/test_records - 按条件删除记录
			channelRoute.DELETE("/test_records", controller.DeleteChannelTestRecords)
			// DELETE /api/channel/test_records/all - 清空全部记录
			channelRoute.DELETE("/test_records/all", controller.ClearChannelTestRecords)
			// POST /api/channel/test_records/:id/retest - 复测指定记录
			channelRoute.POST("/test_records/:id/retest", controller.RetestChannelModel)
			// POST /api/channel/test_records/batch_retest - 批量复测
			channelRoute.POST("/test_records/batch_retest", controller.BatchRetestChannelModels)
			// POST /api/channel/test_records/retest_filtered - 筛选全部复测
			channelRoute.POST("/test_records/retest_filtered", controller.RetestFilteredRecords)
			// GET /api/channel/test_records/model_tags - 查询模型的标签
			channelRoute.GET("/test_records/model_tags", controller.GetModelTags)
			// DELETE /api/channel/:id/model - 从渠道移除指定模型
			channelRoute.DELETE("/:id/model", controller.RemoveModelFromChannel)
			// POST /api/channel/fix - 修复渠道能力配置
			channelRoute.POST("/fix", controller.FixChannelsAbilities)

			// -------------------- 渠道标签管理 --------------------
			// DELETE /api/channel/disabled - 删除所有已禁用的渠道
			channelRoute.DELETE("/disabled", controller.DeleteDisabledChannel)
			// POST /api/channel/tag/disabled - 禁用指定标签的渠道
			channelRoute.POST("/tag/disabled", controller.DisableTagChannels)
			// POST /api/channel/tag/enabled - 启用指定标签的渠道
			channelRoute.POST("/tag/enabled", controller.EnableTagChannels)
			// PUT /api/channel/tag - 编辑标签渠道
			channelRoute.PUT("/tag", controller.EditTagChannels)
			// POST /api/channel/batch/tag - 批量设置渠道标签
			channelRoute.POST("/batch/tag", controller.BatchSetChannelTag)
			// GET /api/channel/tag/models - 获取标签支持的模型
			channelRoute.GET("/tag/models", controller.GetTagModels)

			// -------------------- 模型同步 --------------------
			// GET /api/channel/fetch_models/:id - 从上游获取指定渠道的模型列表
			channelRoute.GET("/fetch_models/:id", controller.FetchUpstreamModels)
			// POST /api/channel/fetch_models - 批量获取模型
			channelRoute.POST("/fetch_models", controller.FetchModels)
			// -------------------- 多密钥管理 --------------------
			// POST /api/channel/multi_key/manage - 管理渠道的多个 API Key
			channelRoute.POST("/multi_key/manage", controller.ManageMultiKeys)
		}

		// ==================== 定时任务管理路由组 ====================
		// 路径前缀: /api/cron_job
		// 权限：管理员
		cronJobRoute := apiRouter.Group("/cron_job")
		cronJobRoute.Use(middleware.AdminAuth())
		{
			cronJobRoute.GET("/", controller.GetCronJobs)
			cronJobRoute.GET("/types", controller.GetCronJobTypes)
			cronJobRoute.GET("/:id", controller.GetCronJob)
			cronJobRoute.POST("/", controller.CreateCronJob)
			cronJobRoute.PUT("/:id", controller.UpdateCronJob)
			cronJobRoute.DELETE("/:id", controller.DeleteCronJob)
			cronJobRoute.POST("/:id/enabled", controller.ToggleCronJob)
			cronJobRoute.POST("/:id/run", controller.RunCronJobNow)
			cronJobRoute.GET("/:id/runs", controller.GetCronJobRuns)
		}
		// ==================== 令牌管理路由组 ====================
		// 路径前缀: /api/token
		// 权限：用户登录
		// 功能：API Token（API Key）的 CRUD 管理
		tokenRoute := apiRouter.Group("/token")
		tokenRoute.Use(middleware.UserAuth()) // 需要用户登录
		{
			// GET /api/token/ - 获取用户的所有 Token
			tokenRoute.GET("/", controller.GetAllTokens)
			// GET /api/token/search - 搜索 Token
			tokenRoute.GET("/search", controller.SearchTokens)
			// GET /api/token/:id - 获取指定 Token 详情
			tokenRoute.GET("/:id", controller.GetToken)
			// GET /api/token/analytics/:id - 获取 Token 使用分析
			tokenRoute.GET("/analytics/:id", controller.GetTokenAnalytics)
			// GET /api/token/webstar_package[?token_id=X] - 下载 WebStar 插件包
			//   带 token_id：内置该令牌密钥/地址（禁缓存）；不带：不含密钥的基础包（可缓存）
			tokenRoute.GET("/webstar_package", controller.DownloadWebStarPackage)
			// POST /api/token/ - 创建新 Token
			tokenRoute.POST("/", controller.AddToken)
			// PUT /api/token/ - 更新 Token
			tokenRoute.PUT("/", controller.UpdateToken)
			// DELETE /api/token/:id - 删除指定 Token
			tokenRoute.DELETE("/:id", controller.DeleteToken)
			// POST /api/token/batch - 批量删除 Token
			tokenRoute.POST("/batch", controller.DeleteTokenBatch)
		}

		// ==================== 令牌访问限制模板路由组 ====================
		// 路径前缀: /api/token_template
		// 权限：用户登录（按用户隔离）
		// 功能：令牌「访问限制」模板的 CRUD，供创建/编辑令牌时快速套用
		tokenTemplateRoute := apiRouter.Group("/token_template")
		tokenTemplateRoute.Use(middleware.UserAuth())
		{
			// GET /api/token_template/ - 获取当前用户的全部模板
			tokenTemplateRoute.GET("/", controller.GetUserTokenAccessTemplates)
			// POST /api/token_template/ - 新建模板
			tokenTemplateRoute.POST("/", controller.AddTokenAccessTemplate)
			// PUT /api/token_template/ - 更新模板
			tokenTemplateRoute.PUT("/", controller.UpdateTokenAccessTemplate)
			// DELETE /api/token_template/:id - 删除模板
			tokenTemplateRoute.DELETE("/:id", controller.DeleteTokenAccessTemplate)
		}

		// ==================== 使用情况查询路由组 ====================
		// 路径前缀: /api/usage
		usageRoute := apiRouter.Group("/usage")
		usageRoute.Use(middleware.CriticalRateLimit()) // 严格限流
		{
			// Token 使用情况查询（通过 API Key 认证）
			// 路径前缀: /api/usage/token
			tokenUsageRoute := usageRoute.Group("/token")
			tokenUsageRoute.Use(middleware.TokenAuth()) // 使用 Token 认证
			{
				// GET /api/usage/token/ - 获取当前 Token 的使用情况
				tokenUsageRoute.GET("/", controller.GetTokenUsage)
			}
		}

		// ==================== 日志系统路由组 ====================
		// 路径前缀: /api/log
		// 功能：请求日志、消费记录、统计分析
		logRoute := apiRouter.Group("/log")
		// GET /api/log/ - 获取所有日志（管理员）
		logRoute.GET("/", middleware.AdminAuth(), controller.GetAllLogs)
		// DELETE /api/log/ - 删除历史日志（管理员）
		logRoute.DELETE("/", middleware.AdminAuth(), controller.DeleteHistoryLogs)
		// GET /api/log/stat - 获取日志统计（管理员）
		logRoute.GET("/stat", middleware.AdminAuth(), controller.GetLogsStat)
		// GET /api/log/self/stat - 获取当前用户的日志统计
		logRoute.GET("/self/stat", middleware.UserAuth(), controller.GetLogsSelfStat)
		// GET /api/log/channel_affinity_usage_cache - 获取渠道亲和性使用缓存（管理员）
		logRoute.GET("/channel_affinity_usage_cache", middleware.AdminAuth(), controller.GetChannelAffinityUsageCacheStats)
		// GET /api/log/search - 搜索所有日志（管理员）
		logRoute.GET("/search", middleware.AdminAuth(), controller.SearchAllLogs)
		// GET /api/log/self - 获取当前用户的日志
		logRoute.GET("/self", middleware.UserAuth(), controller.GetUserLogs)
		// GET /api/log/self/search - 搜索当前用户的日志
		logRoute.GET("/self/search", middleware.UserAuth(), controller.SearchUserLogs)

		// -------------------- 日志 CORS 支持接口 --------------------
		// 用于跨域访问日志
		logRoute.Use(cors.Default())
		{
			// GET /api/log/token - 通过 API Key 查询日志（支持 CORS）
			logRoute.GET("/token", controller.GetLogByKey)
		}
		// ==================== 用户组路由组 ====================
		// 路径前缀: /api/group
		// 权限：管理员
		// 功能：获取用户组列表
		groupRoute := apiRouter.Group("/group")
		groupRoute.Use(middleware.AdminAuth()) // 需要管理员权限
		{
			// GET /api/group/ - 获取所有用户组
			groupRoute.GET("/", controller.GetGroups)
		}

		// ==================== 预填充组管理路由 ====================
		// 路径前缀: /api/prefill_group
		// 权限：管理员
		// 功能：管理预填充内容组（Prompt Caching 等）
		prefillGroupRoute := apiRouter.Group("/prefill_group")
		prefillGroupRoute.Use(middleware.AdminAuth()) // 需要管理员权限
		{
			// GET /api/prefill_group/ - 获取所有预填充组
			prefillGroupRoute.GET("/", controller.GetPrefillGroups)
			// POST /api/prefill_group/ - 创建预填充组
			prefillGroupRoute.POST("/", controller.CreatePrefillGroup)
			// PUT /api/prefill_group/ - 更新预填充组
			prefillGroupRoute.PUT("/", controller.UpdatePrefillGroup)
			// DELETE /api/prefill_group/:id - 删除预填充组
			prefillGroupRoute.DELETE("/:id", controller.DeletePrefillGroup)
		}

		// ==================== 任务系统路由 ====================
		// 路径前缀: /api/task
		// 功能：查询异步任务（视频生成、音乐生成等）
		taskRoute := apiRouter.Group("/task")
		{
			// GET /api/task/self - 获取当前用户的任务
			taskRoute.GET("/self", middleware.UserAuth(), controller.GetUserTask)
			// GET /api/task/ - 获取所有任务（管理员）
			taskRoute.GET("/", middleware.AdminAuth(), controller.GetAllTask)
		}

		// ==================== 供应商元数据管理路由 ====================
		// 路径前缀: /api/vendors
		// 权限：管理员
		// 功能：管理 AI 供应商的元数据（名称、Logo、描述等）
		vendorRoute := apiRouter.Group("/vendors")
		vendorRoute.Use(middleware.AdminAuth()) // 需要管理员权限
		{
			// GET /api/vendors/ - 获取所有供应商
			vendorRoute.GET("/", controller.GetAllVendors)
			// GET /api/vendors/search - 搜索供应商
			vendorRoute.GET("/search", controller.SearchVendors)
			// GET /api/vendors/:id - 获取指定供应商详情
			vendorRoute.GET("/:id", controller.GetVendorMeta)
			// POST /api/vendors/ - 创建供应商元数据
			vendorRoute.POST("/", controller.CreateVendorMeta)
			// PUT /api/vendors/ - 更新供应商元数据
			vendorRoute.PUT("/", controller.UpdateVendorMeta)
			// DELETE /api/vendors/:id - 删除供应商元数据
			vendorRoute.DELETE("/:id", controller.DeleteVendorMeta)
		}

		// ==================== 模型元数据管理路由 ====================
		// 路径前缀: /api/models
		// 权限：管理员
		// 功能：管理 AI 模型的元数据（名称、倍率、价格等）
		modelsRoute := apiRouter.Group("/models")
		modelsRoute.Use(middleware.AdminAuth()) // 需要管理员权限
		{
			// -------------------- 上游同步 --------------------
			// GET /api/models/sync_upstream/preview - 预览上游模型同步
			modelsRoute.GET("/sync_upstream/preview", controller.SyncUpstreamPreview)
			// POST /api/models/sync_upstream - 执行上游模型同步
			modelsRoute.POST("/sync_upstream", controller.SyncUpstreamModels)
			// GET /api/models/missing - 获取缺失的模型列表
			modelsRoute.GET("/missing", controller.GetMissingModels)
			// GET /api/models/tags - 获取所有模型标签
			modelsRoute.GET("/tags", controller.GetModelTags)

			// -------------------- 模型 CRUD --------------------
			// GET /api/models/ - 获取所有模型元数据
			modelsRoute.GET("/", controller.GetAllModelsMeta)
			// GET /api/models/search - 搜索模型
			modelsRoute.GET("/search", controller.SearchModelsMeta)
			// GET /api/models/:id - 获取指定模型详情
			modelsRoute.GET("/:id", controller.GetModelMeta)
			// POST /api/models/ - 创建模型元数据
			modelsRoute.POST("/", controller.CreateModelMeta)
			// PUT /api/models/ - 更新模型元数据
			modelsRoute.PUT("/", controller.UpdateModelMeta)
			// DELETE /api/models/:id - 删除模型元数据
			modelsRoute.DELETE("/:id", controller.DeleteModelMeta)
		}
	}
}
