package router

import (
	"yugu-server/internal/controller"

	"github.com/gin-gonic/gin"
)

func SetupRouter(infoCtrl *controller.InfoController) *gin.Engine {
	// Создаем роутер Gin со стандартными логами и защитой от падений
	r := gin.Default()

	// Группируем маршруты (удобно для масштабирования API)
	infoGroup := r.Group("/info")
	{
		infoGroup.GET("/server", infoCtrl.ServerInfo)
		infoGroup.GET("/client", infoCtrl.ClientInfo)
		infoGroup.GET("/database", infoCtrl.DatabaseInfo)
	}

	return r
}
