package sqld

import (
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/godaddy-x/freego/cache"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/zlog"
)

// 缓存和清理相关的常量
const (
	defaultCacheExpire   = 365 * 24 * 3600 // 1年兜底过期时间(秒)
	cacheExtendThreshold = 5 * time.Second // 缓存延长时间阈值
	idleCleanupDelay     = 5 * time.Second // 闲置清理延迟时间
	invalidStmtExpire    = 10              // 无效stmt缓存时间(秒)
	useFastHash          = true            // 是否使用快速哈希算法（FNV-1a）
	initialExpireTime    = 30 * time.Second
	shutdownTimeout      = 5 * time.Second // 关闭超时时间
)

var (
	ErrStmtClosed   = errors.New("sql: statement is closed")
	ErrInvalidSQL   = errors.New("invalid SQL statement")
	ErrShuttingDown = errors.New("prepareManager is shutting down")
)

// stmtState 定义 stmt 的状态
type stmtState int32

const (
	stateActive  stmtState = iota // 活跃使用中
	stateIdle                     // 空闲等待清理
	stateClosing                  // 正在关闭
	stateClosed                   // 已关闭
)

// String 返回状态的字符串表示
func (s stmtState) String() string {
	switch s {
	case stateActive:
		return "active"
	case stateIdle:
		return "idle"
	case stateClosing:
		return "closing"
	case stateClosed:
		return "closed"
	default:
		return "unknown"
	}
}

// stmtWrapper 包装预编译语句及元数据
type stmtWrapper struct {
	// 指针组 (8字节对齐)
	stmt         *sql.Stmt
	cleanupTimer *time.Timer

	// 原子状态 (4字节)
	state    atomic.Int32 // 使用 atomic.Int32 存储 stmtState
	refCount atomic.Int32

	// 字符串 (16字节)
	sqlHash string

	// 时间组 (24字节)
	createdAt     time.Time
	cacheExpireAt time.Time

	// 同步原语
	closeMu sync.Mutex // 保护关闭操作
	timerMu sync.Mutex // 保护 timer 操作和 cacheExpireAt

	// 通道 (8字节)
	shutdownDone chan struct{}
}

// invalidMarker 标记无效SQL，防御缓存穿透
type invalidMarker struct{}

// prepareManager 预编译语句管理器
type prepareManager struct {
	createMu     sync.Mutex                           // 保护creating map的全局锁
	creating     map[string]*sync.Mutex               // 细粒度创建锁
	cacheStmt    *cache.TTLCache[string, interface{}] // 并发安全缓存
	shutdownChan chan struct{}                        // 关闭信号
	shutdownOnce sync.Once                            // 确保只关闭一次
}

// newPrepareManager 创建新的 prepareManager
func newPrepareManager() *prepareManager {
	pm := &prepareManager{
		creating:     make(map[string]*sync.Mutex),
		cacheStmt:    cache.NewTTLCache[string, interface{}](1000),
		shutdownChan: make(chan struct{}),
	}
	return pm
}

var defaultPrepareManager = newPrepareManager()

// getCacheStmt 获取或创建缓存的预编译语句
func (pm *prepareManager) getCacheStmt(manager *RDBManager, sqlstr string) (*sql.Stmt, func(), string, error) {
	// 快速失败：检查是否正在关闭
	if pm.isShutdown() {
		return nil, nil, "", ErrShuttingDown
	}

	sqlHash := pm.generateSQLHash(manager, sqlstr)
	cacheKey := sqlHash

	// 快速路径：尝试从缓存获取
	if stmt, release, ok := pm.tryGetFromCache(cacheKey, sqlHash); ok {
		return stmt, release, cacheKey, nil
	}

	// 慢路径：加锁创建
	return pm.createWithLock(manager, sqlstr, sqlHash, cacheKey)
}

// generateSQLHash 生成 SQL 哈希
func (pm *prepareManager) generateSQLHash(manager *RDBManager, sqlstr string) string {
	data := utils.AddStr(manager.Option.DsName, manager.Option.Database, sqlstr)
	if useFastHash {
		return utils.FNV1a64(data)
	}
	return utils.SHA256(data)
}

