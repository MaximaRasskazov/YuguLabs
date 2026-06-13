// Package handler содержит HTTP-обработчики API.
//
// Поведение хендлеров намеренно тонкое: парсинг JSON, вызов сервиса,
// маппинг результата в DTO. Бизнес-валидация и проверки прав — в
// сервисах, чтобы они одинаково работали из HTTP и (если понадобится)
// из CLI/tests.
package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/auth"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/token"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/transport/http/dto"
)

// writeJSON сериализует payload в JSON с указанным status-кодом.
// Используется во всех handler'ах для единообразного формата ответа.
func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if payload == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		slog.Error("encode response", "err", err)
	}
}

// writeError отвечает кодом + единым ErrorResponse-форматом.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, dto.ErrorResponse{Error: code, Message: message})
}

// mapAuthError маппит sentinel-ошибки сервисов auth/token в HTTP-коды.
// Незнакомые ошибки → 500, чтобы не маскировать баги под валидацию.
func mapAuthError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrEmailTaken):
		writeError(w, http.StatusConflict, "email_taken", "email уже зарегистрирован")
	case errors.Is(err, auth.ErrInvalidEmail):
		writeError(w, http.StatusBadRequest, "invalid_email", "некорректный email")
	case errors.Is(err, auth.ErrPasswordTooShort):
		writeError(w, http.StatusBadRequest, "password_too_short", "пароль слишком короткий")
	case errors.Is(err, auth.ErrInvalidCredentials):
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "неверный email или пароль")
	case errors.Is(err, auth.ErrEmulatorUnavailable):
		writeError(w, http.StatusServiceUnavailable, "emulator_unavailable", "сервис деканата временно недоступен, попробуйте позже")
	case errors.Is(err, auth.ErrUserNotFound):
		writeError(w, http.StatusNotFound, "user_not_found", "пользователь не найден")
	case errors.Is(err, auth.ErrInvalidPassword):
		writeError(w, http.StatusUnauthorized, "invalid_password", "неверный текущий пароль")
	case errors.Is(err, auth.ErrSamePassword):
		writeError(w, http.StatusBadRequest, "same_password", "новый пароль совпадает с текущим")
	case errors.Is(err, token.ErrRefreshNotFound),
		errors.Is(err, token.ErrRefreshExhausted),
		errors.Is(err, token.ErrRefreshReplay):
		writeError(w, http.StatusUnauthorized, "refresh_invalid", "refresh-токен недействителен")
	default:
		slog.Error("unhandled service error", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "внутренняя ошибка сервиса")
	}
}
