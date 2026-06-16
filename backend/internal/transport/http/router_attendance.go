package http

import (
	"github.com/go-chi/chi/v5"

	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/transport/http/handler"
	mw "github.com/MaximaRasskazov/Academic-debt-system/backend/internal/transport/http/middleware"
)

// mountAttendance регистрирует POST /api/attendance/calculate (лаба №12).
// Защищён авторизацией + permission calculate-attendance — он есть только у
// роли admin (см. миграцию 00030). Auth раньше RequirePermission: без токена
// → 401, с токеном без права → 403.
func mountAttendance(r chi.Router, d Deps) {
	h := handler.NewAttendanceHandler(d.Attendance, d.Cfg.UploadMaxSizeMB)
	r.Route("/api/attendance", func(r chi.Router) {
		r.Use(mw.Auth(d.Tokens))
		r.Use(mw.RequirePermission(d.RBAC, "calculate-attendance"))
		r.Post("/calculate", h.Calculate)
	})
}
