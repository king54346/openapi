package router

import (
	"openapi/constant"
	"openapi/controller"
	"openapi/middleware"
	"openapi/types"

	gin "github.com/king54346/gin-tiny"
	"github.com/king54346/gin-tiny/middleware/cors"
)

// SetRelayRouter 设置中继路由
// 这是核心的 AI 模型请求转发路由，支持多种 AI 服务商的 API 格式
// 包括：OpenAI、Claude、Gemini、Midjourney、Suno 等
func SetRelayRouter(router *gin.Engine) {
	// 应用全局中间件
	router.Use(cors.Default())                           // 跨域资源共享
	router.Use(middleware.DecompressRequestMiddleware()) // 请求体解压缩（gzip/deflate）
	router.Use(middleware.BodyStorageCleanup())          // 清理请求体存储
	router.Use(middleware.StatsMiddleware())             // 统计中间件

	// ==================== 模型列表路由 ====================
	// 参考文档: https://platform.openai.com/docs/api-reference/introduction
	modelsRouter := router.Group("/v1/models")
	modelsRouter.Use(middleware.TokenAuth()) // Token 认证
	{
		// GET /v1/models - 获取可用模型列表
		// 用途: 根据请求头自动识别服务商类型并返回对应的模型列表
		// 支持: OpenAI、Claude、Gemini 格式
		modelsRouter.GET("", func(c gin.Context) {
			switch {
			case c.GetHeader("x-api-key") != "" && c.GetHeader("anthropic-version") != "":
				// Claude 格式（根据 x-api-key 和 anthropic-version 请求头识别）
				controller.ListModels(c, constant.ChannelTypeAnthropic)
			case c.GetHeader("x-goog-api-key") != "" || c.Query("key") != "":
				// Gemini 格式（根据 x-goog-api-key 请求头或 key 查询参数识别）
				controller.RetrieveModel(c, constant.ChannelTypeGemini)
			default:
				// OpenAI 格式（默认）
				controller.ListModels(c, constant.ChannelTypeOpenAI)
			}
		})

		// GET /v1/models/:model - 获取指定模型详情
		// 用途: 查询单个模型的详细信息
		// 参数: model - 模型名称
		modelsRouter.GET("/:model", func(c gin.Context) {
			switch {
			case c.GetHeader("x-api-key") != "" && c.GetHeader("anthropic-version") != "":
				// Claude 格式
				controller.RetrieveModel(c, constant.ChannelTypeAnthropic)
			default:
				// OpenAI 格式（默认）
				controller.RetrieveModel(c, constant.ChannelTypeOpenAI)
			}
		})
	}

	// ==================== Gemini 模型列表路由 ====================
	// Gemini 官方 API 格式，路径为 /v1beta/models
	geminiRouter := router.Group("/v1beta/models")
	geminiRouter.Use(middleware.TokenAuth()) // Token 认证
	{
		// GET /v1beta/models - 获取 Gemini 模型列表
		// 用途: 返回 Gemini 官方格式的模型列表
		geminiRouter.GET("", func(c gin.Context) {
			controller.ListModels(c, constant.ChannelTypeGemini)
		})
	}

	// ==================== Gemini OpenAI 兼容路由 ====================
	// Gemini 提供的 OpenAI 兼容接口，路径为 /v1beta/openai/models
	geminiCompatibleRouter := router.Group("/v1beta/openai/models")
	geminiCompatibleRouter.Use(middleware.TokenAuth()) // Token 认证
	{
		// GET /v1beta/openai/models - 获取模型列表（OpenAI 格式）
		// 用途: 返回 OpenAI 格式的模型列表，方便使用 OpenAI SDK 访问 Gemini
		geminiCompatibleRouter.GET("", func(c gin.Context) {
			controller.ListModels(c, constant.ChannelTypeOpenAI)
		})
	}

	// ==================== Playground 路由 ====================
	// 提供 Web 界面的 AI 对话游乐场功能
	playgroundRouter := router.Group("/pg")
	playgroundRouter.Use(middleware.SystemPerformanceCheck())            // 系统性能检查
	playgroundRouter.Use(middleware.UserAuth(), middleware.Distribute()) // 用户认证 + 渠道分发
	{
		// POST /pg/chat/completions - Playground 聊天对话接口
		// 用途: 用于 Web 界面的 AI 对话测试功能
		// 与 /v1/chat/completions 类似，但使用用户登录凭证而非 API Key
		playgroundRouter.POST("/chat/completions", controller.Playground)

		// POST /pg/images/generations - 绘画创作体验中心接口
		// 用途: 与 /v1/images/generations 等价，但使用用户 session 认证而非 API Key
		playgroundRouter.POST("/images/generations", controller.PlaygroundImage)

		// POST /pg/video/generations - 视频创作体验中心提交接口
		// 用途: 与 /v1/video/generations 等价，但使用用户 session 认证而非 API Key
		playgroundRouter.POST("/video/generations", controller.PlaygroundTask)

		// GET /pg/video/tasks/:task_id - 视频创作体验中心任务查询接口
		// 用途: 查询视频生成任务状态，对应 /v1/video/generations/:task_id
		playgroundRouter.GET("/video/tasks/:task_id", controller.PlaygroundTask)

		// POST /pg/embeddings - 嵌入测试体验中心接口
		// 用途: 与 /v1/embeddings 等价，但使用用户 session 认证而非 API Key
		playgroundRouter.POST("/embeddings", controller.PlaygroundEmbedding)

		// POST /pg/rerank - 重排序体验中心接口
		playgroundRouter.POST("/rerank", controller.PlaygroundRerank)

		// POST /pg/audio/speech - 语音体验中心 TTS 接口
		playgroundRouter.POST("/audio/speech", controller.PlaygroundAudio)

		// POST /pg/audio/transcriptions - 语音体验中心 STT 接口
		playgroundRouter.POST("/audio/transcriptions", controller.PlaygroundAudio)
	}

	// ==================== OpenAI 主要 API 路由 ====================
	// 路径前缀: /v1
	// 支持 OpenAI、Claude、Gemini 等多种服务商的 API 请求转发
	// ==================== Web Search 统一接口路由 ====================
	// 路径: /v1/search
	// 与 LLM relay 解耦：仅做 Token 认证，不经过渠道分发（Distribute，依赖模型）。
	// 内部按分组对 Search 渠道（Exa / Serper / SerpAPI / Brave / Tavily）做负载均衡与配额控制。
	searchRouter := router.Group("/v1")
	searchRouter.Use(middleware.SystemPerformanceCheck()) // 系统性能检查
	searchRouter.Use(middleware.TokenAuth())              // Token 认证
	{
		searchRouter.POST("/search", controller.WebSearch)
		// ==================== Skill 市场对外只读接口 ====================
		// GET /v1/skills - 技能列表（Token 认证，只读）
		searchRouter.GET("/skills", controller.ListPublicSkills)
		// GET /v1/skills/:id - 技能详情
		searchRouter.GET("/skills/:id", controller.GetPublicSkill)
	}

	relayV1Router := router.Group("/v1")
	relayV1Router.Use(middleware.SystemPerformanceCheck()) // 系统性能检查
	relayV1Router.Use(middleware.TokenAuth())              // Token 认证
	relayV1Router.Use(middleware.ModelRequestRateLimit())  // 模型请求速率限制
	{
		// ==================== WebSocket 路由（实时通信） ====================
		wsRouter := relayV1Router.Group("")
		wsRouter.Use(middleware.Distribute()) // 渠道分发

		// GET /v1/realtime - OpenAI Realtime API
		// 用途: 实时语音对话，通过 WebSocket 进行双向通信
		// 支持: gpt-4o-realtime 等实时模型
		wsRouter.GET("/realtime", func(c gin.Context) {
			controller.Relay(c, types.RelayFormatOpenAIRealtime)
		})
	}
	{
		// ==================== HTTP API 路由 ====================
		httpRouter := relayV1Router.Group("")
		httpRouter.Use(middleware.Distribute()) // 渠道分发

		// -------------------- Claude 相关路由 --------------------
		// POST /v1/messages - Claude Messages API
		// 用途: Claude 官方格式的对话接口
		// 文档: https://docs.anthropic.com/claude/reference/messages_post
		httpRouter.POST("/messages", func(c gin.Context) {
			controller.Relay(c, types.RelayFormatClaude)
		})

		// -------------------- 聊天对话相关路由 --------------------
		// POST /v1/completions - 文本补全接口（传统 GPT-3 格式）
		// 用途: 根据给定的提示词生成后续文本
		// 支持模型: text-davinci-003, gpt-3.5-turbo-instruct 等
		httpRouter.POST("/completions", func(c gin.Context) {
			controller.Relay(c, types.RelayFormatOpenAI)
		})

		// POST /v1/chat/completions - 聊天对话接口（最常用）
		// 用途: 基于对话历史生成回复，支持多轮对话
		// 支持模型: gpt-4, gpt-3.5-turbo, claude-3 等
		// 文档: https://platform.openai.com/docs/api-reference/chat
		httpRouter.POST("/chat/completions", func(c gin.Context) {
			controller.Relay(c, types.RelayFormatOpenAI)
		})

		// -------------------- Responses API 相关路由 --------------------
		// POST /v1/responses - OpenAI Responses API
		// 用途: 新的响应格式接口，支持更丰富的输出格式
		httpRouter.POST("/responses", func(c gin.Context) {
			controller.Relay(c, types.RelayFormatOpenAIResponses)
		})

		// POST /v1/responses/compact - 紧凑格式 Responses API
		// 用途: 返回经过压缩的响应数据，减少传输大小
		httpRouter.POST("/responses/compact", func(c gin.Context) {
			controller.Relay(c, types.RelayFormatOpenAIResponsesCompaction)
		})

		// -------------------- 图像生成相关路由 --------------------
		// POST /v1/edits - 图像编辑接口（已废弃）
		// 用途: 编辑或修改图片
		httpRouter.POST("/edits", func(c gin.Context) {
			controller.Relay(c, types.RelayFormatOpenAIImage)
		})

		// POST /v1/images/generations - 图像生成接口
		// 用途: 根据文本描述生成图片
		// 支持模型: dall-e-2, dall-e-3, stable-diffusion 等
		// 文档: https://platform.openai.com/docs/api-reference/images/create
		httpRouter.POST("/images/generations", func(c gin.Context) {
			controller.Relay(c, types.RelayFormatOpenAIImage)
		})

		// POST /v1/images/edits - 图像编辑接口
		// 用途: 基于原始图片和遮罩进行编辑
		// 支持: DALL-E 2
		httpRouter.POST("/images/edits", func(c gin.Context) {
			controller.Relay(c, types.RelayFormatOpenAIImage)
		})

		// -------------------- 嵌入（Embedding）相关路由 --------------------
		// POST /v1/embeddings - 文本嵌入接口
		// 用途: 将文本转换为向量表示，用于相似度搜索、聚类等
		// 支持模型: text-embedding-ada-002, text-embedding-3-small 等
		// 文档: https://platform.openai.com/docs/api-reference/embeddings
		httpRouter.POST("/embeddings", func(c gin.Context) {
			controller.Relay(c, types.RelayFormatEmbedding)
		})

		// -------------------- 音频相关路由 --------------------
		// POST /v1/audio/transcriptions - 语音转文字接口
		// 用途: 将音频文件转换为文字（ASR）
		// 支持模型: whisper-1
		httpRouter.POST("/audio/transcriptions", func(c gin.Context) {
			controller.Relay(c, types.RelayFormatOpenAIAudio)
		})

		// POST /v1/audio/translations - 音频翻译接口
		// 用途: 将音频翻译为英文文字
		// 支持模型: whisper-1
		httpRouter.POST("/audio/translations", func(c gin.Context) {
			controller.Relay(c, types.RelayFormatOpenAIAudio)
		})

		// POST /v1/audio/speech - 文字转语音接口
		// 用途: 将文字转换为语音（TTS）
		// 支持模型: tts-1, tts-1-hd
		httpRouter.POST("/audio/speech", func(c gin.Context) {
			controller.Relay(c, types.RelayFormatOpenAIAudio)
		})

		// -------------------- 重排序（Rerank）相关路由 --------------------
		// POST /v1/rerank - 文本重排序接口
		// 用途: 根据查询词对文档列表进行相关性重排序
		// 支持: Cohere rerank-english-v3.0 等模型
		httpRouter.POST("/rerank", func(c gin.Context) {
			controller.Relay(c, types.RelayFormatRerank)
		})

		// -------------------- Gemini 兼容路由 --------------------
		// POST /v1/engines/:model/embeddings - Gemini Embedding 接口
		// 用途: Gemini 的文本嵌入接口，使用类 OpenAI 格式
		httpRouter.POST("/engines/:model/embeddings", func(c gin.Context) {
			controller.Relay(c, types.RelayFormatGemini)
		})

		// POST /v1/models/*path - Gemini 通用路由
		// 用途: 处理所有 Gemini 模型相关的请求
		// 路径示例: /v1/models/gemini-pro:generateContent
		httpRouter.POST("/models/*path", func(c gin.Context) {
			controller.Relay(c, types.RelayFormatGemini)
		})

		// -------------------- 其他转发路由 --------------------
		// POST /v1/moderations - 内容审查接口
		// 用途: 检测文本内容是否违反使用政策
		// 支持模型: text-moderation-latest, text-moderation-stable
		httpRouter.POST("/moderations", func(c gin.Context) {
			controller.Relay(c, types.RelayFormatOpenAI)
		})
	}
}
