package controller

import (
	"net/http"
	"strconv"
	"yugu-server/internal/dto"
	"yugu-server/internal/service"

	"github.com/gin-gonic/gin"
)

type RoleController struct {
	roleService service.RoleService
}

func NewRoleController(roleService service.RoleService) *RoleController {
	return &RoleController{roleService: roleService}
}

// CreateRole - POST /api/ref/policy/role
func (ctrl *RoleController) CreateRole(c *gin.Context) {
	var req dto.StoreRoleRequest

	// 1. Валидация (сработают наши правила из validator, включая slug_format)
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "Неверный формат данных: " + err.Error()})
		return
	}

	// 2. Достаем ID текущего пользователя (он 100% есть, т.к. маршрут защищен AuthMiddleware)
	userIDObj, _ := c.Get("userID")
	userID := userIDObj.(uint)

	// 3. Передаем в сервис
	roleDTO, err := ctrl.roleService.CreateRole(req, userID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 4. Успешный ответ (Статус 201 Created по ТЗ)
	c.JSON(http.StatusCreated, roleDTO)
}

// GetRoles - GET /api/ref/policy/role
func (ctrl *RoleController) GetRoles(c *gin.Context) {
	roles, err := ctrl.roleService.GetRoles()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка получения списка ролей"})
		return
	}

	c.JSON(http.StatusOK, roles)
}

// SoftDeleteRole - DELETE /api/ref/policy/role/{role}/soft
func (ctrl *RoleController) SoftDeleteRole(c *gin.Context) {
	roleIDStr := c.Param("role")
	roleID, _ := strconv.ParseUint(roleIDStr, 10, 32)

	currentUserIDObj, _ := c.Get("userID")
	currentUserID := currentUserIDObj.(uint)

	if err := ctrl.roleService.SoftDeleteRole(uint(roleID), currentUserID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при мягком удалении роли"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Роль успешно перемещена в корзину"})
}

// RestoreRole - POST /api/ref/policy/role/{role}/restore
func (ctrl *RoleController) RestoreRole(c *gin.Context) {
	roleIDStr := c.Param("role")
	roleID, _ := strconv.ParseUint(roleIDStr, 10, 32)

	if err := ctrl.roleService.RestoreRole(uint(roleID)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при восстановлении роли"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Роль успешно восстановлена"})
}

// HardDeleteRole - DELETE /api/ref/policy/role/{role}
func (ctrl *RoleController) HardDeleteRole(c *gin.Context) {
	roleIDStr := c.Param("role")
	roleID, _ := strconv.ParseUint(roleIDStr, 10, 32)

	if err := ctrl.roleService.HardDeleteRole(uint(roleID)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при жестком удалении роли"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Роль удалена навсегда"})
}

func (ctrl *RoleController) UpdateRole(c *gin.Context) {
	// 1. Достаем ID роли из URL
	roleIDStr := c.Param("role")
	roleID, err := strconv.ParseUint(roleIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный ID роли"})
		return
	}

	// 2. Парсим и валидируем тело запроса
	var req dto.UpdateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "Неверный формат данных: " + err.Error()})
		return
	}

	// 3. Отправляем в сервис
	updatedRole, err := ctrl.roleService.UpdateRole(uint(roleID), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 4. Возвращаем обновленный объект
	c.JSON(http.StatusOK, updatedRole)
}