// tryGetFromCache 尝试从缓存获取 stmt
func (pm *prepareManager) tryGetFromCache(cacheKey, sqlHash string) (*sql.Stmt, func(), bool) {
	value, exists := pm.cacheStmt.Get(cacheKey)
	if !exists || value == nil {
		return nil, nil, false
	}

	// 处理无效标记
	if _, ok := value.(*invalidMarker); ok {
		return nil, nil, false
	}

	wrapper, ok := value.(*stmtWrapper)
	if !ok || wrapper.stmt == nil || wrapper.sqlHash == "" {
		pm.cacheStmt.Delete(cacheKey)
		return nil, nil, false
	}

	// 检查状态并增加引用计数
	if !pm.tryAcquireStmt(wrapper, sqlHash) {
		return nil, nil, false
	}

	// 双重检查：确认缓存中的值没有被替换
	if val, exists := pm.cacheStmt.Get(cacheKey); !exists || val != value {
		pm.releaseStmt(wrapper, cacheKey)
		return nil, nil, false
	}

	// 延长过期时间（如果需要）
	pm.extendExpireIfNeeded(wrapper, cacheKey)

	return wrapper.stmt, pm.createReleaseFunc(wrapper, cacheKey), true
}

// tryAcquireStmt 尝试获取 stmt 的所有权
func (pm *prepareManager) tryAcquireStmt(wrapper *stmtWrapper, sqlHash string) bool {
	// 先验证 sqlHash 和 stmt，避免无效的状态变更
	if wrapper.sqlHash != sqlHash || wrapper.stmt == nil {
		return false
	}

	for {
		state := stmtState(wrapper.state.Load())
		switch state {
		case stateActive, stateIdle:
			if wrapper.state.CompareAndSwap(int32(state), int32(stateActive)) {
				// CAS 成功，增加引用计数
				wrapper.refCount.Add(1)
				return true
			}
			// CAS 失败，继续循环重试
		case stateClosing, stateClosed:
			return false
		default:
			return false
		}
	}
}

// extendExpireIfNeeded 如果需要则延长过期时间
func (pm *prepareManager) extendExpireIfNeeded(wrapper *stmtWrapper, cacheKey string) {
	wrapper.timerMu.Lock()
	defer wrapper.timerMu.Unlock()

	now := time.Now()
	if wrapper.cacheExpireAt.Sub(now) < cacheExtendThreshold {
		// 双重检查：确保缓存中的值仍是当前 wrapper
		if val, ok := pm.cacheStmt.Get(cacheKey); ok && val == wrapper {
			newExpire := now.Add(initialExpireTime)
			wrapper.cacheExpireAt = newExpire
			pm.cacheStmt.Set(cacheKey, wrapper, int64(initialExpireTime.Seconds()))
		}
	}
}

// createWithLock 加锁创建新的 stmt
func (pm *prepareManager) createWithLock(manager *RDBManager, sqlstr, sqlHash, cacheKey string) (*sql.Stmt, func(), string, error) {
	// 第一次检查：快速失败
	if pm.isShutdown() {
		return nil, nil, cacheKey, ErrShuttingDown
	}

	mu := pm.getCreateMutex(cacheKey)
	mu.Lock()
	defer mu.Unlock()

	// 第二次检查：获取锁期间可能已关闭
	if pm.isShutdown() {
		return nil, nil, cacheKey, ErrShuttingDown
	}

	// 再次检查缓存（可能在等待锁期间被其他协程创建）
	if stmt, release, ok := pm.tryGetFromCache(cacheKey, sqlHash); ok {
		return stmt, release, cacheKey, nil
	}

	return pm.createNewStmt(manager.Db, sqlstr, sqlHash, cacheKey)
}

// getCreateMutex 获取或创建细粒度锁
func (pm *prepareManager) getCreateMutex(key string) *sync.Mutex {
	pm.createMu.Lock()
	defer pm.createMu.Unlock()

	if pm.creating == nil {
		pm.creating = make(map[string]*sync.Mutex)
	}

	if mu, exists := pm.creating[key]; exists {
		return mu
	}

	mu := &sync.Mutex{}
	pm.creating[key] = mu
	return mu
}

