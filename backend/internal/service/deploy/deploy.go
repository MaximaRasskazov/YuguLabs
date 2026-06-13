// Package deploy реализует авто-деплой приложения по git-webhook
// (лабораторная работа №6).
//
// Исходное ТЗ написано под Laravel/PHP; здесь — эквивалентная адаптация
// на наш Go-стек с теми же гарантиями:
//
//	GitWebhookController            → handler.GitWebhookHandler
//	DeployService                  → deploy.Service (оркестратор)
//	exec()/Symfony Process         → deploy.Runner  (GitRunner)
//	Cache-lock "deploy_lock" (TTL) → deploy.Locker  (MemoryLock)
//	storage/logs/deployment.log    → deploy.Recorder (FileRecorder)
//
// Service ничего не знает про HTTP и про секретный ключ: проверку ключа
// делает handler, в сервис приходит только IP инициатора. Так секрет
// физически не может попасть в журнал деплоя.
//
// Зависимости сервиса — интерфейсы (Runner, Locker, Recorder), что даёт
// тестируемость без настоящего git и файловой системы (DIP) и позволяет
// заменить реализацию блокировки/запуска команд без правок оркестратора
// (Open/Closed).
package deploy

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var (
	// ErrLocked — другой деплой уже выполняется (handler отдаёт 409 Conflict).
	ErrLocked = errors.New("deploy: already in progress")
	// ErrNotGitRepo — RepoPath не является git-репозиторием (handler отдаёт 500).
	ErrNotGitRepo = errors.New("deploy: not a git repository")
)

// Config — параметры деплоя, собираются из ENV в main.
type Config struct {
	RepoPath string        // каталог, где выполняются git-команды (должен содержать .git)
	Branch   string        // ветка деплоя: git checkout <branch>; git pull origin <branch>
	Timeout  time.Duration // потолок времени на весь набор git-команд
	LockTTL  time.Duration // на сколько берётся блокировка (страховка от зависшего процесса)
}

// Service оркеструет деплой строго по ТЗ:
// лог-старт → блокировка → проверка репозитория → git-команды по порядку →
// снятие блокировки → лог-финиш.
type Service struct {
	cfg    Config
	runner Runner
	lock   Locker
	rec    Recorder
}

// New собирает сервис и подставляет безопасные значения по умолчанию.
func New(cfg Config, runner Runner, lock Locker, rec Recorder) *Service {
	if cfg.Branch == "" {
		cfg.Branch = "main"
	}
	if cfg.RepoPath == "" {
		cfg.RepoPath = "."
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Minute
	}
	if cfg.LockTTL <= 0 {
		cfg.LockTTL = cfg.Timeout
	}
	return &Service{cfg: cfg, runner: runner, lock: lock, rec: rec}
}

// Step — результат одной выполненной git-команды (для JSON-ответа клиенту).
type Step struct {
	Command string
	Output  string
}

// Result — итог успешного деплоя.
type Result struct {
	Branch string
	// Warnings — нефатальные предупреждения (например, обнаруженное «грязное»
	// рабочее дерево, чьи правки были отброшены). Уходят в JSON-ответ.
	Warnings []string
	Steps    []Step
}

