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
	userIDObj, _ := c.Get("user_id")
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

	currentUserIDObj, _ := c.Get("user_id")
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

	// БОНУС: Добавлена проверка на то, что ID - это число (чтобы /role/abc/restore выдавал 400)
	roleID, err := strconv.ParseUint(roleIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный формат ID роли"})
		return
	}

	// Вызываем сервис
	if err := ctrl.roleService.RestoreRole(uint(roleID)); err != nil {
		// Ловим нашу кастомную ошибку "фантомного восстановления"
		if err.Error() == "роль не найдена или была удалена навсегда" {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		// Если упала сама база данных (ошибка синтаксиса, обрыв связи)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Внутренняя ошибка сервера: " + err.Error()})
		return
	}

	// Если ошибок нет - отдаем заветный 200 OK
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

func (ctrl *RoleController) AssignPermission(c *gin.Context) {
	roleIDStr := c.Param("role")
	roleID, _ := strconv.ParseUint(roleIDStr, 10, 32)

	var input struct {
		PermissionID uint `json:"permission_id" binding:"required"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(400, gin.H{"error": "Неверный формат данных"})
		return
	}

	if err := ctrl.roleService.AssignPermissionToRole(uint(roleID), input.PermissionID); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{"message": "Разрешение успешно добавлено роли"})
}

// GetRolePermissions - GET /api/ref/policy/role/{role}/permission
func (ctrl *RoleController) GetRolePermissions(c *gin.Context) {
	roleIDStr := c.Param("role")
	roleID, err := strconv.ParseUint(roleIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный ID роли"})
		return
	}

	permissions, err := ctrl.roleService.GetPermissionsByRoleID(uint(roleID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при получении разрешений роли"})
		return
	}

	var response []dto.PermissionAdminResponse
	for _, p := range permissions {
		response = append(response, dto.PermissionAdminResponse{
			ID:          p.ID,
			Name:        p.Name,
			Slug:        p.Slug,
			CreatedByID: p.CreatedByID,
			DeletedByID: p.DeletedByID,
		})
	}

	c.JSON(http.StatusOK, response)
}

func (ctrl *RoleController) RevokePermission(c *gin.Context) {
	// Достаем ID из URL
	roleID, _ := strconv.ParseUint(c.Param("role"), 10, 32)
	permID, _ := strconv.ParseUint(c.Param("permission"), 10, 32)

	if err := ctrl.roleService.RevokePermission(uint(roleID), uint(permID)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Не удалось отозвать разрешение"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Разрешение успешно отозвано у роли"})
}
