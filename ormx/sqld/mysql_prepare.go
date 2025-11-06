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
	defaultPrepareManager = func() *prepareManager {
		pm := &prepareManager{
			creating:     make(map[string]*sync.Mutex),
			cacheStmt:    cache.NewLocalCache(100000, 365*24*3600), // 1年兜底过期
			shutdownChan: make(chan struct{}),
		}
		return pm
	}()
)

type prepareManager struct {
	createMu     sync.Mutex             // 保护creating map的全局锁
	creating     map[string]*sync.Mutex // 细粒度创建锁
	cacheStmt    cache.Cache            // 并发安全缓存
	shutdownChan chan struct{}          // 关闭信号
	shutdownOnce sync.Once              // 确保只关闭一次
}

// stmtWrapper 包装预编译语句及元数据
type stmtWrapper struct {
	stmt          *sql.Stmt
	refCount      int32
	sqlHash       string
	closed        atomic.Bool
	createdAt     time.Time
	cacheExpireAt time.Time   // 记录缓存过期时间（用于条件性延长）
	reuseMu       sync.Mutex  // 控制过期瞬间的并发复用
	cleanupOnce   sync.Once   // 确保清理操作只执行一次
	cleanupTimer  *time.Timer // 闲置清理定时器
	timerMu       sync.Mutex  // 保护cleanupTimer的并发安全
}

// invalidMarker 标记无效SQL，防御缓存穿透
type invalidMarker struct{}

// getCacheStmt 获取或创建缓存的预编译语句
func (self *prepareManager) getCacheStmt(manager *RDBManager, sqlstr string) (*sql.Stmt, func(), string, error) {
	sqlHash := utils.SHA256(utils.AddStr(manager.Option.DsName, manager.Option.Database, sqlstr))
	cacheKey := sqlHash

	// 快速路径：无锁查询缓存，命中则直接复用
	value, exists, err := self.cacheStmt.Get(cacheKey, nil)
	if err == nil && exists && value != nil {
		// 检查无效标记
		if _, ok := value.(*invalidMarker); ok {
			return nil, nil, cacheKey, fmt.Errorf("%w: SQL preparation previously failed", ErrInvalidSQL)
		}

		// 转换为stmtWrapper并校验基础有效性
		wrapper, ok := value.(*stmtWrapper)
		if !ok || wrapper.stmt == nil || wrapper.sqlHash == "" {
			//zlog.Warn("invalid cache data, recreating", 0, zlog.String("key", cacheKey))
			self.cacheStmt.Del(cacheKey)
		} else {
			// 加锁防止缓存过期瞬间的并发重建
			wrapper.reuseMu.Lock()
			defer wrapper.reuseMu.Unlock()

			// 二次检查缓存是否有效（防止加锁期间过期）
			if val, exists, _ := self.cacheStmt.Get(cacheKey, nil); exists && val == value {
				// 校验stmt状态
				if !wrapper.closed.Load() && wrapper.sqlHash == sqlHash {
					atomic.AddInt32(&wrapper.refCount, 1)
					// 条件性延长缓存：剩余时间<5秒时才延长，减少Put开销
					now := time.Now()
					if wrapper.cacheExpireAt.Sub(now) < 5*time.Second {
						newExpire := now.Add(30 * time.Second)
						wrapper.cacheExpireAt = newExpire
						_ = self.cacheStmt.Put(cacheKey, wrapper, 30) // 非关键操作，忽略错误
					}
					return wrapper.stmt, self.createReleaseFunc(wrapper, cacheKey), cacheKey, nil
				}
			}
		}
	}

	// 慢路径：缓存未命中或无效，加锁创建新stmt
	mu := self.getCreateMutex(cacheKey)
	mu.Lock()
	defer mu.Unlock()

	// 锁内双重检查：防止等待期间其他协程已创建
	value, exists, err = self.cacheStmt.Get(cacheKey, nil)
	if err == nil && exists && value != nil {
		if wrapper, ok := value.(*stmtWrapper); ok && !wrapper.closed.Load() && wrapper.sqlHash == sqlHash {
			atomic.AddInt32(&wrapper.refCount, 1)
			return wrapper.stmt, self.createReleaseFunc(wrapper, cacheKey), cacheKey, nil
		}
	}

	// 创建新stmt
	return self.createNewStmt(manager.Db, sqlstr, sqlHash, cacheKey)
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

	mu := &sync.Mutex{}
	self.creating[key] = mu
	return mu
}

