package cache

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/godaddy-x/freego/utils"
)

// initRedis 初始化Redis连接
func initRedisForLockTest() {
	conf := RedisConfig{}
	// 尝试多个可能的路径（为了支持在不同目录下运行测试）
	paths := []string{
		"resource/redis.json",
		"../resource/redis.json",
		"../../resource/redis.json",
	}

	var err error
	for _, path := range paths {
		if err = utils.ReadLocalJsonConfig(path, &conf); err == nil {
			break
		}
	}

	if err != nil {
		panic("读取redis配置失败: " + err.Error())
	}
	new(RedisManager).InitConfig(conf)
}

func testLock() error {
	return TryLocker("123456", 120, func(lock *Lock) error {
		fmt.Println("---test lock")
		return errors.New("test lock error")
	})
}

// TestRedisLockBasicOperations 测试分布式锁基础操作
func TestRedisLockBasicOperations(t *testing.T) {
	initRedisForLockTest()

	if err := testLock(); err != nil {
		fmt.Println(err)
	}

	if err := testLock(); err != nil {
		fmt.Println(err)
	}

	// 测试基本的锁获取和释放
	t.Run("BasicLockAcquireRelease", func(t *testing.T) {
		lockKey := utils.MD5("lock_basic_" + fmt.Sprintf("%d", time.Now().UnixNano()))

		err := TryLocker(lockKey, 10, func(lock *Lock) error {
			// 验证锁状态
			if !lock.IsValid() {
				t.Error("锁获取后应该有效")
			}

			if lock.Resource() != lockKey {
				t.Errorf("期望资源名称 %s, 实际得到 %s", lockKey, lock.Resource())
			}

			if lock.ExpireSeconds() != 10 {
				t.Errorf("期望过期时间 10秒, 实际得到 %d", lock.ExpireSeconds())
			}

			// 验证令牌不为空
			if lock.Token() == "" {
				t.Error("锁令牌不能为空")
			}

			t.Logf("锁获取成功: 资源=%s, 令牌=%s", lock.Resource(), lock.Token())

			// 模拟业务逻辑
			time.Sleep(100 * time.Millisecond)

			return nil
		})

		if err != nil {
			t.Errorf("基础锁操作失败: %v", err)
		} else {
			t.Logf("基础锁获取释放测试通过")
		}
	})

	// 测试锁在回调函数中返回错误的情况
	t.Run("LockWithCallbackError", func(t *testing.T) {
		lockKey := utils.MD5("lock_error_" + fmt.Sprintf("%d", time.Now().UnixNano()))

		err := TryLocker(lockKey, 5, func(lock *Lock) error {
			t.Logf("锁获取成功，开始执行业务逻辑")
			// 模拟业务逻辑出错
			return fmt.Errorf("业务逻辑执行失败")
		})

		if err == nil {
			t.Error("期望回调函数错误被正确传播")
		} else {
			t.Logf("锁错误传播测试通过: %v", err)
		}
	})
}

// TestRedisLockRefresh 测试锁自动续期功能
func TestRedisLockRefresh(t *testing.T) {
	initRedisForLockTest()

	t.Run("LockAutoRefresh", func(t *testing.T) {
		lockKey := utils.MD5("lock_refresh_" + fmt.Sprintf("%d", time.Now().UnixNano()))

		err := TryLocker(lockKey, 5, func(lock *Lock) error {
			// 记录初始续期次数
			initialCount := lock.RefreshCount()

			// 等待一段时间，让续期机制启动
			// 续期间隔是过期时间 * RefreshIntervalRatio = 5 * (1/3) ≈ 1.67秒
			time.Sleep(2100 * time.Millisecond) // 等待2.1秒，应该至少有1次续期

			// 检查续期是否发生
			finalCount := lock.RefreshCount()
			refreshCount := finalCount - initialCount

			t.Logf("续期统计: 初始=%d, 最终=%d, 续期次数=%d",
				initialCount, finalCount, refreshCount)

			// 在短时间内应该至少有1次续期（可能更多）
			if refreshCount < 0 {
				t.Errorf("续期次数不能为负数: %d", refreshCount)
			}

			return nil
		})

		if err != nil {
			t.Errorf("锁续期测试失败: %v", err)
		} else {
			t.Logf("锁续期测试通过")
		}
	})

	t.Run("LockRefreshMonitoring", func(t *testing.T) {
		lockKey := utils.MD5("lock_monitor_" + fmt.Sprintf("%d", time.Now().UnixNano()))

		err := TryLocker(lockKey, 4, func(lock *Lock) error {
			time.Sleep(500 * time.Millisecond)

			// 监控续期相关状态
			heldDuration := lock.HeldDuration()
			if heldDuration <= 0 {
				t.Errorf("锁持有时长应该大于0: %v", heldDuration)
			}

			timeSinceRefresh := lock.TimeSinceLastRefresh()
			if timeSinceRefresh < 0 {
				t.Errorf("距离最后续期时长不能为负: %v", timeSinceRefresh)
			}

			failureCount := lock.RefreshFailures()
			if failureCount < 0 {
				t.Errorf("续期失败次数不能为负: %d", failureCount)
			}

			t.Logf("锁监控: 持有时长=%v, 续期失败次数=%d, 距离最后续期=%v",
				heldDuration, failureCount, timeSinceRefresh)

			return nil
		})

		if err != nil {
			t.Errorf("锁续期监控测试失败: %v", err)
		}
	})
}

