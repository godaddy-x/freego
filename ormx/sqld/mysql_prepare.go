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

var (
	ErrStmtClosed         = errors.New("sql: statement is closed")
	ErrInvalidSQL         = errors.New("invalid SQL statement")
	defaultPrepareManager = &prepareManager{
		creating:  make(map[string]*sync.Mutex),
		cacheStmt: cache.NewLocalCache(365*24*3600, 60), // 1年有效期，保证不容易过期，增大缓存容量，减少LRU淘汰
	}
)

type prepareManager struct {
	createMu  sync.Mutex             // 保护creating map的全局锁
	creating  map[string]*sync.Mutex // 细粒度创建锁，避免并发创建stmt
	cacheStmt cache.Cache            // 第三方缓存接口（需实现并发安全）
}

// stmtWrapper 包装预编译语句及元数据
type stmtWrapper struct {
	stmt        *sql.Stmt
	refCount    int32
	sqlHash     string
	closed      atomic.Bool
	createdAt   time.Time // 用于监控和调试
	cleanupOnce sync.Once // 确保清理操作只执行一次
}

// invalidMarker 标记无效SQL，防御缓存穿透
type invalidMarker struct{}

// getCacheStmt 获取或创建缓存的预编译语句
func (self *prepareManager) getCacheStmt(manager *RDBManager, sqlstr string) (*sql.Stmt, func(), string, error) {
	sqlHash := utils.SHA256(utils.AddStr(manager.Option.DsName, manager.Option.Database, sqlstr))
	cacheKey := sqlHash

	// 直接使用细粒度锁保护整个获取/创建过程，避免并发竞争
	mu := self.getCreateMutex(cacheKey)
	mu.Lock()
	defer mu.Unlock()

	// 在锁内再次尝试从缓存获取（双重检查）
	value, exists, err := self.cacheStmt.Get(cacheKey, nil)
	if err != nil {
		return nil, nil, "", fmt.Errorf("cache get failed: %w", err)
	}

	if exists && value != nil {
		// 检查是否为无效标记
		if _, ok := value.(*invalidMarker); ok {
			return nil, nil, cacheKey, fmt.Errorf("%w: SQL preparation previously failed", ErrInvalidSQL)
		}

		// 转换为stmtWrapper并校验类型
		wrapper, ok := value.(*stmtWrapper)
		if !ok {
			zlog.Warn("invalid cache data type", 0, zlog.String("key", cacheKey))
			// 缓存数据损坏，删除并重新创建
			self.cacheStmt.Del(cacheKey)
			return self.createNewStmt(manager.Db, sqlstr, sqlHash, cacheKey)
		}

		// 检查是否已关闭或哈希碰撞
		if wrapper.closed.Load() || wrapper.sqlHash != sqlHash {
			zlog.Info("stmt closed or hash collision, recreating", 0,
				zlog.String("key", cacheKey),
				zlog.Bool("hashMismatch", wrapper.sqlHash != sqlHash),
				zlog.Bool("isClosed", wrapper.closed.Load()))
			// 删除无效缓存并重新创建
			self.cacheStmt.Del(cacheKey)
			return self.createNewStmt(manager.Db, sqlstr, sqlHash, cacheKey)
		}

		// 增加引用计数
		atomic.AddInt32(&wrapper.refCount, 1)
		return wrapper.stmt, self.createReleaseFunc(wrapper, cacheKey), cacheKey, nil
	}

	// 缓存未命中，直接创建新stmt
	return self.createNewStmt(manager.Db, sqlstr, sqlHash, cacheKey)
}

// createNewStmtWithLock 带细粒度锁的创建逻辑（避免并发创建）
func (self *prepareManager) createNewStmtWithLock(db *sql.DB, sqlstr, sqlHash, cacheKey string) (*sql.Stmt, func(), string, error) {
	// 获取当前SQL的专属锁
	mu := self.getCreateMutex(cacheKey)
	mu.Lock()
	defer mu.Unlock()

	// 双重检查：防止锁等待期间缓存已被其他协程创建
	if value, exists, err := self.cacheStmt.Get(cacheKey, nil); err == nil && exists {
		if wrapper, ok := value.(*stmtWrapper); ok &&
			!wrapper.closed.Load() &&
			wrapper.sqlHash == sqlHash {
			// 复用其他协程已创建的stmt
			atomic.AddInt32(&wrapper.refCount, 1)
			return wrapper.stmt, self.createReleaseFunc(wrapper, cacheKey), cacheKey, nil
		}
	}

	// 真正创建新stmt
	return self.createNewStmt(db, sqlstr, sqlHash, cacheKey)
}

