package cache

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/godaddy-x/freego/utils"
)

// entry 缓存条目内部结构，存储值与过期状态
type entry[V any] struct {
	value    V
	expireAt int64       // Unix 时间戳（秒），0 表示永不过期
	deleted  atomic.Bool // 原子标记：true = 已逻辑删除
}

// TTLCache 线程安全、自适应采样的 TTL 缓存
type TTLCache[K comparable, V any] struct {
	data            map[K]*entry[V] // 底层缓存存储
	initialCapacity int             // 初始化容量
	mu              sync.RWMutex    // 读写锁

	// 逻辑分段环形队列
	cleanupRing  []K           // 环形队列，只存 key
	ringCap      int           // 环形队列容量
	ringWriteIdx int           // 写入位置
	ringReadIdx  int           // 读取位置
	ringFull     bool          // 队列是否已满
	cleanupStop  chan struct{} // 停止后台清理协程
	cleanupOnce  sync.Once     // 确保只关闭一次

	// 自适应清理配置（内部自动计算）
	interval     time.Duration // 后台清理周期
	batchSize    int           // 基础批量大小
	maxBatchSize int           // 最大批量大小

	// onCleanup 在单次 ring 清理完成后调用（持锁外调用，可安全调用 Len/Get 等）
	onCleanup func(CleanupReport)

	// 条件触发全量扫（复用现有 cleanup tick，不新增额外定时器）
	fullScanMinMapLen        int
	fullScanNoProgressRounds int
	fullScanCooldown         time.Duration
	noPurgeRounds            int
	lastFullScanAt           time.Time
}

// CleanupReport 单次后台 cleanupWithRing 的统计信息（用于观测 / 测试）
type CleanupReport struct {
	Time           time.Time // 回调触发时刻（清理已结束）
	PendingRing    int       // 本轮开始时 ring 中待扫描槽位数
	BatchAllocated int       // 本轮允许扫描的上限（dynamicBatchSize）
	Scanned        int       // 本轮实际扫描的槽位数
	Purged         int       // 本轮从 map 中物理删除的条目数
	MapLen         int       // 本轮结束后 map 条目数（含墓碑，直至被扫掉）
	FullScan       bool      // 本轮是否触发条件全量扫
	FullScanned    int       // 全量扫扫描条目数（未触发为 0）
	FullPurged     int       // 全量扫删除条目数（未触发为 0）
}

// Config 高级配置选项（仅用于特殊场景）
type Config[K comparable, V any] struct {
	CleanupInterval          time.Duration
	RingCapacity             int
	BatchSize                int
	MaxBatchSize             int
	OnCleanup                func(CleanupReport)
	FullScanMinMapLen        int           // map 条目至少达到该值才允许触发条件全量扫（默认 10000）
	FullScanNoProgressRounds int           // 连续无 purged 的轮次数达到该值后触发（默认 8）
	FullScanCooldown         time.Duration // 两次全量扫之间最小间隔（默认 30s）
}

// NewTTLCache 创建一个 TTL 缓存实例
func NewTTLCache[K comparable, V any](initialCapacity int) *TTLCache[K, V] {
	return NewTTLCacheWithConfig[K, V](initialCapacity, nil)
}

// NewTTLCacheWithConfig 使用自定义配置创建缓存
func NewTTLCacheWithConfig[K comparable, V any](initialCapacity int, config *Config[K, V]) *TTLCache[K, V] {
	if initialCapacity <= 0 {
		initialCapacity = 1000
	}
	ringCap := calcRingCap(initialCapacity)
	interval := 2 * time.Second
	batchSize := calcBatchSize(initialCapacity)
	maxBatchSize := batchSize * 5

	var onCleanup func(CleanupReport)
	fullScanMinMapLen := 10000
	fullScanNoProgressRounds := 8
	fullScanCooldown := 30 * time.Second

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
		onCleanup = config.OnCleanup
		if config.FullScanMinMapLen > 0 {
			fullScanMinMapLen = config.FullScanMinMapLen
		}
		if config.FullScanNoProgressRounds > 0 {
			fullScanNoProgressRounds = config.FullScanNoProgressRounds
		}
		if config.FullScanCooldown > 0 {
			fullScanCooldown = config.FullScanCooldown
		}
	}

	c := &TTLCache[K, V]{
		data:                     make(map[K]*entry[V], initialCapacity),
		initialCapacity:          initialCapacity,
		cleanupRing:              make([]K, ringCap),
		ringCap:                  ringCap,
		cleanupStop:              make(chan struct{}),
		interval:                 interval,
		batchSize:                batchSize,
		maxBatchSize:             maxBatchSize,
		onCleanup:                onCleanup,
		fullScanMinMapLen:        fullScanMinMapLen,
		fullScanNoProgressRounds: fullScanNoProgressRounds,
		fullScanCooldown:         fullScanCooldown,
	}

	if c.maxBatchSize < c.batchSize {
		c.maxBatchSize = c.batchSize * 5
	}

	go c.startCleanup()
	return c
}

