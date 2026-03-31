package cache

import (
	"context"
	"sync"
	"time"

	"github.com/godaddy-x/freego/ex"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/zlog"
)

var (
	localLockEntryPoolMu sync.Mutex
	localLockEntryPool   = make(map[string]*localLockEntry)
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
	refs     int32
}

func (e *localLockEntry) touch() {
	e.mu.Lock()
	// 仅在仍被引用时刷新活跃时间，避免在释放引用后人为延长回收窗口。
	if e.refs > 0 {
		e.lastUsed = time.Now().UnixNano()
	}
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
				localLockEntryPoolMu.Lock()
				for key, entry := range localLockEntryPool {
					if entry == nil {
						delete(localLockEntryPool, key)
						continue
					}
					entry.mu.Lock()
					idle := entry.token == ""
					lastUsed := entry.lastUsed
					refs := entry.refs
					entry.mu.Unlock()
					// 仅回收空闲、超时且无引用的entry，避免并发下出现双entry或误删正在使用的entry。
					if idle && refs == 0 && now-lastUsed > int64(localLockIdleTTL) {
						delete(localLockEntryPool, key)
					}
				}
				localLockEntryPoolMu.Unlock()
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
	localLockEntryPoolMu.Lock()
	defer localLockEntryPoolMu.Unlock()
	if e, ok := localLockEntryPool[resource]; ok && e != nil {
		e.mu.Lock()
		e.refs++
		e.mu.Unlock()
		e.touch()
		return e
	}
	e := &localLockEntry{
		token:    "",
		expireAt: 0,
		lastUsed: time.Now().UnixNano(),
		refs:     1,
	}
	localLockEntryPool[resource] = e
	return e
}

func localReleaseEntryRef(entry *localLockEntry) {
	if entry == nil {
		return
	}
	entry.mu.Lock()
	if entry.refs > 0 {
		entry.refs--
	}
	entry.mu.Unlock()
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
	token := utils.GetUUID(true)
	waitTimeout := localLockWaitTimeout
	leaseTTL := time.Duration(expSecond) * time.Second

	if !localTryAcquireLease(entry, token, waitTimeout, leaseTTL) {
		localReleaseEntryRef(entry)
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
			localReleaseEntryRef(entry)
			zlog.Error("panic in local lock callback", 0, zlog.String("resource", resource), zlog.Any("panic", r))
			panic(r)
		}
		_ = lockObj.Unlock()
		localReleaseEntryRef(entry)
	}()
	return call(lockObj)
}