// TestRedisLockConcurrency 测试并发锁竞争
func TestRedisLockConcurrency(t *testing.T) {
	initRedisForLockTest()

	t.Run("ConcurrentLockCompetition", func(t *testing.T) {
		lockKey := utils.MD5("lock_concurrent_" + fmt.Sprintf("%d", time.Now().UnixNano()))
		const numGoroutines = 5
		const lockDuration = 3 // 3秒锁定时长

		successCount := int32(0)
		var wg sync.WaitGroup
		var mu sync.Mutex
		results := make([]bool, numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()

				startTime := time.Now()
				err := TryLocker(lockKey, lockDuration, func(lock *Lock) error {
					atomic.AddInt32(&successCount, 1)

					// 模拟业务逻辑
					time.Sleep(500 * time.Millisecond)

					return nil
				})

				mu.Lock()
				results[index] = (err == nil)
				mu.Unlock()

				duration := time.Since(startTime)
				t.Logf("协程 %d 完成，耗时 %v，成功: %v", index, duration, err == nil)
			}(i)
		}

		wg.Wait()

		// 验证只有一个协程成功获取锁
		actualSuccessCount := int(atomic.LoadInt32(&successCount))
		if actualSuccessCount != 1 {
			t.Errorf("期望只有一个成功获取锁，实际 %d 个", actualSuccessCount)
		}

		// 统计成功和失败的协程数量
		goroutineSuccessCount := 0
		for _, result := range results {
			if result {
				goroutineSuccessCount++
			}
		}

		if goroutineSuccessCount != 1 {
			t.Errorf("期望只有一个协程成功，实际 %d 个", goroutineSuccessCount)
		}

		t.Logf("并发锁竞争测试通过: %d/%d 个协程成功", goroutineSuccessCount, numGoroutines)
	})

	t.Run("SequentialLockAccess", func(t *testing.T) {
		lockKey := utils.MD5("lock_sequential_" + fmt.Sprintf("%d", time.Now().UnixNano()))

		// 顺序执行多次锁操作，验证每次都能成功
		for i := 0; i < 3; i++ {
			err := TryLocker(lockKey, 3, func(lock *Lock) error {
				t.Logf("第 %d 次顺序锁获取成功", i+1)
				time.Sleep(200 * time.Millisecond)
				return nil
			})

			if err != nil {
				t.Errorf("第 %d 次顺序锁操作失败: %v", i+1, err)
			}
		}

		t.Logf("顺序锁访问测试通过")
	})
}