func calcRingCap(capacity int) int {
	ringCap := capacity / 5
	if ringCap < 1000 {
		ringCap = 1000
	}
	if ringCap > 200000 {
		ringCap = 200000
	}
	return ringCap
}

func calcBatchSize(capacity int) int {
	batchSize := capacity / 500
	if batchSize < 100 {
		batchSize = 100
	}
	if batchSize > 500 {
		batchSize = 500
	}
	return batchSize
}

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

func (c *TTLCache[K, V]) cleanupWithRing() {
	c.mu.Lock()

	hasPending := !(c.ringReadIdx == c.ringWriteIdx && !c.ringFull)
	pendingStart := 0
	batchSize := 0
	now := timeNow()
	scanned := 0
	purged := 0
	fullScanTriggered := false
	fullScanned := 0
	fullPurged := 0

	if hasPending {
		pendingStart = c.pendingCount()
		batchSize = c.dynamicBatchSize(pendingStart)

		for scanned < batchSize {
			// 检查是否已处理完所有项
			if c.ringReadIdx == c.ringWriteIdx && !c.ringFull {
				break
			}

			key := c.cleanupRing[c.ringReadIdx]
			if e, exists := c.data[key]; exists {
				// 检查是否被标记删除或已过期
				if e.deleted.Load() || (e.expireAt > 0 && now >= e.expireAt) {
					delete(c.data, key)
					purged++
				}
			}

			// 清空 Ring 槽位并移动读指针
			var zero K
			c.cleanupRing[c.ringReadIdx] = zero
			c.ringReadIdx++
			if c.ringReadIdx >= c.ringCap {
				c.ringReadIdx = 0
			}
			c.ringFull = false // 只要读指针动了，肯定就不满了
			scanned++
		}

		// 如果读追上写，重置指针（优化）
		if c.ringReadIdx == c.ringWriteIdx && !c.ringFull {
			c.ringReadIdx = 0
			c.ringWriteIdx = 0
		}
	}

	mapLen := len(c.data)
	ringIsEmpty := (c.ringReadIdx == c.ringWriteIdx && !c.ringFull)
	if purged > 0 {
		c.noPurgeRounds = 0
	} else if ringIsEmpty && mapLen > c.fullScanMinMapLen {
		// 只有 ring 已无待处理项且 map 仍较大时，才视为“无进展”累加轮次。
		c.noPurgeRounds++
	} else {
		c.noPurgeRounds = 0
	}

	// 条件触发全量扫描
	if c.shouldTriggerFullScanLocked(mapLen, now) {
		fullScanTriggered = true
		fullScanned, fullPurged = c.fullScanLocked(now)
		mapLen = len(c.data)
		c.lastFullScanAt = time.Now()
		c.noPurgeRounds = 0
	}

	cb := c.onCleanup
	c.mu.Unlock()

	if cb != nil && (hasPending || fullScanTriggered) {
		cb(CleanupReport{
			Time:           time.Now(),
			PendingRing:    pendingStart,
			BatchAllocated: batchSize,
			Scanned:        scanned,
			Purged:         purged,
			MapLen:         mapLen,
			FullScan:       fullScanTriggered,
			FullScanned:    fullScanned,
			FullPurged:     fullPurged,
		})
	}
}

func (c *TTLCache[K, V]) shouldTriggerFullScanLocked(mapLen int, now int64) bool {
	if mapLen < c.fullScanMinMapLen {
		return false
	}
	if c.noPurgeRounds < c.fullScanNoProgressRounds {
		return false
	}
	if !c.lastFullScanAt.IsZero() && time.Since(c.lastFullScanAt) < c.fullScanCooldown {
		return false
	}
	// 避免在“全部都还活着”的稳态下误触发全量扫：
	// 只有确认存在残留（deleted/已过期）时才执行。
	live := c.liveLenLocked(now)
	if live >= mapLen {
		return false
	}
	return true
}

