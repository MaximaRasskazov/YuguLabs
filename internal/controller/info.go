package controller

import (
	"net/http"
	"yugu-server/internal/service"

	"github.com/gin-gonic/gin"
)

type InfoController struct {
	service service.InfoService
}

func NewInfoController(s service.InfoService) *InfoController {
	return &InfoController{service: s}
}

func (c *InfoController) ServerInfo(ctx *gin.Context) {
	data := c.service.GetServerInfo()
	// Gin сам ставит Content-Type и сериализует в JSON
	ctx.JSON(http.StatusOK, data)
}

func (c *InfoController) DatabaseInfo(ctx *gin.Context) {
	data := c.service.GetDatabaseInfo()
	ctx.JSON(http.StatusOK, data)
}

func (c *InfoController) ClientInfo(ctx *gin.Context) {
	// 1. Собираем параметры из запроса средствами Gin
	ip := ctx.ClientIP() // Автоматически убирает порт!
	userAgent := ctx.GetHeader("User-Agent")
	lang := ctx.GetHeader("Accept-Language") // Вытаскиваем язык

	// 2. Отдаем в сервис на обработку
	data := c.service.GetClientInfo(ip, userAgent, lang)

	// 3. Возвращаем ответ
	ctx.JSON(http.StatusOK, data)
}