// createNewStmt 实际创建预编译语句并缓存
func (self *prepareManager) createNewStmt(db *sql.DB, sqlstr, sqlHash, cacheKey string) (*sql.Stmt, func(), string, error) {
	// 创建预编译语句
	stmt, err := db.Prepare(sqlstr)
	if err != nil {
		_ = self.cacheStmt.Put(cacheKey, &invalidMarker{}, 10) // 缓存无效标记10秒
		return nil, nil, cacheKey, fmt.Errorf("prepare stmt failed: %w", err)
	}

	// 初始化包装器
	now := time.Now()
	wrapper := &stmtWrapper{
		stmt:          stmt,
		refCount:      1,
		sqlHash:       sqlHash,
		createdAt:     now,
		cacheExpireAt: now.Add(30 * time.Second), // 初始30秒过期
	}
	wrapper.closed.Store(false)

	// 存入缓存（1年兜底过期，实际由动态延长控制）
	if err := self.cacheStmt.Put(cacheKey, wrapper, 365*24*3600); err != nil {
		zlog.Error("cache put failed", 0, zlog.String("key", cacheKey), zlog.AddError(err))
		stmt.Close()
		return nil, nil, cacheKey, fmt.Errorf("cache put failed: %w", err)
	}

	//zlog.Info("new stmt created", 0, zlog.String("key", cacheKey))
	return stmt, self.createReleaseFunc(wrapper, cacheKey), cacheKey, nil
}

// shutdown 优雅关闭，清理所有资源
func (self *prepareManager) Shutdown() {
	self.shutdownOnce.Do(func() {
		close(self.shutdownChan)

		// 强制清理所有缓存的stmt
		self.createMu.Lock()
		defer self.createMu.Unlock()

		// 遍历所有缓存键（假设缓存支持Keys()，若不支持可维护cacheKeys集合）
		if keys, err := self.cacheStmt.Keys(); err == nil {
			for _, key := range keys {
				if value, exists, _ := self.cacheStmt.Get(key, nil); exists {
					if wrapper, ok := value.(*stmtWrapper); ok && !wrapper.closed.Load() {
						wrapper.closed.Store(true)
						wrapper.stmt.Close() // 强制关闭连接
					}
				}
				self.cacheStmt.Del(key)
				delete(self.creating, key)
			}
		}

		zlog.Info("prepareManager shutdown completed", 0)
	})
}

// createReleaseFunc 资源释放函数（核心逻辑）
func (self *prepareManager) createReleaseFunc(wrapper *stmtWrapper, cacheKey string) func() {
	return func() {
		newCount := atomic.AddInt32(&wrapper.refCount, -1)

		// 引用计数归0：设置5秒复用窗口，过期后自动清理
		if newCount == 0 {
			// 更新缓存为5秒过期
			_ = self.cacheStmt.Put(cacheKey, wrapper, 5)
			wrapper.cacheExpireAt = time.Now().Add(5 * time.Second)

			// 停止之前的定时器，避免重复清理
			wrapper.timerMu.Lock()
			if wrapper.cleanupTimer != nil {
				wrapper.cleanupTimer.Stop()
			}
			// 创建新定时器
			wrapper.cleanupTimer = time.AfterFunc(5*time.Second, func() {
				select {
				case <-self.shutdownChan:
					return // 程序关闭时不执行清理
				default:
				}

				// 双重检查：引用计数为0且未关闭
				if atomic.LoadInt32(&wrapper.refCount) == 0 &&
					wrapper.closed.CompareAndSwap(false, true) {

					wrapper.cleanupOnce.Do(func() {
						if err := wrapper.stmt.Close(); err != nil {
							zlog.Error("close idle stmt failed", 0, zlog.String("key", cacheKey), zlog.AddError(err))
						} else if zlog.IsDebug() {
							zlog.Debug("idle stmt closed after 5s", 0, zlog.String("key", cacheKey))
						}
						self.cacheStmt.Del(cacheKey)
						// 清理细粒度锁
						self.createMu.Lock()
						delete(self.creating, cacheKey)
						self.createMu.Unlock()
					})
				}
			})
			wrapper.timerMu.Unlock()

			// 修正负计数（极端竞态保护）
		} else if newCount < 0 {
			zlog.Warn("refCount negative", 0, zlog.String("key", cacheKey), zlog.Int32("count", newCount))
			atomic.StoreInt32(&wrapper.refCount, 0)
		}
	}
}