func (c *TTLCache[K, V]) fullScanLocked(now int64) (scanned int, purged int) {
	for k, e := range c.data {
		scanned++
		if e.deleted.Load() || (e.expireAt > 0 && now >= e.expireAt) {
			delete(c.data, k)
			purged++
		}
	}
	return scanned, purged
}

func (c *TTLCache[K, V]) liveLenLocked(now int64) int {
	n := 0
	for _, e := range c.data {
		if e.deleted.Load() {
			continue
		}
		if e.expireAt > 0 && now >= e.expireAt {
			continue
		}
		n++
	}
	return n
}

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

func (c *TTLCache[K, V]) pushToRing(key K) {
	// 如果队列满，当前 write 位置即将被覆盖
	if c.ringFull {
		var zero K
		c.cleanupRing[c.ringWriteIdx] = zero
		// 移动读指针到下一个位置（跳过被覆盖的最旧数据）
		c.ringReadIdx++
		if c.ringReadIdx >= c.ringCap {
			c.ringReadIdx = 0
		}
	}
	// 写入新 key
	c.cleanupRing[c.ringWriteIdx] = key
	c.ringWriteIdx++
	if c.ringWriteIdx >= c.ringCap {
		c.ringWriteIdx = 0
	}
	// 检查是否变满
	c.ringFull = c.ringWriteIdx == c.ringReadIdx
}

// noteKeyMayExpire 仅在「可能首次需要 TTL 清理」时入队，避免高频更新导致 ring 噪音过大。
func (c *TTLCache[K, V]) noteKeyMayExpire(key K, prev *entry[V], hadEntry bool, expireAt int64) {
	if expireAt <= 0 {
		return
	}
	// 新 key 首次带 TTL
	if !hadEntry {
		c.pushToRing(key)
		return
	}
	// 从“永不过期”切换到“带 TTL”
	if prev != nil && prev.expireAt <= 0 {
		c.pushToRing(key)
	}
}

func (c *TTLCache[K, V]) Close() {
	c.cleanupOnce.Do(func() {
		close(c.cleanupStop)
	})
}

func (c *TTLCache[K, V]) Get(key K) (value V, ok bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, exists := c.data[key]
	if !exists || e.deleted.Load() {
		return
	}
	now := timeNow()
	if e.expireAt > 0 && now >= e.expireAt {
		// 惰性删除：标记为 deleted，等待后台 Ring 清理物理删除
		e.deleted.CompareAndSwap(false, true)
		return
	}
	return e.value, true
}

func (c *TTLCache[K, V]) Set(key K, value V, ttlSeconds int64) {
	var expireAt int64
	if ttlSeconds > 0 {
		expireAt = timeNow() + ttlSeconds
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	prev, exists := c.data[key]
	c.noteKeyMayExpire(key, prev, exists, expireAt)
	c.data[key] = &entry[V]{value: value, expireAt: expireAt}
}

func (c *TTLCache[K, V]) SetWithExpireAt(key K, value V, expireAt int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	prev, exists := c.data[key]
	c.noteKeyMayExpire(key, prev, exists, expireAt)
	c.data[key] = &entry[V]{value: value, expireAt: expireAt}
}

func (c *TTLCache[K, V]) Delete(key K) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, exists := c.data[key]; exists {
		if e.expireAt <= 0 {
			delete(c.data, key)
			return
		}
		e.deleted.Store(true)
	}
}

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

	// Double check
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
	c.noteKeyMayExpire(key, nil, false, expireAt)
	c.data[key] = &entry[V]{value: value, expireAt: expireAt}
	return value, false
}

func (c *TTLCache[K, V]) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.data)
}

// LiveLen 返回当前仍视为「存活」的条目数
func (c *TTLCache[K, V]) LiveLen() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.liveLenLocked(timeNow())
}

// Clear 修复版：增加了锁保护
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

func (c *TTLCache[K, V]) Keys() []K {
	c.mu.RLock()
	defer c.mu.RUnlock()
	now := timeNow()
	keys := make([]K, 0, len(c.data))
	for k, e := range c.data {
		if e.deleted.Load() {
			continue
		}
		if e.expireAt > 0 && now >= e.expireAt {
			continue
		}
		keys = append(keys, k)
	}
	return keys
}

// timeNow 获取当前 Unix 时间戳（秒）
func timeNow() int64 {
	return utils.UnixSecond()
}
