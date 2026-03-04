package repository

import (
	"time"
	"gorm.io/gorm"
)

type User struct {
	ID        uint      `gorm:"primaryKey"`
	Username  string    `gorm:"uniqueIndex;not null;"` 
	Email     string    `gorm:"uniqueIndex;not null;collate:nocase"` 
	Password  string    `gorm:"not null"`
	Birthday  time.Time `gorm:"type:date;not null"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

type TokenSession struct {
	ID        uint      `gorm:"primaryKey"`
	UserID    uint      `gorm:"not null;index"`
	TokenHash string    `gorm:"uniqueIndex;not null"` 
	UserAgent string   
	IPAddress string 
	ExpiresAt time.Time `gorm:"not null"`
	CreatedAt time.Time
}