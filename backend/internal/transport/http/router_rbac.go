package http

import (
	"github.com/go-chi/chi/v5"

	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/transport/http/handler"
	mw "github.com/MaximaRasskazov/Academic-debt-system/backend/internal/transport/http/middleware"
)

// mountUsers регистрирует GET /api/users — список пользователей с
// фильтрами по роли, поиском и пагинацией. Доступен dean и admin
// (permission users.view). Нужен фронту для:
//   - выбора преподавателя при создании пересдачи (multi-select)
//   - выбора студента при привязке к дисциплине
//   - админ-панели управления ролями
func mountUsers(r chi.Router, d Deps) {
	if d.Users == nil {
		return
	}
	h := handler.NewUsersHandler(d.Users)
	r.Route("/api/users", func(r chi.Router) {
		r.Use(mw.Auth(d.Tokens))
		r.Use(mw.RequirePermission(d.RBAC, "users.view"))
		r.Get("/", h.List)
	})
}

// mountRBAC регистрирует /api/roles, /api/permissions и
// /api/users/:id/roles|permissions — HTTP-API управления RBAC.
func mountRBAC(r chi.Router, d Deps) {
	h := handler.NewRBACHandler(d.RBAC)

	r.Route("/api/roles", func(r chi.Router) {
		r.Use(mw.Auth(d.Tokens))
		r.Use(mw.RequirePermission(d.RBAC, "roles.assign"))
		r.Get("/", h.ListRoles)
	})

	r.Route("/api/permissions", func(r chi.Router) {
		r.Use(mw.Auth(d.Tokens))
		r.Use(mw.RequirePermission(d.RBAC, "roles.assign"))
		r.Get("/", h.ListPermissions)
	})

	r.Route("/api/users/{id}", func(r chi.Router) {
		r.Use(mw.Auth(d.Tokens))
		r.Use(mw.RequirePermission(d.RBAC, "users.view"))
		r.Get("/roles", h.ListUserRoles)
		r.Get("/permissions", h.ListUserPermissions)

		// Админская правка профиля пользователя (ФИО/группа) — логируется.
		// Регистрируем в этом же {id}-поддереве, чтобы не плодить второй
		// chi-узел для /api/users/{id}.
		if d.Users != nil {
			uh := handler.NewUsersHandler(d.Users)
			r.With(mw.RequirePermission(d.RBAC, "users.update")).
				Patch("/", uh.Update)
		}

		r.With(mw.RequirePermission(d.RBAC, "roles.assign")).
			Post("/roles", h.AssignRole)
		r.With(mw.RequirePermission(d.RBAC, "roles.assign")).
			Post("/roles/change", h.ChangeRole)
		r.With(mw.RequirePermission(d.RBAC, "roles.assign")).
			Delete("/roles/{slug}", h.RevokeRole)
	})
}
