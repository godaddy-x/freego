package rate_test

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/godaddy-x/freego/cache"
	rate "github.com/godaddy-x/freego/cache/limiter"
	"github.com/godaddy-x/freego/utils"
)

// initRedis 初始化Redis配置
func initRedis() {
	conf := cache.RedisConfig{}
	// 尝试多个可能的路径
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
		panic(utils.AddStr("读取redis配置失败: ", err.Error()))
	}
	new(cache.RedisManager).InitConfig(conf)
}

// === Redis 限流器测试用例 ===

// TestRedisRateLimiterOperations 测试Redis限流器的基本操作
func TestRedisRateLimiterOperations(t *testing.T) {
	initRedis()
	// 跳过Redis不可用的环境
	limiter, err := rate.NewRedisRateLimiter(rate.Option{
		Limit:       10.0, // 每秒10个令牌
		Bucket:      5,    // 桶容量5
		Expire:      3000, // 3秒过期
		Distributed: true,
	})
	if err != nil {
		t.Skipf("Redis not available, skipping rate limiter test: %v", err)
		return
	}
	defer limiter.Close()

	resource := "test_api_endpoint"

	// 测试1: 初始状态下应该允许请求（桶是满的）
	t.Run("InitialAllowance", func(t *testing.T) {
		for i := 0; i < 5; i++ {
			if !limiter.Allow(resource) {
				t.Errorf("Request %d should be allowed (bucket capacity: 5)", i+1)
			}
		}
	})

	// 测试2: 超过桶容量后应该拒绝
	t.Run("ExceedCapacity", func(t *testing.T) {
		for i := 0; i < 3; i++ {
			if limiter.Allow(resource) {
				t.Errorf("Request %d should be denied (bucket empty)", i+1)
			}
		}
	})

	// 测试3: 等待令牌再生
	t.Run("TokenRegeneration", func(t *testing.T) {
		time.Sleep(1200 * time.Millisecond) // 等待1.2秒，至少应该生成10个令牌

		allowed := 0
		for i := 0; i < 10; i++ {
			if limiter.Allow(resource) {
				allowed++
			}
		}

		if allowed < 5 { // 至少应该允许5个请求
			t.Errorf("Expected at least 5 requests allowed after regeneration, got %d", allowed)
		}
	})
}

// TestRedisRateLimiterAllowN 测试批量令牌消耗
func TestRedisRateLimiterAllowN(t *testing.T) {
	initRedis()
	limiter, err := rate.NewRedisRateLimiter(rate.Option{
		Limit:       20.0,
		Bucket:      10,
		Expire:      3000,
		Distributed: true,
	})
	if err != nil {
		t.Skipf("Redis not available, skipping rate limiter test: %v", err)
		return
	}
	defer limiter.Close()

	resource := "test_batch_api"

	// 测试1: 消耗3个令牌
	t.Run("ConsumeMultipleTokens", func(t *testing.T) {
		if !limiter.AllowN(resource, 3) {
			t.Error("AllowN 3 should succeed")
		}

		// 再消耗5个令牌
		if !limiter.AllowN(resource, 5) {
			t.Error("AllowN 5 should succeed")
		}

		// 剩余2个令牌，消耗3个应该失败
		if limiter.AllowN(resource, 3) {
			t.Error("AllowN 3 should fail (insufficient tokens)")
		}

		// 消耗2个应该成功
		if !limiter.AllowN(resource, 2) {
			t.Error("AllowN 2 should succeed")
		}
	})

	// 测试2: 快速失败 - 请求超过桶容量
	t.Run("FastDenyLargeRequest", func(t *testing.T) {
		if limiter.AllowN(resource, 15) { // 请求15个，但桶只有10个容量
			t.Error("AllowN 15 should fail (exceeds bucket capacity)")
		}
	})

	// 测试3: 边界情况
	t.Run("EdgeCases", func(t *testing.T) {
		// 消耗0个令牌总是允许
		if !limiter.AllowN(resource, 0) {
			t.Error("AllowN 0 should always succeed")
		}

		// 消耗负数个令牌总是允许
		if !limiter.AllowN(resource, -1) {
			t.Error("AllowN negative should always succeed")
		}
	})
}

