package handler

import (
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/photo"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/transport/http/dto"
	mw "github.com/MaximaRasskazov/Academic-debt-system/backend/internal/transport/http/middleware"
)

// downloadPath — относительный путь скачивания оригинала. Защищён авторизацией
// (только владелец), поэтому в DTO кладём один и тот же путь без идентификатора:
// сервер берёт фото текущего пользователя из токена.
const downloadPath = "/api/photo/download"

// PhotoHandler — HTTP-слой работы с фотографией профиля.
type PhotoHandler struct {
	photos *photo.Service
}

// NewPhotoHandler собирает обработчики с зависимостью на photo.Service.
func NewPhotoHandler(svc *photo.Service) *PhotoHandler {
	return &PhotoHandler{photos: svc}
}

// avatarURL строит относительный URL аватара пользователя с версией —
// ?v=<unix> заставляет браузер перезапросить картинку после смены.
func avatarURL(id uuid.UUID, ver time.Time) string {
	return fmt.Sprintf("/api/users/%s/avatar?v=%d", id, ver.Unix())
}

// photoResponse собирает DTO из метаданных фотографии.
func photoResponse(info *photo.Info) dto.PhotoResponse {
	return dto.PhotoResponse{
		ID:           info.ID,
		OriginalName: info.OriginalName,
		Description:  info.Description,
		Format:       info.Format,
		SizeBytes:    info.SizeBytes,
		Width:        info.Width,
		Height:       info.Height,
		AvatarURL:    avatarURL(info.UserID, info.UpdatedAt),
		DownloadURL:  downloadPath,
		CreatedAt:    info.CreatedAt,
		UpdatedAt:    info.UpdatedAt,
	}
}

// Upload godoc
//
//	@Summary	Загрузить фотографию профиля
//	@Description	multipart-форма, поле «photo» (или «avatar»). JPEG/PNG/WebP. Сервер проверяет подлинность, сжимает оригинал и делает аватар 128×128.
//	@Tags		photo
//	@Accept		mpfd
//	@Produce	json
//	@Param		photo		formData	file	true	"Файл изображения"
//	@Param		description	formData	string	false	"Описание"
//	@Success	200	{object}	dto.PhotoResponse
//	@Failure	400	{object}	dto.ErrorResponse	"Файл отсутствует/повреждён"
//	@Failure	413	{object}	dto.ErrorResponse	"Файл/разрешение слишком большие"
//	@Failure	415	{object}	dto.ErrorResponse	"Не изображение (в т.ч. подмена расширения)"
//	@Security	BearerAuth
//	@Router		/api/photo [post]
func (h *PhotoHandler) Upload(w http.ResponseWriter, r *http.Request) {
	userID, ok := mw.UserID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Требуется авторизация — войдите в систему.")
		return
	}

	// Жёстко ограничиваем размер тела (+1КБ на multipart-обвязку), чтобы не
	// читать в память заведомо огромный аплоад.
	limit := int64(photo.MaxUploadBytes) + 1<<10
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	if err := r.ParseMultipartForm(limit); err != nil {
		if isMaxBytes(err) {
			writeError(w, http.StatusRequestEntityTooLarge, "photo_too_large",
				"Файл слишком большой — максимум 16 МБ. Выберите файл поменьше.")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid_body",
			"Не удалось обработать загрузку. Выберите изображение и попробуйте снова.")
		return
	}

	file, header, err := formFile(r, "photo", "avatar")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_file",
			"Файл не найден в запросе. Приложите изображение в поле «photo».")
		return
	}
	defer func() { _ = file.Close() }()

	content, err := io.ReadAll(file)
	if err != nil {
		if isMaxBytes(err) {
			writeError(w, http.StatusRequestEntityTooLarge, "photo_too_large",
				"Файл больше допустимого: максимум 16 МБ.")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid_file",
			"Не удалось прочитать содержимое файла. Попробуйте загрузить его ещё раз.")
		return
	}

	var filename string
	if header != nil {
		filename = header.Filename
	}
	info, err := h.photos.Upload(r.Context(), userID, content, filename, r.FormValue("description"), userID)
	if err != nil {
		mapPhotoError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, photoResponse(info))
}

