// Package cache 提供线程安全的 LRU 缓存实现。
//
// 设计说明：
//   - 本实现使用 sync.RWMutex，在读多写少且读操作不涉及状态变更的场景下
//     （如 Contains、Peek、Range 等方法）可以获得更好的并发性能。
//   - Get 方法由于需要更新访问顺序（moveToHead），最终仍需获取写锁，
//     因此 RWMutex 对 Get 的并发优化有限。
//   - 如果主要使用 Get/Put 操作，可考虑改用 sync.Mutex 简化实现。
package cache

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
)

// LRUCache 是一个线程安全、支持泛型的 LRU 缓存，具有高并发读取性能。
// 使用双向链表维护访问顺序，通过原子标记实现安全的并发访问。
type LRUCache[K comparable, V any] struct {
	capacity int
	cache    map[K]*node[K, V]
	head     *node[K, V] // 虚拟头节点（最近使用端）
	tail     *node[K, V] // 虚拟尾节点（最久未使用端）
	mu       sync.RWMutex
}

// node 表示双向链表中的一个缓存项。
// deleted 字段用于在并发访问时安全地标识节点是否已被逻辑删除。
// 字段顺序经过优化以减少内存对齐带来的 padding。
type node[K comparable, V any] struct {
	deleted int32       // 4 字节：0 = 活跃状态，1 = 已逻辑删除
	key     K           // 键
	value   V           // 值
	prev    *node[K, V] // 8 字节：前驱节点指针（64位系统）
	next    *node[K, V] // 8 字节：后继节点指针（64位系统）
}

// NewLRUCache 创建一个指定容量的新 LRU 缓存。
// capacity 必须为正数，否则会 panic。
func NewLRUCache[K comparable, V any](capacity int) *LRUCache[K, V] {
	if capacity <= 0 {
		panic("lru: capacity must be positive")
	}

	// 初始化虚拟头节点和尾节点，以简化链表操作。
	head := &node[K, V]{}
	tail := &node[K, V]{}
	head.next = tail
	tail.prev = head

	return &LRUCache[K, V]{
		capacity: capacity,
		cache:    make(map[K]*node[K, V], capacity),
		head:     head,
		tail:     tail,
	}
}

// Get 根据键获取对应的值。
// 如果找到且未被删除，则返回值、true 并将该项标记为最近使用；
// 否则返回零值和 false。
//
// 并发语义说明：
//   - 如果在 Get 执行期间，另一个 goroutine 对同一 key 执行了 Put 操作，
//     Get 可能返回旧值（在 Put 之前）或新值（在 Put 之后），
//     但绝不会返回一个已被标记删除的值。
//   - 如果在 Get 执行期间，另一个 goroutine 对同一 key 执行了 Delete 操作，
//     Get 可能返回值（在 Delete 之前）或 false（在 Delete 之后）。
//   - 这些行为符合线性一致性模型中的常见预期。
func (c *LRUCache[K, V]) Get(key K) (value V, ok bool) {
	// 第一阶段：使用读锁快速查找
	c.mu.RLock()
	n, exists := c.cache[key]
	c.mu.RUnlock()

	if !exists {
		return
	}

	// 快速检查该节点是否已被标记为删除（无锁原子操作）
	if atomic.LoadInt32(&n.deleted) == 1 {
		return
	}

	// 第二阶段：获取写锁，将节点移动到链表头部
	c.mu.Lock()
	defer c.mu.Unlock()

	// 重新验证节点有效性（获取写锁期间状态可能已改变）
	n2, exists := c.cache[key]
	if !exists || n2 != n || atomic.LoadInt32(&n2.deleted) == 1 {
		return
	}

	// 检查节点是否仍在链表中（prev/next 不为 nil）
	// 如果节点正在被操作（如 removeNode），可能暂时不在链表中
	if n2.prev == nil || n2.next == nil {
		return
	}

	// 移动到链表头部（标记为最近使用）
	c.moveToHead(n2)
	return n2.value, true
}

// Put 插入或更新一个键值对。
func (c *LRUCache[K, V]) Put(key K, value V) {
	c.mu.Lock()
	defer c.mu.Unlock()

	n, exists := c.cache[key]

	if exists && atomic.LoadInt32(&n.deleted) == 0 {
		n.value = value
		if n.prev != nil && n.next != nil {
			c.moveToHead(n)
		} else {
			c.addToHead(n)
		}
		return
	}

	newNode := &node[K, V]{
		key:     key,
		value:   value,
		deleted: 0,
	}
	c.cache[key] = newNode
	c.addToHead(newNode)

	// 🔴 修复：使用 countValid 而不是 len(c.cache)
	for c.countValid() > c.capacity {
		if !c.evictLRU() {
			break // 防止死循环
		}
	}
}

// Delete 从缓存中删除指定的键值对。
func (c *LRUCache[K, V]) Delete(key K) {
	c.mu.Lock()
	defer c.mu.Unlock()

	n, exists := c.cache[key]
	if !exists {
		return
	}

	// 从链表中移除
	if n.prev != nil && n.next != nil {
		c.removeNode(n)
	}

	// 标记为已删除并从 map 中移除
	atomic.StoreInt32(&n.deleted, 1)
	delete(c.cache, key)
}

