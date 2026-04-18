package cache

import (
	"fmt"
	"testing"
	"time"
)

type User struct {
	ID   int
	Name string
}

func TestLocalCacheTTL_BasicOperations(t *testing.T) {
	// 示例 1：使用默认配置
	cache1 := NewTTLCache[string, User](10000)
	fmt.Printf("Cache1: interval=%v, ratio=%.2f\n",
		cache1.GetCleanupMetrics().Interval,
		cache1.GetCleanupMetrics().SampleRatio)

	// 示例 2：只自定义清理间隔
	_ = NewTTLCache[string, User](10000,
		WithCleanupInterval[string, User](15*time.Second),
	)

	// 示例 3：完全自定义配置（无冲突）
	cache3 := NewTTLCache[string, User](10000,
		WithCleanupInterval[string, User](15*time.Second),
		WithSampleSize[string, User](200, 3000),
		WithSampleRatio[string, User](0.03),
	)

	// 使用缓存
	cache3.Set("user:1", User{ID: 1, Name: "Alice"}, 300)
	if user, ok := cache3.Get("user:1"); ok {
		fmt.Printf("Found: %v\n", user)
	}

	// 查看清理指标
	metrics := cache3.GetCleanupMetrics()
	fmt.Printf("Data size: %d\n", metrics.DataSize)
	fmt.Printf("Sample size: %d\n", metrics.SampleSize)
	fmt.Printf("Scan rate: %.2f/s\n", metrics.ScanRate)
	fmt.Printf("Full scan time: %s\n", metrics.FullScanTime)

	// 查看统计
	sets, gets, hits, misses, expired, removed := cache3.Stats()
	fmt.Printf("Sets: %d, Gets: %d, Hits: %d, Misses: %d\n", sets, gets, hits, misses)
	fmt.Printf("Expired: %d, Removed: %d\n", expired, removed)
	fmt.Printf("Hit rate: %.2f%%\n", cache3.HitRate()*100)

	// 关闭缓存
	defer cache3.Close()
}
