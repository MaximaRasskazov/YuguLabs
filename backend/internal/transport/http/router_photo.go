package http

import (
	"github.com/go-chi/chi/v5"

	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/transport/http/handler"
	mw "github.com/MaximaRasskazov/Academic-debt-system/backend/internal/transport/http/middleware"
)

// mountPhoto регистрирует группу /api/photo (фотография профиля).
//
//	POST   /api/photo           — загрузить фото (multipart «photo»)
//	GET    /api/photo           — метаданные текущего фото
//	DELETE /api/photo           — удалить своё фото
//	GET    /api/photo/download  — скачать оригинал (только владелец)
//	POST   /api/photo/archive   — архив всех фото (право photos.export)
//
// Публичная отдача миниатюры GET /api/users/{id}/avatar монтируется в
// router.go (она без авторизации — для тега <img>).
func mountPhoto(r chi.Router, d Deps) {
	if d.Photos == nil {
		return
	}
	h := handler.NewPhotoHandler(d.Photos)

	r.Group(func(r chi.Router) {
		r.Use(mw.Auth(d.Tokens))

		r.Post("/api/photo", h.Upload)
		r.Get("/api/photo", h.Info)
		r.Delete("/api/photo", h.Delete)
		r.Get("/api/photo/download", h.Download)

		// Админские: просмотр всех фото (список) и выгрузка архива.
		r.With(mw.RequirePermission(d.RBAC, "photos.export")).
			Get("/api/photo/all", h.ListAll)
		r.With(mw.RequirePermission(d.RBAC, "photos.export")).
			Post("/api/photo/archive", h.Archive)
	})
}
