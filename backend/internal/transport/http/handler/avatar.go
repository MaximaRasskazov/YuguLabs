package handler

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/auth"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/transport/http/dto"
	mw "github.com/MaximaRasskazov/Academic-debt-system/backend/internal/transport/http/middleware"
)

// avatarURL строит относительный URL аватара пользователя с версией —
// ?v=<unix> заставляет браузер перезапросить картинку после смены.
func avatarURL(id uuid.UUID, ver time.Time) string {
	return fmt.Sprintf("/api/users/%s/avatar?v=%d", id, ver.Unix())
}

// UploadAvatar godoc
//
//	@Summary	Загрузить аватар текущего пользователя
//	@Description	multipart-форма, поле «avatar». Допустимы JPEG/PNG/WebP до 2 МБ.
//	@Tags		auth
//	@Accept		mpfd
//	@Produce	json
//	@Param		avatar	formData	file	true	"Файл изображения"
//	@Success	200	{object}	dto.AvatarResponse
//	@Failure	400	{object}	dto.ErrorResponse	"Файл отсутствует/повреждён"
//	@Failure	413	{object}	dto.ErrorResponse	"Файл больше 2 МБ"
//	@Failure	415	{object}	dto.ErrorResponse	"Недопустимый тип файла"
//	@Security	BearerAuth
//	@Router		/api/me/avatar [post]
func (h *AuthHandler) UploadAvatar(w http.ResponseWriter, r *http.Request) {
	userID, ok := mw.UserID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	// Жёстко ограничиваем размер тела (+1КБ на multipart-обвязку).
	r.Body = http.MaxBytesReader(w, r.Body, auth.MaxAvatarBytes+1<<10)
	if err := r.ParseMultipartForm(auth.MaxAvatarBytes + 1<<10); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "avatar_too_large", "файл слишком большой (макс 2 МБ)")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid_body", "не удалось разобрать форму")
		return
	}

	file, _, err := r.FormFile("avatar")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_file", "ожидался файл в поле «avatar»")
		return
	}
	defer func() { _ = file.Close() }()

	content, err := io.ReadAll(file)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "avatar_too_large", "файл слишком большой (макс 2 МБ)")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid_file", "не удалось прочитать файл")
		return
	}

	if err := h.auth.SetAvatar(r.Context(), userID, content); err != nil {
		mapAuthError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, dto.AvatarResponse{AvatarURL: avatarURL(userID, time.Now())})
}

// DeleteAvatar godoc
//
//	@Summary	Удалить аватар текущего пользователя
//	@Tags		auth
//	@Success	204
//	@Security	BearerAuth
//	@Router		/api/me/avatar [delete]
func (h *AuthHandler) DeleteAvatar(w http.ResponseWriter, r *http.Request) {
	userID, ok := mw.UserID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}
	if err := h.auth.DeleteAvatar(r.Context(), userID); err != nil {
		mapAuthError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ServeAvatar godoc
//
//	@Summary	Отдать аватар пользователя (публично)
//	@Description	Без авторизации — чтобы тег <img> мог загрузить картинку. 404, если аватара нет.
//	@Tags		auth
//	@Produce	image/*
//	@Param		id	path	string	true	"ID пользователя"
//	@Success	200	{file}	binary
//	@Failure	404	{object}	dto.ErrorResponse
//	@Router		/api/users/{id}/avatar [get]
func (h *AuthHandler) ServeAvatar(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "некорректный id пользователя")
		return
	}

	av, err := h.auth.GetAvatar(r.Context(), id)
	if err != nil {
		if errors.Is(err, auth.ErrAvatarNotFound) {
			http.NotFound(w, r)
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "внутренняя ошибка сервиса")
		return
	}

	w.Header().Set("Content-Type", av.ContentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(av.Content)))
	// max-age=0 + must-revalidate: браузер закеширует, но проверит свежесть
	// (Last-Modified) — после смены аватара URL ещё и версионируется ?v=.
	w.Header().Set("Cache-Control", "private, max-age=0, must-revalidate")
	w.Header().Set("Last-Modified", av.UpdatedAt.UTC().Format(http.TimeFormat))
	_, _ = w.Write(av.Content)
}
