package mysql

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"
)

func TestIsStmtInvalid(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"generic", errors.New("syntax error"), false},
		{"ErrConnDone", sql.ErrConnDone, true},
		{"ErrStmtClosed sentinel", ErrStmtClosed, true},
		{"stdlib fresh closed", errors.New("sql: statement is closed"), true},
		{"wrapped closed", errors.New("query failed: sql: statement is closed"), true},
		{"ErrBadConn", driver.ErrBadConn, true},
		{"broken pipe", errors.New("write: broken pipe"), true},
		{"invalid connection", errors.New("invalid connection"), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isStmtInvalid(tc.err); got != tc.want {
				t.Fatalf("isStmtInvalid(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestInvalidMarkerShortCircuitsPrepare(t *testing.T) {
	pm := newPrepareManager()
	key := "test-invalid-marker"
	root := errors.New("prepare denied")
	_ = pm.cacheStmt.Put(key, &invalidMarker{err: root}, invalidStmtExpire)

	stmt, release, ok, err := pm.tryGetFromCache(key, key)
	if ok || stmt != nil || release != nil {
		t.Fatalf("expected miss without stmt, got ok=%v stmt=%v", ok, stmt)
	}
	if err == nil {
		t.Fatal("expected error from invalidMarker")
	}
	if !errors.Is(err, root) {
		t.Fatalf("err = %v, want wrap of %v", err, root)
	}
}

func TestInvalidateRemovesIdleStmtFromCache(t *testing.T) {
	pm := newPrepareManager()
	key := "test-invalidate-idle"
	wrapper := &stmtWrapper{
		sqlHash:      key,
		shutdownDone: make(chan struct{}),
	}
	wrapper.state.Store(int32(stateIdle))
	wrapper.refCount.Store(0)
	_ = pm.cacheStmt.Put(key, wrapper, defaultCacheExpire)

	pm.invalidateCacheStmt(key)

	if _, exists, _ := pm.cacheStmt.Get(key, nil); exists {
		t.Fatal("cache entry should be removed after invalidate")
	}
	if stmtState(wrapper.state.Load()) != stateClosed {
		t.Fatalf("state = %v, want closed", stmtState(wrapper.state.Load()))
	}
}

func TestInvalidateDefersCloseWhileInUse(t *testing.T) {
	pm := newPrepareManager()
	key := "test-invalidate-active"
	wrapper := &stmtWrapper{
		sqlHash:      key,
		shutdownDone: make(chan struct{}),
	}
	wrapper.state.Store(int32(stateActive))
	wrapper.refCount.Store(1)
	_ = pm.cacheStmt.Put(key, wrapper, defaultCacheExpire)

	pm.invalidateCacheStmt(key)

	if _, exists, _ := pm.cacheStmt.Get(key, nil); exists {
		t.Fatal("cache entry should be removed immediately")
	}
	if stmtState(wrapper.state.Load()) != stateActive {
		t.Fatalf("in-use stmt should stay active, got %v", stmtState(wrapper.state.Load()))
	}
}

func TestCreateWithLockHonorsInvalidMarker(t *testing.T) {
	pm := newPrepareManager()
	key := "ds|db|select 1"
	root := errors.New("boom")
	_ = pm.cacheStmt.Put(key, &invalidMarker{err: root}, invalidStmtExpire)

	mgr := &RDBManager{Db: nil} // must not reach Prepare
	stmt, release, _, err := pm.createWithLock(mgr, "select 1", key, key)
	if stmt != nil || release != nil {
		t.Fatal("should not create stmt while invalidMarker present")
	}
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("err = %v", err)
	}
}

func TestInvalidateEmptyKeyNoop(t *testing.T) {
	pm := newPrepareManager()
	pm.invalidateCacheStmt("")
	pm.invalidateCacheStmt("missing")
}

func TestReleaseThenCleanupIdle(t *testing.T) {
	pm := newPrepareManager()
	key := "test-release-idle"
	wrapper := &stmtWrapper{
		sqlHash:      key,
		shutdownDone: make(chan struct{}),
	}
	wrapper.state.Store(int32(stateActive))
	wrapper.refCount.Store(1)
	_ = pm.cacheStmt.Put(key, wrapper, defaultCacheExpire)

	pm.releaseStmt(wrapper, key)
	if stmtState(wrapper.state.Load()) != stateIdle {
		t.Fatalf("state = %v, want idle", stmtState(wrapper.state.Load()))
	}

	pm.cleanupIdleStmt(wrapper, key)
	if stmtState(wrapper.state.Load()) != stateClosed {
		t.Fatalf("state = %v, want closed", stmtState(wrapper.state.Load()))
	}
	if _, exists, _ := pm.cacheStmt.Get(key, nil); exists {
		t.Fatal("cache entry should be removed after idle cleanup")
	}
}

// TestCleanupClosingStmtDoesNotDeleteReplacementWrapper 回归 §10.1：
// invalidate 后旧 wrapper idle cleanup 不得 Del 已替换的新 cache 条目。
func TestCleanupClosingStmtDoesNotDeleteReplacementWrapper(t *testing.T) {
	pm := newPrepareManager()
	key := "test-replace-key"

	oldW := &stmtWrapper{
		sqlHash:      key,
		shutdownDone: make(chan struct{}),
	}
	oldW.state.Store(int32(stateClosing))

	newW := &stmtWrapper{
		sqlHash:      key,
		shutdownDone: make(chan struct{}),
	}
	newW.state.Store(int32(stateActive))
	newW.refCount.Store(1)
	_ = pm.cacheStmt.Put(key, newW, defaultCacheExpire)

	pm.cleanupClosingStmt(oldW, key, "test old wrapper close")

	val, exists, _ := pm.cacheStmt.Get(key, nil)
	if !exists {
		t.Fatal("replacement wrapper should remain in cache")
	}
	if val != newW {
		t.Fatalf("cache value = %p, want new wrapper %p", val, newW)
	}
	if stmtState(oldW.state.Load()) != stateClosed {
		t.Fatalf("old wrapper state = %v, want closed", stmtState(oldW.state.Load()))
	}
}
