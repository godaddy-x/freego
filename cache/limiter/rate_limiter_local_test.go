package rate_test

import (
	"testing"
	"time"

	rate "github.com/godaddy-x/freego/cache/limiter"
)

// TestLocalRateLimiterOperations 测试本地限流器的基本操作
func TestLocalRateLimiterOperations(t *testing.T) {
	// 测试有效配置 - 使用较低的速率和较小的桶便于精确测试
	limiter := rate.NewRateLimiter(rate.Option{
		Limit:       1.0,    // 每秒1个令牌
		Bucket:      2,      // 桶容量2
		Expire:      300000, // 5分钟过期（毫秒）
		Distributed: false,
	})

	resource := "test_api_operations"

	// 测试1: 初始状态应该允许请求（桶是满的）
	allowed := 0
	for i := 0; i < 2; i++ {
		if limiter.Allow(resource) {
			allowed++
		}
	}
	if allowed != 2 {
		t.Errorf("Expected 2 requests allowed initially, got %d", allowed)
	}

	// 测试2: 超过桶容量后应该拒绝
	denied := 0
	for i := 0; i < 3; i++ {
		if !limiter.Allow(resource) {
			denied++
		}
	}
	if denied != 3 {
		t.Errorf("Expected 3 requests denied, got %d", denied)
	}

	// 测试3: 等待令牌再生 - 等待1秒，应该生成1个令牌
	time.Sleep(1100 * time.Millisecond)

	// 现在应该可以再允许1个请求
	if !limiter.Allow(resource) {
		t.Error("Expected request allowed after token regeneration")
	}

	// 再次请求应该被拒绝
	if limiter.Allow(resource) {
		t.Error("Expected request denied after using regenerated token")
	}
}

// TestLocalRateLimiterInvalidConfig 测试无效配置的处理
func TestLocalRateLimiterInvalidConfig(t *testing.T) {
	testCases := []struct {
		name   string
		option rate.Option
	}{
		{
			name: "ZeroLimit",
			option: rate.Option{
				Limit:       0,
				Bucket:      5,
				Expire:      30000, // 30秒（毫秒）
				Distributed: false,
			},
		},
		{
			name: "NegativeLimit",
			option: rate.Option{
				Limit:       -1,
				Bucket:      5,
				Expire:      30000, // 30秒（毫秒）
				Distributed: false,
			},
		},
		{
			name: "ZeroBucket",
			option: rate.Option{
				Limit:       2.0,
				Bucket:      0,
				Expire:      30000, // 30秒（毫秒）
				Distributed: false,
			},
		},
		{
			name: "NegativeBucket",
			option: rate.Option{
				Limit:       2.0,
				Bucket:      -1,
				Expire:      30000, // 30秒（毫秒）
				Distributed: false,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			limiter := rate.NewRateLimiter(tc.option)

			// 验证限流器能正常工作（配置应该被自动修正）
			localLimiter, ok := limiter.(*rate.LocalRateLimiter)
			if !ok {
				t.Skip("Not a local limiter")
				return
			}

			// 验证限流器能正常处理请求
			result := localLimiter.Allow("test")
			if !result {
				t.Error("Limiter should work even with invalid config (auto-corrected)")
			}
		})
	}
}

// TestLocalRateLimiterResourceIsolation 测试资源隔离
func TestLocalRateLimiterResourceIsolation(t *testing.T) {
	limiter := rate.NewRateLimiter(rate.Option{
		Limit:       1.0,    // 每秒1个令牌
		Bucket:      2,      // 桶容量2
		Expire:      300000, // 5分钟过期（毫秒）
		Distributed: false,
	})

	resource1 := "api_v1_isolation"
	resource2 := "api_v2_isolation"

	// 消耗resource1的所有令牌
	allowed1 := 0
	for i := 0; i < 2; i++ {
		if limiter.Allow(resource1) {
			allowed1++
		}
	}
	if allowed1 != 2 {
		t.Errorf("Expected 2 requests allowed for resource1 initially, got %d", allowed1)
	}

	// resource1应该被限流
	if limiter.Allow(resource1) {
		t.Error("Resource1 should be rate limited")
	}

	// 验证resource1确实被限流了
	if limiter.Allow(resource1) {
		t.Error("Resource1 should still be rate limited")
	}

	// resource2应该不受影响
	allowed2 := 0
	for i := 0; i < 2; i++ {
		if limiter.Allow(resource2) {
			allowed2++
		}
	}
	if allowed2 != 2 {
		t.Errorf("Expected 2 requests allowed for resource2 (isolated), got %d", allowed2)
	}
}

// TestLocalRateLimiterCacheErrorHandling 测试缓存错误处理
// 注意：现在的行为是缓存失败时返回false，不再使用备用存储
func TestLocalRateLimiterCacheErrorHandling(t *testing.T) {
	limiter := rate.NewRateLimiter(rate.Option{
		Limit:       10.0,
		Bucket:      3,
		Expire:      30000, // 30秒（毫秒）
		Distributed: false,
	})

	resource := "test_cache_error"

	// 验证基本功能正常
	allowed := 0
	for i := 0; i < 3; i++ {
		if limiter.Allow(resource) {
			allowed++
		}
	}
	if allowed != 3 {
		t.Errorf("Expected 3 requests allowed initially, got %d", allowed)
	}

	// 验证正常限流行为
	// 继续请求应该被拒绝（超出桶容量）
	for i := 0; i < 2; i++ {
		if limiter.Allow(resource) {
			t.Errorf("Request %d should be denied (bucket empty)", i+1)
		}
	}

	// 验证新资源也能正常工作
	resource2 := "test_cache_error_2"
	allowed2 := 0
	for i := 0; i < 3; i++ {
		if limiter.Allow(resource2) {
			allowed2++
		}
	}
	if allowed2 != 3 {
		t.Errorf("Expected 3 requests allowed for resource2, got %d", allowed2)
	}
}

// TestLocalRateLimiterEmptyResource 测试空资源名
func TestLocalRateLimiterEmptyResource(t *testing.T) {
	limiter := rate.NewRateLimiter(rate.Option{
		Limit:       2.0,
		Bucket:      3,
		Expire:      30000, // 30秒（毫秒）
		Distributed: false,
	})

	// 空资源名应该返回false
	if limiter.Allow("") {
		t.Error("Empty resource should return false")
	}
}

// TestLocalRateLimiterExpireConversion 测试过期时间转换
func TestLocalRateLimiterExpireConversion(t *testing.T) {
	testCases := []struct {
		name        string
		expireInput int
		description string
	}{
		{"NormalExpire", 30000, "30秒（毫秒）"},
		{"ZeroExpire", 0, "默认5分钟"},
		{"LongExpire", 600000, "10分钟（毫秒）"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			limiter := rate.NewRateLimiter(rate.Option{
				Limit:       2.0,
				Bucket:      2,
				Expire:      tc.expireInput,
				Distributed: false,
			})

			localLimiter, ok := limiter.(*rate.LocalRateLimiter)
			if !ok {
				t.Skip("Not a local limiter")
				return
			}

			// 验证限流器能正常工作（过期时间转换应该在内部处理）
			result := localLimiter.Allow("test_expire_" + tc.name)
			if !result {
				t.Errorf("Limiter should work with expire %s", tc.description)
			}
		})
	}
}
