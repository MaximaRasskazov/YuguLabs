package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"yugu-server/internal/dto"
	"yugu-server/internal/service"
)

type AuthController struct {
	authService service.AuthService
}

func NewAuthController(as service.AuthService) *AuthController {
	return &AuthController{authService: as}
}

func (c *AuthController) Register(ctx *gin.Context) {
	var req dto.RegisterRequest

	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusUnprocessableEntity, gin.H{
			"error":   "Ошибка валидации данных",
			"details": err.Error(),
		})
		return
	}

	userDTO, err := c.authService.Register(req)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusCreated, userDTO)
}

func (c *AuthController) Login(ctx *gin.Context) {
	var req dto.LoginRequest

	// username и password
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusUnprocessableEntity, gin.H{"error": "Некорректный запрос"})
		return
	}

	userAgent := ctx.GetHeader("User-Agent")
	ip := ctx.ClientIP()

	authSuccessDTO, err := c.authService.Login(req, userAgent, ip)
	if err != nil {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, authSuccessDTO)
}