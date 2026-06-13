package deploy

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Стадии и статусы записи журнала деплоя.
const (
	StageStarted   = "started"   // запрос принят, начинаем деплой
	StageLocked    = "locked"    // деплой пропущен — уже идёт другой
	StagePreflight = "preflight" // проверка рабочего дерева до основных команд
	StageCommand   = "command"   // выполнена git-команда
	StageFinished  = "finished"  // деплой завершён (успехом или ошибкой)

	StatusStarted = "started"
	StatusSuccess = "success"
	StatusError   = "error"
	StatusSkipped = "skipped"
	StatusWarning = "warning"
)

// Entry — одна строка журнала деплоя. Секрет сюда не попадает by design:
// сервис его не получает.
type Entry struct {
	At      time.Time `json:"time"`
	IP      string    `json:"ip,omitempty"`
	Stage   string    `json:"stage"`
	Status  string    `json:"status"`
	Command string    `json:"command,omitempty"`
	Detail  string    `json:"detail,omitempty"`
}

// Recorder фиксирует ход деплоя. Интерфейс позволяет в тестах собирать
// записи в память, а в бою — писать в файл.
type Recorder interface {
	Record(e Entry)
}

// FileRecorder дописывает события в файл по строке на запись (формат JSON
// Lines) и дублирует их в slog. Если файл недоступен (нет прав, нельзя
// создать каталог) — деплой не падает, остаётся только slog.
type FileRecorder struct {
	mu   sync.Mutex
	path string
}

// NewFileRecorder создаёт рекордер, пишущий в указанный файл.
func NewFileRecorder(path string) *FileRecorder {
	return &FileRecorder{path: path}
}

// Record проставляет время (если не задано), пишет в slog и дописывает
// строку в файл-журнал.
func (r *FileRecorder) Record(e Entry) {
	if e.At.IsZero() {
		e.At = time.Now()
	}

	slog.Info("deploy",
		"stage", e.Stage, "status", e.Status,
		"command", e.Command, "ip", e.IP, "detail", e.Detail)

	line, err := json.Marshal(e)
	if err != nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		slog.Warn("deploy: cannot create log dir", "err", err)
		return
	}
	f, err := os.OpenFile(r.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		slog.Warn("deploy: cannot open log file", "err", err)
		return
	}
	defer f.Close()
	_, _ = f.Write(append(line, '\n'))
}
