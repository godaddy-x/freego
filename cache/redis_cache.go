package cache

import (
	"github.com/garyburd/redigo/redis"
	"github.com/godaddy-x/freego/util"
	"time"
)

var (
	MASTER         = "MASTER"
	redis_sessions = make(map[string]*RedisManager, 0)
)

// redis配置参数
type RedisConfig struct {
	DsName      string
	Host        string
	Port        int
	Password    string
	MaxIdle     int
	MaxActive   int
	IdleTimeout int
	Network     string
	LockTimeout int
}

// redis缓存管理器
type RedisManager struct {
	CacheManager
	DsName      string
	LockTimeout int
	Pool        *redis.Pool
}

func (self *RedisManager) InitConfig(input ...RedisConfig) (*RedisManager, error) {
	for _, v := range input {
		dsName := MASTER
		if len(v.DsName) > 0 {
			dsName = v.DsName
		}
		if _, b := redis_sessions[dsName]; b {
			return nil, util.Error("初始化redis连接池失败: [", v.DsName, "]已存在")
		}
		pool := &redis.Pool{MaxIdle: v.MaxIdle, MaxActive: v.MaxActive, IdleTimeout: time.Duration(v.IdleTimeout) * time.Second, Dial: func() (redis.Conn, error) {
			c, err := redis.Dial(v.Network, util.AddStr(v.Host, ":", util.AnyToStr(v.Port)))
			if err != nil {
				return nil, err
			}
			if len(v.Password) > 0 {
				if _, err := c.Do("AUTH", v.Password); err != nil {
					c.Close()
					return nil, err
				}
			}
			return c, err
		}}
		redis_sessions[dsName] = &RedisManager{Pool: pool, DsName: dsName, LockTimeout: v.LockTimeout}
	}
	if len(redis_sessions) == 0 {
		return nil, util.Error("初始化redis连接池失败: 数据源为0")
	}
	return self, nil
}

func (self *RedisManager) Client(dsname ...string) (*RedisManager, error) {
	var ds string
	if len(dsname) > 0 && len(dsname[0]) > 0 {
		ds = dsname[0]
	} else {
		ds = MASTER
	}
	manager := redis_sessions[ds]
	if manager == nil {
		return nil, util.Error("redis数据源[", ds, "]未找到,请检查...")
	}
	return manager, nil
}

/********************************** redis缓存接口实现 **********************************/

func (self *RedisManager) Get(key string, input interface{}) (interface{}, bool, error) {
	client := self.Pool.Get()
	defer client.Close()
	value, err := redis.Bytes(client.Do("GET", key))
	if err != nil && err != redis.ErrNil {
		return nil, false, err
	}
	if value == nil || len(value) == 0 {
		return nil, false, nil
	}
	return value, true, util.JsonUnmarshal(value, input)
}

func (self *RedisManager) GetInt64(key string) (int64, error) {
	client := self.Pool.Get()
	defer client.Close()
	value, err := redis.Bytes(client.Do("GET", key))
	if err != nil && err != redis.ErrNil {
		return 0, err
	}
	if value == nil || len(value) == 0 {
		return 0, nil
	}
	if ret, err := util.StrToInt64(util.Bytes2Str(value)); err != nil {
		return 0, err
	} else {
		return ret, nil
	}
}

func (self *RedisManager) GetFloat64(key string) (float64, error) {
	client := self.Pool.Get()
	defer client.Close()
	value, err := redis.Bytes(client.Do("GET", key))
	if err != nil && err != redis.ErrNil {
		return 0, err
	}
	if value == nil || len(value) == 0 {
		return 0, nil
	}
	if ret, err := util.StrToFloat(util.Bytes2Str(value)); err != nil {
		return 0, err
	} else {
		return ret, nil
	}
}

func (self *RedisManager) GetString(key string) (string, error) {
	client := self.Pool.Get()
	defer client.Close()
	value, err := redis.Bytes(client.Do("GET", key))
	if err != nil && err != redis.ErrNil {
		return "", err
	}
	if value == nil || len(value) == 0 {
		return "", nil
	}
	return util.Bytes2Str(value), nil
}

func (self *RedisManager) GetBool(key string) (bool, error) {
	client := self.Pool.Get()
	defer client.Close()
	value, err := redis.Bytes(client.Do("GET", key))
	if err != nil && err != redis.ErrNil {
		return false, err
	}
	if value == nil || len(value) == 0 {
		return false, nil
	}
	return util.StrToBool(util.Bytes2Str(value))
}

func (self *RedisManager) Put(key string, input interface{}, expire ...int) error {
	if len(key) == 0 || input == nil {
		return nil
	}
	value := util.Str2Bytes(util.AnyToStr(input))
	client := self.Pool.Get()
	defer client.Close()
	if len(expire) > 0 && expire[0] > 0 {
		if _, err := client.Do("SET", key, value, "EX", util.AnyToStr(expire[0])); err != nil {
			return err
		}
	} else {
		if _, err := client.Do("SET", key, value); err != nil {
			return err
		}
	}
	return nil
}

func (self *RedisManager) Del(key ...string) error {
	client := self.Pool.Get()
	defer client.Close()
	if len(key) > 0 {
		if _, err := client.Do("DEL", key); err != nil {
			return err
		}
	}
	client.Send("MULTI")
	for _, v := range key {
		client.Send("DEL", v)
	}
	if _, err := client.Do("EXEC"); err != nil {
		return err
	}
	return nil
}

// 数据量大时请慎用
func (self *RedisManager) Keys(pattern ...string) ([]string, error) {
	client := self.Pool.Get()
	defer client.Close()
	p := "*"
	if len(pattern) > 0 && len(pattern[0]) > 0 {
		p = pattern[0]
	}
	keys, err := redis.Strings(client.Do("KEYS", p))
	if err != nil {
		return nil, err
	}
	return keys, nil
}

// 数据量大时请慎用
func (self *RedisManager) Size(pattern ...string) (int, error) {
	keys, err := self.Keys(pattern...)
	if err != nil {
		return 0, err
	}
	return len(keys), nil
}

// 数据量大时请慎用
func (self *RedisManager) Values(pattern ...string) ([]interface{}, error) {
	return nil, util.Error("No implementation method [Values] was found")
}

func (self *RedisManager) Flush() error {
	return util.Error("No implementation method [Flush] was found")
}
