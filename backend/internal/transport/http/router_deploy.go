package http

import (
	"github.com/go-chi/chi/v5"

	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/transport/http/handler"
)

// mountDeploy регистрирует webhook авто-деплоя (лаба №6).
//
//	POST /api/hooks/git — основной путь (через него ходит обратный прокси,
//	                      проксирующий на бэкенд только /api/*).
//	POST /hooks/git     — литеральный путь из ТЗ (для прямого обращения к
//	                      бэкенду, как в методичке). Тот же обработчик.
//
// Оба сгруппированы под префиксом hooks (требование ТЗ о группировке) и
// намеренно вынесены ВНЕ 30-секундного таймаута — как ws/swagger: git pull
// может идти дольше. Собственный потолок времени задаёт сам deploy.Service
// (GIT_DEPLOY_TIMEOUT).
func mountDeploy(r chi.Router, d Deps) {
	if d.Deploy == nil {
		return
	}
	h := handler.NewGitWebhookHandler(d.Cfg.GitWebhookSecret, d.Deploy)
	for _, prefix := range []string{"/api/hooks", "/hooks"} {
		r.Route(prefix, func(r chi.Router) {
			r.Post("/git", h.Handle)
		})
	}
}
