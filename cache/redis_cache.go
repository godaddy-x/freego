package cache

import (
	"sync"
	"time"

	"github.com/garyburd/redigo/redis"
	DIC "github.com/godaddy-x/freego/common"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/zlog"
)

var (
	redisSessions = make(map[string]*RedisManager, 0)
	redisMutex    sync.RWMutex
)

type RedisConfig struct {
	DsName       string
	Host         string
	Port         int
	Password     string
	MaxIdle      int
	MaxActive    int
	IdleTimeout  int
	Network      string
	ConnTimeout  int
	ReadTimeout  int
	WriteTimeout int
}

type RedisManager struct {
	CacheManager
	DsName string
	Pool   *redis.Pool
}

func (self *RedisManager) InitConfig(input ...RedisConfig) (*RedisManager, error) {
	for _, v := range input {
		// 1. 配置参数校验
		if len(v.Host) == 0 {
			return nil, utils.Error("redis config invalid: host is required")
		}
		if v.Port <= 0 {
			return nil, utils.Error("redis config invalid: port is required")
		}

		// 2. 设置连接池默认值
		if v.MaxIdle <= 0 {
			v.MaxIdle = 10
		}
		if v.MaxActive <= 0 {
			v.MaxActive = 100
		}
		if v.IdleTimeout <= 0 {
			v.IdleTimeout = 300 // 5分钟
		}

		// 3. 设置网络和超时默认值
		if len(v.Network) == 0 {
			v.Network = "tcp"
		}
		connTimeout := 10
		readTimeout := 10
		writeTimeout := 10
		if v.ConnTimeout > 0 {
			connTimeout = v.ConnTimeout
		}
		if v.ReadTimeout > 0 {
			readTimeout = v.ReadTimeout
		}
		if v.WriteTimeout > 0 {
			writeTimeout = v.WriteTimeout
		}

		// 4. 生成数据源名称
		dsName := DIC.MASTER
		if len(v.DsName) > 0 {
			dsName = v.DsName
		}

		// 5. 并发安全检查：检查是否已存在
		redisMutex.Lock()
		if _, b := redisSessions[dsName]; b {
			redisMutex.Unlock()
			return nil, utils.Error("redis init failed: [", v.DsName, "] exist")
		}
		redisMutex.Unlock()

		// 6. 创建连接池
		pool := &redis.Pool{
			MaxIdle:     v.MaxIdle,
			MaxActive:   v.MaxActive,
			IdleTimeout: time.Duration(v.IdleTimeout) * time.Second,
			Dial: func() (redis.Conn, error) {
				c, err := redis.Dial(
					v.Network,
					utils.AddStr(v.Host, ":", utils.AnyToStr(v.Port)),
					redis.DialConnectTimeout(time.Duration(connTimeout)*time.Second), // 连接建立超时
					redis.DialReadTimeout(time.Duration(readTimeout)*time.Second),    // 读取超时
					redis.DialWriteTimeout(time.Duration(writeTimeout)*time.Second),  // 写入超时
				)
				if err != nil {
					return nil, err
				}
				// 密码认证
				if len(v.Password) > 0 {
					if _, err := c.Do("AUTH", v.Password); err != nil {
						if closeErr := c.Close(); closeErr != nil {
							zlog.Error("redis close failed", 0, zlog.AddError(closeErr))
						}
						return nil, err
					}
				}
				return c, nil
			},
		}

		// 7. 验证连接
		conn := pool.Get()
		defer conn.Close()
		if _, err := conn.Do("PING"); err != nil {
			return nil, utils.Error("redis connect failed: ", err)
		}

		// 8. 并发安全地注册数据源（再次检查避免重复）
		redisMutex.Lock()
		if _, b := redisSessions[dsName]; b {
			redisMutex.Unlock()
			return nil, utils.Error("redis init failed: [", v.DsName, "] exist (concurrent init)")
		}
		redisSessions[dsName] = &RedisManager{Pool: pool, DsName: dsName}
		redisMutex.Unlock()

		zlog.Printf("redis service【%s】has been started successful", dsName)
	}

	// 9. 验证至少初始化一个数据源
	redisMutex.RLock()
	defer redisMutex.RUnlock()
	if len(redisSessions) == 0 {
		return nil, utils.Error("redis init failed: sessions is nil")
	}

	return self, nil
}

func (self *RedisManager) Client(ds ...string) (*RedisManager, error) {
	dsName := DIC.MASTER
	if len(ds) > 0 && len(ds[0]) > 0 {
		dsName = ds[0]
	}

	redisMutex.RLock()
	manager := redisSessions[dsName]
	redisMutex.RUnlock()

	if manager == nil {
		return nil, utils.Error("redis session [", dsName, "] not found...")
	}
	return manager, nil
}

