package cache

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/godaddy-x/freego/ex"
	"github.com/godaddy-x/freego/zlog"
)

var (
	localLockEntryPool   sync.Map
	localLockEntryInitMu sync.Mutex
	localLockCleaner     sync.Once
	localLockCleanerStop context.CancelFunc
)

const (
	localLockIdleTTL       = 10 * time.Minute
	localLockCleanupPeriod = 2 * time.Minute
	localLockRetryInterval = 20 * time.Millisecond
	localLockWaitTimeout   = 100 * time.Millisecond
)

type localLockEntry struct {
	mu       sync.Mutex
	token    string
	expireAt int64
	lastUsed int64
}

func (e *localLockEntry) touch() {
	e.mu.Lock()
	e.lastUsed = time.Now().UnixNano()
	e.mu.Unlock()
}

func startLocalLockCleaner() {
	localLockCleaner.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())
		localLockCleanerStop = cancel
		go func() {
			ticker := time.NewTicker(localLockCleanupPeriod)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}
				now := time.Now().UnixNano()
				localLockEntryPool.Range(func(key, value interface{}) bool {
					entry, ok := value.(*localLockEntry)
					if !ok || entry == nil {
						localLockEntryPool.Delete(key)
						return true
					}
					entry.mu.Lock()
					idle := entry.token == ""
					lastUsed := entry.lastUsed
					entry.mu.Unlock()
					// 仅清理空闲且未持锁的资源，避免影响活跃锁
					if idle && now-lastUsed > int64(localLockIdleTTL) {
						localLockEntryPool.Delete(key)
					}
					return true
				})
			}
		}()
	})
}

func stopLocalLockCleaner() {
	if localLockCleanerStop != nil {
		localLockCleanerStop()
	}
}

func getLocalLockEntry(resource string) *localLockEntry {
	startLocalLockCleaner()
	if entry, ok := localLockEntryPool.Load(resource); ok {
		e := entry.(*localLockEntry)
		e.touch()
		return e
	}
	localLockEntryInitMu.Lock()
	defer localLockEntryInitMu.Unlock()
	// 双检，避免并发下重复创建entry对象。
	if entry, ok := localLockEntryPool.Load(resource); ok {
		e := entry.(*localLockEntry)
		e.touch()
		return e
	}
	e := &localLockEntry{token: "", expireAt: 0, lastUsed: time.Now().UnixNano()}
	localLockEntryPool.Store(resource, e)
	return e
}

func localTryAcquireLease(entry *localLockEntry, token string, wait time.Duration, leaseTTL time.Duration) bool {
	deadline := time.Now().Add(wait)
	if wait <= 0 {
		deadline = time.Now()
	}
	for {
		now := time.Now()
		nowNS := now.UnixNano()
		entry.mu.Lock()
		if entry.token == "" || nowNS >= entry.expireAt {
			entry.token = token
			entry.expireAt = nowNS + int64(leaseTTL)
			entry.mu.Unlock()
			entry.touch()
			return true
		}
		entry.mu.Unlock()

		if !now.Before(deadline) {
			// 获取失败时也触发一次访问时间，避免被误判为长期闲置。
			entry.touch()
			return false
		}
		remaining := deadline.Sub(now)
		sleep := localLockRetryInterval
		if remaining < sleep {
			sleep = remaining
		}
		if sleep > 0 {
			time.Sleep(sleep)
		}
	}
}

func localReleaseLease(entry *localLockEntry, token string) {
	if entry == nil {
		return
	}
	entry.mu.Lock()
	if entry.token == token {
		entry.token = ""
		entry.expireAt = 0
	}
	entry.mu.Unlock()
	entry.touch()
}

func tryLocalLocker(resource string, expSecond int, call func(lock *Lock) error) (err error) {
	if len(resource) == 0 {
		return ex.Throw{Code: ex.LOCAL_LOCK_ACQUIRE, Msg: "resource cannot be empty"}
	}
	if call == nil {
		return ex.Throw{Code: ex.LOCAL_LOCK_ACQUIRE, Msg: "callback function cannot be nil"}
	}
	if expSecond <= 0 {
		expSecond = 1
	}

	entry := getLocalLockEntry(resource)
	token := fmt.Sprintf("local-%d", time.Now().UnixNano())
	waitTimeout := localLockWaitTimeout
	leaseTTL := time.Duration(expSecond) * time.Second

	if !localTryAcquireLease(entry, token, waitTimeout, leaseTTL) {
		return ex.Throw{Code: ex.LOCAL_LOCK_ACQUIRE, Msg: "resource is already locked"}
	}

	now := time.Now()
	lockObj := &Lock{
		resource:        resource,
		token:           token,
		exp:             expSecond,
		acquireTime:     now,
		isValid:         1,
		isLocal:         1,
		refreshCount:    0,
		refreshFailures: 0,
		localEntry:      entry,
	}
	lockObj.lastRefresh.Store(now)
	defer func() {
		// 先recover，再解锁，确保panic路径不会跳过解锁
		if r := recover(); r != nil {
			_ = lockObj.Unlock()
			zlog.Error("panic in local lock callback", 0, zlog.String("resource", resource), zlog.Any("panic", r))
			panic(r)
		}
		_ = lockObj.Unlock()
	}()
	return call(lockObj)
}
