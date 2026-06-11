package http

import (
	"github.com/go-chi/chi/v5"

	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/transport/http/handler"
	mw "github.com/MaximaRasskazov/Academic-debt-system/backend/internal/transport/http/middleware"
)

// mountChangelog регистрирует историю изменений (story) и откат (undo).
//
//	GET  /api/users/{id}/story        — история пользователя   (changelog.view)
//	GET  /api/roles/{id}/story        — история роли           (changelog.view)
//	GET  /api/permissions/{id}/story  — история права          (changelog.view)
//	POST /api/changelog/{id}/restore  — откат к состоянию из лога (changelog.restore)
func mountChangelog(r chi.Router, d Deps) {
	if d.Changelog == nil {
		return
	}
	h := handler.NewChangeLogHandler(d.Changelog)

	r.Group(func(r chi.Router) {
		r.Use(mw.Auth(d.Tokens))

		r.With(mw.RequirePermission(d.RBAC, "changelog.view")).
			Get("/api/changelog", h.Recent)

		r.With(mw.RequirePermission(d.RBAC, "changelog.view")).
			Get("/api/users/{id}/story", h.UserStory)
		r.With(mw.RequirePermission(d.RBAC, "changelog.view")).
			Get("/api/roles/{id}/story", h.RoleStory)
		r.With(mw.RequirePermission(d.RBAC, "changelog.view")).
			Get("/api/permissions/{id}/story", h.PermissionStory)

		r.With(mw.RequirePermission(d.RBAC, "changelog.restore")).
			Post("/api/changelog/{id}/restore", h.Restore)
	})
}
