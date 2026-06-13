package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/changelog"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/transport/http/dto"
	mw "github.com/MaximaRasskazov/Academic-debt-system/backend/internal/transport/http/middleware"
)

// ChangeLogHandler — история изменений сущностей (story) и откат (undo).
type ChangeLogHandler struct {
	cl *changelog.Service
}

func NewChangeLogHandler(cl *changelog.Service) *ChangeLogHandler {
	return &ChangeLogHandler{cl: cl}
}

const (
	defaultStoryLimit = 50
	maxStoryLimit     = 200
)

// UserStory godoc
//
//	@Summary	История изменений пользователя
//	@Tags		changelog
//	@Produce	json
//	@Param		id	path	string	true	"ID пользователя"
//	@Success	200	{array}	dto.ChangeLogEntry
//	@Security	BearerAuth
//	@Router		/api/users/{id}/story [get]
func (h *ChangeLogHandler) UserStory(w http.ResponseWriter, r *http.Request) {
	h.entityStory(w, r, changelog.EntityUser)
}

// RoleStory godoc
//
//	@Summary	История изменений роли
//	@Tags		changelog
//	@Produce	json
//	@Param		id	path	string	true	"ID роли"
//	@Success	200	{array}	dto.ChangeLogEntry
//	@Security	BearerAuth
//	@Router		/api/roles/{id}/story [get]
func (h *ChangeLogHandler) RoleStory(w http.ResponseWriter, r *http.Request) {
	h.entityStory(w, r, changelog.EntityRole)
}

// PermissionStory godoc
//
//	@Summary	История изменений права
//	@Tags		changelog
//	@Produce	json
//	@Param		id	path	string	true	"ID права"
//	@Success	200	{array}	dto.ChangeLogEntry
//	@Security	BearerAuth
//	@Router		/api/permissions/{id}/story [get]
func (h *ChangeLogHandler) PermissionStory(w http.ResponseWriter, r *http.Request) {
	h.entityStory(w, r, changelog.EntityPermission)
}

// Recent godoc
//
//	@Summary	Журнал последних изменений по всем сущностям
//	@Tags		changelog
//	@Produce	json
//	@Param		limit	query	int	false	"Сколько записей (по умолчанию 50, максимум 200)"
//	@Param		offset	query	int	false	"Смещение"
//	@Success	200	{array}	dto.ChangeLogEntry
//	@Security	BearerAuth
//	@Router		/api/changelog [get]
func (h *ChangeLogHandler) Recent(w http.ResponseWriter, r *http.Request) {
	limit, offset := storyPaging(r)
	rows, err := h.cl.ListRecent(r.Context(), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "не удалось получить журнал")
		return
	}
	entries, err := dto.FromRecentChangeLogs(rows)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "не удалось разобрать журнал")
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

func (h *ChangeLogHandler) entityStory(w http.ResponseWriter, r *http.Request, entityType string) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid_id", "не указан id сущности")
		return
	}
	limit, offset := storyPaging(r)

	rows, err := h.cl.ListForEntity(r.Context(), entityType, id, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "не удалось получить историю")
		return
	}
	entries, err := dto.FromChangeLogs(rows)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "не удалось разобрать историю")
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

// Restore godoc
//
//	@Summary	Откат сущности к состоянию из записи истории (undo)
//	@Description	Применяет before из записи лога. Поддержано для профиля пользователя и выдачи/снятия роли.
//	@Tags		changelog
//	@Param		id	path	int	true	"ID записи истории"
//	@Success	204
//	@Failure	400	{object}	dto.ErrorResponse	"Откат для этого типа записи не поддержан"
//	@Failure	404	{object}	dto.ErrorResponse	"Запись истории не найдена"
//	@Security	BearerAuth
//	@Router		/api/changelog/{id}/restore [post]
func (h *ChangeLogHandler) Restore(w http.ResponseWriter, r *http.Request) {
	logID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Идентификатор записи истории должен быть числом.")
		return
	}
	actorID, ok := mw.UserID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Требуется авторизация — войдите в систему.")
		return
	}

	if err := h.cl.RestoreFromLog(r.Context(), logID, actorID); err != nil {
		switch {
		case errors.Is(err, changelog.ErrLogNotFound):
			writeError(w, http.StatusNotFound, "log_not_found",
				"Запись истории не найдена — возможно, неверный id или запись удалена.")
		case errors.Is(err, changelog.ErrRestoreUnsupported):
			writeError(w, http.StatusUnprocessableEntity, "restore_unsupported",
				"Эту запись нельзя откатить: для событий «создание» и «смена пароля» откат не предусмотрен — возвращать нечего.")
		case errors.Is(err, changelog.ErrRestoreNoop):
			writeError(w, http.StatusConflict, "restore_noop",
				"Откат не требуется: запись уже в этом состоянии (вероятно, откат был выполнен ранее).")
		default:
			writeError(w, http.StatusInternalServerError, "internal",
				"Не удалось выполнить откат. Попробуйте позже.")
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// storyPaging читает limit/offset из query с дефолтами и потолком.
func storyPaging(r *http.Request) (limit, offset int32) {
	limit, offset = defaultStoryLimit, 0
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 {
		if v > maxStoryLimit {
			v = maxStoryLimit
		}
		limit = int32(v) //nolint:gosec // ограничено maxStoryLimit
	}
	if v, err := strconv.Atoi(r.URL.Query().Get("offset")); err == nil && v > 0 {
		offset = int32(v) //nolint:gosec
	}
	return limit, offset
}
