package cache

import (
	"github.com/garyburd/redigo/redis"
	DIC "github.com/godaddy-x/freego/common"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/zlog"
	"time"
)

var (
	redisSessions = make(map[string]*RedisManager, 0)
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
}

// redis缓存管理器
type RedisManager struct {
	CacheManager
	DsName string
	Pool   *redis.Pool
}

func (self *RedisManager) InitConfig(input ...RedisConfig) (*RedisManager, error) {
	for _, v := range input {
		dsName := DIC.MASTER
		if len(v.DsName) > 0 {
			dsName = v.DsName
		}
		if _, b := redisSessions[dsName]; b {
			return nil, utils.Error("init redis pool failed: [", v.DsName, "] exist")
		}
		pool := &redis.Pool{MaxIdle: v.MaxIdle, MaxActive: v.MaxActive, IdleTimeout: time.Duration(v.IdleTimeout) * time.Second, Dial: func() (redis.Conn, error) {
			c, err := redis.Dial(v.Network, utils.AddStr(v.Host, ":", utils.AnyToStr(v.Port)))
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
		redisSessions[dsName] = &RedisManager{Pool: pool, DsName: dsName}
		zlog.Printf("redis service【%s】has been started successful", dsName)
	}
	if len(redisSessions) == 0 {
		return nil, utils.Error("init redis pool failed: sessions is nil")
	}
	return self, nil
}

func (self *RedisManager) Client(ds ...string) (*RedisManager, error) {
	dsName := DIC.MASTER
	if len(ds) > 0 && len(ds[0]) > 0 {
		dsName = ds[0]
	}
	manager := redisSessions[dsName]
	if manager == nil {
		return nil, utils.Error("redis session [", ds, "] not found...")
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
	if input == nil {
		return value, true, nil
	}
	return value, true, utils.JsonUnmarshal(value, input)
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
	if ret, err := utils.StrToInt64(utils.Bytes2Str(value)); err != nil {
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
	if ret, err := utils.StrToFloat(utils.Bytes2Str(value)); err != nil {
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
	return utils.Bytes2Str(value), nil
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
	return utils.StrToBool(utils.Bytes2Str(value))
}

func (self *RedisManager) Put(key string, input interface{}, expire ...int) error {
	if len(key) == 0 || input == nil {
		return nil
	}
	value := utils.Str2Bytes(utils.AnyToStr(input))
	client := self.Pool.Get()
	defer client.Close()
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
}

func (self *RedisManager) PutBatch(objs ...*PutObj) error {
	if objs == nil || len(objs) == 0 {
		return nil
	}
	client := self.Pool.Get()
	defer client.Close()
	client.Send("MULTI")
	for _, v := range objs {
		if v.Expire > 0 {
			if err := client.Send("SET", v.Key, v.Value, "EX", v.Expire); err != nil {
				return err
			}
		} else {
			if err := client.Send("SET", v.Key, v.Value); err != nil {
				return err
			}
		}
	}
	if _, err := client.Do("EXEC"); err != nil {
		return err
	}
	return nil
}

func (self *RedisManager) Del(key ...string) error {
	client := self.Pool.Get()
	defer client.Close()
	client.Send("MULTI")
	for _, v := range key {
		if err := client.Send("DEL", v); err != nil {
			return err
		}
	}
	if _, err := client.Do("EXEC"); err != nil {
		return err
	}
	return nil
}

func (self *RedisManager) Brpop(key string, expire int64, result interface{}) error {
	ret, err := self.BrpopString(key, expire)
	if err != nil || len(ret) == 0 {
		return err
	}
	if err := utils.JsonUnmarshal(utils.Str2Bytes(ret), &result); err != nil {
		return err
	}
	return nil
}

func (self *RedisManager) BrpopString(key string, expire int64) (string, error) {
	if len(key) == 0 || expire <= 0 {
		return "", nil
	}
	client := self.Pool.Get()
	defer client.Close()
	ret, err := redis.ByteSlices(client.Do("BRPOP", key, expire))
	if err != nil {
		return "", err
	} else if len(ret) != 2 {
		return "", utils.Error("data len error")
	}
	return utils.Bytes2Str(ret[1]), nil
}

func (self *RedisManager) BrpopInt64(key string, expire int64) (int64, error) {
	ret, err := self.BrpopString(key, expire)
	if err != nil || len(ret) == 0 {
		return 0, err
	}
	return utils.StrToInt64(ret)
}

func (self *RedisManager) BrpopFloat64(key string, expire int64) (float64, error) {
	ret, err := self.BrpopString(key, expire)
	if err != nil || len(ret) == 0 {
		return 0, err
	}
	return utils.StrToFloat(ret)
}

func (self *RedisManager) BrpopBool(key string, expire int64) (bool, error) {
	ret, err := self.BrpopString(key, expire)
	if err != nil || len(ret) == 0 {
		return false, err
	}
	return utils.StrToBool(ret)
}

func (self *RedisManager) Rpush(key string, val interface{}) error {
	if val == nil || len(key) == 0 {
		return nil
	}
	client := self.Pool.Get()
	defer client.Close()
	_, err := client.Do("RPUSH", key, utils.AnyToStr(val))
	if err != nil {
		return err
	}
	return nil
}

func (self *RedisManager) Publish(key string, val interface{}) error {
	if val == nil || len(key) == 0 {
		return nil
	}
	client := self.Pool.Get()
	defer client.Close()
	_, err := client.Do("PUBLISH", key, utils.AnyToStr(val))
	if err != nil {
		return err
	}
	return nil
}

// exp second
func (self *RedisManager) Subscribe(key string, timeout int, call func(msg string) (bool, error)) error {
	if call == nil || len(key) == 0 {
		return nil
	}
	if timeout <= 0 {
		timeout = 5
	}
	client := self.Pool.Get()
	defer client.Close()
	c := redis.PubSubConn{Conn: client}
	c.Subscribe(key)
	for {
		switch v := c.ReceiveWithTimeout(time.Duration(timeout) * time.Second).(type) {
		case redis.Message:
			if v.Channel == key {
				r, err := call(utils.Bytes2Str(v.Data))
				if err == nil && r {
					return nil
				}
			}
		case error:
			return v
		}
	}
	return nil
}

func (self *RedisManager) LuaScript(cmd string, key []string, val ...interface{}) (interface{}, error) {
	if len(cmd) == 0 || key == nil || len(key) == 0 {
		return nil, nil
	}
	args := make([]interface{}, 0, len(key)+len(val))
	for _, v := range key {
		args = append(args, v)
	}
	if val != nil && len(val) > 0 {
		for _, v := range val {
			args = append(args, utils.AddStr(v))
		}
	}
	client := self.Pool.Get()
	defer client.Close()
	return redis.NewScript(len(key), cmd).Do(client, args...)
}

// 数据量大时请慎用
func (self *RedisManager) Keys(pattern ...string) ([]string, error) {
	if pattern == nil || len(pattern) == 0 || pattern[0] == "*" {
		return nil, nil
	}
	client := self.Pool.Get()
	defer client.Close()
	keys, err := redis.Strings(client.Do("KEYS", pattern[0]))
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
	return nil, utils.Error("No implementation method [Values] was found")
}

func (self *RedisManager) Flush() error {
	return utils.Error("No implementation method [Flush] was found")
}
