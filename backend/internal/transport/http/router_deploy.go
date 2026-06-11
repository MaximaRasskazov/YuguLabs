package http

import (
	"github.com/go-chi/chi/v5"

	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/transport/http/handler"
)

// mountDeploy регистрирует webhook авто-деплоя (лаба №6).
//
//	POST /api/hooks/git — открыт без авторизации, защищён secret_key.
//
// Маршрут сгруппирован под префиксом /api/hooks (требование ТЗ о группировке
// по hooks) и намеренно вынесен ВНЕ 30-секундного таймаута — как ws/swagger:
// git pull может идти дольше. Собственный потолок времени задаёт сам
// deploy.Service (GIT_DEPLOY_TIMEOUT).
func mountDeploy(r chi.Router, d Deps) {
	if d.Deploy == nil {
		return
	}
	h := handler.NewGitWebhookHandler(d.Cfg.GitWebhookSecret, d.Deploy)
	r.Route("/api/hooks", func(r chi.Router) {
		r.Post("/git", h.Handle)
	})
}
