package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/rbac"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/transport/http/dto"
	mw "github.com/MaximaRasskazov/Academic-debt-system/backend/internal/transport/http/middleware"
)

// RBACHandler — хендлер HTTP-API управления ролями и правами.
type RBACHandler struct {
	svc *rbac.Service
}

func NewRBACHandler(svc *rbac.Service) *RBACHandler {
	return &RBACHandler{svc: svc}
}

// ListRoles — GET /api/roles. Только admin.
func (h *RBACHandler) ListRoles(w http.ResponseWriter, r *http.Request) {
	roles, err := h.svc.ListRoles(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "не удалось получить список ролей")
		return
	}
	writeJSON(w, http.StatusOK, dto.FromRoles(roles))
}

// ListPermissions — GET /api/permissions. Только admin.
func (h *RBACHandler) ListPermissions(w http.ResponseWriter, r *http.Request) {
	perms, err := h.svc.ListPermissions(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "не удалось получить список прав")
		return
	}
	writeJSON(w, http.StatusOK, dto.FromPermissions(perms))
}

// AssignRole — POST /api/users/:id/roles.
// Выдать роль пользователю. Защищён privilege escalation guard в RBACService.
func (h *RBACHandler) AssignRole(w http.ResponseWriter, r *http.Request) {
	actorID, ok := mw.UserID(r.Context())
	if !ok {
		writeError(w, http.StatusInternalServerError, "internal", "нет userID в контексте")
		return
	}

	targetID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "некорректный UUID пользователя")
		return
	}

	var req dto.AssignRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.RoleSlug == "" {
		writeError(w, http.StatusBadRequest, "invalid_body", "поле role_slug обязательно")
		return
	}

	if err := h.svc.AssignRole(r.Context(), actorID, targetID, req.RoleSlug); err != nil {
		mapRBACError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ChangeRole — POST /api/users/:id/roles/change.
// Атомарно меняет роль (снять from_slug + выдать to_slug в одной транзакции).
// Заменяет связку AssignRole+RevokeRole двумя запросами с фронта.
func (h *RBACHandler) ChangeRole(w http.ResponseWriter, r *http.Request) {
	actorID, ok := mw.UserID(r.Context())
	if !ok {
		writeError(w, http.StatusInternalServerError, "internal", "нет userID в контексте")
		return
	}

	targetID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "некорректный UUID пользователя")
		return
	}

	var req dto.ChangeRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ToSlug == "" {
		writeError(w, http.StatusBadRequest, "invalid_body", "поле to_slug обязательно")
		return
	}

	if err := h.svc.ChangeRole(r.Context(), actorID, targetID, req.FromSlug, req.ToSlug); err != nil {
		mapRBACError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// RevokeRole — DELETE /api/users/:id/roles/:slug.
func (h *RBACHandler) RevokeRole(w http.ResponseWriter, r *http.Request) {
	actorID, ok := mw.UserID(r.Context())
	if !ok {
		writeError(w, http.StatusInternalServerError, "internal", "нет userID в контексте")
		return
	}

	targetID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "некорректный UUID пользователя")
		return
	}

	slug := chi.URLParam(r, "slug")
	if slug == "" {
		writeError(w, http.StatusBadRequest, "invalid_slug", "slug роли обязателен")
		return
	}

	if err := h.svc.RevokeRole(r.Context(), actorID, targetID, slug); err != nil {
		mapRBACError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListUserPermissions — GET /api/users/:id/permissions.
// Плоский список slug'ов прав пользователя.
func (h *RBACHandler) ListUserPermissions(w http.ResponseWriter, r *http.Request) {
	targetID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "некорректный UUID пользователя")
		return
	}

	slugs, err := h.svc.ListUserPermissions(r.Context(), targetID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "не удалось получить права пользователя")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"permissions": slugs})
}

// ListUserRoles — GET /api/users/:id/roles.
func (h *RBACHandler) ListUserRoles(w http.ResponseWriter, r *http.Request) {
	targetID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "некорректный UUID пользователя")
		return
	}

	roles, err := h.svc.ListUserRoles(r.Context(), targetID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "не удалось получить роли пользователя")
		return
	}
	writeJSON(w, http.StatusOK, dto.FromRoles(roles))
}

func mapRBACError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, rbac.ErrRoleNotFound):
		writeError(w, http.StatusNotFound, "role_not_found", "роль не найдена")
	case errors.Is(err, rbac.ErrUserNotFound):
		writeError(w, http.StatusNotFound, "user_not_found", "пользователь не найден")
	case errors.Is(err, rbac.ErrPermissionDenied):
		writeError(w, http.StatusForbidden, "forbidden", "недостаточно прав")
	case errors.Is(err, rbac.ErrPrivilegeEscalation):
		writeError(w, http.StatusForbidden, "privilege_escalation", "нельзя выдать роль выше своего уровня")
	default:
		writeError(w, http.StatusInternalServerError, "internal", "внутренняя ошибка сервиса")
	}
}
