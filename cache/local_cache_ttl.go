// Package cache 提供线程安全、高性能、自适应采样的 TTL 内存缓存。
// 采用逻辑分段环形队列清理策略，实现 O(1) 恒定开销，海量数据下依然保持极低延迟。

package cache

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/godaddy-x/freego/utils"
)

// entry 缓存条目内部结构，存储值与过期状态
type entry[V any] struct {
	value    V           // 缓存值
	expireAt int64       // Unix 时间戳（秒），0 表示永不过期
	deleted  atomic.Bool // 原子标记：true = 已逻辑删除
}

// TTLCache 线程安全、自适应采样的 TTL 缓存
type TTLCache[K comparable, V any] struct {
	data            map[K]*entry[V] // 底层缓存存储
	initialCapacity int             // 初始化容量
	mu              sync.RWMutex    // 读写锁

	// 逻辑分段环形队列
	cleanupRing  []K  // 环形队列，只存 key
	ringCap      int  // 环形队列容量
	ringWriteIdx int  // 写入位置
	ringReadIdx  int  // 读取位置
	ringFull     bool // 队列是否已满

	cleanupStop chan struct{} // 停止后台清理协程
	cleanupOnce sync.Once     // 确保只关闭一次

	// 自适应清理配置（内部自动计算）
	interval     time.Duration // 后台清理周期
	batchSize    int           // 基础批量大小
	maxBatchSize int           // 最大批量大小
}

// Config 高级配置选项（仅用于特殊场景）
type Config[K comparable, V any] struct {
	// 自定义清理间隔（可选）
	CleanupInterval time.Duration
	// 自定义环形队列容量（可选，0 表示自动计算）
	RingCapacity int
	// 自定义基础批量大小（可选，0 表示自动计算）
	BatchSize int
	// 自定义最大批量大小（可选，0 表示自动计算）
	MaxBatchSize int
}

// NewTTLCache 创建一个 TTL 缓存实例
// initialCapacity: 预估的缓存条目数量，用于预分配内存和计算清理参数
func NewTTLCache[K comparable, V any](initialCapacity int) *TTLCache[K, V] {
	return NewTTLCacheWithConfig[K, V](initialCapacity, nil)
}

// NewTTLCacheWithConfig 使用自定义配置创建缓存（一般不需要，仅用于特殊调优）
func NewTTLCacheWithConfig[K comparable, V any](initialCapacity int, config *Config[K, V]) *TTLCache[K, V] {
	if initialCapacity <= 0 {
		initialCapacity = 1000
	}

	// 自动计算最优参数
	ringCap := calcRingCap(initialCapacity)
	interval := 2 * time.Second
	batchSize := calcBatchSize(initialCapacity)
	maxBatchSize := batchSize * 5

	// 应用自定义配置（如果提供）
	if config != nil {
		if config.CleanupInterval > 0 {
			interval = config.CleanupInterval
		}
		if config.RingCapacity > 0 {
			ringCap = config.RingCapacity
		}
		if config.BatchSize > 0 {
			batchSize = config.BatchSize
		}
		if config.MaxBatchSize > 0 {
			maxBatchSize = config.MaxBatchSize
		}
	}

	c := &TTLCache[K, V]{
		data:            make(map[K]*entry[V], initialCapacity),
		initialCapacity: initialCapacity,
		cleanupRing:     make([]K, ringCap),
		ringCap:         ringCap,
		cleanupStop:     make(chan struct{}),
		interval:        interval,
		batchSize:       batchSize,
		maxBatchSize:    maxBatchSize,
	}

	if c.maxBatchSize < c.batchSize {
		c.maxBatchSize = c.batchSize * 5
	}

	go c.startCleanup()
	return c
}

// calcRingCap 根据预期容量自动计算环形队列容量
func calcRingCap(capacity int) int {
	// 环形队列容量约为容量的 1/5，限制在 1000 ~ 200000 之间
	ringCap := capacity / 5
	if ringCap < 1000 {
		ringCap = 1000
	}
	if ringCap > 200000 {
		ringCap = 200000
	}
	return ringCap
}

