package controller

import (
	"net/http"
	"strconv"
	"yugu-server/internal/dto"
	"yugu-server/internal/service"

	"github.com/gin-gonic/gin"
)

type PermissionController struct {
	permService service.PermissionService
}

func NewPermissionController(permService service.PermissionService) *PermissionController {
	return &PermissionController{permService: permService}
}

func (ctrl *PermissionController) CreatePermission(c *gin.Context) {
	var req dto.StorePermissionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "Неверный формат данных: " + err.Error()})
		return
	}

	userIDObj, _ := c.Get("user_id")
	permDTO, err := ctrl.permService.CreatePermission(req, userIDObj.(uint))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, permDTO)
}

func (ctrl *PermissionController) GetPermissions(c *gin.Context) {
	perms, err := ctrl.permService.GetPermissions()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка получения списка разрешений"})
		return
	}
	c.JSON(http.StatusOK, perms)
}

func (ctrl *PermissionController) UpdatePermission(c *gin.Context) {
	permID, err := strconv.ParseUint(c.Param("permission"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный ID разрешения"})
		return
	}

	var req dto.UpdateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "Неверный формат данных"})
		return
	}

	updatedPerm, err := ctrl.permService.UpdatePermission(uint(permID), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, updatedPerm)
}

func (ctrl *PermissionController) HardDeletePermission(c *gin.Context) {
	permID, err := strconv.ParseUint(c.Param("permission"), 10, 32)
	if err == nil && ctrl.permService.HardDeletePermission(uint(permID)) == nil {
		c.JSON(http.StatusOK, gin.H{"message": "Разрешение удалено навсегда"})
		return
	}
	c.JSON(http.StatusBadRequest, gin.H{"error": "Ошибка при удалении"})
}

func (ctrl *PermissionController) SoftDeletePermission(c *gin.Context) {
	permID, err := strconv.ParseUint(c.Param("permission"), 10, 32)
	userIDObj, _ := c.Get("user_id")
	if err == nil && ctrl.permService.SoftDeletePermission(uint(permID), userIDObj.(uint)) == nil {
		c.JSON(http.StatusOK, gin.H{"message": "Разрешение мягко удалено"})
		return
	}
	c.JSON(http.StatusBadRequest, gin.H{"error": "Ошибка при удалении"})
}

func (ctrl *PermissionController) RestorePermission(c *gin.Context) {
	permID, err := strconv.ParseUint(c.Param("permission"), 10, 32)
	if err == nil && ctrl.permService.RestorePermission(uint(permID)) == nil {
		c.JSON(http.StatusOK, gin.H{"message": "Разрешение восстановлено"})
		return
	}
	c.JSON(http.StatusBadRequest, gin.H{"error": "Ошибка при восстановлении"})
}