// createNewStmt 实际创建预编译语句并缓存
func (pm *prepareManager) createNewStmt(db *sql.DB, sqlstr, sqlHash, cacheKey string) (*sql.Stmt, func(), string, error) {
	// 创建前再次检查关闭状态
	if pm.isShutdown() {
		return nil, nil, cacheKey, ErrShuttingDown
	}

	stmt, err := db.Prepare(sqlstr)
	if err != nil {
		// 缓存失败标记，防止缓存穿透
		pm.cacheStmt.Set(cacheKey, &invalidMarker{}, invalidStmtExpire)
		return nil, nil, cacheKey, fmt.Errorf("prepare stmt failed: %w", err)
	}

	now := time.Now()
	wrapper := &stmtWrapper{
		stmt:          stmt,
		sqlHash:       sqlHash,
		createdAt:     now,
		cacheExpireAt: now.Add(initialExpireTime),
		shutdownDone:  make(chan struct{}),
	}
	wrapper.state.Store(int32(stateActive))
	wrapper.refCount.Store(1)

	pm.cacheStmt.Set(cacheKey, wrapper, defaultCacheExpire)

	zlog.Debug("created new prepared statement", 0,
		zlog.String("key", cacheKey),
		zlog.String("sql_hash", sqlHash))

	return stmt, pm.createReleaseFunc(wrapper, cacheKey), cacheKey, nil
}

// createReleaseFunc 创建资源释放函数
func (pm *prepareManager) createReleaseFunc(wrapper *stmtWrapper, cacheKey string) func() {
	return func() {
		pm.releaseStmt(wrapper, cacheKey)
	}
}

// releaseStmt 释放 stmt 引用
func (pm *prepareManager) releaseStmt(wrapper *stmtWrapper, cacheKey string) {
	newCount := wrapper.refCount.Add(-1)

	if newCount < 0 {
		zlog.Warn("refCount negative", 0,
			zlog.String("key", cacheKey),
			zlog.Int32("count", newCount))
		wrapper.refCount.Store(0)
		return
	}

	if newCount == 0 {
		pm.transitionToIdle(wrapper, cacheKey)
	}
}

// transitionToIdle 将 stmt 转换为空闲状态并启动清理定时器
func (pm *prepareManager) transitionToIdle(wrapper *stmtWrapper, cacheKey string) {
	// 尝试从活跃状态转换为空闲状态
	if !wrapper.state.CompareAndSwap(int32(stateActive), int32(stateIdle)) {
		return
	}

	// CAS 成功后立即检查 shutdown 状态
	if pm.isShutdown() {
		// 系统正在关闭，同步执行清理
		// 同步调用确保在 Shutdown 流程处理前完成，避免竞争
		zlog.Debug("stmt transitioned to idle during shutdown, triggering cleanup synchronously", 0,
			zlog.String("key", cacheKey))
		pm.cleanupIdleStmt(wrapper, cacheKey)
		return
	}

	now := time.Now()
	newExpireAt := now.Add(idleCleanupDelay)

	wrapper.timerMu.Lock()
	wrapper.cacheExpireAt = newExpireAt

	if wrapper.cleanupTimer != nil {
		wrapper.cleanupTimer.Stop()
	}

	wrapper.cleanupTimer = time.AfterFunc(idleCleanupDelay, func() {
		pm.cleanupIdleStmt(wrapper, cacheKey)
	})
	wrapper.timerMu.Unlock()

	pm.cacheStmt.Set(cacheKey, wrapper, int64(idleCleanupDelay.Seconds()))

	zlog.Debug("stmt transitioned to idle", 0, zlog.String("key", cacheKey))
}

// cleanupIdleStmt 清理空闲的 stmt
// 此方法是幂等的，可以安全地被多次调用
func (pm *prepareManager) cleanupIdleStmt(wrapper *stmtWrapper, cacheKey string) {
	// 尝试从空闲状态转换为关闭中状态
	// 这个 CAS 保证了只有一个 goroutine 能成功执行清理
	if !wrapper.state.CompareAndSwap(int32(stateIdle), int32(stateClosing)) {
		// 状态已改变（可能被重新使用、已被清理、或正在被其他 goroutine 清理）
		zlog.Debug("cleanupIdleStmt skipped: state not idle", 0,
			zlog.String("key", cacheKey),
			zlog.String("state", stmtState(wrapper.state.Load()).String()))
		return
	}

	// 再次确认引用计数为 0
	if wrapper.refCount.Load() != 0 {
		// 引用计数不为 0，使用 CAS 恢复为空闲状态
		wrapper.state.CompareAndSwap(int32(stateClosing), int32(stateIdle))
		zlog.Debug("cleanupIdleStmt skipped: refCount not zero", 0,
			zlog.String("key", cacheKey),
			zlog.Int32("refCount", wrapper.refCount.Load()))
		return
	}

	// 执行关闭操作
	wrapper.closeMu.Lock()
	defer wrapper.closeMu.Unlock()

	// 最终状态检查
	if wrapper.state.Load() != int32(stateClosing) {
		return
	}

	// 关闭 stmt
	if err := wrapper.stmt.Close(); err != nil {
		zlog.Error("close idle stmt failed", 0,
			zlog.String("key", cacheKey),
			zlog.AddError(err))
	} else {
		zlog.Debug("idle stmt closed", 0, zlog.String("key", cacheKey))
	}

	// 标记为已关闭
	wrapper.state.Store(int32(stateClosed))

	// 从缓存中删除（幂等操作）
	pm.cacheStmt.Delete(cacheKey)

	// 清理创建锁，防止内存泄漏
	pm.cleanupCreateMutex(cacheKey)

	// 安全关闭 shutdownDone
	select {
	case <-wrapper.shutdownDone:
		// 已经关闭
	default:
		close(wrapper.shutdownDone)
	}
}

