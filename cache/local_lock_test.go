package cache

import (
	"testing"
	"time"
)

func TestLocalLockReacquireAfterUnlock(t *testing.T) {
	resource := "local-lock-reacquire"

	err := tryLocalLocker(resource, 5, func(lock *Lock) error {
		return nil
	})
	if err != nil {
		t.Fatalf("first local lock should succeed: %v", err)
	}

	err = tryLocalLocker(resource, 5, func(lock *Lock) error {
		return nil
	})
	if err != nil {
		t.Fatalf("second local lock should succeed immediately after unlock: %v", err)
	}
}

func TestLocalLockContendedAcquireFailsDuringHold(t *testing.T) {
	resource := "local-lock-contended"
	holdDone := make(chan struct{})
	firstEntered := make(chan struct{})

	go func() {
		_ = tryLocalLocker(resource, 2, func(lock *Lock) error {
			close(firstEntered)
			<-holdDone
			return nil
		})
	}()

	select {
	case <-firstEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("first locker did not enter in time")
	}

	start := time.Now()
	err := tryLocalLocker(resource, 1, func(lock *Lock) error {
		return nil
	})
	elapsed := time.Since(start)
	close(holdDone)

	if err == nil {
		t.Fatal("second local lock should fail while first lock is held")
	}
	if elapsed < 80*time.Millisecond {
		t.Fatalf("contended acquire should wait close to timeout, got: %v", elapsed)
	}
}
