// Package http собирает chi-роутер приложения: middleware-стек,
// /health и регистрация всех /api/* + /ws/* маршрутов через
// доменные mount*-функции в router_*.go.
//
// NewRouter и Deps — единственные публичные имена пакета: main.go
// собирает зависимости в Deps и получает готовый http.Handler.
//
// Доменные маршруты разнесены по файлам router_<domain>.go (10 групп).
// Каждый mount-функция короткая, ничего не экспортирует и видит весь
// Deps — это сознательный компромисс: иначе пришлось бы дублировать
// для каждой группы свой Deps-сабсет, что неудобно при добавлении
// новых сервисов.
package http

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	httpSwagger "github.com/swaggo/http-swagger"

	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/config"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/auth"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/changelog"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/changerequest"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/debt"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/deploy"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/discipline"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/notify"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/photo"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/rbac"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/report"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/retake"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/retakerequest"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/statement"
	teacherrequest "github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/teacher_request"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/token"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/user"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/transport/http/handler"
	mw "github.com/MaximaRasskazov/Academic-debt-system/backend/internal/transport/http/middleware"
)

// Deps — зависимости, которые роутер получает извне.
// Сделано отдельной структурой, чтобы main.go не разрастался
// сигнатурой NewRouter(a, b, c, d, ...).
type Deps struct {
	Cfg             *config.Config
	Pool            *pgxpool.Pool
	Auth            *auth.Service
	Tokens          *token.Service
	RBAC            *rbac.Service
	Disciplines     *discipline.Service
	Debts           *debt.Service
	Retakes         *retake.Service
	Statements      *statement.Service
	ChangeRequests  *changerequest.Service
	RetakeRequests  *retakerequest.Service
	Reports         *report.Service
	Notify          *notify.Service
	NotifyHub       *notify.Hub
	TeacherRequests *teacherrequest.Service
	Sync            handler.Syncer
	Users           *user.Service
	Changelog       *changelog.Service
	Deploy          *deploy.Service
	Photos          *photo.Service
}

// NewRouter собирает chi-роутер: middleware → /health → /api/* → /ws/*.
//
// Группа с chimw.Timeout(30s) — для обычных HTTP. WebSocket и Swagger
// вынесены ВНЕ группы: Timeout отменил бы их контексты через 30 с,
// закрывая долгоживущие соединения и блокируя fallthrough на static-ассеты.
func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()

	r.Use(chimw.RequestID)
	r.Use(mw.LoggingContext) // кладёт request_id и user_id в slog-контекст
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)
	r.Use(mw.SecurityHeaders)
	r.Use(mw.CORS(d.Cfg.AllowedOrigins))
	r.Use(mw.RequestMetrics) // Prometheus-метрики по запросам

	r.Get("/health", healthHandler(d.Pool))
	r.Handle("/metrics", mw.MetricsHandler())

	r.Group(func(r chi.Router) {
		r.Use(chimw.Timeout(30 * time.Second))

		authH := handler.NewAuthHandler(d.Auth, d.Cfg)
		r.Route("/api/auth", func(r chi.Router) {
			r.With(mw.RateLimit(3.0/60, 3)).Post("/register", authH.Register)
			r.With(mw.RateLimit(5.0/60, 5)).Post("/login", authH.Login)
			r.Post("/refresh", authH.Refresh)

			r.Group(func(r chi.Router) {
				r.Use(mw.Auth(d.Tokens))
				r.Post("/logout", authH.Logout)
				r.Get("/me", authH.Me)
			})
		})

		// /api/me — обновление профиля и смена пароля текущего пользователя.
		// Bearer-токен в header — самодостаточная защита от CSRF, поэтому
		// CSRF-токены не используем (refresh-cookie HttpOnly, JS его не
		// видит).
		// Аватар-маршруты обслуживает PhotoHandler. /api/me/avatar
		// сохранён как алиас старого фронтового пути загрузки/удаления;
		// полноценная группа /api/photo монтируется в mountPhoto.
		photoH := handler.NewPhotoHandler(d.Photos)
		r.Route("/api/me", func(r chi.Router) {
			r.Use(mw.Auth(d.Tokens))
			r.Patch("/", authH.UpdateMe)
			r.Post("/password", authH.ChangePassword)
			r.Post("/avatar", photoH.Upload)
			r.Delete("/avatar", photoH.Delete)
		})

		// Отдача аватара — публично (без auth), чтобы тег <img> мог
		// загрузить картинку: он не умеет слать Bearer-заголовок.
		r.Get("/api/users/{id}/avatar", photoH.ServeAvatar)

		mountDisciplines(r, d)
		mountMeDisciplines(r, d)
		mountDebts(r, d)
		mountRetakes(r, d)
		mountChangeRequests(r, d)
		mountRetakeRequests(r, d)
		mountReports(r, d)
		mountNotificationsREST(r, d)
		mountTeacherRequests(r, d)
		mountRBAC(r, d)
		mountSync(r, d)
		mountUsers(r, d)
		mountDirectory(r, d)
		mountChangelog(r, d)
		mountPhoto(r, d)
	})

	// Webhook авто-деплоя — вне таймаут-группы (git pull может идти дольше
	// 30 c), собственный потолок задаёт deploy.Service.
	mountDeploy(r, d)

	// Swagger UI — без таймаута, статика подаётся напрямую.
	r.Get("/swagger/*", httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"),
	))

	// WebSocket — без таймаута, соединение живёт пока клиент не отключится.
	mountNotificationsWS(r, d)

	return r
}

// healthHandler — копия логики из cmd/server/main.go, вынесенная в
// один пакет с роутером. Возвращает 200 если pool.Ping проходит,
// иначе 503. Используется healthcheck'ом docker-compose.
func healthHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		w.Header().Set("Content-Type", "application/json")
		if err := pool.Ping(ctx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "db_unreachable"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}
