package controller

import (
	"net/http"
	"strconv"
	"yugu-server/internal/dto"
	"yugu-server/internal/service"

	"github.com/gin-gonic/gin"
)

type UserRoleController struct {
	userRoleService service.UserRoleService
}

func NewUserRoleController(userRoleService service.UserRoleService) *UserRoleController {
	return &UserRoleController{userRoleService: userRoleService}
}

// AssignRole - POST /api/ref/user/{user}/role
func (ctrl *UserRoleController) AssignRole(c *gin.Context) {
	// 1. Достаем ID целевого пользователя из URL (параметр :user)
	targetUserIDStr := c.Param("user")
	targetUserID, err := strconv.ParseUint(targetUserIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный ID пользователя в URL"})
		return
	}

	// 2. Валидация JSON-тела (ожидаем {"role_id": 1})
	var req dto.AttachUserRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "Неверный формат данных: " + err.Error()})
		return
	}

	// 3. Достаем ID текущего админа, который делает запрос
	currentUserIDObj, _ := c.Get("user_id")
	currentUserID := currentUserIDObj.(uint)

	// 4. Передаем в сервис
	if err := ctrl.userRoleService.AssignRole(uint(targetUserID), req.RoleID, currentUserID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Роль успешно назначена пользователю"})
}

// GetUserRoles - GET /api/ref/user/{user}/role
func (ctrl *UserRoleController) GetUserRoles(c *gin.Context) {
	targetUserIDStr := c.Param("user")
	targetUserID, err := strconv.ParseUint(targetUserIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный ID пользователя в URL"})
		return
	}

	roles, err := ctrl.userRoleService.GetUserRoles(uint(targetUserID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, roles)
}

// HardRemoveRole - DELETE /api/ref/user/{user}/role/{role}
func (ctrl *UserRoleController) HardRemoveRole(c *gin.Context) {
	targetUserID, err1 := strconv.ParseUint(c.Param("user"), 10, 32)
	roleID, err2 := strconv.ParseUint(c.Param("role"), 10, 32)

	if err1 != nil || err2 != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный ID пользователя или роли"})
		return
	}

	if err := ctrl.userRoleService.HardRemoveRole(uint(targetUserID), uint(roleID)); err != nil {
		// Ловим кастомную ошибку "фантомного" удаления
		if err.Error() == "связь не найдена (нечего удалять)" {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при жестком удалении роли: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Роль успешно и навсегда удалена у пользователя"})
}

// SoftRemoveRole - DELETE /api/ref/user/{user}/role/{role}/soft
func (ctrl *UserRoleController) SoftRemoveRole(c *gin.Context) {
	targetUserID, err1 := strconv.ParseUint(c.Param("user"), 10, 32)
	roleID, err2 := strconv.ParseUint(c.Param("role"), 10, 32)

	if err1 != nil || err2 != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный ID пользователя или роли"})
		return
	}

	currentUserIDObj, _ := c.Get("user_id")
	currentUserID := currentUserIDObj.(uint)

	if err := ctrl.userRoleService.SoftRemoveRole(uint(targetUserID), uint(roleID), currentUserID); err != nil {
		// Ловим кастомную ошибку "фантомного" мягкого удаления
		if err.Error() == "активная связь не найдена (возможно, уже удалена)" {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при мягком удалении роли: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Роль мягко удалена у пользователя"})
}

// RestoreUserRole - POST /api/ref/user/{user}/role/{role}/restore
func (ctrl *UserRoleController) RestoreUserRole(c *gin.Context) {
	// Достаем параметры из URL
	userStr := c.Param("user")
	roleStr := c.Param("role")

	userID, err1 := strconv.ParseUint(userStr, 10, 32)
	roleID, err2 := strconv.ParseUint(roleStr, 10, 32)

	if err1 != nil || err2 != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный формат ID"})
		return
	}

	// Вызываем сервис
	if err := ctrl.userRoleService.RestoreUserRole(uint(userID), uint(roleID)); err != nil {
		// Ловим нашу кастомную ошибку и отдаем честный 404
		if err.Error() == "связь не найдена или была удалена навсегда" {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		// Для остальных ошибок 500
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при восстановлении: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Успех: Роль возвращена пользователю"})
}
