package cache

import (
	"time"

	"github.com/godaddy-x/freego/utils"
)

// LocalMapManager 使用go-cache的本地缓存管理器
type LocalMapManager struct {
	CacheManager
	cache *TTLCache[string, interface{}]
}

// 默认配置常量
const (
	defaultExpiration = 5 * time.Minute  // 默认过期时间5分钟
	cleanupInterval   = 10 * time.Minute // 清理间隔10分钟
)

// NewLocalCache 创建新的go-cache缓存实例
func NewLocalCache(initialCapacity int) Cache {
	return new(LocalMapManager).NewCache(initialCapacity)
}

func (self *LocalMapManager) NewCache(initialCapacity int) Cache {
	return &LocalMapManager{
		cache: NewTTLCache[string, interface{}](initialCapacity),
	}
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
	if ret, check := v.(int64); check {
		return ret, nil
	}
	return utils.StrToInt64(utils.AnyToStr(v))
}

func (self *LocalMapManager) GetFloat64(key string) (float64, error) {
	v, b := self.cache.Get(key)
	if !b || v == nil {
		return 0, nil
	}
	if ret, check := v.(float64); check {
		return ret, nil
	}
	return utils.StrToFloat(utils.AnyToStr(v))
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
	var d int
	if len(expire) > 0 {
		d = expire[0]
	}
	// go-cache的Set方法：如果d为0，使用默认过期时间；如果d为-1，永不过期
	self.cache.Set(key, input, d)
	return nil
}

//func (self *LocalMapManager) Keys(pattern ...string) ([]string, error) {
//	return keys, nil
//}

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

//func (self *LocalMapManager) Size(pattern ...string) (int, error) {
//	// go-cache没有直接获取大小的方法，返回估算值
//	items := self.cache.Items()
//	return len(items), nil
//}

func (self *LocalMapManager) Values(pattern ...string) ([]interface{}, error) {
	// go-cache不支持直接获取所有values
	return []interface{}{}, nil
}

func (self *LocalMapManager) Flush() error {
	// go-cache没有Flush方法，这里调用DeleteExpired来清理过期项目
	self.cache.Clear()
	return nil
}
