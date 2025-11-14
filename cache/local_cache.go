package cache

import (
	"strings"
	"time"

	"github.com/godaddy-x/freego/utils"
	gocache "github.com/patrickmn/go-cache"
)

// LocalMapManager 使用go-cache的本地缓存管理器
type LocalMapManager struct {
	CacheManager
	cache *gocache.Cache // go-cache实例，线程安全
}

// 默认配置常量
const (
	defaultExpiration = 5 * time.Minute  // 默认过期时间5分钟
	cleanupInterval   = 10 * time.Minute // 清理间隔10分钟
)

// NewLocalCache 创建新的go-cache缓存实例
func NewLocalCache(a, b int) Cache {
	return new(LocalMapManager).NewCache(a, b)
}

// NewLocalCacheWithEvict 创建带淘汰回调的go-cache缓存实例
// 注意：go-cache不支持淘汰回调，此方法保留API兼容性
func NewLocalCacheWithEvict(a, b int, f func(item interface{})) Cache {
	return new(LocalMapManager).NewCache(a, b)
}

// NewCache 创建go-cache缓存配置
func (self *LocalMapManager) NewCache(a, b int) Cache {
	// a: 默认过期时间（分钟），b: 清理间隔（分钟）
	defaultExp := defaultExpiration
	cleanupInt := cleanupInterval

	if a > 0 {
		defaultExp = time.Duration(a) * time.Minute
	}
	if b > 0 {
		cleanupInt = time.Duration(b) * time.Minute
	}

	c := gocache.New(defaultExp, cleanupInt)

	return &LocalMapManager{
		cache: c,
	}
}

// NewCacheWithEvict 创建带淘汰回调的go-cache缓存
// 注意：go-cache不支持淘汰回调，此方法保留API兼容性
func (self *LocalMapManager) NewCacheWithEvict(a, b int, f func(interface{})) Cache {
	return self.NewCache(a, b)
}

func (self *LocalMapManager) Mode() string {
	return LOCAL
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
		return ret, nil
	}
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
		return ret, nil
	}
	return utils.StrToBool(utils.AnyToStr(v))
}

func (self *LocalMapManager) Put(key string, input interface{}, expire ...int) error {
	var d time.Duration
	if len(expire) > 0 && expire[0] > 0 {
		d = time.Duration(expire[0]) * time.Second
	}
	// go-cache的Set方法：如果d为0，使用默认过期时间；如果d为-1，永不过期
	self.cache.Set(key, input, d)
	return nil
}

func (self *LocalMapManager) Keys(pattern ...string) ([]string, error) {
	items := self.cache.Items()

	// 如果没有指定模式，返回所有键
	if len(pattern) == 0 || (len(pattern) == 1 && pattern[0] == "") {
		keys := make([]string, 0, len(items))
		for k := range items {
			keys = append(keys, k)
		}
		return keys, nil
	}

	// 支持简单的模式匹配（*通配符）
	matchPattern := pattern[0]
	keys := make([]string, 0, len(items))

	for k := range items {
		if matchesPattern(k, matchPattern) {
			keys = append(keys, k)
		}
	}

	return keys, nil
}

func (self *LocalMapManager) Del(key ...string) error {
	for _, k := range key {
		self.cache.Delete(k)
	}
	return nil
}

func (self *LocalMapManager) Exists(key string) (bool, error) {
	_, exists := self.cache.Get(key)
	return exists, nil
}

func (self *LocalMapManager) Size(pattern ...string) (int, error) {
	// go-cache没有直接获取大小的方法，返回估算值
	items := self.cache.Items()
	return len(items), nil
}

func (self *LocalMapManager) Values(pattern ...string) ([]interface{}, error) {
	// go-cache不支持直接获取所有values
	return []interface{}{}, nil
}

func (self *LocalMapManager) Flush() error {
	// go-cache没有Flush方法，这里调用DeleteExpired来清理过期项目
	self.cache.DeleteExpired()
	return nil
}

// matchesPattern 检查字符串是否匹配给定的模式（支持*通配符）
// pattern: 匹配模式，如 "user:*", "cache_*"
// str: 要匹配的字符串
// 返回: 是否匹配
func matchesPattern(str, pattern string) bool {
	// 如果模式为空或只有*，匹配所有
	if pattern == "" || pattern == "*" {
		return true
	}

	// 如果没有通配符，直接字符串比较
	if !strings.Contains(pattern, "*") {
		return str == pattern
	}

	// 简单的通配符匹配实现
	// 将*替换为.*进行正则匹配
	regexPattern := strings.ReplaceAll(pattern, "*", ".*")
	return strings.Contains(str, strings.TrimSuffix(strings.TrimPrefix(regexPattern, ".*"), ".*")) ||
		strings.HasPrefix(str, strings.TrimSuffix(regexPattern, ".*")) ||
		strings.HasSuffix(str, strings.TrimPrefix(regexPattern, ".*"))
}
