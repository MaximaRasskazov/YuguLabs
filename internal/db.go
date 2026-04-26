package repository

import (
	"log"
	"os"
	"path/filepath"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func SetupDatabase() *gorm.DB {
	dbDir := "database"
	if err := os.MkdirAll(dbDir, os.ModePerm); err != nil {
		log.Fatalf("Не удалось создать папку: %v", err)
	}

	dbPath := filepath.Join(dbDir, "app.db")

	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		log.Fatalf("Ошибка БД: %v", err)
	}

	err = db.AutoMigrate(
		&User{},
		&Role{},
		&Permission{},
		&UserRole{},
		&RolePermission{},
		&TokenSession{},
	)
	if err != nil {
		log.Fatalf("Ошибка при миграции таблиц: %v", err)
	}

	SeedDatabase(db)

	log.Println("✅ База данных SQLite готова")
	return db
}

func SeedDatabase(db *gorm.DB) {
	var count int64
	db.Model(&Role{}).Count(&count)

	// Если база не пустая, сидер не запускаем
	if count > 0 {
		return
	}

	log.Println("🌱 Запуск сидеров: наполнение базы ролями и разрешениями...")

	// 1. Создаем системного пользователя для полей created_by (если его нет)
	systemUserID := uint(1)

	// 2. Создаем роли
	adminRole := Role{Name: "Admin", Slug: "admin", CreatedByID: systemUserID}
	userRole := Role{Name: "User", Slug: "user", CreatedByID: systemUserID}
	guestRole := Role{Name: "Guest", Slug: "guest", CreatedByID: systemUserID}

	db.Create(&adminRole)
	db.Create(&userRole)
	db.Create(&guestRole)

	// 3. Создаем 18 разрешений (по ТЗ: get-list, read, create, update, delete, restore)
	entities := []string{"user", "role", "permission"}
	actions := []string{"get-list", "read", "create", "update", "delete", "restore"}

	var allPermissions []Permission

	for _, entity := range entities {
		for _, action := range actions {
			perm := Permission{
				Name:        action + " " + entity,
				Slug:        action + "-" + entity, // Например: get-list-user
				CreatedByID: systemUserID,
			}
			db.Create(&perm)
			allPermissions = append(allPermissions, perm)
		}
	}

	// 4. Связываем роли и разрешения ВРУЧНУЮ, чтобы заполнить created_by
	// Админу даем ВСЕ 18 разрешений
	for _, perm := range allPermissions {
		db.Create(&RolePermission{RoleID: adminRole.ID, PermissionID: perm.ID, CreatedByID: systemUserID})
	}

	// Юзеру даем: get-list-user, read-user, update-user
	var userPermissions []Permission
	db.Where("slug IN ?", []string{"get-list-user", "read-user", "update-user"}).Find(&userPermissions)
	for _, perm := range userPermissions {
		db.Create(&RolePermission{RoleID: userRole.ID, PermissionID: perm.ID, CreatedByID: systemUserID})
	}

	// Гостю даем только get-list-user
	var guestPermissions []Permission
	db.Where("slug = ?", "get-list-user").Find(&guestPermissions)
	for _, perm := range guestPermissions {
		db.Create(&RolePermission{RoleID: guestRole.ID, PermissionID: perm.ID, CreatedByID: systemUserID})
	}

	log.Println("✅ Сиды успешно отработали!")
}
