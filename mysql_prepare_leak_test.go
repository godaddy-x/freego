package main

import (
	"testing"
	"time"

	sqlc "github.com/godaddy-x/freego/core/query"
	"github.com/godaddy-x/freego/store/orm/mysql"
)

const prepareLeakIdleWait = 6 * time.Second // idleCleanupDelay(5s) + 1s buffer

func TestPrepareCacheDrainsAfterIdle(t *testing.T) {
	initMysqlDB()
	db, err := mysql.NewMysqlTx(false)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	before := mysql.DefaultPrepareManagerStats()
	for i := 0; i < 500; i++ {
		var w OwWallet
		if err := db.FindOne(sqlc.M().Eq("id", benchCompareFindOneID), &w); err != nil {
			t.Fatalf("FindOne iter %d: %v", i, err)
		}
	}
	during := mysql.DefaultPrepareManagerStats()
	t.Logf("before=%+v during=%+v", before, during)

	if during.CacheSize > 3 {
		t.Errorf("hot loop cache too large: CacheSize=%d (expect <=3 for fixed FindOne SQL)", during.CacheSize)
	}
	if during.ActiveStmts+during.ClosingStmts > 0 {
		t.Errorf("unexpected in-flight wrappers after loop: active=%d closing=%d",
			during.ActiveStmts, during.ClosingStmts)
	}

	time.Sleep(prepareLeakIdleWait)
	after := mysql.DefaultPrepareManagerStats()
	t.Logf("after idle=%+v", after)

	if after.CacheSize != 0 {
		t.Errorf("stmt cache leak after idle: CacheSize=%d active=%d idle=%d closing=%d closed=%d",
			after.CacheSize, after.ActiveStmts, after.IdleStmts, after.ClosingStmts, after.ClosedStmts)
	}
}

func TestPrepareCacheManyTemplatesDrain(t *testing.T) {
	initMysqlDB()
	db, err := mysql.NewMysqlTx(false)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Different LIMIT values produce different SQL strings → multiple cache keys.
	for i := 1; i <= 30; i++ {
		result := make([]*OwWallet, 0, 10)
		if err := db.FindList(
			sqlc.M(&OwWallet{}).Between("id", benchCompareListMin, benchCompareListMax).Limit(1, int64(i)).Orderby("id", sqlc.DESC_),
			&result,
		); err != nil {
			t.Fatalf("FindList limit=%d: %v", i, err)
		}
	}

	during := mysql.DefaultPrepareManagerStats()
	t.Logf("during many templates=%+v", during)
	if during.CacheSize == 0 {
		t.Fatal("expected multiple cache entries during loop")
	}

	time.Sleep(prepareLeakIdleWait)
	after := mysql.DefaultPrepareManagerStats()
	t.Logf("after idle many templates=%+v", after)

	if after.CacheSize != 0 {
		t.Errorf("multi-template cache leak: CacheSize=%d idle=%d closing=%d",
			after.CacheSize, after.IdleStmts, after.ClosingStmts)
	}
}

func TestPrepareCacheInvalidateThenDrain(t *testing.T) {
	initMysqlDB()
	db, err := mysql.NewMysqlTx(false)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	for i := 0; i < 50; i++ {
		var w OwWallet
		if err := db.FindOne(sqlc.M().Eq("id", benchCompareFindOneID), &w); err != nil {
			t.Fatal(err)
		}
	}

	time.Sleep(prepareLeakIdleWait)
	after := mysql.DefaultPrepareManagerStats()
	if after.CacheSize != 0 {
		t.Errorf("cache not drained after repeated FindOne: %+v", after)
	}
}

func TestPrepareCacheConnectionPoolStable(t *testing.T) {
	initMysqlDB()
	db, err := mysql.NewMysqlTx(false)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	openBefore := db.Db.Stats().OpenConnections
	for i := 0; i < 200; i++ {
		var w OwWallet
		if err := db.FindOne(sqlc.M().Eq("id", benchCompareFindOneID), &w); err != nil {
			t.Fatalf("FindOne: %v", err)
		}
	}
	time.Sleep(prepareLeakIdleWait)
	openAfter := db.Db.Stats().OpenConnections
	t.Logf("pool open conns before=%d after=%d", openBefore, openAfter)

	if openAfter > openBefore+5 {
		t.Errorf("possible connection leak: before=%d after=%d", openBefore, openAfter)
	}

	stats := mysql.DefaultPrepareManagerStats()
	if stats.CacheSize != 0 {
		t.Errorf("prepare cache not empty after idle: %+v", stats)
	}
}

func TestPrepareCacheCreatingMapGrowth(t *testing.T) {
	initMysqlDB()
	db, err := mysql.NewMysqlTx(false)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	before := mysql.DefaultPrepareManagerStats().CreatingCount
	for i := 1; i <= 20; i++ {
		result := make([]*OwWallet, 0, 5)
		_ = db.FindList(sqlc.M(&OwWallet{}).Between("id", benchCompareListMin, benchCompareListMax).Limit(1, int64(i)), &result)
	}
	afterLoop := mysql.DefaultPrepareManagerStats().CreatingCount
	time.Sleep(prepareLeakIdleWait)
	afterIdle := mysql.DefaultPrepareManagerStats()

	t.Logf("creating map: before=%d afterLoop=%d afterIdle=%d cacheSize=%d",
		before, afterLoop, afterIdle.CreatingCount, afterIdle.CacheSize)

	if afterLoop <= before {
		t.Errorf("creating map should grow with new SQL templates: before=%d after=%d", before, afterLoop)
	}
	if afterIdle.CacheSize != 0 {
		t.Errorf("cache entries leaked: %+v", afterIdle)
	}
	// creating map retains mutexes — documented design debt, not stmt leak.
	if afterIdle.CreatingCount < afterLoop {
		t.Errorf("creating map shrank unexpectedly: loop=%d idle=%d", afterLoop, afterIdle.CreatingCount)
	}
	t.Logf("NOTE: creating map retains %d mutexes after cache drain (known design, not server stmt leak)",
		afterIdle.CreatingCount)
}
