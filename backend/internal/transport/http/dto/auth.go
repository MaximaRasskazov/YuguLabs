// Package dto содержит API-схемы запросов и ответов HTTP-слоя.
//
// Назначение — не выпускать наружу sqlc-структуры (которые содержат
// password_hash и pgtype.*-типы) и держать API-контракт стабильным,
// даже если меняется схема БД.
package dto

import (
	"time"

	"github.com/google/uuid"

	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/pgutil"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/repo/queries"
)

// RegisterRequest — тело POST /api/auth/register.
type RegisterRequest struct {
	Email      string     `json:"email"`
	Password   string     `json:"password"`
	FirstName  string     `json:"first_name"`
	LastName   string     `json:"last_name"`
	MiddleName *string    `json:"middle_name,omitempty"`
	Birthday   *time.Time `json:"birthday,omitempty"`
	GroupName  *string    `json:"group_name,omitempty"`
}

// LoginRequest — тело POST /api/auth/login.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// AuthResponse — ответ на register/login/refresh. Refresh-токен в JSON
// не возвращаем — он уходит в httpOnly-cookie. Клиент знает только
// о существовании access-токена и хранит его в памяти.
//
// User присутствует в ответах register и login (там сервис всё равно
// читает пользователя из БД). В refresh User не передаётся — клиент
// уже знает, кто он, и при необходимости запрашивает /api/auth/me.
type AuthResponse struct {
	AccessToken     string        `json:"access_token"`
	AccessExpiresAt time.Time     `json:"access_expires_at"`
	TokenType       string        `json:"token_type"`
	User            *UserResponse `json:"user,omitempty"`
}

// UserResponse — публичные данные пользователя. password_hash и
// pgtype.*-поля сюда не маппятся.
type UserResponse struct {
	ID         uuid.UUID  `json:"id"`
	Email      string     `json:"email"`
	FirstName  string     `json:"first_name"`
	LastName   string     `json:"last_name"`
	MiddleName *string    `json:"middle_name,omitempty"`
	Birthday   *time.Time `json:"birthday,omitempty"`
	GroupName  *string    `json:"group_name,omitempty"`
	// AvatarURL — относительный URL аватара с версией (?v=unix) для
	// cache-busting; nil, если аватар не загружен. Заполняется только там,
	// где сервис проверил наличие (напр. /api/auth/me).
	AvatarURL *string   `json:"avatar_url,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// RoleResponse — компактное представление роли для /me.
type RoleResponse struct {
	ID    uuid.UUID `json:"id"`
	Slug  string    `json:"slug"`
	Name  string    `json:"name"`
	Level int32     `json:"level"`
}

// ProfileResponse — ответ на GET /api/auth/me. Permissions — плоский
// список slug'ов, фронту удобно проверять через .includes().
type ProfileResponse struct {
	User        UserResponse   `json:"user"`
	Roles       []RoleResponse `json:"roles"`
	Permissions []string       `json:"permissions"`
}

// ErrorResponse — единый формат ошибок API.
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

// FromUser маппит sqlc-структуру в публичный UserResponse.
func FromUser(u queries.User) UserResponse {
	out := UserResponse{
		ID:         pgutil.UUID(u.ID),
		Email:      u.Email,
		FirstName:  u.FirstName,
		LastName:   u.LastName,
		MiddleName: u.MiddleName,
		GroupName:  u.GroupName,
		CreatedAt:  u.CreatedAt.Time,
	}
	if u.Birthday.Valid {
		t := u.Birthday.Time
		out.Birthday = &t
	}
	return out
}

// FromRole маппит sqlc-роль в RoleResponse.
func FromRole(r queries.Role) RoleResponse {
	return RoleResponse{
		ID:    pgutil.UUID(r.ID),
		Slug:  r.Slug,
		Name:  r.Name,
		Level: r.Level,
	}
}

// FromRoles превращает срез ролей в срез response-структур.
func FromRoles(rs []queries.Role) []RoleResponse {
	out := make([]RoleResponse, 0, len(rs))
	for _, r := range rs {
		out = append(out, FromRole(r))
	}
	return out
}
