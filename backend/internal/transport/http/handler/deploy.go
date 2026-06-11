package handler

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/deploy"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/transport/http/dto"
)

// maxSecretBody — потолок на тело запроса webhook'а. secret_key крошечный,
// больше 64 КБ читать незачем (защита от раздувания тела).
const maxSecretBody = 1 << 16

// GitWebhookHandler принимает webhook авто-деплоя (лаба №6).
//
// Маршрут открыт (без auth) — единственная защита это секретный ключ
// secret_key, который сравнивается с GIT_WEBHOOK_SECRET в постоянном
// времени (crypto/subtle), чтобы не подсказывать ответ по таймингу.
type GitWebhookHandler struct {
	secret string
	svc    *deploy.Service
}

// NewGitWebhookHandler создаёт обработчик. secret — значение из
// GIT_WEBHOOK_SECRET; пустое держит эндпоинт выключенным (503).
func NewGitWebhookHandler(secret string, svc *deploy.Service) *GitWebhookHandler {
	return &GitWebhookHandler{secret: secret, svc: svc}
}

// Handle godoc
//
//	@Summary		Webhook авто-деплоя по git
//	@Description	Проверяет secret_key и выполняет git checkout/reset/pull под блокировкой. Открыт без авторизации.
//	@Tags			deploy
//	@Accept			json
//	@Produce		json
//	@Param			secret_key	body		string	true	"Секретный ключ (тело JSON {\"secret_key\":...} или form-поле secret_key)"
//	@Success		200			{object}	dto.DeployResponse
//	@Failure		403			{object}	dto.ErrorResponse	"Invalid secret key"
//	@Failure		409			{object}	dto.ErrorResponse	"Deployment already in progress"
//	@Failure		500			{object}	dto.ErrorResponse
//	@Failure		503			{object}	dto.ErrorResponse	"Webhook не настроен"
//	@Router			/api/hooks/git [post]
func (h *GitWebhookHandler) Handle(w http.ResponseWriter, r *http.Request) {
	// Ключ не задан в окружении — фича выключена. Важно ответить ДО
	// сравнения: иначе пустой secret совпал бы с пустым secret_key, и
	// деплой стал бы доступен кому угодно.
	if h.secret == "" {
		writeError(w, http.StatusServiceUnavailable, "webhook_disabled", "git webhook is not configured")
		return
	}

	got := extractSecret(r)
	if subtle.ConstantTimeCompare([]byte(got), []byte(h.secret)) != 1 {
		writeError(w, http.StatusForbidden, "invalid_secret", "Invalid secret key")
		return
	}

	res, err := h.svc.Deploy(r.Context(), clientIP(r))
	switch {
	case errors.Is(err, deploy.ErrLocked):
		writeError(w, http.StatusConflict, "deploy_in_progress", "Deployment already in progress")
	case err != nil:
		// err содержит вывод git (без секрета) — полезно для диагностики.
		writeError(w, http.StatusInternalServerError, "deploy_failed", err.Error())
	default:
		writeJSON(w, http.StatusOK, dto.FromDeployResult(res))
	}
}

// extractSecret достаёт secret_key и из JSON-тела, и из form-data — как
// $request->input('secret_key') в исходном ТЗ.
func extractSecret(r *http.Request) string {
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		var body struct {
			SecretKey string `json:"secret_key"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, maxSecretBody)).Decode(&body); err != nil {
			return ""
		}
		return body.SecretKey
	}
	return r.FormValue("secret_key")
}
