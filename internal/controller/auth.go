package controller

import (
	"errors"
	"net/http"
	"yugu-server/internal/dto"
	"yugu-server/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
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
			"error":   "Ошибка проверки данных",
			"details": translateError(err),
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

	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusUnprocessableEntity, gin.H{
			"error":   "Ошибка проверки данных",
			"details": translateError(err),
		})
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

// GET /api/auth/me
func (c *AuthController) Me(ctx *gin.Context) {
	userIDVal, exists := ctx.Get("user_id")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Не удалось идентифицировать пользователя"})
		return
	}

	userID := userIDVal.(uint)

	userDTO, err := c.authService.GetMe(userID)
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, userDTO)
}

// GET /api/auth/tokens
func (c *AuthController) GetTokens(ctx *gin.Context) {
	userIDVal, _ := ctx.Get("user_id")
	userID := userIDVal.(uint)

	sessions, err := c.authService.GetTokens(userID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка получения сессий"})
		return
	}

	if sessions == nil {
		sessions = []dto.SessionDTO{}
	}

	ctx.JSON(http.StatusOK, sessions)
}

// POST /api/auth/out_all
func (c *AuthController) LogoutAll(ctx *gin.Context) {
	userIDVal, _ := ctx.Get("user_id")
	userID := userIDVal.(uint)

	if err := c.authService.LogoutAll(userID); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при выходе"})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "Выполнен выход со всех устройств"})
}

// POST /api/auth/refresh
func (c *AuthController) Refresh(ctx *gin.Context) {
	var req dto.RefreshRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusUnprocessableEntity, gin.H{"error": "Необходим refresh_token"})
		return
	}

	userAgent := ctx.GetHeader("User-Agent")
	ip := ctx.ClientIP()

	tokens, err := c.authService.RefreshTokens(req, userAgent, ip)
	if err != nil {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"access_token":  tokens.AccessToken,
		"refresh_token": tokens.RefreshToken,
	})
}

// POST /api/auth/out
func (c *AuthController) Logout(ctx *gin.Context) {
	var req dto.RefreshRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusUnprocessableEntity, gin.H{"error": "Необходим refresh_token"})
		return
	}

	_ = c.authService.Logout(req) // Ошибку игнорируем, если токена нет - значит уже вышли
	ctx.JSON(http.StatusOK, gin.H{"message": "Успешный выход"})
}

// Функция-переводчик для валидатора
func translateError(err error) string {
	var ve validator.ValidationErrors
	if errors.As(err, &ve) {
		out := ""
		for _, fe := range ve {
			switch fe.Tag() {
			case "required":
				out += "Поле '" + fe.Field() + "' обязательно для заполнения. "
			case "min":
				out += "Поле '" + fe.Field() + "' слишком короткое (минимум " + fe.Param() + " симв.). "
			case "email":
				out += "Неверный формат почты. "
			case "alpha_capital":
				out += "Логин должен начинаться с большой буквы и содержать только латиницу. "
			case "password_complex":
				out += "Пароль должен быть сложным (цифры, спецсимволы, разные регистры). "
			case "eqfield":
				out += "Введенные пароли не совпадают. "
			case "datetime":
				out += "Неверный формат даты (ожидается ГГГГ-ММ-ДД). "
			case "age_14":
				out += "Регистрация разрешена только с 14 лет. "
			default:
				out += "Ошибка в поле '" + fe.Field() + "'. "
			}
		}
		return out
	}
	return "Некорректный формат данных."
}
