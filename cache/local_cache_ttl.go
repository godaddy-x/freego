// Package cache 提供线程安全的 TTL（生存时间）缓存实现。
// 采用自适应采样清理策略，在大数据量场景下仍能保持高性能。
package cache

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/godaddy-x/freego/utils"
)

// entry 缓存条目的内部结构，包含值和过期信息。
type entry[V any] struct {
	value    V           // 缓存的值
	expireAt int64       // Unix 秒级时间戳，0 或负数表示永不过期
	deleted  atomic.Bool // true = 已逻辑删除，等待物理清理
}

// TTLCache 线程安全的内存缓存，支持 TTL 过期。
// 采用自适应采样清理策略，在保证清理效率的同时控制持锁时间。
type TTLCache[K comparable, V any] struct {
	data            map[K]*entry[V] // 底层存储 map
	initialCapacity int             // 初始容量，用于 Clear 时重建
	mu              sync.RWMutex    // 读写锁，保证并发安全
	cleanupStop     chan struct{}   // 停止清理协程的信号通道
	cleanupOnce     sync.Once       // 确保清理协程只被关闭一次

	// 清理配置参数
	interval      time.Duration // 清理间隔
	minSampleSize int           // 单次清理的最小采样量
	maxSampleSize int           // 单次清理的最大采样量（防止持锁过久）
	sampleRatio   float64       // 采样比例，例如 0.02 表示每次采样 2% 的数据

	// 统计信息
	stats CacheStats
}

// CacheStats 运行时统计信息。
// 所有计数器都是原子的，并发安全。
type CacheStats struct {
	sets    atomic.Int64 // Set 操作次数
	gets    atomic.Int64 // Get 操作次数
	hits    atomic.Int64 // 命中次数（返回有效值）
	misses  atomic.Int64 // 未命中次数（不存在、已过期或已删除）
	expired atomic.Int64 // Get 时发现过期并标记删除的次数
	removed atomic.Int64 // 物理删除的条目数（清理或覆盖）
}

// Option 配置函数类型，用于函数选项模式。
type Option[K comparable, V any] func(*TTLCache[K, V])

// WithCleanupInterval 设置清理间隔。
// interval 必须在 5 秒到 5 分钟之间。
func WithCleanupInterval[K comparable, V any](interval time.Duration) Option[K, V] {
	return func(c *TTLCache[K, V]) {
		if interval < 5*time.Second {
			interval = 5 * time.Second
		}
		if interval > 5*time.Minute {
			interval = 5 * time.Minute
		}
		c.interval = interval
	}
}

// WithSampleSize 设置单次清理的采样量范围。
// minSize 必须 > 0，maxSize 必须 >= minSize。
func WithSampleSize[K comparable, V any](minSize, maxSize int) Option[K, V] {
	return func(c *TTLCache[K, V]) {
		if minSize > 0 {
			c.minSampleSize = minSize
		}
		if maxSize > 0 && maxSize >= c.minSampleSize {
			c.maxSampleSize = maxSize
		}
	}
}

// WithSampleRatio 设置自适应采样比例。
// ratio 必须在 0.01（1%）到 0.10（10%）之间。
func WithSampleRatio[K comparable, V any](ratio float64) Option[K, V] {
	return func(c *TTLCache[K, V]) {
		if ratio >= 0.01 && ratio <= 0.10 {
			c.sampleRatio = ratio
		}
	}
}

// NewTTLCache 创建一个新的 TTLCache 实例。
// - initialCapacity: 预分配的 map 大小，减少 rehash 和 GC 压力
// - opts: 可选的配置函数
//
// 示例：
//
//	// 使用默认配置
//	cache := NewTTLCache[string, User](10000)
//
//	// 自定义配置
//	cache := NewTTLCache[string, User](10000,
//	    WithCleanupInterval[string, User](15*time.Second),
//	    WithSampleSize[string, User](200, 3000),
//	    WithSampleRatio[string, User](0.03),
//	)
func NewTTLCache[K comparable, V any](initialCapacity int, opts ...Option[K, V]) *TTLCache[K, V] {
	if initialCapacity <= 0 {
		initialCapacity = 1000
	}

	// 默认配置
	c := &TTLCache[K, V]{
		data:            make(map[K]*entry[V], initialCapacity),
		initialCapacity: initialCapacity,
		cleanupStop:     make(chan struct{}),
		interval:        30 * time.Second, // 默认 30 秒清理一次
		minSampleSize:   100,              // 默认最小采样 100 条
		maxSampleSize:   5000,             // 默认最大采样 5000 条
		sampleRatio:     0.02,             // 默认采样 2% 的数据
	}

	// 应用自定义配置
	for _, opt := range opts {
		opt(c)
	}

	// 启动后台清理协程
	go c.startCleanup()
	return c
}

