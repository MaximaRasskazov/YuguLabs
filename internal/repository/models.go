package repository

import (
	"time"

	"gorm.io/gorm"
)

type User struct {
	ID        uint      `gorm:"primaryKey"`
	Username  string    `gorm:"unique;uniqueIndex;not null;"`
	Email     string    `gorm:"uniqueIndex;not null;collate:nocase"`
	Password  string    `gorm:"not null"`
	Birthday  time.Time `gorm:"type:date;not null"`
	CreatedAt time.Time
	UpdatedAt time.Time

	// GORM сам поймет, что нужно использовать промежуточную таблицу role_user
	Roles []Role `gorm:"many2many:role_user;"`
}

type TokenSession struct {
	ID        uint   `gorm:"primaryKey"`
	UserID    uint   `gorm:"not null;index"`
	TokenHash string `gorm:"uniqueIndex;not null"`
	UserAgent string
	IPAddress string
	ExpiresAt time.Time `gorm:"not null"`
	CreatedAt time.Time
}

// Role представляет таблицу roles
type Role struct {
	ID          uint      `gorm:"primaryKey"`
	Name        string    `gorm:"not null;uniqueIndex"`
	Slug        string    `gorm:"not null;uniqueIndex"`
	Description *string   `gorm:"type:text"` // Указатель делает поле nullable
	CreatedAt   time.Time `gorm:"not null"`
	CreatedByID uint      `gorm:"column:created_by;not null"`
	UpdatedAt   time.Time
	DeletedAt   gorm.DeletedAt `gorm:"index;column:deleted_at"` // Мягкое удаление
	DeletedByID *uint          `gorm:"column:deleted_by"`       // Nullable, так как при создании никто не удалял

	Permissions []Permission `gorm:"many2many:permission_role;"` // Связь с разрешениями
}

type Permission struct {
	ID          uint      `gorm:"primaryKey"`
	Name        string    `gorm:"not null;uniqueIndex"`
	Slug        string    `gorm:"not null;uniqueIndex"`
	Description *string   `gorm:"type:text"`
	CreatedAt   time.Time `gorm:"not null"`
	CreatedByID uint      `gorm:"column:created_by;not null"`
	UpdatedAt   time.Time
	DeletedAt   gorm.DeletedAt `gorm:"index;column:deleted_at"`
	DeletedByID *uint          `gorm:"column:deleted_by"`
}

type UserRole struct {
	ID          uint           `gorm:"primaryKey"`
	UserID      uint           `gorm:"not null;index"`
	RoleID      uint           `gorm:"not null;index"`
	CreatedAt   time.Time      `gorm:"not null"`
	CreatedByID uint           `gorm:"column:created_by;not null"`
	DeletedAt   gorm.DeletedAt `gorm:"index;column:deleted_at"`
	DeletedByID *uint          `gorm:"column:deleted_by"`
}

// TableName переопределяет имя таблицы, чтобы оно строго соответствовало ТЗ (role_user)
func (UserRole) TableName() string {
	return "role_user"
}

type RolePermission struct {
	ID           uint           `gorm:"primaryKey"`
	RoleID       uint           `gorm:"not null;index"`
	PermissionID uint           `gorm:"not null;index"`
	CreatedAt    time.Time      `gorm:"not null"`
	CreatedByID  uint           `gorm:"column:created_by;not null"`
	DeletedAt    gorm.DeletedAt `gorm:"index;column:deleted_at"`
	DeletedByID  *uint          `gorm:"column:deleted_by"`
}

func (RolePermission) TableName() string {
	return "permission_role"
}
