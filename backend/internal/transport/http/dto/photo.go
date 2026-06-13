package dto

import (
	"time"

	"github.com/google/uuid"
)

// AdminPhotoResponse — элемент списка «все фотографии» для администратора
// (GET /api/photo/all). Миниатюру админ смотрит по avatar_url; бинарного
// содержимого в списке нет.
type AdminPhotoResponse struct {
	ID           int64     `json:"id"`
	UserID       uuid.UUID `json:"user_id"`
	Email        string    `json:"email"`
	FullName     string    `json:"full_name"`
	GroupName    string    `json:"group_name,omitempty"`
	OriginalName string    `json:"original_name"`
	Description  *string   `json:"description,omitempty"`
	Format       string    `json:"format"`
	SizeBytes    int32     `json:"size_bytes"`
	Width        int32     `json:"width"`
	Height       int32     `json:"height"`
	AvatarURL    string    `json:"avatar_url"`
	CreatedAt    time.Time `json:"created_at"`
}

// PhotoResponse — метаданные фотографии профиля (ответ GET/POST /api/photo).
// Бинарное содержимое сюда не входит: аватар отдаётся по avatar_url,
// оригинал — по download_url (требует авторизации владельца).
type PhotoResponse struct {
	ID           int64     `json:"id"`
	OriginalName string    `json:"original_name"`
	Description  *string   `json:"description,omitempty"`
	Format       string    `json:"format"`
	SizeBytes    int32     `json:"size_bytes"`
	Width        int32     `json:"width"`
	Height       int32     `json:"height"`
	AvatarURL    string    `json:"avatar_url"`
	DownloadURL  string    `json:"download_url"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
