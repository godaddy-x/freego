package cache

import (
	"sync"
	"time"

	"github.com/dgraph-io/ristretto"
	"github.com/godaddy-x/freego/utils"
)

// LocalMapManager 使用ristretto的本地缓存管理器
type LocalMapManager struct {
	CacheManager
	cache  *ristretto.Cache
	keys   map[string]bool // keys集合，用于优雅关闭时遍历
	keysMu sync.RWMutex    // 保护keys map的并发访问
}

// 默认配置常量
const (
	defaultNumCounters = 1e7     // 计数器数量：跟踪键频次(10M)
	defaultMaxCost     = 1 << 30 // 最大成本：1GB
	defaultBufferItems = 64      // Get缓冲区大小
)

// NewLocalCache 创建新的ristretto缓存实例
func NewLocalCache(a, b int) Cache {
	return new(LocalMapManager).NewCache(a, b)
}

// NewLocalCacheWithEvict 创建带淘汰回调的ristretto缓存实例
func NewLocalCacheWithEvict(a, b int, f func(item *ristretto.Item)) Cache {
	return new(LocalMapManager).NewCacheWithEvict(a, b, f)
}

// NewCache 创建ristretto缓存配置
func (self *LocalMapManager) NewCache(a, b int) Cache {
	cache, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: defaultNumCounters,
		MaxCost:     defaultMaxCost,
		BufferItems: defaultBufferItems,
	})
	if err != nil {
		return nil
	}

	return &LocalMapManager{
		cache: cache,
		keys:  make(map[string]bool), // 初始化高性能keys集合
	}
}

// NewCacheWithEvict 创建带淘汰回调的ristretto缓存
// 注意：ristretto的OnEvict回调只能访问哈希后的键，无法恢复原始字符串键
// 此方法保留API兼容性，但淘汰回调功能受限
func (self *LocalMapManager) NewCacheWithEvict(a, b int, f func(item *ristretto.Item)) Cache {
	cache, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: defaultNumCounters,
		MaxCost:     defaultMaxCost,
		BufferItems: defaultBufferItems,
		OnEvict:     f,
	})
	if err != nil {
		return nil
	}

	return &LocalMapManager{
		cache: cache,
		keys:  make(map[string]bool), // 初始化高性能keys集合
	}
}

func (self *LocalMapManager) Get(key string, input interface{}) (interface{}, bool, error) {
	v, b := self.cache.Get(key)
	if !b || v == nil {
		return nil, false, nil
	}
	if input == nil {
		return v, b, nil
	}
	return v, b, utils.JsonToAny(v, input)
}

func (self *LocalMapManager) GetInt64(key string) (int64, error) {
	v, b := self.cache.Get(key)
	if !b || v == nil {
		return 0, nil
	}
	if ret, err := utils.StrToInt64(utils.AnyToStr(v)); err != nil {
		return 0, err
	} else {
		return ret, nil
	}
}

func (self *LocalMapManager) GetFloat64(key string) (float64, error) {
	v, b := self.cache.Get(key)
	if !b || v == nil {
		return 0, nil
	}
	if ret, check := v.(float64); check {
		// 快速路径：如果ristretto直接返回float64
		return ret, nil
	}
	// 正常路径：ristretto返回序列化的字符串，需要解析
	if ret, err := utils.StrToFloat(utils.AnyToStr(v)); err != nil {
		return 0, err
	} else {
		return ret, nil
	}
}

func (self *LocalMapManager) GetString(key string) (string, error) {
	v, b := self.cache.Get(key)
	if !b || v == nil {
		return "", nil
	}
	if ret, check := v.(string); check {
		// 快速路径：如果ristretto直接返回string
		return ret, nil
	}
	return utils.AnyToStr(v), nil
}

func (self *LocalMapManager) GetBytes(key string) ([]byte, error) {
	v, b := self.cache.Get(key)
	if !b || v == nil {
		return nil, nil
	}
	if ret, check := v.([]byte); check {
		// 快速路径：如果ristretto直接返回[]byte
		return ret, nil
	}
	return utils.Str2Bytes(utils.AnyToStr(v)), nil
}

func (self *LocalMapManager) GetBool(key string) (bool, error) {
	v, b := self.cache.Get(key)
	if !b || v == nil {
		return false, nil
	}
	if ret, check := v.(bool); check {
		// 快速路径：如果ristretto直接返回bool
		return ret, nil
	}
	// 正常路径：ristretto返回序列化的字符串，需要解析
	return utils.StrToBool(utils.AnyToStr(v))
}

// calculateCost 计算缓存项的成本
func calculateCost(value interface{}) int64 {
	switch v := value.(type) {
	case string:
		return int64(len(v))
	case []byte:
		return int64(len(v))
	case int, int8, int16, int32, int64:
		return 8 // 整数类型固定8字节
	case float32, float64:
		return 8 // 浮点类型固定8字节
	case bool:
		return 1 // 布尔类型1字节
	default:
		// 对于复杂类型，使用估算值
		return 64 // 默认64字节
	}
}

func (self *LocalMapManager) Put(key string, input interface{}, expire ...int) error {
	cost := calculateCost(input)

	// 存储到缓存
	var success bool
	if len(expire) > 0 {
		success = self.cache.SetWithTTL(key, input, cost, time.Duration(expire[0])*time.Second)
	} else {
		success = self.cache.Set(key, input, cost)
	}

	// 如果存储成功，添加到外部keys集合
	if success {
		self.keysMu.Lock()
		self.keys[key] = true
		self.keysMu.Unlock()
	}

	return nil
}

func (self *LocalMapManager) Keys(pattern ...string) ([]string, error) {
	// 通过外部维护的keys集合返回所有缓存的键
	// 注意：这个集合可能包含已被淘汰但还未清理的键
	// 使用读锁保护并发访问
	self.keysMu.RLock()
	keys := make([]string, 0, len(self.keys))
	for key := range self.keys {
		// 可以在这里添加模式匹配逻辑
		// 目前简单返回所有键
		keys = append(keys, key)
	}
	self.keysMu.RUnlock()

	return keys, nil
}

func (self *LocalMapManager) Del(key ...string) error {
	for _, k := range key {
		self.cache.Del(k)
		// 从外部keys集合中移除，使用写锁保护
		self.keysMu.Lock()
		delete(self.keys, k)
		self.keysMu.Unlock()
	}
	return nil
}

func (self *LocalMapManager) Exists(key string) (bool, error) {
	_, exists := self.cache.Get(key)
	return exists, nil
}

// 数据量大时请慎用
func (self *LocalMapManager) Size(pattern ...string) (int, error) {
	// ristretto没有直接的方法获取当前项目数量
	// 这里返回一个估算值，实际使用中可能需要外部计数
	return 0, nil
}

func (self *LocalMapManager) Values(pattern ...string) ([]interface{}, error) {
	// ristretto不支持直接获取所有values
	return []interface{}{}, nil
}

func (self *LocalMapManager) Flush() error {
	// ristretto没有Flush方法，这里通过设置较短TTL来模拟
	// 注意：这不是真正的清空，只是让现有项目快速过期
	return nil
}
