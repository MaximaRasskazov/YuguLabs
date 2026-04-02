package router

import (
	"yugu-server/internal/controller"
	"yugu-server/internal/middleware"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Обрати внимание на добавленный 5-й аргумент: userRoleCtrl *controller.UserRoleController
func SetupRouter(db *gorm.DB, infoCtrl *controller.InfoController, authCtrl *controller.AuthController, roleCtrl *controller.RoleController, userRoleCtrl *controller.UserRoleController, permCtrl *controller.PermissionController) *gin.Engine {
	r := gin.Default()

	infoGroup := r.Group("/info")
	{
		infoGroup.GET("/server", infoCtrl.ServerInfo)
		infoGroup.GET("/database", infoCtrl.DatabaseInfo)
		infoGroup.GET("/client", infoCtrl.ClientInfo)
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
		}

		// Блок RBAC (ролевая система)
		ref := api.Group("/ref")

		ref.Use(middleware.AuthRequired())

		// Подгруппа /policy/role
		roleGroup := ref.Group("/policy/role")
		{
			roleGroup.GET("", roleCtrl.GetRoles)
			roleGroup.POST("", middleware.RequirePermission(db, "create-role"), roleCtrl.CreateRole)
			roleGroup.DELETE("/:role", middleware.RequirePermission(db, "delete-role"), roleCtrl.HardDeleteRole)
			roleGroup.DELETE("/:role/soft", middleware.RequirePermission(db, "delete-role"), roleCtrl.SoftDeleteRole)
			roleGroup.POST("/:role/restore", middleware.RequirePermission(db, "restore-role"), roleCtrl.RestoreRole)
			roleGroup.PUT("/:role", middleware.RequirePermission(db, "update-role"), roleCtrl.UpdateRole)
			roleGroup.PATCH("/:role", middleware.RequirePermission(db, "update-role"), roleCtrl.UpdateRole)
		}

		// Подгруппа /user
		userGroup := ref.Group("/user")
		{
			userGroup.GET("/:user/role", userRoleCtrl.GetUserRoles)
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
			permGroup.GET("", permCtrl.GetPermissions)
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