func NewRedis(ds ...string) (*RedisManager, error) {
	return new(RedisManager).Client(ds...)
}

/********************************** redis缓存接口实现 **********************************/

func (self *RedisManager) Get(key string, input interface{}) (interface{}, bool, error) {
	client := self.Pool.Get()
	defer self.Close(client)
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
	defer self.Close(client)
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
	defer self.Close(client)
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
	defer self.Close(client)
	value, err := redis.Bytes(client.Do("GET", key))
	if err != nil && err != redis.ErrNil {
		return "", err
	}
	if value == nil || len(value) == 0 {
		return "", nil
	}
	return utils.Bytes2Str(value), nil
}

func (self *RedisManager) GetBytes(key string) ([]byte, error) {
	client := self.Pool.Get()
	defer self.Close(client)
	value, err := redis.Bytes(client.Do("GET", key))
	if err != nil && err != redis.ErrNil {
		return nil, err
	}
	if value == nil || len(value) == 0 {
		return nil, nil
	}
	return value, nil
}

func (self *RedisManager) GetBool(key string) (bool, error) {
	client := self.Pool.Get()
	defer self.Close(client)
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
	var value []byte
	if v, b := input.([]byte); b {
		value = v
	} else {
		value = utils.Str2Bytes(utils.AnyToStr(input))
	}
	client := self.Pool.Get()
	defer self.Close(client)
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
	defer self.Close(client)
	if err := client.Send("MULTI"); err != nil {
		return err
	}
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
	defer self.Close(client)
	if err := client.Send("MULTI"); err != nil {
		return err
	}
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
	defer self.Close(client)
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
	defer self.Close(client)
	_, err := client.Do("RPUSH", key, utils.AnyToStr(val))
	if err != nil {
		return err
	}
	return nil
}

func (self *RedisManager) Publish(key string, val interface{}, try ...int) (bool, error) {
	if val == nil || len(key) == 0 {
		return false, nil
	}
	client := self.Pool.Get()
	defer self.Close(client)
	trySend := 5
	if try != nil && try[0] > 0 {
		trySend = try[0]
	}
	for i := 0; i < trySend; i++ {
		reply, err := client.Do("PUBLISH", key, utils.AnyToStr(val))
		if err != nil {
			return false, err
		}
		if r, b := reply.(int64); b && r > 0 {
			return true, nil
		}
		time.Sleep(time.Duration(100*i) * time.Millisecond)
	}
	return false, nil
}

func (self *RedisManager) Subscribe(key string, expSecond int, call func(msg string) (bool, error)) error {
	if call == nil || len(key) == 0 {
		return nil
	}
	if expSecond <= 0 {
		expSecond = 5
	}
	client := self.Pool.Get()
	defer self.Close(client)
	c := redis.PubSubConn{Conn: client}
	if err := c.Subscribe(key); err != nil {
		return err
	}
	for {
		switch v := c.ReceiveWithTimeout(time.Duration(expSecond) * time.Second).(type) {
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
	defer self.Close(client)
	return redis.NewScript(len(key), cmd).Do(client, args...)
}

func (self *RedisManager) Keys(pattern ...string) ([]string, error) {
	if pattern == nil || len(pattern) == 0 || pattern[0] == "*" {
		return nil, nil
	}
	client := self.Pool.Get()
	defer self.Close(client)
	keys, err := redis.Strings(client.Do("KEYS", pattern[0]))
	if err != nil {
		return nil, err
	}
	return keys, nil
}

func (self *RedisManager) Size(pattern ...string) (int, error) {
	keys, err := self.Keys(pattern...)
	if err != nil {
		return 0, err
	}
	return len(keys), nil
}

func (self *RedisManager) Values(pattern ...string) ([]interface{}, error) {
	return nil, utils.Error("No implementation method [Values] was found")
}

func (self *RedisManager) Exists(key string) (bool, error) {
	client := self.Pool.Get()
	defer self.Close(client)
	ret, err := client.Do("EXISTS", key)
	if err != nil {
		return false, err
	}
	b, err := redis.Int(ret, err)
	return b == 1, err
}

func (self *RedisManager) Flush() error {
	return utils.Error("No implementation method [Flush] was found")
}

func (self *RedisManager) Close(conn redis.Conn) {
	if err := conn.Close(); err != nil {
		zlog.Error("redis conn close failed", 0, zlog.AddError(err))
	}
}