// TestRedisRateLimiterConcurrency 测试并发场景
func TestRedisRateLimiterConcurrency(t *testing.T) {
	initRedis()
	limiter, err := rate.NewRedisRateLimiter(rate.Option{
		Limit:       50.0, // 较高的速率以便测试
		Bucket:      20,
		Expire:      5000,
		Distributed: true,
	})
	if err != nil {
		t.Skipf("Redis not available, skipping rate limiter test: %v", err)
		return
	}
	defer limiter.Close()

	resource := "test_concurrent_api"

	// 启动10个goroutine并发请求
	numGoroutines := 10
	numRequestsPerGoroutine := 20
	totalRequests := numGoroutines * numRequestsPerGoroutine

	allowed := int64(0)
	denied := int64(0)

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(goroutineID int) {
			defer wg.Done()

			for j := 0; j < numRequestsPerGoroutine; j++ {
				if limiter.Allow(resource) {
					atomic.AddInt64(&allowed, 1)
				} else {
					atomic.AddInt64(&denied, 1)
				}

				// 小延迟模拟真实请求间隔
				time.Sleep(time.Millisecond)
			}
		}(i)
	}

	wg.Wait()

	totalProcessed := atomic.LoadInt64(&allowed) + atomic.LoadInt64(&denied)
	if totalProcessed != int64(totalRequests) {
		t.Errorf("Expected %d total requests, got %d", totalRequests, totalProcessed)
	}

	t.Logf("Concurrent test results: allowed=%d, denied=%d, total=%d",
		atomic.LoadInt64(&allowed), atomic.LoadInt64(&denied), totalProcessed)

	// 验证统计数据
	statsAllowed, statsDenied, fastDenied, _ := limiter.GetStats()
	if statsAllowed != atomic.LoadInt64(&allowed) {
		t.Errorf("Stats allowed mismatch: expected %d, got %d",
			atomic.LoadInt64(&allowed), statsAllowed)
	}
	if statsDenied != atomic.LoadInt64(&denied)+fastDenied {
		t.Errorf("Stats denied mismatch: expected %d, got %d (including %d fast denied)",
			atomic.LoadInt64(&denied)+fastDenied, statsDenied, fastDenied)
	}
}

// TestRedisRateLimiterStats 测试统计功能
func TestRedisRateLimiterStats(t *testing.T) {
	initRedis()
	limiter, err := rate.NewRedisRateLimiter(rate.Option{
		Limit:       100.0,
		Bucket:      10,
		Expire:      3000,
		Distributed: true,
	})
	if err != nil {
		t.Skipf("Redis not available, skipping rate limiter test: %v", err)
		return
	}
	defer limiter.Close()

	resource := "test_stats_api"

	// 初始状态验证
	allowed, denied, fastDenied, lastReset := limiter.GetStats()
	if allowed != 0 || denied != 0 || fastDenied != 0 {
		t.Errorf("Initial stats should be 0,0,0 got %d,%d,%d", allowed, denied, fastDenied)
	}
	if time.Since(lastReset) > time.Second {
		t.Error("Initial lastReset should be recent")
	}

	initialReset := lastReset

	// 执行一些请求 - 先快速消耗完桶容量
	for i := 0; i < 10; i++ { // 桶容量是10，确保消耗完
		limiter.Allow(resource)
	}

	// 验证统计更新 - 前10个应该都被允许
	allowed, denied, fastDenied, lastReset = limiter.GetStats()
	if allowed < 8 || allowed > 12 { // 允许一些误差
		t.Errorf("Expected around 10 allowed requests, got %d", allowed)
	}
	if denied != 0 {
		t.Errorf("Expected 0 denied requests initially, got %d", denied)
	}

	// 现在尝试更多请求，这些应该被拒绝
	for i := 0; i < 5; i++ {
		limiter.Allow(resource) // 这些应该被拒绝
	}

	// 再次验证统计更新
	allowed, denied, fastDenied, _ = limiter.GetStats()
	if denied < 3 { // 至少3个被拒绝
		t.Errorf("Expected at least 3 denied requests, got %d (allowed: %d, fastDenied: %d)", denied, allowed, fastDenied)
	}

	// 重置统计
	limiter.ResetStats()

	// 验证重置
	allowed, denied, fastDenied, newLastReset := limiter.GetStats()
	if allowed != 0 || denied != 0 || fastDenied != 0 {
		t.Errorf("Stats not reset properly: %d allowed, %d denied, %d fast denied",
			allowed, denied, fastDenied)
	}
	if !newLastReset.After(initialReset) {
		t.Error("LastReset not updated after reset")
	}
}