// startCleanup 运行后台清理协程，定期删除已过期或标记删除的条目。
func (c *TTLCache[K, V]) startCleanup() {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.cleanupWithSampling()
		case <-c.cleanupStop:
			return
		}
	}
}

// cleanupWithSampling 执行自适应采样清理。
// 采样量根据当前缓存大小动态计算，保证覆盖率的同时控制持锁时间。
func (c *TTLCache[K, V]) cleanupWithSampling() {
	c.mu.Lock()
	defer c.mu.Unlock()

	size := len(c.data)
	if size == 0 {
		return
	}

	sampleSize := c.calculateSampleSize(size)

	scanned := 0
	now := utils.UnixSecond()

	for k, e := range c.data {
		if scanned >= sampleSize {
			break
		}

		// 检查是否需要删除
		if e.deleted.Load() || (e.expireAt > 0 && now >= e.expireAt) {
			delete(c.data, k)
			c.stats.removed.Add(1)
		}

		scanned++
	}
}

// calculateSampleSize 根据当前数据量计算采样大小。
// 公式：dataSize * sampleRatio，受 minSampleSize 和 maxSampleSize 约束。
func (c *TTLCache[K, V]) calculateSampleSize(dataSize int) int {
	targetSize := int(float64(dataSize) * c.sampleRatio)

	if targetSize < c.minSampleSize {
		targetSize = c.minSampleSize
	}
	if targetSize > c.maxSampleSize {
		targetSize = c.maxSampleSize
	}

	return targetSize
}

// Close 优雅关闭后台清理协程。
func (c *TTLCache[K, V]) Close() {
	c.cleanupOnce.Do(func() {
		close(c.cleanupStop)
	})
}

// Get 获取指定键的值。
// 使用读锁保证高并发读取性能。过期的条目会被标记，由后台清理协程统一删除。
//
// 统计信息：
//   - gets: 总是递增
//   - hits: 返回有效值时递增
//   - misses: 键不存在、已标记删除或已过期时递增
//   - expired: 发现过期并成功标记时递增
func (c *TTLCache[K, V]) Get(key K) (value V, ok bool) {
	c.stats.gets.Add(1)

	c.mu.RLock()
	defer c.mu.RUnlock()

	e, exists := c.data[key]
	if !exists {
		c.stats.misses.Add(1)
		return
	}

	// 检查是否已标记删除
	if e.deleted.Load() {
		c.stats.misses.Add(1)
		return
	}

	now := utils.UnixSecond()
	// 检查是否已过期
	if e.expireAt > 0 && now >= e.expireAt {
		// 原子标记删除，不获取写锁
		if e.deleted.CompareAndSwap(false, true) {
			c.stats.expired.Add(1)
		}
		c.stats.misses.Add(1)
		return
	}

	c.stats.hits.Add(1)
	return e.value, true
}

