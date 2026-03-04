package router

import (
	"yugu-server/internal/controller"

	"github.com/gin-gonic/gin"
)

func SetupRouter(infoCtrl *controller.InfoController) *gin.Engine {
	r := gin.Default()

	infoGroup := r.Group("/info")
	{
		infoGroup.GET("/server", infoCtrl.ServerInfo)
		infoGroup.GET("/client", infoCtrl.ClientInfo)
		infoGroup.GET("/database", infoCtrl.DatabaseInfo)
	}

	return r
}