// Contains 检查缓存中是否存在指定的键。
// 注意：即使键存在，如果对应的项已被标记为删除，也会返回 false。
func (c *LRUCache[K, V]) Contains(key K) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	n, exists := c.cache[key]
	if !exists {
		return false
	}
	return atomic.LoadInt32(&n.deleted) == 0
}

// Peek 获取键对应的值，但不改变其在 LRU 中的位置。
// 如果找到且未被删除，则返回值和 true；否则返回零值和 false。
func (c *LRUCache[K, V]) Peek(key K) (value V, ok bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	n, exists := c.cache[key]
	if !exists {
		return
	}

	if atomic.LoadInt32(&n.deleted) == 1 {
		return
	}

	return n.value, true
}

// Clear 清空缓存中的所有项。
func (c *LRUCache[K, V]) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 标记所有节点为删除
	for _, n := range c.cache {
		atomic.StoreInt32(&n.deleted, 1)
	}

	// 重置缓存
	c.cache = make(map[K]*node[K, V], c.capacity)
	c.head.next = c.tail
	c.tail.prev = c.head
}

// Range 遍历所有有效的缓存项。
// 遍历顺序为从最近使用到最久未使用。
// 如果 f 返回 false，则停止遍历。
func (c *LRUCache[K, V]) Range(f func(K, V) bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for n := c.head.next; n != c.tail; n = n.next {
		if atomic.LoadInt32(&n.deleted) == 0 {
			if !f(n.key, n.value) {
				break
			}
		}
	}
}

// RangeReverse 反向遍历所有有效的缓存项。
// 遍历顺序为从最久未使用到最近使用。
// 如果 f 返回 false，则停止遍历。
func (c *LRUCache[K, V]) RangeReverse(f func(K, V) bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for n := c.tail.prev; n != c.head; n = n.prev {
		if atomic.LoadInt32(&n.deleted) == 0 {
			if !f(n.key, n.value) {
				break
			}
		}
	}
}

// Keys 返回所有有效缓存项的键（从最近使用到最久未使用）。
func (c *LRUCache[K, V]) Keys() []K {
	c.mu.RLock()
	defer c.mu.RUnlock()

	keys := make([]K, 0, len(c.cache))
	for n := c.head.next; n != c.tail; n = n.next {
		if atomic.LoadInt32(&n.deleted) == 0 {
			keys = append(keys, n.key)
		}
	}
	return keys
}

// Values 返回所有有效缓存项的值（从最近使用到最久未使用）。
func (c *LRUCache[K, V]) Values() []V {
	c.mu.RLock()
	defer c.mu.RUnlock()

	values := make([]V, 0, len(c.cache))
	for n := c.head.next; n != c.tail; n = n.next {
		if atomic.LoadInt32(&n.deleted) == 0 {
			values = append(values, n.value)
		}
	}
	return values
}

// Len 返回当前缓存中的有效项数量。
func (c *LRUCache[K, V]) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.countValid()
}

// Cap 返回缓存的最大容量。
func (c *LRUCache[K, V]) Cap() int {
	return c.capacity
}

// IsFull 返回缓存是否已满。
func (c *LRUCache[K, V]) IsFull() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.countValid() >= c.capacity
}

// Resize 调整缓存容量。
func (c *LRUCache[K, V]) Resize(newCapacity int) {
	if newCapacity <= 0 {
		panic("lru: capacity must be positive")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.capacity = newCapacity

	// 🔴 修复：使用 countValid 而不是 len(c.cache)
	for c.countValid() > c.capacity {
		if !c.evictLRU() {
			break
		}
	}
}

// GetOldest 获取最久未使用的项（不删除）。
// 如果缓存为空，返回零值和 false。
func (c *LRUCache[K, V]) GetOldest() (key K, value V, ok bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// 从尾节点向前找第一个有效的节点
	for n := c.tail.prev; n != c.head; n = n.prev {
		if atomic.LoadInt32(&n.deleted) == 0 {
			return n.key, n.value, true
		}
	}
	return
}

// GetNewest 获取最近使用的项（不删除）。
// 如果缓存为空，返回零值和 false。
func (c *LRUCache[K, V]) GetNewest() (key K, value V, ok bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// 从头节点向后找第一个有效的节点
	for n := c.head.next; n != c.tail; n = n.next {
		if atomic.LoadInt32(&n.deleted) == 0 {
			return n.key, n.value, true
		}
	}
	return
}

// RemoveOldest 删除并返回最久未使用的项。
// 如果缓存为空，返回零值和 false。
func (c *LRUCache[K, V]) RemoveOldest() (key K, value V, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 从尾节点向前找第一个有效的节点
	for n := c.tail.prev; n != c.head; n = n.prev {
		if atomic.LoadInt32(&n.deleted) == 0 {
			key, value = n.key, n.value
			c.removeNode(n)
			atomic.StoreInt32(&n.deleted, 1)
			delete(c.cache, n.key)
			return key, value, true
		}
	}
	return
}

// RemoveNewest 删除并返回最近使用的项。
// 如果缓存为空，返回零值和 false。
func (c *LRUCache[K, V]) RemoveNewest() (key K, value V, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 从头节点向后找第一个有效的节点
	for n := c.head.next; n != c.tail; n = n.next {
		if atomic.LoadInt32(&n.deleted) == 0 {
			key, value = n.key, n.value
			c.removeNode(n)
			atomic.StoreInt32(&n.deleted, 1)
			delete(c.cache, n.key)
			return key, value, true
		}
	}
	return
}

// Stats 返回缓存统计信息。
type Stats struct {
	Size     int `json:"size"`     // 当前有效项数量
	Capacity int `json:"capacity"` // 最大容量
	Items    int `json:"items"`    // map 中的总项数（包括已标记删除的）
}

// Stats 返回当前缓存的统计信息。
func (c *LRUCache[K, V]) Stats() Stats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return Stats{
		Size:     c.countValid(),
		Capacity: c.capacity,
		Items:    len(c.cache),
	}
}

