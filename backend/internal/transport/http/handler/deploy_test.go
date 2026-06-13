package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/deploy"
)

const webhookSecret = "test-secret-key-do-not-use-in-prod"

// stubRunner имитирует git без настоящего git: успех на любую команду.
// На `git status` отдаёт пусто — чистое рабочее дерево (preflight без clean).
type stubRunner struct{}

func (stubRunner) Run(_ context.Context, _ string, c deploy.Command) (string, error) {
	if strings.HasPrefix(c.String(), "git status") {
		return "", nil
	}
	return "ok", nil
}

// newDeploySvc собирает реальный deploy.Service с подменённым git и
// временным каталогом, где есть .git (проверка репозитория проходит).
func newDeploySvc(t *testing.T, lock deploy.Locker) *deploy.Service {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if lock == nil {
		lock = deploy.NewMemoryLock()
	}
	return deploy.New(
		deploy.Config{RepoPath: dir, Branch: "main", Timeout: time.Minute},
		stubRunner{},
		lock,
		deploy.NewFileRecorder(filepath.Join(dir, "deploy.log")),
	)
}

func postJSON(secretKey string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/hooks/git",
		strings.NewReader(`{"secret_key":"`+secretKey+`"}`))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func TestGitWebhook_WrongSecretReturns403(t *testing.T) {
	h := NewGitWebhookHandler(webhookSecret, newDeploySvc(t, nil))
	rr := httptest.NewRecorder()

	h.Handle(rr, postJSON("wrong-key"))

	if rr.Code != http.StatusForbidden {
		t.Fatalf("статус = %d, ожидалось 403", rr.Code)
	}
}

func TestGitWebhook_DifferentCaseReturns403(t *testing.T) {
	h := NewGitWebhookHandler(webhookSecret, newDeploySvc(t, nil))
	rr := httptest.NewRecorder()

	// Тот же ключ в другом регистре — сравнение регистрозависимое.
	h.Handle(rr, postJSON(strings.ToUpper(webhookSecret)))

	if rr.Code != http.StatusForbidden {
		t.Fatalf("статус = %d, ожидалось 403 (регистр важен)", rr.Code)
	}
}

func TestGitWebhook_NoSecretConfiguredReturns503(t *testing.T) {
	h := NewGitWebhookHandler("", newDeploySvc(t, nil))
	rr := httptest.NewRecorder()

	h.Handle(rr, postJSON(""))

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("статус = %d, ожидалось 503", rr.Code)
	}
}

func TestGitWebhook_ValidSecretReturns200JSON(t *testing.T) {
	h := NewGitWebhookHandler(webhookSecret, newDeploySvc(t, nil))
	rr := httptest.NewRecorder()

	h.Handle(rr, postJSON(webhookSecret))

	if rr.Code != http.StatusOK {
		t.Fatalf("статус = %d, ожидалось 200 (тело: %s)", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type = %q, ожидался application/json", ct)
	}
	var resp struct {
		Status string `json:"status"`
		Branch string `json:"branch"`
		Steps  []struct {
			Command string `json:"command"`
		} `json:"steps"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("ответ не JSON: %v", err)
	}
	if resp.Status != "success" || resp.Branch != "main" || len(resp.Steps) != 3 {
		t.Fatalf("ответ = %+v", resp)
	}
}

func TestGitWebhook_FormEncodedSecretReturns200(t *testing.T) {
	h := NewGitWebhookHandler(webhookSecret, newDeploySvc(t, nil))
	req := httptest.NewRequest(http.MethodPost, "/api/hooks/git",
		strings.NewReader("secret_key="+webhookSecret))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	h.Handle(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("статус = %d, ожидалось 200 (form-data)", rr.Code)
	}
}

func TestGitWebhook_ConcurrentReturns409(t *testing.T) {
	lock := deploy.NewMemoryLock()
	lock.TryLock(time.Minute) // деплой «уже идёт»

	h := NewGitWebhookHandler(webhookSecret, newDeploySvc(t, lock))
	rr := httptest.NewRecorder()

	h.Handle(rr, postJSON(webhookSecret))

	if rr.Code != http.StatusConflict {
		t.Fatalf("статус = %d, ожидалось 409", rr.Code)
	}
}
