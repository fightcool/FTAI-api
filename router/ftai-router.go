package router

import (
	ftaiController "github.com/QuantumNous/new-api/controller/ftai"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-gonic/gin"
)

// SetFTAIRouter 设置 FTAI 业务路由
func SetFTAIRouter(router *gin.Engine) {
	ftaiRoute := router.Group("/api/ftai")
	ftaiRoute.Use(middleware.UserAuth())
	{
		// 项目管理
		projects := ftaiRoute.Group("/projects")
		{
			projects.GET("", ftaiController.GetProjects)
			projects.POST("", ftaiController.CreateProject)
			projects.GET("/:id", ftaiController.GetProject)
			projects.PUT("/:id", ftaiController.UpdateProject)
			projects.DELETE("/:id", ftaiController.DeleteProject)
		}

		// 镜头管理
		shots := ftaiRoute.Group("/shots")
		{
			shots.GET("/project/:project_id", ftaiController.GetShots)
		}
	}
}