// Set 存储键值对，指定 TTL（秒）。
// - ttlSeconds > 0: 条目在指定秒数后过期
// - ttlSeconds <= 0: 条目永不过期
func (c *TTLCache[K, V]) Set(key K, value V, ttlSeconds int64) {
	c.stats.sets.Add(1)

	var expireAt int64
	if ttlSeconds > 0 {
		expireAt = utils.UnixSecond() + ttlSeconds
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// 如果覆盖了已过期或已删除的旧值，计入物理删除统计
	if old, exists := c.data[key]; exists {
		if old.deleted.Load() || (old.expireAt > 0 && utils.UnixSecond() >= old.expireAt) {
			c.stats.removed.Add(1)
		}
	}

	c.data[key] = &entry[V]{value: value, expireAt: expireAt}
}

// SetWithExpireAt 存储键值对，指定绝对过期时间戳（Unix 秒）。
// 适用于需要统一过期时间的场景（如每天午夜过期）。
func (c *TTLCache[K, V]) SetWithExpireAt(key K, value V, expireAt int64) {
	c.stats.sets.Add(1)

	c.mu.Lock()
	defer c.mu.Unlock()

	if old, exists := c.data[key]; exists {
		if old.deleted.Load() || (old.expireAt > 0 && utils.UnixSecond() >= old.expireAt) {
			c.stats.removed.Add(1)
		}
	}

	c.data[key] = &entry[V]{value: value, expireAt: expireAt}
}

// Delete 标记删除指定键（逻辑删除）。
// 条目将在下次清理周期中被物理删除。
func (c *TTLCache[K, V]) Delete(key K) {
	c.mu.RLock()
	e, exists := c.data[key]
	c.mu.RUnlock()

	if exists {
		e.deleted.Store(true)
	}
}

// DeleteIf 条件删除：仅当谓词函数返回 true 时才标记删除。
// 返回 true 表示成功标记删除。
func (c *TTLCache[K, V]) DeleteIf(key K, predicate func(V) bool) bool {
	c.mu.RLock()
	e, exists := c.data[key]
	c.mu.RUnlock()

	if !exists {
		return false
	}

	if predicate(e.value) {
		return e.deleted.CompareAndSwap(false, true)
	}
	return false
}

// GetOrSet 获取值，如果不存在/已过期/已删除则设置新值。
// 返回 (value, true) 表示值已存在且有效，(value, false) 表示新设置。
func (c *TTLCache[K, V]) GetOrSet(key K, value V, ttlSeconds int64) (V, bool) {
	// 先用读锁尝试获取
	c.mu.RLock()
	e, exists := c.data[key]
	c.mu.RUnlock()

	if exists && !e.deleted.Load() {
		now := utils.UnixSecond()
		if e.expireAt <= 0 || now < e.expireAt {
			return e.value, true
		}
		// 已过期，标记删除
		if e.deleted.CompareAndSwap(false, true) {
			c.stats.expired.Add(1)
		}
	}

	// 需要设置新值
	c.mu.Lock()
	defer c.mu.Unlock()

	// 双重检查
	if e, exists := c.data[key]; exists {
		// 检查是否已被标记删除
		if e.deleted.Load() {
			// ✅ 修复：旧值已被标记删除，计入 removed
			delete(c.data, key)
			c.stats.removed.Add(1)
		} else {
			now := utils.UnixSecond()
			if e.expireAt <= 0 || now < e.expireAt {
				// 值仍然有效，直接返回
				return e.value, true
			}
			// ✅ 值已过期，物理删除并计入统计
			delete(c.data, key)
			c.stats.removed.Add(1)
		}
	}

	var expireAt int64
	if ttlSeconds > 0 {
		expireAt = utils.UnixSecond() + ttlSeconds
	}

	c.data[key] = &entry[V]{value: value, expireAt: expireAt}
	c.stats.sets.Add(1)
	return value, false
}

// Range 遍历所有有效（未过期、未删除）的缓存条目。
// 遍历顺序为 map 的随机顺序。如果 f 返回 false 则停止遍历。
func (c *TTLCache[K, V]) Range(f func(K, V) bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	now := utils.UnixSecond()

	for k, e := range c.data {
		if e.deleted.Load() {
			continue
		}
		if e.expireAt > 0 && now >= e.expireAt {
			continue
		}
		if !f(k, e.value) {
			return
		}
	}
}

// RangeRaw 遍历所有物理存在的条目，包括已过期和已标记删除的。
// 适用于关闭时导出所有数据等场景。
func (c *TTLCache[K, V]) RangeRaw(f func(K, V)) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for k, e := range c.data {
		f(k, e.value)
	}
}

// Len 返回缓存中物理存在的条目总数。
// 包含已过期和已标记删除但尚未被清理的条目。
func (c *TTLCache[K, V]) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.data)
}

