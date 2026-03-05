package router

import (
	"yugu-server/internal/controller"

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
	}

	return r
}