// calcBatchSize 根据预期容量自动计算基础批量大小
func calcBatchSize(capacity int) int {
	// 批量大小约为容量的 1/500，限制在 100 ~ 500 之间
	batchSize := capacity / 500
	if batchSize < 100 {
		batchSize = 100
	}
	if batchSize > 500 {
		batchSize = 500
	}
	return batchSize
}

// startCleanup 后台定时清理任务
func (c *TTLCache[K, V]) startCleanup() {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.cleanupWithRing()
		case <-c.cleanupStop:
			return
		}
	}
}

// cleanupWithRing 使用环形队列清理（动态批量优化）
func (c *TTLCache[K, V]) cleanupWithRing() {
	c.mu.Lock()
	defer c.mu.Unlock()

	pending := c.pendingCount()
	if pending == 0 {
		return
	}

	batchSize := c.dynamicBatchSize(pending)
	now := timeNow()
	scanned := 0

	for scanned < batchSize && pending > 0 {
		key := c.cleanupRing[c.ringReadIdx]

		if e, exists := c.data[key]; exists {
			if e.deleted.Load() || (e.expireAt > 0 && now >= e.expireAt) {
				delete(c.data, key)
			}
		}

		c.ringReadIdx++
		if c.ringReadIdx >= c.ringCap {
			c.ringReadIdx = 0
			c.ringFull = false
		}

		scanned++
		pending--

		if c.ringReadIdx == c.ringWriteIdx && !c.ringFull {
			break
		}
	}

	if c.ringReadIdx == c.ringWriteIdx && !c.ringFull {
		c.ringReadIdx = 0
		c.ringWriteIdx = 0
	}
}

// pendingCount 计算待处理数量
func (c *TTLCache[K, V]) pendingCount() int {
	if c.ringWriteIdx == c.ringReadIdx {
		if c.ringFull {
			return c.ringCap
		}
		return 0
	}

	if c.ringWriteIdx > c.ringReadIdx {
		return c.ringWriteIdx - c.ringReadIdx
	}

	return c.ringCap - c.ringReadIdx + c.ringWriteIdx
}

// dynamicBatchSize 动态计算批量大小
func (c *TTLCache[K, V]) dynamicBatchSize(pending int) int {
	batchSize := c.batchSize

	if c.ringFull {
		batchSize = c.batchSize * 3
	}

	fillRatio := float64(pending) / float64(c.ringCap)
	if fillRatio > 0.5 {
		multiplier := 1.0 + (fillRatio-0.5)*4
		batchSize = int(float64(c.batchSize) * multiplier)
	}

	if batchSize > c.maxBatchSize {
		batchSize = c.maxBatchSize
	}
	if batchSize > pending {
		batchSize = pending
	}
	if batchSize < 1 {
		batchSize = 1
	}

	return batchSize
}

// pushToRing 将 key 推入环形队列
func (c *TTLCache[K, V]) pushToRing(key K) {
	c.cleanupRing[c.ringWriteIdx] = key

	c.ringWriteIdx++
	if c.ringWriteIdx >= c.ringCap {
		c.ringWriteIdx = 0
		c.ringFull = true
	}

	if c.ringFull && c.ringWriteIdx == c.ringReadIdx {
		c.ringReadIdx++
		if c.ringReadIdx >= c.ringCap {
			c.ringReadIdx = 0
		}
	}
}

// Close 优雅关闭缓存
func (c *TTLCache[K, V]) Close() {
	c.cleanupOnce.Do(func() {
		close(c.cleanupStop)
	})
}

// Get 获取缓存条目
func (c *TTLCache[K, V]) Get(key K) (value V, ok bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	e, exists := c.data[key]
	if !exists || e.deleted.Load() {
		return
	}

	now := timeNow()
	if e.expireAt > 0 && now >= e.expireAt {
		e.deleted.CompareAndSwap(false, true)
		return
	}

	return e.value, true
}