// Deploy выполняет последовательность git-команд под блокировкой.
// clientIP пишется в журнал (кто инициировал). Секрет сюда не передаётся.
//
// Синхронный: возвращает управление только после завершения всех команд —
// клиент получает финальный статус, а не «принято».
func (s *Service) Deploy(ctx context.Context, clientIP string) (Result, error) {
	s.rec.Record(Entry{IP: clientIP, Stage: StageStarted, Status: StatusStarted})

	// Блокировка ДО основных операций: второй параллельный запрос получит
	// false и уйдёт с 409, не запуская второй git pull.
	if !s.lock.TryLock(s.cfg.LockTTL) {
		s.rec.Record(Entry{IP: clientIP, Stage: StageLocked, Status: StatusSkipped,
			Detail: "another deployment is in progress"})
		return Result{}, ErrLocked
	}
	defer s.lock.Unlock()

	if err := ensureGitRepo(s.cfg.RepoPath); err != nil {
		s.rec.Record(Entry{IP: clientIP, Stage: StageFinished, Status: StatusError, Detail: err.Error()})
		return Result{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, s.cfg.Timeout)
	defer cancel()

	res := Result{Branch: s.cfg.Branch}
	// Preflight: фиксируем «грязное» рабочее дерево ДО разрушительных команд.
	res.Warnings = s.prepareWorktree(ctx, clientIP)
	for _, c := range DeployCommands(s.cfg.Branch) {
		out, err := s.runner.Run(ctx, s.cfg.RepoPath, c)
		out = strings.TrimSpace(out)
		if err != nil {
			s.rec.Record(Entry{IP: clientIP, Stage: StageCommand, Command: c.String(),
				Status: StatusError, Detail: joinErr(out, err)})
			s.rec.Record(Entry{IP: clientIP, Stage: StageFinished, Status: StatusError,
				Detail: c.String() + " failed"})
			// Блокировка снимется через defer — даже на ошибке деплой не
			// останется «навсегда занятым».
			return Result{}, fmt.Errorf("%s: %w", c.String(), err)
		}
		s.rec.Record(Entry{IP: clientIP, Stage: StageCommand, Command: c.String(),
			Status: StatusSuccess, Detail: out})
		res.Steps = append(res.Steps, Step{Command: c.String(), Output: out})
	}

	s.rec.Record(Entry{IP: clientIP, Stage: StageFinished, Status: StatusSuccess})
	return res, nil
}

// prepareWorktree проверяет рабочее дерево перед основными командами и
// готовит его к чистому pull. Если есть незакоммиченные/неотслеживаемые
// изменения — фиксирует warning (в журнал и в ответ): мы НЕ stash-им и не
// коммитим их (на сервере локальные правки случайны или подозрительны), а
// `git clean -fd` убирает неотслеживаемые файлы. Отслеживаемые правки
// отбросит обязательный `git reset --hard HEAD`. `git clean` без `-x` НЕ
// трогает gitignored-файлы (например, .env).
//
// Это прямой ответ на типичный вопрос на защите: «а если на сервере есть
// незакоммиченные изменения?» — мы их обнаруживаем, логируем и зачищаем.
func (s *Service) prepareWorktree(ctx context.Context, clientIP string) []string {
	statusCmd := Command{Name: "git", Args: []string{"status", "--porcelain", "--untracked-files=all"}}
	out, err := s.runner.Run(ctx, s.cfg.RepoPath, statusCmd)
	out = strings.TrimSpace(out)
	if err != nil || out == "" {
		return nil // дерево чистое (или статус недоступен) — готовить нечего
	}

	warning := "рабочее дерево содержит локальные изменения — они будут отброшены (reset --hard + clean)"
	s.rec.Record(Entry{IP: clientIP, Stage: StagePreflight, Status: StatusWarning, Detail: warning + "\n" + out})

	cleanCmd := Command{Name: "git", Args: []string{"clean", "-fd"}}
	cleanOut, cleanErr := s.runner.Run(ctx, s.cfg.RepoPath, cleanCmd)
	if cleanErr != nil {
		// Ошибка очистки не валит деплой: обязательный reset --hard всё равно
		// приведёт отслеживаемые файлы в порядок.
		s.rec.Record(Entry{IP: clientIP, Stage: StageCommand, Command: cleanCmd.String(),
			Status: StatusError, Detail: joinErr(strings.TrimSpace(cleanOut), cleanErr)})
	} else {
		s.rec.Record(Entry{IP: clientIP, Stage: StageCommand, Command: cleanCmd.String(),
			Status: StatusSuccess, Detail: strings.TrimSpace(cleanOut)})
	}
	return []string{warning}
}

// ensureGitRepo проверяет, что каталог — git-репозиторий. .git может быть
// и каталогом, и файлом (worktree/submodule) — Stat покрывает оба случая.
func ensureGitRepo(dir string) error {
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		return fmt.Errorf("%w: %s", ErrNotGitRepo, dir)
	}
	return nil
}

func joinErr(out string, err error) string {
	if out == "" {
		return err.Error()
	}
	return out + ": " + err.Error()
}
