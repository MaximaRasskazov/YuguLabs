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
	targetUserIDStr := c.Param("user")
	targetUserID, err := strconv.ParseUint(targetUserIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный ID пользователя в URL"})
		return
	}

	// Здесь мы теперь ждем структуру с массивом RoleIDs
	var req dto.AttachUserRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "Неверный формат данных. Ожидается массив role_ids: " + err.Error()})
		return
	}

	currentUserIDObj, _ := c.Get("user_id")
	currentUserID := currentUserIDObj.(uint)

	// Вызываем новый метод AssignRoles
	if err := ctrl.userRoleService.AssignRoles(uint(targetUserID), req.RoleIDs, currentUserID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Ошибка при назначении ролей: " + err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Роли успешно назначены пользователю"})
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

// GetMyPermissions - GET /api/auth/me/permissions
// НОВАЯ ФУНКЦИЯ ДЛЯ ПРОСМОТРА СВОИХ ПРАВ
func (ctrl *UserRoleController) GetMyPermissions(c *gin.Context) {
	currentUserIDObj, _ := c.Get("user_id")
	currentUserID := currentUserIDObj.(uint)

	permissions, err := ctrl.userRoleService.GetUserPermissions(currentUserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при получении ваших разрешений"})
		return
	}

	// Перекладываем данные, отсекая лишнее
	var response []dto.UserPermissionResponse
	for _, p := range permissions {
		desc := ""
		if p.Description != nil {
			desc = *p.Description
		}

		response = append(response, dto.UserPermissionResponse{
			Name:        p.Name,
			Slug:        p.Slug,
			Description: desc,
		})
	}

	c.JSON(http.StatusOK, response)
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
		if err.Error() == "связь не найдена (нечего удалять)" {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при жестком удалении: " + err.Error()})
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
		if err.Error() == "активная связь не найдена (возможно, уже удалена)" {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при мягком удалении: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Роль мягко удалена у пользователя"})
}

// RestoreUserRole - POST /api/ref/user/{user}/role/{role}/restore
func (ctrl *UserRoleController) RestoreUserRole(c *gin.Context) {
	userID, err1 := strconv.ParseUint(c.Param("user"), 10, 32)
	roleID, err2 := strconv.ParseUint(c.Param("role"), 10, 32)

	if err1 != nil || err2 != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный формат ID"})
		return
	}

	if err := ctrl.userRoleService.RestoreUserRole(uint(userID), uint(roleID)); err != nil {
		if err.Error() == "связь не найдена или была удалена навсегда" {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при восстановлении: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Успех: Роль возвращена пользователю"})
}

// GetMyRoles - GET /api/auth/me/roles
// Позволяет текущему пользователю увидеть список своих должностей (ролей)
func (ctrl *UserRoleController) GetMyRoles(c *gin.Context) {
	currentUserIDObj, _ := c.Get("user_id")
	currentUserID := currentUserIDObj.(uint)

	// Мы переиспользуем УЖЕ СУЩЕСТВУЮЩИЙ метод сервиса! Нам не нужно писать SQL-запрос заново.
	roles, err := ctrl.userRoleService.GetUserRoles(currentUserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при получении ваших ролей"})
		return
	}

	// Перекладываем данные в красивую DTO
	var response []dto.UserRoleResponse
	for _, r := range roles {
		response = append(response, dto.UserRoleResponse{
			Name:        r.Name,
			Slug:        r.Slug,
			Description: r.Description, // Просто берем напрямую!
		})
	}

	c.JSON(http.StatusOK, response)
}
