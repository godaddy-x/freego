package cache

import (
	"github.com/godaddy-x/freego/utils"
	"github.com/patrickmn/go-cache"
	"time"
)

// 本地缓存管理器
type LocalMapManager struct {
	CacheManager
	c *cache.Cache
}

// a默认缓存时间/分钟 b默认校验数据间隔时间/分钟
func NewLocalCache(a, b int) Cache {
	return new(LocalMapManager).NewCache(a, b)
}

// a默认缓存时间/分钟 b默认校验数据间隔时间/分钟
func (self *LocalMapManager) NewCache(a, b int) Cache {
	c := cache.New(time.Duration(a)*time.Minute, time.Duration(b)*time.Minute)
	return &LocalMapManager{c: c}
}

func (self *LocalMapManager) Get(key string, input interface{}) (interface{}, bool, error) {
	v, b := self.c.Get(key)
	if !b || v == nil {
		return nil, false, nil
	}
	if input == nil {
		return v, b, nil
	}
	return v, b, utils.JsonToAny(v, input)
}

func (self *LocalMapManager) GetInt64(key string) (int64, error) {
	v, b := self.c.Get(key)
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
	v, b := self.c.Get(key)
	if !b || v == nil {
		return 0, nil
	}
	if ret, err := utils.StrToFloat(utils.AnyToStr(v)); err != nil {
		return 0, err
	} else {
		return ret, nil
	}
}

func (self *LocalMapManager) GetString(key string) (string, error) {
	v, b := self.c.Get(key)
	if !b || v == nil {
		return "", nil
	}
	return utils.AnyToStr(v), nil
}

func (self *LocalMapManager) GetBool(key string) (bool, error) {
	v, b := self.c.Get(key)
	if !b || v == nil {
		return false, nil
	}
	return utils.StrToBool(utils.AnyToStr(v))
}

func (self *LocalMapManager) Put(key string, input interface{}, expire ...int) error {
	if expire != nil && len(expire) > 0 {
		self.c.Set(key, input, time.Duration(expire[0])*time.Second)
	} else {
		self.c.SetDefault(key, input)
	}
	return nil
}

func (self *LocalMapManager) Del(key ...string) error {
	if key != nil {
		for _, v := range key {
			self.c.Delete(v)
		}
	}
	return nil
}

func (self *LocalMapManager) Exists(key string) (bool, error) {
	_, b := self.c.Get(key)
	return b, nil
}

// 数据量大时请慎用
func (self *LocalMapManager) Size(pattern ...string) (int, error) {
	return self.c.ItemCount(), nil
}

func (self *LocalMapManager) Values(pattern ...string) ([]interface{}, error) {
	return []interface{}{self.c.Items()}, nil
}

func (self *LocalMapManager) Flush() error {
	self.c.Flush()
	return nil
}
