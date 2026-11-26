package router

import (
	relayctrl "openapi/controller/relay"
	taskctrl "openapi/controller/task"
	"openapi/middleware"

	gin "github.com/king54346/gin-tiny"
)

// SetVideoRouter 设置视频生成相关的路由
func SetVideoRouter(router *gin.Engine) {
	videoV1Router := router.Group("/v1")
	videoV1Router.Use(middleware.TokenAuth(), middleware.Distribute())
	{
		videoV1Router.GET("/videos/:task_id/content", taskctrl.VideoProxy)
		videoV1Router.POST("/video/generations", relayctrl.RelayTask)
		videoV1Router.GET("/video/generations/:task_id", relayctrl.RelayTask)
		videoV1Router.POST("/videos/:video_id/remix", relayctrl.RelayTask)
	}
	{
		videoV1Router.POST("/videos", relayctrl.RelayTask)
		videoV1Router.GET("/videos/:task_id", relayctrl.RelayTask)
	}
}
