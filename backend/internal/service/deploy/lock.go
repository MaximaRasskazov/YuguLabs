package deploy

import (
	"sync"
	"time"
)

// Locker защищает деплой от параллельного запуска. TryLock неблокирующий:
// если занято — возвращает false, и handler отвечает 409 Conflict.
type Locker interface {
	TryLock(ttl time.Duration) bool
	Unlock()
}

// MemoryLock — потокобезопасная блокировка в пределах процесса. Бэкенд
// работает одним процессом, поэтому in-process мьютекса достаточно для
// требования ТЗ «второй конкурентный запрос получает 409».
//
// ttl страхует от зависшего деплоя: если процесс упал, не сняв блокировку,
// по истечении ttl она считается протухшей и перехватывается следующим
// запросом — деплой не блокируется навсегда.
//
// Для кластера из нескольких инстансов интерфейс Locker позволяет
// заменить реализацию на распределённую (advisory-lock в Postgres или
// flock на общий том) без изменений в Service — принцип Open/Closed.
type MemoryLock struct {
	mu    sync.Mutex
	held  bool
	until time.Time
}

// NewMemoryLock создаёт свободную блокировку.
func NewMemoryLock() *MemoryLock { return &MemoryLock{} }

// TryLock берёт блокировку на ttl. Возвращает false, если она уже занята
// и срок ещё не истёк.
func (l *MemoryLock) TryLock(ttl time.Duration) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	if l.held && now.Before(l.until) {
		return false
	}
	l.held = true
	l.until = now.Add(ttl)
	return true
}

// Unlock освобождает блокировку. Безопасно вызывать повторно.
func (l *MemoryLock) Unlock() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.held = false
	l.until = time.Time{}
}