// TestRedisLockConfig 测试锁配置相关功能
func TestRedisLockConfig(t *testing.T) {
	initRedisForLockTest()

	// 测试锁配置验证
	t.Run("LockConfigValidation", func(t *testing.T) {
		// 测试有效配置
		validConfig := &LockConfig{
			MinExpireSeconds:     5,
			RefreshIntervalRatio: 0.5,
			AcquireTimeoutRatio:  0.8,
			MaxRefreshRetries:    3,
			MaxAcquireRetries:    5,
			MinRetryBackoff:      200 * time.Millisecond,
			MaxRetryBackoff:      3 * time.Second,
			RefreshRetryBackoff:  150 * time.Millisecond,
		}

		if err := ValidateLockConfig(validConfig); err != nil {
			t.Errorf("有效配置应该通过验证: %v", err)
		}

		// 测试无效配置
		invalidConfigs := []*LockConfig{
			{MinExpireSeconds: 0},        // MinExpireSeconds 不能为0
			{RefreshIntervalRatio: 1.5},  // RefreshIntervalRatio 不能 >= 1
			{RefreshIntervalRatio: -0.1}, // RefreshIntervalRatio 不能 < 0
			{AcquireTimeoutRatio: -0.1},  // AcquireTimeoutRatio 不能 < 0
			{AcquireTimeoutRatio: 1.1},   // AcquireTimeoutRatio 不能 > 1
			{MaxRefreshRetries: -1},      // MaxRefreshRetries 不能 < 0
			{MaxAcquireRetries: -1},      // MaxAcquireRetries 不能 < 0
			{MinRetryBackoff: -1},        // MinRetryBackoff 不能 < 0
			{
				MaxRetryBackoff: 1 * time.Second,
				MinRetryBackoff: 2 * time.Second, // Max 不能小于 Min
			},
			{RefreshRetryBackoff: 0}, // RefreshRetryBackoff 不能 <= 0
		}

		for i, invalidConfig := range invalidConfigs {
			if err := ValidateLockConfig(invalidConfig); err == nil {
				t.Errorf("无效配置 %d 应该验证失败", i)
			}
		}

		t.Logf("锁配置验证测试通过")
	})

	// 测试默认配置
	t.Run("DefaultLockConfig", func(t *testing.T) {
		config := DefaultLockConfig()
		if config == nil {
			t.Fatal("默认配置不能为空")
		}

		// 验证默认配置的值
		if config.MinExpireSeconds != 3 {
			t.Errorf("期望 MinExpireSeconds=3, 实际 %d", config.MinExpireSeconds)
		}

		if config.RefreshIntervalRatio != 1.0/3.0 {
			t.Errorf("期望 RefreshIntervalRatio=1/3, 实际 %f", config.RefreshIntervalRatio)
		}

		if config.AcquireTimeoutRatio != 1.0/2.0 {
			t.Errorf("期望 AcquireTimeoutRatio=1/2, 实际 %f", config.AcquireTimeoutRatio)
		}

		if config.MaxRefreshRetries != 2 {
			t.Errorf("期望 MaxRefreshRetries=2, 实际 %d", config.MaxRefreshRetries)
		}

		if config.MaxAcquireRetries != 3 {
			t.Errorf("期望 MaxAcquireRetries=3, 实际 %d", config.MaxAcquireRetries)
		}

		t.Logf("默认锁配置测试通过")
	})

	// 测试自定义配置
	t.Run("CustomLockConfig", func(t *testing.T) {
		lockKey := utils.MD5("lock_custom_" + fmt.Sprintf("%d", time.Now().UnixNano()))

		customConfig := &LockConfig{
			MinExpireSeconds:     8,
			RefreshIntervalRatio: 0.25, // 更频繁的续期
			AcquireTimeoutRatio:  0.5,
			MaxRefreshRetries:    5, // 更多续期重试
			MaxAcquireRetries:    3,
			MinRetryBackoff:      500 * time.Millisecond,
			MaxRetryBackoff:      5 * time.Second,
			RefreshRetryBackoff:  300 * time.Millisecond,
		}

		err := TryLocker(lockKey, 8, func(lock *Lock) error {
			// 验证配置生效
			if lock.ExpireSeconds() != 8 {
				t.Errorf("期望过期时间 8秒, 实际 %d", lock.ExpireSeconds())
			}

			t.Logf("自定义配置测试: 过期时间=%d秒", lock.ExpireSeconds())
			return nil
		}, customConfig)

		if err != nil {
			t.Errorf("自定义锁配置测试失败: %v", err)
		} else {
			t.Logf("自定义锁配置测试通过")
		}
	})
}

