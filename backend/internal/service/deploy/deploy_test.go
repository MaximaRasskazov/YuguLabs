package deploy

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeRunner записывает выполненные команды и умеет падать на заданной.
// statusOutput — что вернёт `git status` (пусто = чистое рабочее дерево).
type fakeRunner struct {
	calls        []string
	failOn       string
	failErr      error
	statusOutput string
}

func (f *fakeRunner) Run(_ context.Context, _ string, c Command) (string, error) {
	f.calls = append(f.calls, c.String())
	if f.failOn != "" && c.String() == f.failOn {
		return "boom output", f.failErr
	}
	if strings.HasPrefix(c.String(), "git status") {
		return f.statusOutput, nil
	}
	return "ok", nil
}

// sliceRecorder собирает записи журнала в память.
type sliceRecorder struct{ entries []Entry }

func (r *sliceRecorder) Record(e Entry) { r.entries = append(r.entries, e) }

// gitDir создаёт временный каталог с .git — чтобы прошла проверка репозитория.
func gitDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestDeploy_SuccessRunsCommandsInOrder(t *testing.T) {
	runner := &fakeRunner{}
	rec := &sliceRecorder{}
	svc := New(Config{RepoPath: gitDir(t), Branch: "main"}, runner, NewMemoryLock(), rec)

	res, err := svc.Deploy(context.Background(), "10.0.0.1")
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}

	// Чистое дерево: preflight `git status` (пусто) → без clean → 3 основные.
	want := []string{
		"git status --porcelain --untracked-files=all",
		"git checkout main",
		"git reset --hard HEAD",
		"git pull origin main",
	}
	if len(runner.calls) != len(want) {
		t.Fatalf("вызвано %d команд (%v), ожидалось %d", len(runner.calls), runner.calls, len(want))
	}
	for i, w := range want {
		if runner.calls[i] != w {
			t.Fatalf("команда[%d] = %q, ожидалось %q", i, runner.calls[i], w)
		}
	}
	if len(res.Steps) != 3 || res.Branch != "main" {
		t.Fatalf("res = %+v", res)
	}

	// Журнал: первая запись — старт, последняя — успешный финиш.
	if rec.entries[0].Stage != StageStarted {
		t.Fatalf("первая запись журнала = %+v", rec.entries[0])
	}
	last := rec.entries[len(rec.entries)-1]
	if last.Stage != StageFinished || last.Status != StatusSuccess {
		t.Fatalf("последняя запись журнала = %+v", last)
	}
}

func TestDeploy_BranchTakenFromConfig(t *testing.T) {
	runner := &fakeRunner{}
	svc := New(Config{RepoPath: gitDir(t), Branch: "develop"}, runner, NewMemoryLock(), &sliceRecorder{})

	if _, err := svc.Deploy(context.Background(), ""); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	// calls[0] — preflight status; основные команды идут после него.
	if runner.calls[1] != "git checkout develop" || runner.calls[3] != "git pull origin develop" {
		t.Fatalf("команды = %v", runner.calls)
	}
}

// TestDeploy_DirtyWorktreeCleansAndWarns: при «грязном» дереве перед основными
// командами выполняется git clean и возвращается warning.
func TestDeploy_DirtyWorktreeCleansAndWarns(t *testing.T) {
	runner := &fakeRunner{statusOutput: " M app/main.go\n?? newfile.txt"}
	svc := New(Config{RepoPath: gitDir(t), Branch: "main"}, runner, NewMemoryLock(), &sliceRecorder{})

	res, err := svc.Deploy(context.Background(), "10.0.0.1")
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if len(res.Warnings) != 1 {
		t.Fatalf("ожидался 1 warning, получено %v", res.Warnings)
	}
	want := []string{
		"git status --porcelain --untracked-files=all",
		"git clean -fd",
		"git checkout main",
		"git reset --hard HEAD",
		"git pull origin main",
	}
	if len(runner.calls) != len(want) {
		t.Fatalf("calls = %v", runner.calls)
	}
	for i, w := range want {
		if runner.calls[i] != w {
			t.Fatalf("calls[%d] = %q, ожидалось %q", i, runner.calls[i], w)
		}
	}
}

func TestDeploy_NotGitRepoFailsBeforeCommands(t *testing.T) {
	runner := &fakeRunner{}
	svc := New(Config{RepoPath: t.TempDir(), Branch: "main"}, runner, NewMemoryLock(), &sliceRecorder{})

	_, err := svc.Deploy(context.Background(), "")
	if !errors.Is(err, ErrNotGitRepo) {
		t.Fatalf("err = %v, ожидалось ErrNotGitRepo", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("git-команды не должны выполняться, получено %v", runner.calls)
	}
}

func TestDeploy_CommandFailureReleasesLock(t *testing.T) {
	lock := NewMemoryLock()
	runner := &fakeRunner{failOn: "git pull origin main", failErr: errors.New("merge conflict")}
	svc := New(Config{RepoPath: gitDir(t), Branch: "main"}, runner, lock, &sliceRecorder{})

	if _, err := svc.Deploy(context.Background(), ""); err == nil {
		t.Fatal("ожидалась ошибка на падающем git pull")
	}
	// Блокировка должна быть снята даже при ошибке — повторный захват проходит.
	if !lock.TryLock(time.Minute) {
		t.Fatal("блокировка не снята после ошибки деплоя")
	}
}

func TestDeploy_AlreadyLockedReturns409Marker(t *testing.T) {
	lock := NewMemoryLock()
	lock.TryLock(time.Minute) // деплой «уже идёт»

	runner := &fakeRunner{}
	svc := New(Config{RepoPath: gitDir(t), Branch: "main"}, runner, lock, &sliceRecorder{})

	_, err := svc.Deploy(context.Background(), "")
	if !errors.Is(err, ErrLocked) {
		t.Fatalf("err = %v, ожидалось ErrLocked", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("при занятой блокировке команды не должны выполняться, получено %v", runner.calls)
	}
}
