package cache

import (
	"strings"
	"time"

	"github.com/godaddy-x/freego/utils"
	gocache "github.com/patrickmn/go-cache"
)

// LocalMapManager 基于 github.com/patrickmn/go-cache 的本地内存缓存
type LocalMapManager struct {
	CacheManager
	cache *gocache.Cache
}

// NewDefaultLocalCache 创建新的本地缓存实例
func NewDefaultLocalCache() Cache {
	return new(LocalMapManager).NewCache(600, 60)
}

// NewLocalCache 创建新的本地缓存实例
func NewLocalCache() Cache {
	return new(LocalMapManager).NewCache(600, 60)
}

// NewCache 创建本地缓存（默认项 TTL 与清理周期仅作用于未显式指定过期的路径）
func (self *LocalMapManager) NewCache(a, b int) Cache {
	// 第一项：未使用 DefaultExpiration(0) 的 Put 不会走到该默认；第二项：过期键扫描周期
	c := gocache.New(time.Duration(a)*time.Second, time.Duration(b)*time.Second)
	return &LocalMapManager{cache: c}
}

// NewDefaultCache 创建本地缓存实例
func (self *LocalMapManager) NewDefaultCache() Cache {
	return self.NewCache(600, 60)
}

func (self *LocalMapManager) Mode() string {
	return LOCAL
}

func (self *LocalMapManager) Get(key string, input interface{}) (interface{}, bool, error) {
	v, ok := self.cache.Get(key)
	if !ok || v == nil {
		return nil, false, nil
	}
	if input == nil {
		return v, ok, nil
	}
	return v, ok, utils.JsonToAny(v, input)
}

func (self *LocalMapManager) GetInt64(key string) (int64, error) {
	v, ok := self.cache.Get(key)
	if !ok || v == nil {
		return 0, nil
	}
	if ret, check := v.(int64); check {
		return ret, nil
	}
	if ret, err := utils.StrToInt64(utils.AnyToStr(v)); err != nil {
		return 0, err
	} else {
		return ret, nil
	}
}

func (self *LocalMapManager) GetFloat64(key string) (float64, error) {
	v, ok := self.cache.Get(key)
	if !ok || v == nil {
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
	v, ok := self.cache.Get(key)
	if !ok || v == nil {
		return "", nil
	}
	if ret, check := v.(string); check {
		return ret, nil
	}
	return utils.AnyToStr(v), nil
}

func (self *LocalMapManager) GetBytes(key string) ([]byte, error) {
	v, ok := self.cache.Get(key)
	if !ok || v == nil {
		return nil, nil
	}
	if ret, check := v.([]byte); check {
		return ret, nil
	}
	return utils.Str2Bytes(utils.AnyToStr(v)), nil
}

func (self *LocalMapManager) GetBool(key string) (bool, error) {
	v, ok := self.cache.Get(key)
	if !ok || v == nil {
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
	} else {
		d = gocache.NoExpiration
	}
	self.cache.Set(key, input, d)
	return nil
}

func (self *LocalMapManager) Keys(pattern ...string) ([]string, error) {
	items := self.cache.Items()
	keys := make([]string, 0, len(items))
	for k := range items {
		keys = append(keys, k)
	}
	if len(pattern) == 0 || (len(pattern) == 1 && pattern[0] == "") {
		return keys, nil
	}
	matchPattern := pattern[0]
	filtered := make([]string, 0, len(keys))
	for _, k := range keys {
		if matchesPattern(k, matchPattern) {
			filtered = append(filtered, k)
		}
	}
	return filtered, nil
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
	keys, err := self.Keys(pattern...)
	if err != nil {
		return 0, err
	}
	return len(keys), nil
}

func (self *LocalMapManager) Values(pattern ...string) ([]interface{}, error) {
	keys, err := self.Keys(pattern...)
	if err != nil {
		return nil, err
	}
	values := make([]interface{}, 0, len(keys))
	for _, k := range keys {
		if v, ok := self.cache.Get(k); ok {
			values = append(values, v)
		}
	}
	return values, nil
}

func (self *LocalMapManager) Flush() error {
	self.cache.Flush()
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