// TestRedisRateLimiterConfiguration 测试配置验证
func TestRedisRateLimiterConfiguration(t *testing.T) {
	// 测试有效配置
	t.Run("ValidConfiguration", func(t *testing.T) {
		initRedis()
		limiter, err := rate.NewRedisRateLimiter(rate.Option{
			Limit:       50.0,
			Bucket:      10,
			Expire:      5000,
			Distributed: true,
		})
		if err != nil {
			t.Skipf("Redis not available: %v", err)
			return
		}
		defer limiter.Close()

		// 验证配置
		bucket, limit, expire := limiter.GetConfig()
		if bucket != 10 {
			t.Errorf("Expected bucket 10, got %d", bucket)
		}
		if limit != 50.0 {
			t.Errorf("Expected limit 50.0, got %v", limit)
		}
		if expire != 5000 {
			t.Errorf("Expected expire 5000, got %d", expire)
		}
	})

	// 测试无效配置
	invalidConfigs := []struct {
		name string
		opt  rate.Option
	}{
		{"non-distributed", rate.Option{Limit: 10, Bucket: 5, Distributed: false}},
		{"zero bucket", rate.Option{Limit: 10, Bucket: 0, Distributed: true}},
		{"negative bucket", rate.Option{Limit: 10, Bucket: -1, Distributed: true}},
		{"zero limit", rate.Option{Limit: 0, Bucket: 5, Distributed: true}},
		{"negative limit", rate.Option{Limit: -1, Bucket: 5, Distributed: true}},
	}

	for _, tc := range invalidConfigs {
		t.Run("Invalid_"+tc.name, func(t *testing.T) {
			_, err := rate.NewRedisRateLimiter(tc.opt)
			if err == nil {
				t.Errorf("Expected error for %s", tc.name)
			}
		})
	}
}

// TestRedisRateLimiterGetRemaining 测试剩余令牌查询
func TestRedisRateLimiterGetRemaining(t *testing.T) {
	initRedis()
	limiter, err := rate.NewRedisRateLimiter(rate.Option{
		Limit:       20.0,
		Bucket:      10,
		Expire:      3000,
		Distributed: true,
	})
	if err != nil {
		t.Skipf("Redis not available, skipping rate limiter test: %v", err)
		return
	}
	defer limiter.Close()

	resource := "test_remaining_api"

	// 初始状态应该接近满桶
	remaining, err := limiter.GetRemaining(resource)
	if err != nil {
		t.Fatalf("GetRemaining failed: %v", err)
	}
	if remaining < 9.5 || remaining > 10.5 { // 允许小幅误差
		t.Errorf("Expected ~10 remaining tokens initially, got %v", remaining)
	}

	// 消耗一些令牌
	limiter.AllowN(resource, 3)
	limiter.Allow(resource) // 再消耗1个

	// 验证剩余令牌减少
	remaining, err = limiter.GetRemaining(resource)
	if err != nil {
		t.Fatalf("GetRemaining after consumption failed: %v", err)
	}
	if remaining < 5.5 || remaining > 6.5 { // 应该剩下6个左右
		t.Errorf("Expected ~6 remaining tokens after consumption, got %v", remaining)
	}

	// 等待令牌再生
	time.Sleep(1000 * time.Millisecond) // 等待1秒，令牌生成速率20/秒，应该生成约20个令牌

	remaining, err = limiter.GetRemaining(resource)
	if err != nil {
		t.Fatalf("GetRemaining after regeneration failed: %v", err)
	}
	// 检查是否有令牌再生（应该比初始的6个多）
	if remaining <= 6.0 {
		t.Errorf("Expected more than 6 remaining tokens after regeneration, got %.1f", remaining)
	}
	// 应该不超过桶容量10
	if remaining > 10.0 {
		t.Errorf("Expected at most 10 remaining tokens (bucket capacity), got %.1f", remaining)
	}
}

// BenchmarkRedisRateLimiter 限流器基准测试
func BenchmarkRedisRateLimiter(b *testing.B) {
	initRedis()
	limiter, err := rate.NewRedisRateLimiter(rate.Option{
		Limit:       1000.0, // 高频限制
		Bucket:      100,    // 大容量
		Expire:      60000,  // 1分钟
		Distributed: true,
	})
	if err != nil {
		b.Skip("Redis not available, skipping benchmark:", err)
		return
	}
	defer limiter.Close()

	resource := "bench_limiter"

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		localResource := resource + "_parallel"
		for pb.Next() {
			limiter.Allow(localResource)
		}
	})
}
