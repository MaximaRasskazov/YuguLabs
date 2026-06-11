package deploy

import (
	"testing"
	"time"
)

func TestMemoryLock_SecondTryFailsWhileHeld(t *testing.T) {
	l := NewMemoryLock()

	if !l.TryLock(time.Minute) {
		t.Fatal("первый TryLock должен пройти")
	}
	if l.TryLock(time.Minute) {
		t.Fatal("второй TryLock должен вернуть false, пока блокировка занята")
	}

	l.Unlock()
	if !l.TryLock(time.Minute) {
		t.Fatal("после Unlock TryLock должен снова пройти")
	}
}

func TestMemoryLock_ExpiredLockIsStolen(t *testing.T) {
	l := NewMemoryLock()

	if !l.TryLock(10 * time.Millisecond) {
		t.Fatal("первый TryLock должен пройти")
	}
	time.Sleep(20 * time.Millisecond)

	if !l.TryLock(time.Minute) {
		t.Fatal("протухшую блокировку должно быть можно перехватить")
	}
}
