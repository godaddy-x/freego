package cache

import (
	"github.com/godaddy-x/freego/util"
	"time"
)

// 本地缓存管理器
type LocalMapManager struct {
	CacheManager
	c *Cache
}

// a默认缓存时间/分钟 b默认校验数据间隔时间/分钟
func (self *LocalMapManager) NewCache(a, b int) ICache {
	c := New(time.Duration(a)*time.Minute, time.Duration(b)*time.Minute)
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
	return v, b, util.JsonToAny(v, input)
}

func (self *LocalMapManager) GetInt64(key string) (int64, error) {
	v, b := self.c.Get(key)
	if !b || v == nil {
		return 0, nil
	}
	if ret, err := util.StrToInt64(util.AnyToStr(v)); err != nil {
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
	if ret, err := util.StrToFloat(util.AnyToStr(v)); err != nil {
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
	return util.AnyToStr(v), nil
}

func (self *LocalMapManager) GetBool(key string) (bool, error) {
	v, b := self.c.Get(key)
	if !b || v == nil {
		return false, nil
	}
	return util.StrToBool(util.AnyToStr(v))
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

// 数据量大时请慎用
func (self *LocalMapManager) Size(pattern ...string) (int, error) {
	return self.c.ItemCount(), nil
}

func (self *LocalMapManager) Flush() error {
	self.c.Flush()
	return nil
}
