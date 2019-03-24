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
		if len(v.DsName) > 0 {
			redis_sessions[v.DsName] = &RedisManager{Pool: pool, DsName: v.DsName, LockTimeout: v.LockTimeout}
		} else {
			redis_sessions[MASTER] = &RedisManager{Pool: pool, DsName: MASTER, LockTimeout: v.LockTimeout}
		}
	}
	if len(redis_sessions) == 0 {
		panic("初始化redis连接池失败: 数据源为0")
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
	r1, r2, r3 := func() (interface{}, bool, error) {
		value, err := redis.Bytes(client.Do("GET", key))
		if err != nil && err.Error() != "redigo: nil returned" {
			return nil, false, err
		}
		if value != nil && len(value) > 0 {
			if input == nil {
				return nil, true, nil
			}
			err := util.JsonUnmarshal(value, input);
			return input, true, err
		}
		return input, false, nil
	}()
	client.Close()
	return r1, r2, r3
}

func (self *RedisManager) Put(key string, input interface{}, expire ...int) error {
	value, err := util.JsonMarshal(input)
	if err != nil {
		return err
	}
	client := self.Pool.Get()
	r1 := func() error {
		if len(expire) > 0 && expire[0] > 0 {
			if _, err := client.Do("SET", key, value, "EX", expire[0]); err != nil {
				return err
			}
		} else {
			if _, err := client.Do("SET", key, value); err != nil {
				return err
			}
		}
		return nil
	}()
	client.Close()
	return r1
}

func (self *RedisManager) Del(key ...string) error {
	client := self.Pool.Get()
	r1 := func() error {
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
	}()
	client.Close()
	return r1
}

// 数据量大时请慎用
func (self *RedisManager) Keys(pattern ...string) ([]string, error) {
	client := self.Pool.Get()
	r1, r2 := func() ([]string, error) {
		p := "*"
		if len(pattern) > 0 && len(pattern[0]) > 0 {
			p = pattern[0]
		}
		keys, err := redis.Strings(client.Do("KEYS", p))
		if err != nil {
			return nil, err
		}
		return keys, nil
	}()
	client.Close()
	return r1, r2
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
