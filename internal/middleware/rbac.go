package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// RequirePermission проверяет наличие конкретного права (slug) у текущего пользователя
func RequirePermission(db *gorm.DB, requiredSlug string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. Достаем ID пользователя из контекста
		// (Его туда должен был положить твой базовый AuthMiddleware проверки токена)
		userIDObj, exists := c.Get("userID")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Пользователь не авторизован"})
			c.Abort()
			return
		}

		userID := userIDObj.(uint)

		// 2. Делаем умный SQL-запрос через GORM.
		// Нам нужно пройти по цепочке: User -> UserRole -> Role -> RolePermission -> Permission
		var count int64
		err := db.Table("users").
			Joins("JOIN role_user ON users.id = role_user.user_id").
			Joins("JOIN roles ON role_user.role_id = roles.id").
			Joins("JOIN permission_role ON roles.id = permission_role.role_id").
			Joins("JOIN permissions ON permission_role.permission_id = permissions.id").
			Where("users.id = ? AND permissions.slug = ?", userID, requiredSlug).
			// ВАЖНО: При ручных JOIN-ах нужно самим проверять Soft Deletes!
			Where("role_user.deleted_at IS NULL AND permission_role.deleted_at IS NULL AND roles.deleted_at IS NULL").
			Count(&count).Error

		// 3. Проверяем результат
		if err != nil || count == 0 {
			// Требование ТЗ: Статус 403 и строго определенный JSON
			c.JSON(http.StatusForbidden, gin.H{
				"error": "Доступ запрещен. Необходимое разрешение: " + requiredSlug,
			})
			c.Abort()
			return
		}

		// 4. Если права есть — пропускаем запрос дальше в контроллер
		c.Next()
	}
}
