package router

import (
	"yugu-server/internal/controller"
	"yugu-server/internal/middleware"
	"yugu-server/internal/repository"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func SetupRouter(db *gorm.DB, infoCtrl *controller.InfoController, authCtrl *controller.AuthController, roleCtrl *controller.RoleController, userRoleCtrl *controller.UserRoleController, permCtrl *controller.PermissionController) *gin.Engine {
	r := gin.Default()

	infoGroup := r.Group("/info")
	{
		infoGroup.GET("/server", infoCtrl.ServerInfo)
		infoGroup.GET("/database", infoCtrl.DatabaseInfo)
		infoGroup.GET("/client", infoCtrl.ClientInfo)

		// --- ДЛЯ ЛАБЫ ---
		infoGroup.GET("/make-me-admin", func(c *gin.Context) {
			// Легально создаем связь через твою структуру, указывая CreatedByID
			link := repository.UserRole{
				UserID:      1,
				RoleID:      1,
				CreatedByID: 1, // ID того, кто выдал роль
			}

			if err := db.FirstOrCreate(&link, repository.UserRole{UserID: 1, RoleID: 1}).Error; err != nil {
				c.JSON(500, gin.H{"error": "Ошибка выдачи роли: " + err.Error()})
				return
			}

			c.JSON(200, gin.H{"message": "Успех! База пробита легально. ID=1 теперь Админ 👑"})
		})
	}

	api := r.Group("/api")
	{
		// Блок авторизации
		auth := api.Group("/auth")
		{
			auth.POST("/register", authCtrl.Register)
			auth.POST("/login", authCtrl.Login)
			auth.POST("/refresh", authCtrl.Refresh)

			// Защищенные
			auth.GET("/me", middleware.AuthRequired(), authCtrl.Me)
			auth.GET("/tokens", middleware.AuthRequired(), authCtrl.GetTokens)
			auth.POST("/out_all", middleware.AuthRequired(), authCtrl.LogoutAll)
			auth.POST("/out", middleware.AuthRequired(), authCtrl.Logout)
			auth.GET("/me/permissions", middleware.AuthRequired(), userRoleCtrl.GetMyPermissions)
			auth.GET("/me/roles", middleware.AuthRequired(), userRoleCtrl.GetMyRoles)
		}

		// Блок RBAC (ролевая система)
		ref := api.Group("/ref")

		ref.Use(middleware.AuthRequired())

		roleGroup := ref.Group("/policy/role")
		{
			roleGroup.GET("", middleware.RequirePermission(db, "get-list-role"), roleCtrl.GetRoles)
			roleGroup.POST("", middleware.RequirePermission(db, "create-role"), roleCtrl.CreateRole)
			roleGroup.DELETE("/:role", middleware.RequirePermission(db, "delete-role"), roleCtrl.HardDeleteRole)
			roleGroup.DELETE("/:role/soft", middleware.RequirePermission(db, "delete-role"), roleCtrl.SoftDeleteRole)
			roleGroup.POST("/:role/restore", middleware.RequirePermission(db, "restore-role"), roleCtrl.RestoreRole)
			roleGroup.PUT("/:role", middleware.RequirePermission(db, "update-role"), roleCtrl.UpdateRole)
			roleGroup.PATCH("/:role", middleware.RequirePermission(db, "update-role"), roleCtrl.UpdateRole)
			roleGroup.POST("/:role/permission", roleCtrl.AssignPermission)
		}

		userGroup := ref.Group("/user")
		{
			userGroup.GET("/:user/role", middleware.RequirePermission(db, "read-user"), userRoleCtrl.GetUserRoles)
			userGroup.POST("/:user/role", middleware.RequirePermission(db, "update-user"), userRoleCtrl.AssignRole)
			// Жесткое удаление
			userGroup.DELETE("/:user/role/:role", middleware.RequirePermission(db, "update-user"), userRoleCtrl.HardRemoveRole)

			// Мягкое удаление
			userGroup.DELETE("/:user/role/:role/soft", middleware.RequirePermission(db, "update-user"), userRoleCtrl.SoftRemoveRole)

			// Восстановление
			userGroup.POST("/:user/role/:role/restore", middleware.RequirePermission(db, "update-user"), userRoleCtrl.RestoreUserRole)
		}

		permGroup := ref.Group("/policy/permission")
		{
			permGroup.GET("", middleware.RequirePermission(db, "get-list-permission"), permCtrl.GetPermissions)
			permGroup.POST("", middleware.RequirePermission(db, "create-permission"), permCtrl.CreatePermission)

			permGroup.PUT("/:permission", middleware.RequirePermission(db, "update-permission"), permCtrl.UpdatePermission)
			permGroup.PATCH("/:permission", middleware.RequirePermission(db, "update-permission"), permCtrl.UpdatePermission)

			permGroup.DELETE("/:permission", middleware.RequirePermission(db, "delete-permission"), permCtrl.HardDeletePermission)
			permGroup.DELETE("/:permission/soft", middleware.RequirePermission(db, "delete-permission"), permCtrl.SoftDeletePermission)
			permGroup.POST("/:permission/restore", middleware.RequirePermission(db, "restore-permission"), permCtrl.RestorePermission)
		}
	}

	return r
}