// Info godoc
//
//	@Summary	Метаданные текущей фотографии профиля
//	@Tags		photo
//	@Produce	json
//	@Success	200	{object}	dto.PhotoResponse
//	@Failure	404	{object}	dto.ErrorResponse	"Фотографии нет"
//	@Security	BearerAuth
//	@Router		/api/photo [get]
func (h *PhotoHandler) Info(w http.ResponseWriter, r *http.Request) {
	userID, ok := mw.UserID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Требуется авторизация — войдите в систему.")
		return
	}
	info, err := h.photos.Info(r.Context(), userID)
	if err != nil {
		mapPhotoError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, photoResponse(info))
}

// Delete godoc
//
//	@Summary	Удалить фотографию профиля
//	@Tags		photo
//	@Success	204
//	@Security	BearerAuth
//	@Router		/api/photo [delete]
func (h *PhotoHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID, ok := mw.UserID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Требуется авторизация — войдите в систему.")
		return
	}
	if err := h.photos.Delete(r.Context(), userID, userID); err != nil {
		mapPhotoError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Download godoc
//
//	@Summary	Скачать оригинал своей фотографии
//	@Description	Защищено: оригинал отдаётся только владельцу, прямого URL у файла нет.
//	@Tags		photo
//	@Produce	image/*
//	@Success	200	{file}	binary
//	@Failure	404	{object}	dto.ErrorResponse	"Фотографии нет"
//	@Security	BearerAuth
//	@Router		/api/photo/download [get]
func (h *PhotoHandler) Download(w http.ResponseWriter, r *http.Request) {
	userID, ok := mw.UserID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Требуется авторизация — войдите в систему.")
		return
	}
	orig, err := h.photos.Original(r.Context(), userID)
	if err != nil {
		mapPhotoError(w, err)
		return
	}
	w.Header().Set("Content-Type", orig.ContentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(orig.Content)))
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", downloadFilename(orig)))
	_, _ = w.Write(orig.Content)
}

// Archive godoc
//
//	@Summary	Выгрузить архив всех фотографий (только админ)
//	@Description	ZIP: сжатые оригиналы, аватары и Excel-реестр. Требует право photos.export.
//	@Tags		photo
//	@Produce	application/zip
//	@Success	200	{file}	binary
//	@Security	BearerAuth
//	@Router		/api/photo/archive [post]
func (h *PhotoHandler) Archive(w http.ResponseWriter, r *http.Request) {
	arc, err := h.photos.BuildArchive(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "archive_failed",
			"Не удалось собрать архив фотографий. Попробуйте позже.")
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Length", strconv.Itoa(len(arc.Content)))
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", arc.Filename))
	_, _ = w.Write(arc.Content)
}

