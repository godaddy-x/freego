package sqld

import (
	"database/sql"
	"errors"
	"fmt"
	"hash/fnv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/godaddy-x/freego/cache"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/zlog"
)

// 缓存和清理相关的常量
const (
	defaultCacheCapacity = 100000           // 默认缓存容量
	defaultCacheExpire   = 365 * 24 * 3600  // 1年兜底过期时间(秒)
	cacheExtendThreshold = 5 * time.Second  // 缓存延长时间阈值
	idleCleanupDelay     = 5 * time.Second  // 闲置清理延迟时间
	invalidStmtExpire    = 10               // 无效stmt缓存时间(秒)
	initialExpireTime    = 30 * time.Second // 初始缓存过期时间
	useFastHash          = true             // 是否使用快速哈希算法（FNV-1a）
)

var (
	ErrStmtClosed         = errors.New("sql: statement is closed")
	ErrInvalidSQL         = errors.New("invalid SQL statement")
	defaultPrepareManager = func() *prepareManager {
		pm := &prepareManager{
			creating: make(map[string]*sync.Mutex),
			cacheStmt: cache.NewLocalCacheWithEvict(defaultCacheCapacity, defaultCacheExpire, func(key string, value interface{}) {
				if wrapper, ok := value.(*stmtWrapper); ok && !wrapper.closed.Load() {
					zlog.Warn("stmt cache evicted, may cause recreate", 0, zlog.String("key", key))
				}
			}),
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

// stmtWrapper 包装预编译语句及元数据 (优化内存对齐)
// 字段按类型和大小重新排列：指针(8字节) -> int32(4字节) -> string(16字节) -> atomic.Bool(1字节) -> time.Time(24字节) -> sync.Mutex(8字节) -> sync.Once(8字节) -> *time.Timer(8字节) -> sync.Mutex(8字节)
type stmtWrapper struct {
	// 8字节指针组
	stmt         *sql.Stmt
	cleanupTimer *time.Timer // 移动到指针组

	// 4字节整数组
	refCount int32

	// 16字节字符串组
	sqlHash string

	// 1字节原子布尔组
	closed atomic.Bool

	// 24字节时间组 (time.Time 内部对齐)
	createdAt     time.Time
	cacheExpireAt time.Time

	// 8字节互斥锁组
	reuseMu sync.Mutex
	timerMu sync.Mutex

	// 8字节 Once 组
	cleanupOnce sync.Once
}

// invalidMarker 标记无效SQL，防御缓存穿透
type invalidMarker struct{}

// fastHash 使用 FNV-1a 算法生成快速哈希（比 SHA256 快得多）
func fastHash(data string) string {
	h := fnv.New64a()
	h.Write([]byte(data))
	return fmt.Sprintf("%x", h.Sum64())
}

// getCacheStmt 获取或创建缓存的预编译语句
func (self *prepareManager) getCacheStmt(manager *RDBManager, sqlstr string) (*sql.Stmt, func(), string, error) {
	// 根据配置选择哈希算法：快速哈希 vs 强哈希
	var sqlHash string
	if useFastHash {
		sqlHash = fastHash(manager.Option.DsName + manager.Option.Database + sqlstr)
	} else {
		sqlHash = utils.SHA256(utils.AddStr(manager.Option.DsName, manager.Option.Database, sqlstr))
	}
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
					// 条件性延长缓存：剩余时间<阈值时才延长，减少Put开销
					now := time.Now()
					if wrapper.cacheExpireAt.Sub(now) < cacheExtendThreshold {
						newExpire := now.Add(initialExpireTime)
						wrapper.cacheExpireAt = newExpire
						_ = self.cacheStmt.Put(cacheKey, wrapper, int(initialExpireTime.Seconds())) // 非关键操作，忽略错误
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
		_ = self.cacheStmt.Put(cacheKey, &invalidMarker{}, invalidStmtExpire) // 缓存无效标记
		return nil, nil, cacheKey, fmt.Errorf("prepare stmt failed: %w", err)
	}

	// 初始化包装器
	now := time.Now()
	wrapper := &stmtWrapper{
		stmt:          stmt,
		refCount:      1,
		sqlHash:       sqlHash,
		createdAt:     now,
		cacheExpireAt: now.Add(initialExpireTime), // 初始过期时间
	}
	wrapper.closed.Store(false)

	// 存入缓存（使用兜底过期时间，实际由动态延长控制）
	if err := self.cacheStmt.Put(cacheKey, wrapper, defaultCacheExpire); err != nil {
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

		// 引用计数归0：设置复用窗口，过期后自动清理
		if newCount == 0 {
			// 更新缓存为短时间过期
			_ = self.cacheStmt.Put(cacheKey, wrapper, int(idleCleanupDelay.Seconds()))
			wrapper.cacheExpireAt = time.Now().Add(idleCleanupDelay)

			// 停止之前的定时器，避免重复清理
			wrapper.timerMu.Lock()
			if wrapper.cleanupTimer != nil {
				wrapper.cleanupTimer.Stop()
			}
			// 创建新定时器
			wrapper.cleanupTimer = time.AfterFunc(idleCleanupDelay, func() {
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
