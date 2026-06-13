package middleware

import (
	"encoding/json"
	"net/http"

	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/rbac"
)

// RequirePermission возвращает middleware, который пропускает запрос
// только если у текущего пользователя есть permission с указанным
// slug. Должен вызываться ПОСЛЕ Auth — иначе UserID(ctx) вернёт false
// и middleware ответит 401.
//
// 403 — есть пользователь, но нет permission. 401 — пользователя нет
// в контексте (запрос не прошёл Auth). 500 — ошибка БД при проверке.
func RequirePermission(svc *rbac.Service, permissionSlug string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID, ok := UserID(r.Context())
			if !ok {
				writeUnauthorized(w, "not authenticated")
				return
			}

			has, err := svc.HasPermission(r.Context(), userID, permissionSlug)
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error":"internal","message":"permission check failed"}`))
				return
			}
			if !has {
				// В сообщении называем конкретное требуемое право — чтобы
				// пользователь/разработчик сразу понимал, чего не хватает,
				// а не гадал над абстрактным «недостаточно прав».
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"error":               "forbidden",
					"message":             "Недостаточно прав: для этого действия требуется разрешение «" + permissionSlug + "», которого у вас нет.",
					"required_permission": permissionSlug,
				})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireAnyPermission — пропускает запрос, если у пользователя есть
// ХОТЯ БЫ ОДНО из указанных разрешений. Полезно для эндпойнтов,
// которыми пользуются разные роли с разными правами — например,
// /api/teachers нужен и преподавателю (retakes.request), и декану
// (retakes.create). Делать два роута с одинаковым handler'ом некрасиво,
// проверку OR — самый чистый путь.
//
// Семантика 401/403/500 та же, что у RequirePermission.
func RequireAnyPermission(svc *rbac.Service, slugs ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID, ok := UserID(r.Context())
			if !ok {
				writeUnauthorized(w, "not authenticated")
				return
			}

			for _, slug := range slugs {
				has, err := svc.HasPermission(r.Context(), userID, slug)
				if err != nil {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte(`{"error":"internal","message":"permission check failed"}`))
					return
				}
				if has {
					next.ServeHTTP(w, r)
					return
				}
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"forbidden","message":"insufficient permissions"}`))
		})
	}
}
