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

	err = db.AutoMigrate(&User{}, &TokenSession{})
	if err != nil {
		log.Fatalf("Ошибка при миграции таблиц: %v", err)
	}

	log.Println("✅ База данных SQLite готова")
	return db
}
