package cache

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/bsm/redislock"
	"github.com/godaddy-x/freego/ex"
	"github.com/godaddy-x/freego/zlog"
)

// LockConfig 分布式锁配置参数
type LockConfig struct {
	// 基础时间配置（8字节字段）
	MinExpireSeconds int // 最小过期时间（秒），默认3秒，防止过期时间过短

	// 比例配置（8字节字段）
	RefreshIntervalRatio float64 // 续期间隔比例，默认1/3，即过期时间的1/3时进行续期
	AcquireTimeoutRatio  float64 // 获取锁超时比例，默认1/2，即最多等待过期时间的一半

	// 重试次数配置（8字节字段）
	MaxRefreshRetries int // 最大续期重试次数，默认2次，防止无限重试
	MaxAcquireRetries int // 获取锁最大重试次数，默认3次，防止无限重试

	// 时间间隔配置（8字节字段）
	MinRetryBackoff     time.Duration // 最小重试间隔，默认100毫秒，避免过于频繁重试
	MaxRetryBackoff     time.Duration // 最大重试间隔，默认2秒，避免等待过久
	RefreshRetryBackoff time.Duration // 续期重试间隔，默认100毫秒，续期需要及时重试
}

// DefaultLockConfig 返回默认的锁配置
func DefaultLockConfig() *LockConfig {
	return &LockConfig{
		MinExpireSeconds:     3,
		RefreshIntervalRatio: 1.0 / 3.0,
		AcquireTimeoutRatio:  1.0 / 2.0,
		MaxRefreshRetries:    2,
		MaxAcquireRetries:    3,
		MinRetryBackoff:      100 * time.Millisecond,
		MaxRetryBackoff:      2 * time.Second,
		RefreshRetryBackoff:  100 * time.Millisecond,
	}
}

// ValidateLockConfig 验证锁配置参数的合理性
func ValidateLockConfig(config *LockConfig) error {
	if config == nil {
		return fmt.Errorf("lock config cannot be nil")
	}

	if config.MinExpireSeconds <= 0 {
		return fmt.Errorf("MinExpireSeconds must be greater than 0")
	}

	if config.RefreshIntervalRatio <= 0 || config.RefreshIntervalRatio >= 1 {
		return fmt.Errorf("RefreshIntervalRatio must be between 0 and 1 (exclusive)")
	}

	if config.AcquireTimeoutRatio <= 0 || config.AcquireTimeoutRatio > 1 {
		return fmt.Errorf("AcquireTimeoutRatio must be between 0 and 1 (inclusive)")
	}

	if config.MaxRefreshRetries < 0 {
		return fmt.Errorf("MaxRefreshRetries must be non-negative")
	}

	if config.MaxAcquireRetries < 0 {
		return fmt.Errorf("MaxAcquireRetries must be non-negative")
	}

	if config.MinRetryBackoff < 0 {
		return fmt.Errorf("MinRetryBackoff must be non-negative")
	}

	if config.MaxRetryBackoff < config.MinRetryBackoff {
		return fmt.Errorf("MaxRetryBackoff must be greater than or equal to MinRetryBackoff")
	}

	if config.RefreshRetryBackoff <= 0 {
		return fmt.Errorf("RefreshRetryBackoff must be greater than 0")
	}

	return nil
}

// Lock 表示一个已获取的分布式锁，包含自动续期能力
type Lock struct {
	// 字符串字段（16字节对齐）
	resource string // 锁资源标识
	token    string // 锁持有者唯一标识

	// 指针和函数字段（8字节对齐）
	locker        *redislock.Lock    // 底层锁实例（bsm/redis-lock）
	cancelRefresh context.CancelFunc // 取消续期goroutine的函数

	// 整数字段（8字节对齐）
	exp int // 锁初始过期时间（秒）

	// 时间字段（8字节对齐）
	acquireTime time.Time // 锁获取时间，用于监控锁的持有时长

	// 原子字段（8字节对齐）
	lastRefresh atomic.Value // 最后续期时间，使用原子操作保证线程安全

	// 32位整数字段（4字节对齐）
	isValid         int32 // 锁是否仍然有效（1=有效，0=失效），使用原子操作
	refreshCount    int32 // 续期成功次数，使用原子操作
	refreshFailures int32 // 续期失败次数，用于监控续期稳定性
}

