package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/user"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/transport/http/dto"
	mw "github.com/MaximaRasskazov/Academic-debt-system/backend/internal/transport/http/middleware"
)

// UsersHandler — обёртка GET /api/users с фильтрами.
type UsersHandler struct {
	svc *user.Service
}

func NewUsersHandler(svc *user.Service) *UsersHandler {
	return &UsersHandler{svc: svc}
}

// List godoc
//
//	@Summary	Список пользователей (admin / dean)
//	@Tags		users
//	@Param		role		query	string	false	"Фильтр по slug роли: student / teacher / dean / admin"
//	@Param		search		query	string	false	"Подстрока по email / first_name / last_name"
//	@Param		group_name	query	string	false	"Точный матч по group_name (для студентов)"
//	@Param		limit		query	int		false	"Лимит (по умолчанию 50, max 200)"
//	@Param		offset		query	int		false	"Смещение"
//	@Success	200	{object}	dto.UsersListResponse
//	@Failure	403	{object}	dto.ErrorResponse
//	@Security	BearerAuth
//	@Router		/api/users [get]
func (h *UsersHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	in := user.ListInput{
		RoleSlug:  q.Get("role"),
		Search:    q.Get("search"),
		GroupName: q.Get("group_name"),
		Limit:     parseInt32(q.Get("limit"), 50),
		Offset:    parseInt32(q.Get("offset"), 0),
	}

	result, err := h.svc.List(r.Context(), in)
	if err != nil {
		// Здесь специфичных sentinel-ошибок нет — все ошибки сервиса
		// внутренние (БД). Возвращаем 500.
		_ = errors.Unwrap(err) // подавляем ineffassign в линтере для err — это документально
		writeError(w, http.StatusInternalServerError, "internal", "не удалось получить список пользователей")
		return
	}
	writeJSON(w, http.StatusOK, dto.FromUserListResult(result))
}

// Update godoc
//
//	@Summary	Изменить профиль пользователя (админ)
//	@Description	Правка ФИО / группы / даты рождения любого пользователя. Изменение логируется в change_logs с автором-администратором. Требует право users.update.
//	@Tags		users
//	@Accept		json
//	@Produce	json
//	@Param		id		path	string						true	"ID пользователя"
//	@Param		body	body	dto.UpdateProfileRequest	true	"Поля для обновления (все опциональны)"
//	@Success	200		{object}	dto.UserResponse
//	@Failure	400		{object}	dto.ErrorResponse
//	@Failure	404		{object}	dto.ErrorResponse
//	@Security	BearerAuth
//	@Router		/api/users/{id} [patch]
func (h *UsersHandler) Update(w http.ResponseWriter, r *http.Request) {
	actorID, ok := mw.UserID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Требуется авторизация — войдите в систему.")
		return
	}
	targetID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Некорректный идентификатор пользователя в адресе.")
		return
	}

	var req dto.UpdateProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Тело запроса должно быть JSON с полями профиля.")
		return
	}

	updated, err := h.svc.UpdateUser(r.Context(), targetID, user.UpdateInput{
		FirstName:  req.FirstName,
		LastName:   req.LastName,
		MiddleName: req.MiddleName,
		GroupName:  req.GroupName,
		Birthday:   req.Birthday,
	}, actorID)
	if err != nil {
		switch {
		case errors.Is(err, user.ErrUserNotFound):
			writeError(w, http.StatusNotFound, "user_not_found", "Пользователь не найден.")
		case errors.Is(err, user.ErrInvalidProfile):
			writeError(w, http.StatusBadRequest, "invalid_profile", "Имя и фамилия не могут быть пустыми.")
		default:
			writeError(w, http.StatusInternalServerError, "internal", "Не удалось обновить пользователя. Попробуйте позже.")
		}
		return
	}
	writeJSON(w, http.StatusOK, dto.FromUser(updated))
}
