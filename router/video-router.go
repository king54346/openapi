package router

import (
	"openapi/controller"
	"openapi/middleware"

	gin "github.com/king54346/gin-tiny"
)

// SetVideoRouter 设置视频生成相关的路由
// 支持多种视频生成服务的 API 格式：统一格式、Kling 格式、即梦格式
func SetVideoRouter(router *gin.Engine) {
	// ==================== 统一格式视频 API 路由组 ====================
	// 路径前缀: /v1
	// 中间件: TokenAuth(令牌认证) + Distribute(渠道分发)
	videoV1Router := router.Group("/v1")
	videoV1Router.Use(middleware.TokenAuth(), middleware.Distribute())
	{
		// GET /v1/videos/:task_id/content - 代理获取视频文件内容
		// 用途: 通过任务ID获取生成好的视频文件，支持跨域代理访问
		// 示例: GET /v1/videos/task_12345/content
		videoV1Router.GET("/videos/:task_id/content", controller.VideoProxy)

		// POST /v1/video/generations - 创建视频生成任务（统一格式）
		// 用途: 提交视频生成请求，支持文生视频和图生视频
		// 请求体: {"model": "kling-v1", "prompt": "描述文本", "image": "图片URL"}
		videoV1Router.POST("/video/generations", controller.RelayTask)

		// GET /v1/video/generations/:task_id - 查询视频生成任务状态
		// 用途: 根据任务ID查询任务状态和结果
		// 示例: GET /v1/video/generations/task_12345
		videoV1Router.GET("/video/generations/:task_id", controller.RelayTask)

		// POST /v1/videos/:video_id/remix - 视频混剪/重制
		// 用途: 基于已生成的视频进行二次创作或混剪
		// 示例: POST /v1/videos/video_12345/remix
		videoV1Router.POST("/videos/:video_id/remix", controller.RelayTask)
	}

	// ==================== OpenAI 兼容格式视频 API 路由 ====================
	// 参考文档: https://platform.openai.com/docs/api-reference/videos/create
	// 提供与 OpenAI 视频 API 兼容的接口格式
	{
		// POST /v1/videos - 创建视频（OpenAI 兼容格式）
		videoV1Router.POST("/videos", controller.RelayTask)

		// GET /v1/videos/:task_id - 查询视频任务（OpenAI 兼容格式）
		videoV1Router.GET("/videos/:task_id", controller.RelayTask)
	}
}