// --- 私有辅助方法 ---

// countValid 统计有效的缓存项数量（不加锁，由调用者持有锁）。
func (c *LRUCache[K, V]) countValid() int {
	count := 0
	for n := c.head.next; n != c.tail; n = n.next {
		if atomic.LoadInt32(&n.deleted) == 0 {
			count++
		}
	}
	return count
}

// evictLRU 淘汰最久未使用的项，返回是否成功淘汰。
func (c *LRUCache[K, V]) evictLRU() bool {
	for n := c.tail.prev; n != c.head; n = n.prev {
		if atomic.LoadInt32(&n.deleted) == 0 {
			c.removeNode(n)
			atomic.StoreInt32(&n.deleted, 1)
			delete(c.cache, n.key)
			return true
		}
	}
	return false
}

// addToHead 将节点插入到虚拟头节点之后（即 MRU 位置）。
// 不加锁，由调用者持有写锁。
func (c *LRUCache[K, V]) addToHead(n *node[K, V]) {
	n.prev = c.head
	n.next = c.head.next
	c.head.next.prev = n
	c.head.next = n
}

// removeNode 从双向链表中移除指定节点。
// 移除后会断开节点的 prev/next 指针，防止悬垂引用。
// 不加锁，由调用者持有写锁。
func (c *LRUCache[K, V]) removeNode(n *node[K, V]) {
	// 安全检查
	if n.prev == nil || n.next == nil {
		return
	}

	n.prev.next = n.next
	n.next.prev = n.prev

	// 断开引用，帮助 GC 并防止误用
	n.prev = nil
	n.next = nil
}

// moveToHead 将现有节点移动到链表头部（标记为最近使用）。
// 不加锁，由调用者持有写锁。
func (c *LRUCache[K, V]) moveToHead(n *node[K, V]) {
	// 安全检查
	if n.prev == nil || n.next == nil {
		// 节点不在链表中，直接添加到头部
		c.addToHead(n)
		return
	}

	// 从当前位置移除
	n.prev.next = n.next
	n.next.prev = n.prev

	// 添加到头部
	n.prev = c.head
	n.next = c.head.next
	c.head.next.prev = n
	c.head.next = n
}

// --- 内部调试方法 ---

// Validate 验证内部数据结构的一致性（仅用于测试和调试）。
// 如果发现问题，返回错误描述；否则返回空字符串。
func (c *LRUCache[K, V]) Validate() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// 检查链表完整性
	nodeCount := 0
	for n := c.head.next; n != c.tail; n = n.next {
		if n.prev == nil || n.next == nil {
			return "node has nil prev or next pointer"
		}
		if n.prev.next != n {
			return "prev.next != node"
		}
		if n.next.prev != n {
			return "next.prev != node"
		}
		nodeCount++
	}

	// 检查反向链表完整性
	reverseCount := 0
	for n := c.tail.prev; n != c.head; n = n.prev {
		reverseCount++
	}

	if nodeCount != reverseCount {
		return "forward and reverse list counts mismatch"
	}

	// 检查 map 中的节点是否都在链表中（活跃节点）
	for key, n := range c.cache {
		if atomic.LoadInt32(&n.deleted) == 0 {
			if n.prev == nil || n.next == nil {
				// 使用 fmt.Sprintf 安全格式化任意类型
				return fmt.Sprintf("active node not in list: %v", key)
			}
		}
	}

	return ""
}

// String 返回缓存的字符串表示（用于调试）。
func (c *LRUCache[K, V]) String() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("LRUCache[capacity=%d, size=%d]", c.capacity, c.countValid()))
	sb.WriteString("\nMRU -> ")

	for n := c.head.next; n != c.tail; n = n.next {
		if atomic.LoadInt32(&n.deleted) == 0 {
			sb.WriteString(fmt.Sprintf("[%v:%v] ", n.key, n.value))
		} else {
			sb.WriteString("[deleted] ")
		}
	}

	sb.WriteString("<- LRU")
	return sb.String()
}
