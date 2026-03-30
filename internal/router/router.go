package router

import (
	"yugu-server/internal/controller"
	"yugu-server/internal/middleware"

	"github.com/gin-gonic/gin"
)

func SetupRouter(infoCtrl *controller.InfoController, authCtrl *controller.AuthController) *gin.Engine {
	r := gin.Default()

	infoGroup := r.Group("/info")
	{
		infoGroup.GET("/server", infoCtrl.ServerInfo)
		infoGroup.GET("/database", infoCtrl.DatabaseInfo)
		infoGroup.GET("/client", infoCtrl.ClientInfo)
	}

	authGroup := r.Group("/api/auth")
	{
		authGroup.POST("/register", authCtrl.Register)
		authGroup.POST("/login", authCtrl.Login)
		authGroup.POST("/refresh", authCtrl.Refresh)

		// Защищенные
		authGroup.GET("/me", middleware.AuthRequired(), authCtrl.Me)
		authGroup.GET("/tokens", middleware.AuthRequired(), authCtrl.GetTokens)       
		authGroup.POST("/out_all", middleware.AuthRequired(), authCtrl.LogoutAll)
		authGroup.POST("/out", middleware.AuthRequired(), authCtrl.Logout)    
	}

	return r
}
