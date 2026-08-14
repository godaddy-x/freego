package cachelocal

import (
	"context"
	"sync/atomic"
	"time"
)

type Lock struct {
	resource        string
	token           string
	locker          interface{}
	cancelRefresh   context.CancelFunc
	localEntry      *localLockEntry
	exp             int
	acquireTime     time.Time
	lastRefresh     atomic.Value
	isValid         int32
	isLocal         int32
	refreshCount    int32
	refreshFailures int32
}

func (lock *Lock) Unlock() error {
	if lock == nil {
		return nil
	}
	if atomic.CompareAndSwapInt32(&lock.isValid, 1, 0) && lock.localEntry != nil {
		localReleaseLease(lock.localEntry, lock.token)
		localReleaseEntryRef(lock.localEntry)
	}
	return nil
}