// TryLockWithTimeout 尝试获取分布式锁，带超时控制，并执行回调函数
// resource: 锁资源名称（如"order:10086"）
// expSecond: 锁初始过期时间（秒，建议>=3）
// call: 临界区回调函数，接收Lock实例用于状态检查（获取锁后执行）
// config: 可选的锁配置，默认使用DefaultLockConfig()
func (self *RedisManager) TryLockWithTimeout(resource string, expSecond int, call func(lock *Lock) error, config ...*LockConfig) error {
	// 获取配置参数，默认使用DefaultLockConfig
	lockConfig := DefaultLockConfig()
	if len(config) > 0 && config[0] != nil {
		lockConfig = config[0]
	}

	// 验证配置参数
	if err := ValidateLockConfig(lockConfig); err != nil {
		zlog.Error("invalid lock configuration", 0,
			zlog.String("ds_name", self.DsName),
			zlog.String("resource", resource),
			zlog.AddError(err))
		return ex.Throw{Code: ex.REDIS_LOCK_ACQUIRE, Msg: "invalid lock configuration: " + err.Error()}
	}

	// 1. 参数校验
	if len(resource) == 0 {
		zlog.Warn("lock resource is empty", 0, zlog.String("ds_name", self.DsName))
		return ex.Throw{Code: ex.REDIS_LOCK_ACQUIRE, Msg: "resource cannot be empty"}
	}
	if expSecond < lockConfig.MinExpireSeconds {
		zlog.Warn("lock expiration too short", 0,
			zlog.String("ds_name", self.DsName),
			zlog.String("resource", resource),
			zlog.Int("exp_second", expSecond),
			zlog.Int("min_expire_seconds", lockConfig.MinExpireSeconds))
		return ex.Throw{Code: ex.REDIS_LOCK_ACQUIRE, Msg: fmt.Sprintf("expiration time must be at least %d seconds", lockConfig.MinExpireSeconds)}
	}
	if call == nil {
		zlog.Warn("lock callback is nil", 0,
			zlog.String("ds_name", self.DsName),
			zlog.String("resource", resource))
		return ex.Throw{Code: ex.REDIS_LOCK_ACQUIRE, Msg: "callback function cannot be nil"}
	}

	// 2. 配置锁参数
	expiry := time.Duration(expSecond) * time.Second
	// 续期间隔：根据配置的比例计算（确保在锁过期前完成续期）
	watchDogInterval := time.Duration(float64(expiry) * lockConfig.RefreshIntervalRatio)
	if watchDogInterval < time.Second {
		watchDogInterval = time.Second // 最小续期间隔1秒
	}
	// 尝试获取锁的超时时间：根据配置的比例计算（避免无限阻塞）
	acquireTimeout := time.Duration(float64(expiry) * lockConfig.AcquireTimeoutRatio)
	if acquireTimeout < time.Second {
		acquireTimeout = time.Second
	}

	// 3. 配置锁选项（重试策略）
	// 注意：bsm/redis-lock没有内置WatchDog，自动续期需要应用层实现
	opts := &redislock.Options{
		// 重试策略：指数退避，使用配置参数
		RetryStrategy: redislock.LimitRetry(
			redislock.ExponentialBackoff(lockConfig.MinRetryBackoff, lockConfig.MaxRetryBackoff),
			lockConfig.MaxAcquireRetries,
		),
	}

	// 4. 尝试获取锁（带超时上下文）
	ctx, cancel := context.WithTimeout(context.Background(), acquireTimeout)
	defer cancel()
	lock, err := self.lockClient.Obtain(ctx, resource, expiry, opts)
	if err != nil {
		if err == redislock.ErrNotObtained {
			zlog.Debug("lock already held by other processes", 0,
				zlog.String("ds_name", self.DsName),
				zlog.String("resource", resource))
			return ex.Throw{Code: ex.REDIS_LOCK_ACQUIRE, Msg: "resource is already locked"}
		}
		zlog.Error("failed to acquire lock", 0,
			zlog.String("ds_name", self.DsName),
			zlog.String("resource", resource),
			zlog.AddError(err))
		return ex.Throw{Code: ex.REDIS_LOCK_ACQUIRE, Msg: "acquire lock failed: " + err.Error()}
	}

	// 5. 创建锁实例并启动自动续期
	refreshCtx, cancelRefresh := context.WithCancel(context.Background())
	lockObj := &Lock{
		resource:        resource,
		token:           lock.Token(),
		exp:             expSecond,
		locker:          lock,
		cancelRefresh:   cancelRefresh,
		isValid:         1,
		refreshCount:    0,
		acquireTime:     time.Now(), // 记录锁获取时间
		refreshFailures: 0,
	}
	lockObj.lastRefresh.Store(time.Now()) // 初始化最后续期时间为获取时间

	// 启动自动续期
	go lockObj.startAutoRefresh(refreshCtx, watchDogInterval, lockConfig.MaxRefreshRetries, lockConfig.RefreshRetryBackoff)

	// 确保资源清理
	defer func() {
		// 先停止续期
		cancelRefresh()

		// 检查锁状态
		if !lockObj.IsValid() {
			zlog.Warn("lock became invalid during execution", 0,
				zlog.String("resource", resource),
				zlog.String("token", lockObj.token))
		}

		// 释放锁
		if unlockErr := lockObj.Unlock(); unlockErr != nil {
			zlog.Warn("failed to release lock", 0,
				zlog.String("resource", resource),
				zlog.String("token", lockObj.token),
				zlog.AddError(unlockErr))
		} else {
			zlog.Debug("lock released successfully", 0,
				zlog.String("resource", resource),
				zlog.String("token", lockObj.token))
		}
	}()

	zlog.Debug("lock acquired successfully", 0,
		zlog.String("ds_name", self.DsName),
		zlog.String("resource", resource),
		zlog.String("token", lockObj.token),
		zlog.Int("exp_second", expSecond))

	// 6. 执行临界区回调，传入Lock实例供业务代码检查状态
	return call(lockObj)
}

