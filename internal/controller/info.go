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
	ctx.JSON(http.StatusOK, data)
}

func (c *InfoController) DatabaseInfo(ctx *gin.Context) {
	data := c.service.GetDatabaseInfo()
	ctx.JSON(http.StatusOK, data)
}

func (c *InfoController) ClientInfo(ctx *gin.Context) {
	ip := ctx.ClientIP()
	userAgent := ctx.GetHeader("User-Agent")
	lang := ctx.GetHeader("Accept-Language")

	data := c.service.GetClientInfo(ip, userAgent, lang)

	ctx.JSON(http.StatusOK, data)
}
