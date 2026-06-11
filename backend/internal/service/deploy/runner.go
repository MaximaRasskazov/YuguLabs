package deploy

import (
	"context"
	"os/exec"
	"strings"
)

// Command — одна внешняя команда: имя и аргументы.
type Command struct {
	Name string
	Args []string
}

// String — человекочитаемое представление для журнала и ответа клиенту,
// например "git pull origin main". Секретов в командах деплоя нет.
func (c Command) String() string {
	return strings.TrimSpace(c.Name + " " + strings.Join(c.Args, " "))
}

// DeployCommands возвращает фиксированную последовательность из ТЗ:
//
//  1. git checkout <branch>     — переключиться на ветку деплоя
//  2. git reset --hard HEAD     — снять локальные изменения
//  3. git pull origin <branch>  — подтянуть последнюю версию
//
// Ветка приходит из конфига (GIT_DEFAULT_BRANCH), не захардкожена.
func DeployCommands(branch string) []Command {
	return []Command{
		{Name: "git", Args: []string{"checkout", branch}},
		{Name: "git", Args: []string{"reset", "--hard", "HEAD"}},
		{Name: "git", Args: []string{"pull", "origin", branch}},
	}
}

// Runner запускает команду в каталоге dir и возвращает её вывод. Интерфейс
// (а не прямой вызов exec) позволяет подменить git моком в тестах.
type Runner interface {
	Run(ctx context.Context, dir string, c Command) (output string, err error)
}

// GitRunner — боевая реализация поверх os/exec.
type GitRunner struct{}

// NewGitRunner возвращает запускатель реальных git-команд.
func NewGitRunner() GitRunner { return GitRunner{} }

// Run выполняет команду с учётом отмены по ctx (таймаут деплоя или
// graceful shutdown). CombinedOutput включает stderr — это и есть текст
// ошибки git при ненулевом коде возврата.
func (GitRunner) Run(ctx context.Context, dir string, c Command) (string, error) {
	cmd := exec.CommandContext(ctx, c.Name, c.Args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}
