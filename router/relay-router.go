package router

import (
	"openapi/constant"
	pgctrl "openapi/controller/playground"
	relayctrl "openapi/controller/relay"
	"openapi/middleware"
	"openapi/types"

	gin "github.com/king54346/gin-tiny"
	"github.com/king54346/gin-tiny/middleware/cors"
)

// SetRelayRouter 设置中继路由
func SetRelayRouter(router *gin.Engine) {
	router.Use(cors.Default())
	router.Use(middleware.DecompressRequestMiddleware())
	router.Use(middleware.BodyStorageCleanup())
	router.Use(middleware.StatsMiddleware())

	// ==================== 模型列表路由 ====================
	modelsRouter := router.Group("/v1/models")
	modelsRouter.Use(middleware.TokenAuth())
	{
		modelsRouter.GET("", func(c gin.Context) {
			switch {
			case c.GetHeader("x-api-key") != "" && c.GetHeader("anthropic-version") != "":
				relayctrl.ListModels(c, constant.ChannelTypeAnthropic)
			case c.GetHeader("x-goog-api-key") != "" || c.Query("key") != "":
				relayctrl.RetrieveModel(c, constant.ChannelTypeGemini)
			default:
				relayctrl.ListModels(c, constant.ChannelTypeOpenAI)
			}
		})

		modelsRouter.GET("/:model", func(c gin.Context) {
			switch {
			case c.GetHeader("x-api-key") != "" && c.GetHeader("anthropic-version") != "":
				relayctrl.RetrieveModel(c, constant.ChannelTypeAnthropic)
			default:
				relayctrl.RetrieveModel(c, constant.ChannelTypeOpenAI)
			}
		})
	}

	// ==================== Gemini 官方路由 (/v1beta) ====================
	geminiModelsRouter := router.Group("/v1beta/models")
	geminiModelsRouter.Use(middleware.TokenAuth())
	{
		geminiModelsRouter.GET("", func(c gin.Context) {
			relayctrl.ListModels(c, constant.ChannelTypeGemini)
		})
		geminiModelsRouter.GET("/:model", func(c gin.Context) {
			relayctrl.RetrieveModel(c, constant.ChannelTypeGemini)
		})
	}

	geminiRelayRouter := router.Group("/v1beta")
	geminiRelayRouter.Use(middleware.SystemPerformanceCheck(), middleware.TokenAuth(), middleware.AdmissionControl(), middleware.ModelRequestRateLimit(), middleware.Distribute())
	{
		geminiRelayRouter.POST("/models/*path", func(c gin.Context) {
			relayctrl.Relay(c, types.RelayFormatGemini)
		})
	}

	// ==================== Gemini OpenAI 兼容路由 (/v1beta/openai) ====================
	geminiCompatibleModelsRouter := router.Group("/v1beta/openai/models")
	geminiCompatibleModelsRouter.Use(middleware.TokenAuth())
	{
		geminiCompatibleModelsRouter.GET("", func(c gin.Context) {
			relayctrl.ListModels(c, constant.ChannelTypeOpenAI)
		})
	}
	geminiCompatibleRelayRouter := router.Group("/v1beta/openai")
	geminiCompatibleRelayRouter.Use(middleware.SystemPerformanceCheck(), middleware.TokenAuth(), middleware.AdmissionControl(), middleware.ModelRequestRateLimit(), middleware.Distribute())
	{
		geminiCompatibleRelayRouter.POST("/chat/completions", func(c gin.Context) {
			relayctrl.Relay(c, types.RelayFormatOpenAI)
		})
		geminiCompatibleRelayRouter.POST("/embeddings", func(c gin.Context) {
			relayctrl.Relay(c, types.RelayFormatEmbedding)
		})
	}

	// ==================== Playground 路由 ====================
	playgroundRouter := router.Group("/pg")
	playgroundRouter.Use(middleware.SystemPerformanceCheck())
	playgroundRouter.Use(middleware.UserAuth(), middleware.AdmissionControl(), middleware.Distribute())
	{
		playgroundRouter.POST("/chat/completions", pgctrl.Playground)
		playgroundRouter.POST("/images/generations", pgctrl.PlaygroundImage)
		playgroundRouter.POST("/video/generations", pgctrl.PlaygroundTask)
		playgroundRouter.GET("/video/tasks/:task_id", pgctrl.PlaygroundTask)
		playgroundRouter.POST("/embeddings", pgctrl.PlaygroundEmbedding)
		playgroundRouter.POST("/rerank", pgctrl.PlaygroundRerank)
		playgroundRouter.POST("/audio/speech", pgctrl.PlaygroundAudio)
		playgroundRouter.POST("/audio/transcriptions", pgctrl.PlaygroundAudio)
	}

	// ==================== OpenAI / Claude / Gemini 统一 /v1 路由 ====================
	relayV1Router := router.Group("/v1")
	relayV1Router.Use(middleware.SystemPerformanceCheck())
	relayV1Router.Use(middleware.TokenAuth())
	relayV1Router.Use(middleware.AdmissionControl())
	relayV1Router.Use(middleware.ModelRequestRateLimit())
	{
		wsRouter := relayV1Router.Group("")
		wsRouter.Use(middleware.Distribute())
		wsRouter.GET("/realtime", func(c gin.Context) {
			relayctrl.Relay(c, types.RelayFormatOpenAIRealtime)
		})
	}
	{
		httpRouter := relayV1Router.Group("")
		httpRouter.Use(middleware.Distribute())

		httpRouter.POST("/messages", func(c gin.Context) {
			relayctrl.Relay(c, types.RelayFormatClaude)
		})
		httpRouter.POST("/completions", func(c gin.Context) {
			relayctrl.Relay(c, types.RelayFormatOpenAI)
		})
		httpRouter.POST("/chat/completions", func(c gin.Context) {
			relayctrl.Relay(c, types.RelayFormatOpenAI)
		})
		httpRouter.POST("/responses", func(c gin.Context) {
			relayctrl.Relay(c, types.RelayFormatOpenAIResponses)
		})
		httpRouter.POST("/responses/compact", func(c gin.Context) {
			relayctrl.Relay(c, types.RelayFormatOpenAIResponsesCompaction)
		})
		httpRouter.POST("/edits", func(c gin.Context) {
			relayctrl.Relay(c, types.RelayFormatOpenAIImage)
		})
		httpRouter.POST("/images/generations", func(c gin.Context) {
			relayctrl.Relay(c, types.RelayFormatOpenAIImage)
		})
		httpRouter.POST("/images/edits", func(c gin.Context) {
			relayctrl.Relay(c, types.RelayFormatOpenAIImage)
		})
		httpRouter.POST("/embeddings", func(c gin.Context) {
			relayctrl.Relay(c, types.RelayFormatEmbedding)
		})
		httpRouter.POST("/audio/transcriptions", func(c gin.Context) {
			relayctrl.Relay(c, types.RelayFormatOpenAIAudio)
		})
		httpRouter.POST("/audio/translations", func(c gin.Context) {
			relayctrl.Relay(c, types.RelayFormatOpenAIAudio)
		})
		httpRouter.POST("/audio/speech", func(c gin.Context) {
			relayctrl.Relay(c, types.RelayFormatOpenAIAudio)
		})
		httpRouter.POST("/rerank", func(c gin.Context) {
			relayctrl.Relay(c, types.RelayFormatRerank)
		})
		httpRouter.POST("/engines/:model/embeddings", func(c gin.Context) {
			relayctrl.Relay(c, types.RelayFormatGemini)
		})
		httpRouter.POST("/models/*path", func(c gin.Context) {
			relayctrl.Relay(c, types.RelayFormatGemini)
		})
		httpRouter.POST("/moderations", func(c gin.Context) {
			relayctrl.Relay(c, types.RelayFormatOpenAI)
		})
	}
}