// GetWithTTL 获取值及剩余 TTL
func (c *TTLCache[K, V]) GetWithTTL(key K) (value V, remainingTTL int64, ok bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	e, exists := c.data[key]
	if !exists || e.deleted.Load() {
		return
	}

	now := timeNow()
	if e.expireAt > 0 && now >= e.expireAt {
		return
	}

	if e.expireAt > 0 {
		remainingTTL = e.expireAt - now
		if remainingTTL < 0 {
			remainingTTL = 0
		}
	}
	return e.value, remainingTTL, true
}

// Set 插入/更新缓存
func (c *TTLCache[K, V]) Set(key K, value V, ttlSeconds int64) {
	var expireAt int64
	if ttlSeconds > 0 {
		expireAt = timeNow() + ttlSeconds
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.data[key]; !exists {
		c.pushToRing(key)
	}

	c.data[key] = &entry[V]{value: value, expireAt: expireAt}
}

// SetWithExpireAt 使用绝对时间戳设置
func (c *TTLCache[K, V]) SetWithExpireAt(key K, value V, expireAt int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.data[key]; !exists {
		c.pushToRing(key)
	}

	c.data[key] = &entry[V]{value: value, expireAt: expireAt}
}

// Delete 逻辑删除
func (c *TTLCache[K, V]) Delete(key K) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if e, exists := c.data[key]; exists {
		e.deleted.Store(true)
	}
}

// DeleteIf 条件删除
func (c *TTLCache[K, V]) DeleteIf(key K, predicate func(V) bool) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	e, exists := c.data[key]
	if !exists || e.deleted.Load() {
		return false
	}

	now := timeNow()
	if e.expireAt > 0 && now >= e.expireAt {
		return false
	}

	if predicate(e.value) {
		return e.deleted.CompareAndSwap(false, true)
	}
	return false
}

// GetOrSet 原子操作
func (c *TTLCache[K, V]) GetOrSet(key K, value V, ttlSeconds int64) (V, bool) {
	c.mu.RLock()
	e, exists := c.data[key]
	if exists && !e.deleted.Load() {
		now := timeNow()
		if e.expireAt <= 0 || now < e.expireAt {
			c.mu.RUnlock()
			return e.value, true
		}
		e.deleted.CompareAndSwap(false, true)
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	if e, exists := c.data[key]; exists {
		if e.deleted.Load() {
			delete(c.data, key)
		} else {
			now := timeNow()
			if e.expireAt > 0 && now >= e.expireAt {
				delete(c.data, key)
			} else {
				return e.value, true
			}
		}
	}

	var expireAt int64
	if ttlSeconds > 0 {
		expireAt = timeNow() + ttlSeconds
	}

	c.pushToRing(key)
	c.data[key] = &entry[V]{value: value, expireAt: expireAt}
	return value, false
}

// Refresh 刷新过期时间
func (c *TTLCache[K, V]) Refresh(key K, ttlSeconds int64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	e, exists := c.data[key]
	if !exists || e.deleted.Load() {
		return false
	}

	now := timeNow()
	if e.expireAt > 0 && now >= e.expireAt {
		e.deleted.Store(true)
		return false
	}

	if ttlSeconds > 0 {
		e.expireAt = now + ttlSeconds
	} else {
		e.expireAt = 0
	}
	return true
}

// Range 遍历有效条目
func (c *TTLCache[K, V]) Range(f func(K, V) bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	now := timeNow()
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

// Len 返回物理存储条目数
func (c *TTLCache[K, V]) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.data)
}

// Clear 清空缓存
func (c *TTLCache[K, V]) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.data = make(map[K]*entry[V], c.initialCapacity)
	c.ringWriteIdx = 0
	c.ringReadIdx = 0
	c.ringFull = false

	var zero K
	for i := range c.cleanupRing {
		c.cleanupRing[i] = zero
	}
}

// timeNow 获取当前时间戳
func timeNow() int64 {
	return utils.UnixSecond()
}