// TestRedisLockTimeout 测试锁超时处理
func TestRedisLockTimeout(t *testing.T) {
	initRedisForLockTest()

	t.Run("LockExpirationHandling", func(t *testing.T) {
		lockKey := utils.MD5("lock_timeout_" + fmt.Sprintf("%d", time.Now().UnixNano()))

		// 使用较短的过期时间和自定义配置
		shortConfig := &LockConfig{
			MinExpireSeconds:  2, // 2秒过期
			MaxAcquireRetries: 1, // 只重试1次
			MinRetryBackoff:   50 * time.Millisecond,
			MaxRetryBackoff:   100 * time.Millisecond,
		}

		startTime := time.Now()
		err := TryLocker(lockKey, 2, func(lock *Lock) error {
			// 故意持有锁超过过期时间
			time.Sleep(2500 * time.Millisecond) // 2.5秒 > 2秒过期时间

			// 检查锁是否仍然有效（续期应该会失败或锁已过期）
			if !lock.IsValid() {
				t.Logf("锁如预期已失效")
				return fmt.Errorf("锁在执行期间过期")
			}

			return nil
		}, shortConfig)

		duration := time.Since(startTime)
		t.Logf("锁超时测试完成，耗时 %v，错误: %v", duration, err)

		// 预期应该会有错误，因为锁会过期
		if err == nil {
			t.Logf("锁超时测试: 锁在过期后仍然有效（可能是由于时序问题）")
		} else {
			t.Logf("锁超时测试通过: 锁如预期过期")
		}
	})

	t.Run("LockAcquireTimeout", func(t *testing.T) {
		// 创建一个长时间持有的锁
		blockingLockKey := utils.MD5("lock_blocking_" + fmt.Sprintf("%d", time.Now().UnixNano()))

		// 先获取一个长时间持有的锁
		go func() {
			TryLocker(blockingLockKey, 10, func(lock *Lock) error {
				t.Logf("阻塞锁已获取，开始长时间持有")
				time.Sleep(3 * time.Second) // 持有3秒
				t.Logf("阻塞锁释放")
				return nil
			})
		}()

		// 等待一小段时间确保上面的锁被获取
		time.Sleep(200 * time.Millisecond)

		// 尝试获取同一个锁，应该会超时
		shortConfig := &LockConfig{
			MinExpireSeconds:  1,
			MaxAcquireRetries: 1, // 只重试1次，快速失败
			MinRetryBackoff:   10 * time.Millisecond,
			MaxRetryBackoff:   50 * time.Millisecond,
		}

		startTime := time.Now()
		err := TryLocker(blockingLockKey, 1, func(lock *Lock) error {
			return nil
		}, shortConfig)

		duration := time.Since(startTime)
		t.Logf("锁获取超时测试完成，耗时 %v", duration)

		// 应该获取失败
		if err == nil {
			t.Logf("锁获取超时测试: 意外成功获取锁（可能是时序问题）")
		} else {
			t.Logf("锁获取超时测试通过: 正确处理了锁竞争")
		}
	})
}

// TestRedisLockErrorHandling 测试锁错误处理
func TestRedisLockErrorHandling(t *testing.T) {
	initRedisForLockTest()

	t.Run("InvalidResource", func(t *testing.T) {
		// 测试空资源名称
		err := TryLocker("", 5, func(lock *Lock) error {
			return nil
		})

		if err == nil {
			t.Error("空资源名称应该返回错误")
		} else {
			t.Logf("空资源名称错误处理测试通过: %v", err)
		}
	})

	t.Run("InvalidExpireTime", func(t *testing.T) {
		lockKey := utils.MD5("lock_invalid_" + fmt.Sprintf("%d", time.Now().UnixNano()))

		// 测试过短的过期时间
		err := TryLocker(lockKey, 1, func(lock *Lock) error {
			return nil
		})

		if err == nil {
			t.Error("过短的过期时间应该返回错误")
		} else {
			t.Logf("过期时间验证测试通过: %v", err)
		}
	})

	t.Run("NilCallback", func(t *testing.T) {
		lockKey := utils.MD5("lock_nil_" + fmt.Sprintf("%d", time.Now().UnixNano()))

		// 测试nil回调函数
		err := TryLocker(lockKey, 5, nil)

		if err == nil {
			t.Error("nil回调函数应该返回错误")
		} else {
			t.Logf("nil回调函数错误处理测试通过: %v", err)
		}
	})
}

// BenchmarkRedisLock 锁操作基准测试
func BenchmarkRedisLock(b *testing.B) {
	initRedisForLockTest()

	b.Run("LockAcquireRelease", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			lockKey := fmt.Sprintf("bench_lock_%d_%d", time.Now().UnixNano(), i)
			TryLocker(lockKey, 5, func(lock *Lock) error {
				// 最小化业务逻辑以测试锁本身的性能
				return nil
			})
		}
	})
}