// ListAll godoc
//
//	@Summary	Список всех фотографий (только админ)
//	@Description	Метаданные всех активных фото + ссылки на миниатюры. Требует право photos.export.
//	@Tags		photo
//	@Produce	json
//	@Success	200	{array}	dto.AdminPhotoResponse
//	@Security	BearerAuth
//	@Router		/api/photo/all [get]
func (h *PhotoHandler) ListAll(w http.ResponseWriter, r *http.Request) {
	items, err := h.photos.ListAll(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "не удалось получить список фотографий")
		return
	}
	out := make([]dto.AdminPhotoResponse, 0, len(items))
	for _, p := range items {
		out = append(out, dto.AdminPhotoResponse{
			ID:           p.ID,
			UserID:       p.UserID,
			Email:        p.Email,
			FullName:     p.FullName,
			GroupName:    p.GroupName,
			OriginalName: p.OriginalName,
			Description:  p.Description,
			Format:       p.Format,
			SizeBytes:    p.SizeBytes,
			Width:        p.Width,
			Height:       p.Height,
			AvatarURL:    avatarURL(p.UserID, p.CreatedAt),
			CreatedAt:    p.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// ServeAvatar godoc
//
//	@Summary	Отдать аватар пользователя (публично)
//	@Description	Без авторизации — чтобы тег <img> мог загрузить миниатюру. 404, если фото нет.
//	@Tags		photo
//	@Produce	image/*
//	@Param		id	path	string	true	"ID пользователя"
//	@Success	200	{file}	binary
//	@Failure	404	{object}	dto.ErrorResponse
//	@Router		/api/users/{id}/avatar [get]
func (h *PhotoHandler) ServeAvatar(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Некорректный идентификатор пользователя в адресе.")
		return
	}
	av, err := h.photos.Avatar(r.Context(), id)
	if err != nil {
		if errors.Is(err, photo.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "Не удалось получить аватар. Попробуйте позже.")
		return
	}
	w.Header().Set("Content-Type", av.ContentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(av.Content)))
	// max-age=0 + must-revalidate: браузер кеширует, но проверяет свежесть
	// (Last-Modified); URL дополнительно версионируется ?v=.
	w.Header().Set("Cache-Control", "private, max-age=0, must-revalidate")
	w.Header().Set("Last-Modified", av.UpdatedAt.UTC().Format(http.TimeFormat))
	_, _ = w.Write(av.Content)
}

// formFile достаёт файл из первого присутствующего поля формы (поддержка
// и ТЗ-имени «photo», и старого фронтового «avatar»).
func formFile(r *http.Request, fields ...string) (multipart.File, *multipart.FileHeader, error) {
	var lastErr error
	for _, f := range fields {
		file, header, err := r.FormFile(f)
		if err == nil {
			return file, header, nil
		}
		lastErr = err
	}
	return nil, nil, lastErr
}

// downloadFilename подбирает имя файла для скачивания, гарантируя расширение.
func downloadFilename(o *photo.Original) string {
	name := o.OriginalName
	if name == "" {
		name = "photo"
	}
	if !strings.Contains(name, ".") {
		ext := "jpg"
		if o.Format == "png" {
			ext = "png"
		}
		name = name + "." + ext
	}
	return name
}

// isMaxBytes сообщает, является ли ошибка превышением http.MaxBytesReader.
func isMaxBytes(err error) bool {
	var maxErr *http.MaxBytesError
	return errors.As(err, &maxErr)
}

// mapPhotoError маппит ошибки photo-сервиса в HTTP-коды с понятными
// пользователю сообщениями. Сообщения ориентированы на конечного
// пользователя: коротко «что не так» и «что сделать», без технических
// подробностей реализации (механизм проверки описан в docs/lr-avatar.md).
func mapPhotoError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, photo.ErrEmpty):
		writeError(w, http.StatusBadRequest, "photo_empty",
			"Файл пустой: похоже, он не выбран или имеет нулевой размер. Выберите изображение и попробуйте снова.")
	case errors.Is(err, photo.ErrTooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, "photo_too_large",
			"Файл слишком большой — максимум 16 МБ. Выберите файл поменьше.")
	case errors.Is(err, photo.ErrImageTooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, "image_too_large",
			"Слишком большое разрешение изображения — максимум 40 мегапикселей. Уменьшите разрешение снимка.")
	case errors.Is(err, photo.ErrInvalidImage):
		writeError(w, http.StatusUnsupportedMediaType, "invalid_image",
			"Не удалось распознать файл как изображение. Загрузите фото в формате JPEG, PNG или WebP — возможно, файл повреждён или это не изображение.")
	case errors.Is(err, photo.ErrNotFound):
		writeError(w, http.StatusNotFound, "photo_not_found",
			"У вас пока нет загруженной фотографии.")
	default:
		writeError(w, http.StatusInternalServerError, "internal",
			"Внутренняя ошибка при обработке фотографии. Попробуйте ещё раз.")
	}
}