// isShutdown 检查是否已关闭
func (pm *prepareManager) isShutdown() bool {
	select {
	case <-pm.shutdownChan:
		return true
	default:
		return false
	}
}

// cleanupCreateMutex 清理创建锁
func (pm *prepareManager) cleanupCreateMutex(key string) {
	pm.createMu.Lock()
	defer pm.createMu.Unlock()
	delete(pm.creating, key)
}

// Shutdown 优雅关闭
func (pm *prepareManager) Shutdown() {
	pm.shutdownOnce.Do(func() {
		zlog.Info("prepareManager shutdown starting", 0)
		close(pm.shutdownChan)

		// 收集所有待处理的 stmt
		items := pm.collectStmtItems()

		// 清理 creating map
		pm.createMu.Lock()
		pm.creating = nil
		pm.createMu.Unlock()

		// 处理收集到的 stmt
		pendingCleanups, activeStmts, idleStmts := pm.processShutdownItems(items)

		zlog.Info("prepareManager stmt cleanup summary", 0,
			zlog.Int("active_stmts_closed", activeStmts),
			zlog.Int("idle_stmts_waiting", idleStmts))

		// 等待异步清理完成
		pm.waitForPendingCleanups(pendingCleanups)

		// 关闭 TTLCache
		if pm.cacheStmt != nil {
			pm.cacheStmt.Close()
		}

		zlog.Info("prepareManager shutdown completed", 0)
	})
}

// stmtItem 用于收集待处理的 stmt 信息
type stmtItem struct {
	key     string
	wrapper *stmtWrapper
}

// collectStmtItems 收集所有缓存中的 stmt
func (pm *prepareManager) collectStmtItems() []stmtItem {
	var items []stmtItem
	var keysToDelete []string

	// 第一步：收集所有 key 和 wrapper
	pm.cacheStmt.RangeRaw(func(key string, value interface{}) {
		if wrapper, ok := value.(*stmtWrapper); ok {
			items = append(items, stmtItem{key: key, wrapper: wrapper})
			keysToDelete = append(keysToDelete, key)
		}
	})

	// 第二步：批量删除
	for _, key := range keysToDelete {
		pm.cacheStmt.Delete(key)
	}

	return items
}

// processShutdownItems 处理关闭时的 stmt 项
func (pm *prepareManager) processShutdownItems(items []stmtItem) ([]chan struct{}, int, int) {
	var pendingCleanups []chan struct{}
	var activeStmts, idleStmts int

	for _, item := range items {
		refCount := item.wrapper.refCount.Load()
		state := stmtState(item.wrapper.state.Load())

		switch state {
		case stateActive:
			if pm.tryForceClose(item.wrapper, item.key, refCount) {
				activeStmts++
			}
		case stateIdle:
			pendingCleanups = append(pendingCleanups, item.wrapper.shutdownDone)
			idleStmts++
			zlog.Debug("waiting for idle stmt cleanup", 0, zlog.String("key", item.key))
		case stateClosing:
			pendingCleanups = append(pendingCleanups, item.wrapper.shutdownDone)
			zlog.Debug("waiting for closing stmt", 0, zlog.String("key", item.key))
		case stateClosed:
			// 已关闭，忽略
		}
	}

	return pendingCleanups, activeStmts, idleStmts
}