// ValidLen 返回有效（未过期、未删除）的条目数量。
// 注意：此方法需要遍历所有条目，时间复杂度 O(n)，应谨慎使用。
func (c *TTLCache[K, V]) ValidLen() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	now := utils.UnixSecond()
	count := 0
	for _, e := range c.data {
		if e.deleted.Load() {
			continue
		}
		if e.expireAt > 0 && now >= e.expireAt {
			continue
		}
		count++
	}
	return count
}

// Clear 清空缓存中的所有条目，同时保留初始容量。
func (c *TTLCache[K, V]) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data = make(map[K]*entry[V], c.initialCapacity)
}

// Resize 调整底层 map 的容量。
// 适用于预估数据量变化时提前扩容或缩容。
func (c *TTLCache[K, V]) Resize(newCapacity int) {
	if newCapacity <= 0 {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	newData := make(map[K]*entry[V], newCapacity)
	for k, v := range c.data {
		newData[k] = v
	}
	c.data = newData
	c.initialCapacity = newCapacity
}

// Stats 返回当前缓存的统计信息。
//
// 返回值：
//   - sets: Set 操作总次数
//   - gets: Get 操作总次数
//   - hits: 命中次数
//   - misses: 未命中次数
//   - expired: Get 时发现过期的次数
//   - removed: 物理删除的条目数
func (c *TTLCache[K, V]) Stats() (sets, gets, hits, misses, expired, removed int64) {
	return c.stats.sets.Load(),
		c.stats.gets.Load(),
		c.stats.hits.Load(),
		c.stats.misses.Load(),
		c.stats.expired.Load(),
		c.stats.removed.Load()
}

// HitRate 返回缓存命中率，范围 0 到 1。
// 如果没有 Get 操作，返回 0。
func (c *TTLCache[K, V]) HitRate() float64 {
	gets := c.stats.gets.Load()
	if gets == 0 {
		return 0
	}
	return float64(c.stats.hits.Load()) / float64(gets)
}

// Cap 返回缓存的初始容量提示。
func (c *TTLCache[K, V]) Cap() int {
	return c.initialCapacity
}

// GetScanRate 返回当前的扫描速率（每秒扫描的条目数）。
func (c *TTLCache[K, V]) GetScanRate() float64 {
	c.mu.RLock()
	dataSize := len(c.data)
	c.mu.RUnlock()

	sampleSize := c.calculateSampleSize(dataSize)
	return float64(sampleSize) / c.interval.Seconds()
}

// GetFullScanTime 返回预估的全量扫描时间。
func (c *TTLCache[K, V]) GetFullScanTime() time.Duration {
	c.mu.RLock()
	dataSize := len(c.data)
	c.mu.RUnlock()

	if dataSize == 0 {
		return 0
	}

	sampleSize := c.calculateSampleSize(dataSize)
	scanRate := float64(sampleSize) / c.interval.Seconds()

	if scanRate <= 0 {
		return 0
	}

	return time.Duration(float64(dataSize)/scanRate) * time.Second
}

// CleanupMetrics 清理性能指标。
type CleanupMetrics struct {
	DataSize     int     `json:"data_size"`      // 当前缓存大小
	SampleSize   int     `json:"sample_size"`    // 单次采样量
	ScanRate     float64 `json:"scan_rate"`      // 扫描速率（条/秒）
	FullScanTime string  `json:"full_scan_time"` // 预估全量扫描时间
	Interval     string  `json:"interval"`       // 清理间隔
	SampleRatio  float64 `json:"sample_ratio"`   // 采样比例
}

// GetCleanupMetrics 返回当前清理性能指标。
func (c *TTLCache[K, V]) GetCleanupMetrics() CleanupMetrics {
	c.mu.RLock()
	dataSize := len(c.data)
	c.mu.RUnlock()

	sampleSize := c.calculateSampleSize(dataSize)

	return CleanupMetrics{
		DataSize:     dataSize,
		SampleSize:   sampleSize,
		ScanRate:     float64(sampleSize) / c.interval.Seconds(),
		FullScanTime: c.GetFullScanTime().String(),
		Interval:     c.interval.String(),
		SampleRatio:  c.sampleRatio,
	}
}
