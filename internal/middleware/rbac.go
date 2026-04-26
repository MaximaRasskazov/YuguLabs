package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Наличие конкретного права slug
func RequirePermission(db *gorm.DB, requiredSlug string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userIDObj, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Пользователь не авторизован"})
			c.Abort()
			return
		}

		userID := userIDObj.(uint)

		db.Table("permission_role").Update("deleted_at", "2026-04-22 18:40:55.3819249+05:00").Where("permission_id=1")
		var count int64
		err := db.Table("role_user").
			Joins("INNER JOIN permission_role ON role_user.role_id = permission_role.role_id").
			Joins("INNER JOIN permissions ON permission_role.permission_id = permissions.id").
			Where("role_user.user_id = ? AND permissions.slug = ?", userID, requiredSlug).
			Where("role_user.deleted_at IS NULL").
			Count(&count).Error

		if err != nil || count == 0 {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "Доступ запрещен. Необходимое разрешение: " + requiredSlug,
			})
			c.Abort()
			return
		}

		c.Next()
	}
}