// startAutoRefresh 启动自动续期goroutine
func (lock *Lock) startAutoRefresh(ctx context.Context, interval time.Duration, maxRetry int, retryBackoff time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if success := lock.refresh(maxRetry, retryBackoff); !success {
				atomic.StoreInt32(&lock.isValid, 0)
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

// refresh 执行锁续期操作，包含重试逻辑
// maxRetry: 最大重试次数
// retryBackoff: 重试间隔
// 返回值: true表示续期成功，false表示续期失败
func (lock *Lock) refresh(maxRetry int, retryBackoff time.Duration) bool {
	for retryCount := 0; retryCount <= maxRetry; retryCount++ {
		// 防止空指针访问：在每次使用前检查locker是否为nil
		if lock.locker == nil {
			atomic.StoreInt32(&lock.isValid, 0)
			return false
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err := lock.locker.Refresh(ctx, time.Duration(lock.exp)*time.Second, nil)
		cancel()

		if err == nil {
			newCount := atomic.AddInt32(&lock.refreshCount, 1)
			lock.lastRefresh.Store(time.Now()) // 更新最后续期时间

			// 记录续期日志：首次续期和每10次续期
			if newCount == 1 {
				zlog.Debug("first lock refresh completed", 0,
					zlog.String("resource", lock.resource))
			} else if newCount%10 == 0 {
				zlog.Debug("successfully refreshed lock", 0,
					zlog.String("resource", lock.resource),
					zlog.Int32("refresh_count", newCount))
			}
			return true
		}

		// 续期失败时增加失败计数
		atomic.AddInt32(&lock.refreshFailures, 1)

		if retryCount < maxRetry {
			time.Sleep(retryBackoff)
		} else {
			zlog.Warn("failed to refresh lock after retries", 0,
				zlog.String("resource", lock.resource),
				zlog.Int("retries", maxRetry),
				zlog.AddError(err))
		}
	}
	return false
}

// IsValid 检查锁是否仍然有效
// 返回值: true表示锁仍然有效且可用，false表示锁已失效或已释放（续期失败或已释放）
func (lock *Lock) IsValid() bool {
	// 同时检查locker存在性和原子状态，确保锁完全有效
	return lock.locker != nil && atomic.LoadInt32(&lock.isValid) == 1
}

// RefreshCount 获取续期成功次数
func (lock *Lock) RefreshCount() int32 {
	return atomic.LoadInt32(&lock.refreshCount)
}

// Resource 获取锁的资源名称
func (lock *Lock) Resource() string {
	return lock.resource
}

// Token 获取锁的令牌
func (lock *Lock) Token() string {
	return lock.token
}

// ExpireSeconds 获取锁的过期时间（秒）
func (lock *Lock) ExpireSeconds() int {
	return lock.exp
}

// AcquireTime 获取锁的获取时间
func (lock *Lock) AcquireTime() time.Time {
	return lock.acquireTime
}

// LastRefresh 获取最后续期时间
func (lock *Lock) LastRefresh() time.Time {
	if v := lock.lastRefresh.Load(); v != nil {
		if t, ok := v.(time.Time); ok {
			return t
		}
		// 理论上不会发生，但增加保护防止类型断言失败
		zlog.Warn("unexpected type in lastRefresh", 0,
			zlog.String("resource", lock.resource),
			zlog.String("type", fmt.Sprintf("%T", v)))
	}
	return time.Time{}
}

// RefreshFailures 获取续期失败次数
func (lock *Lock) RefreshFailures() int32 {
	return atomic.LoadInt32(&lock.refreshFailures)
}

// HeldDuration 获取锁的持有时长
func (lock *Lock) HeldDuration() time.Duration {
	return time.Since(lock.acquireTime)
}

// TimeSinceLastRefresh 获取距离最后续期的时长
func (lock *Lock) TimeSinceLastRefresh() time.Duration {
	return time.Since(lock.LastRefresh())
}

// Unlock 释放锁
func (lock *Lock) Unlock() error {
	if lock.locker == nil {
		return nil // 已经释放
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := lock.locker.Release(ctx)

	// 无论释放成功与否，都清理状态
	lock.locker = nil
	atomic.StoreInt32(&lock.isValid, 0)
	// 注意：不清理cancelRefresh，因为它在defer中已经调用过了

	return err
}

// TryLocker 便捷函数：使用默认Redis管理器获取锁并执行回调
// lockObj: 锁资源名称
// expSecond: 锁过期时间（秒，>=3）
// callObj: 临界区回调函数，接收Lock实例用于状态检查
// config: 可选的锁配置，默认使用DefaultLockConfig()
func TryLocker(lockObj string, expSecond int, callObj func(lock *Lock) error, config ...*LockConfig) error {
	manager, err := NewRedis()
	if err != nil {
		zlog.Error("failed to initialize Redis manager", 0, zlog.AddError(err))
		return ex.Throw{Code: ex.REDIS_LOCK_ACQUIRE, Msg: "failed to initialize Redis manager: " + err.Error()}
	}
	if manager == nil {
		zlog.Error("Redis manager initialization returned nil", 0)
		return ex.Throw{Code: ex.REDIS_LOCK_ACQUIRE, Msg: "Redis manager initialization returned nil"}
	}
	return manager.TryLockWithTimeout(lockObj, expSecond, callObj, config...)
}