// getCreateMutex 获取或创建细粒度锁
func (self *prepareManager) getCreateMutex(key string) *sync.Mutex {
	self.createMu.Lock()
	defer self.createMu.Unlock()

	if self.creating == nil {
		self.creating = make(map[string]*sync.Mutex)
	}

	if mu, exists := self.creating[key]; exists {
		return mu
	}

	// 新建锁并存储
	mu := &sync.Mutex{}
	self.creating[key] = mu
	return mu
}

// createNewStmt 实际创建预编译语句并缓存
func (self *prepareManager) createNewStmt(db *sql.DB, sqlstr, sqlHash, cacheKey string) (*sql.Stmt, func(), string, error) {
	// 创建预编译语句
	stmt, err := db.Prepare(sqlstr)
	if err != nil {
		// 缓存无效标记（10秒过期，允许重试）
		_ = self.cacheStmt.Put(cacheKey, &invalidMarker{}, 10)
		return nil, nil, cacheKey, fmt.Errorf("prepare stmt failed: %w", err)
	}

	// 创建包装器
	wrapper := &stmtWrapper{
		stmt:      stmt,
		refCount:  1,
		sqlHash:   sqlHash,
		createdAt: time.Now(),
	}
	wrapper.closed.Store(false)

	// 存入缓存（不设置自动过期，完全依赖引用计数管理，避免竞态条件）
	if err := self.cacheStmt.Put(cacheKey, wrapper); err != nil {
		zlog.Error("cache put failed", 0, zlog.String("key", cacheKey), zlog.AddError(err))
		stmt.Close() // 缓存失败时释放资源
		return nil, nil, cacheKey, fmt.Errorf("cache put failed: %w", err)
	}

	// 在并发基准测试中，stmt创建是正常行为，不需要记录info日志
	zlog.Info("new stmt created", 0, zlog.String("key", cacheKey))
	return stmt, self.createReleaseFunc(wrapper, cacheKey), cacheKey, nil
}

// createReleaseFunc 创建统一的资源释放函数（核心优化：移除延迟清理）
func (self *prepareManager) createReleaseFunc(wrapper *stmtWrapper, cacheKey string) func() {
	return func() {
		newCount := atomic.AddInt32(&wrapper.refCount, -1)

		// 引用计数归0：关闭stmt并**立即清理**缓存
		if newCount == 0 {
			// 使用sync.Once确保清理操作只执行一次，避免竞态条件
			wrapper.cleanupOnce.Do(func() {
				// 原子标记关闭状态并同步关闭stmt
				if wrapper.closed.CompareAndSwap(false, true) {
					if err := wrapper.stmt.Close(); err != nil {
						zlog.Error("close stmt failed", 0, zlog.String("key", cacheKey), zlog.AddError(err))
					} else {
						// 在并发基准测试中，stmt关闭是正常行为，不需要记录info日志
						// 只有在异常情况下才记录日志
						zlog.Info("stmt closed normally", 0, zlog.String("key", cacheKey))
					}
				}

				// 关键优化：立即删除缓存，避免后续请求命中已关闭的stmt
				self.cacheStmt.Del(cacheKey)

				// 立即清理细粒度锁，防止内存泄漏
				self.createMu.Lock()
				delete(self.creating, cacheKey) // 直接删除，因为sync.Once保证只执行一次
				self.createMu.Unlock()
			})

			// 引用计数为负：仅修正计数，不强制关闭
		} else if newCount < 0 {
			zlog.Info("refCount negative (race condition)", 0,
				zlog.String("key", cacheKey),
				zlog.Int32("count", newCount))
			atomic.StoreInt32(&wrapper.refCount, 0) // 仅修正计数
		}
	}
}
