package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func RequirePermission(db *gorm.DB, requiredSlug string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userIDObj, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Не авторизован"})
			c.Abort()
			return
		}
		userID := userIDObj.(uint)

		// Если Админ
		var isAdmin int64
		db.Table("role_user").
			Joins("INNER JOIN roles ON role_user.role_id = roles.id").
			Where("role_user.user_id = ? AND roles.slug = 'admin'", userID). // Ищем роль со слагом admin
			Where("role_user.deleted_at IS NULL AND roles.deleted_at IS NULL").
			Count(&isAdmin)

		if isAdmin > 0 {
			c.Next()
			return
		}

		// Юзеры
		var count int64
		err := db.Table("users").
			Joins("INNER JOIN role_user ON users.id = role_user.user_id").                      // какие должности занимает человек
			Joins("INNER JOIN permission_role ON role_user.role_id = permission_role.role_id"). // какие ключи (права) выданы этой должности
			Joins("INNER JOIN permissions ON permission_role.permission_id = permissions.id").  // как именно называются эти права (slug)
			Where("users.id = ? AND permissions.slug = ?", userID, requiredSlug).
			Where("users.deleted_at IS NULL").           // Защита от забаненных
			Where("role_user.deleted_at IS NULL").       // Защита от уволенных
			Where("permission_role.deleted_at IS NULL"). // Защита от отзыва прав
			Where("permissions.deleted_at IS NULL").     // Защита от удаленных фичей
			Count(&count).Error

		if err != nil || count == 0 {
			c.JSON(http.StatusForbidden, gin.H{"error": "Доступ запрещен: нет прав или они были отозваны"})
			c.Abort()
			return
		}

		c.Next()
	}
}