// tryForceClose 尝试强制关闭活跃的 stmt
func (pm *prepareManager) tryForceClose(wrapper *stmtWrapper, key string, refCount int32) bool {
	// 尝试转换为关闭中状态
	if !wrapper.state.CompareAndSwap(int32(stateActive), int32(stateClosing)) {
		return false
	}

	wrapper.closeMu.Lock()
	defer wrapper.closeMu.Unlock()

	if err := wrapper.stmt.Close(); err != nil {
		zlog.Warn("force close stmt failed", 0,
			zlog.String("key", key),
			zlog.AddError(err))
		return false
	}

	wrapper.state.Store(int32(stateClosed))

	// ✅ 安全关闭 shutdownDone
	select {
	case <-wrapper.shutdownDone:
		// 已经关闭
	default:
		close(wrapper.shutdownDone)
	}

	zlog.Debug("force closed active stmt", 0,
		zlog.String("key", key),
		zlog.Int32("ref_count", refCount))
	return true
}

// waitForPendingCleanups 等待待处理的清理完成
func (pm *prepareManager) waitForPendingCleanups(pendingCleanups []chan struct{}) {
	if len(pendingCleanups) == 0 {
		return
	}

	zlog.Info("prepareManager waiting for async cleanup", 0,
		zlog.Int("pending_count", len(pendingCleanups)))

	timeout := time.After(shutdownTimeout)
	cleanupCount := 0

	for i, done := range pendingCleanups {
		select {
		case <-done:
			cleanupCount++
		case <-timeout:
			zlog.Warn("timeout waiting for stmt cleanup", 0,
				zlog.Int("completed", cleanupCount),
				zlog.Int("remaining", len(pendingCleanups)-cleanupCount))
			return
		}

		// 每完成10个记录一次进度
		if (i+1)%10 == 0 {
			zlog.Debug("cleanup progress", 0,
				zlog.Int("completed", cleanupCount),
				zlog.Int("total", len(pendingCleanups)))
		}
	}

	zlog.Info("prepareManager async cleanup completed", 0,
		zlog.Int("cleanup_count", cleanupCount))
}

// Stats 返回缓存统计信息
type PrepareManagerStats struct {
	CacheSize      int   `json:"cache_size"`
	CreatingCount  int   `json:"creating_count"`
	ActiveStmts    int64 `json:"active_stmts"`
	IdleStmts      int64 `json:"idle_stmts"`
	ClosingStmts   int64 `json:"closing_stmts"`
	ClosedStmts    int64 `json:"closed_stmts"`
	IsShuttingDown bool  `json:"is_shutting_down"`
}

// GetStats 获取统计信息
func (pm *prepareManager) GetStats() *PrepareManagerStats {
	stats := &PrepareManagerStats{
		IsShuttingDown: pm.isShutdown(),
	}

	// 统计缓存中的 stmt 状态
	var activeCount, idleCount, closingCount, closedCount int64

	pm.cacheStmt.RangeRaw(func(key string, value interface{}) {
		if wrapper, ok := value.(*stmtWrapper); ok {
			switch stmtState(wrapper.state.Load()) {
			case stateActive:
				atomic.AddInt64(&activeCount, 1)
			case stateIdle:
				atomic.AddInt64(&idleCount, 1)
			case stateClosing:
				atomic.AddInt64(&closingCount, 1)
			case stateClosed:
				atomic.AddInt64(&closedCount, 1)
			}
		}
	})

	stats.ActiveStmts = activeCount
	stats.IdleStmts = idleCount
	stats.ClosingStmts = closingCount
	stats.ClosedStmts = closedCount
	stats.CacheSize = pm.cacheStmt.Len()

	pm.createMu.Lock()
	stats.CreatingCount = len(pm.creating)
	pm.createMu.Unlock()

	return stats
}

func (pm *prepareManager) HealthCheck() error {
	if pm.isShutdown() {
		return ErrShuttingDown
	}

	pm.createMu.Lock()
	creatingCount := len(pm.creating)
	pm.createMu.Unlock()

	cacheSize := pm.cacheStmt.Len()
	var threshold int

	if cacheSize == 0 {
		threshold = 100 // 默认阈值
	} else if cacheSize < 100 {
		threshold = 200 // 极小缓存场景更宽容
	} else if cacheSize > 10000 {
		threshold = cacheSize / 10
	} else {
		threshold = cacheSize * 2
	}

	if creatingCount > threshold {
		return fmt.Errorf("too many pending creates: %d (threshold: %d)", creatingCount, threshold)
	}

	return nil
}
