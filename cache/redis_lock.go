package cache

import (
	"fmt"
	"github.com/garyburd/redigo/redis"
	"github.com/godaddy-x/freego/ex"
	"github.com/godaddy-x/freego/util"
	"time"
)

const (
	lockKey      = "redis:lock:"
	subscribeKey = "redis:subscribe:lock:"
)

var unlockScript = redis.NewScript(1, `
	if redis.call("get", KEYS[1]) == ARGV[1]
	then
		return redis.call("del", KEYS[1])
	else
		return 0
	end
`)

// Lock represents a held lock.
type Lock struct {
	resource string
	token    string
	conn     redis.Conn
	timeout  time.Duration // second
}

func (lock *Lock) key() string {
	return lockKey + lock.resource
}

func (lock *Lock) subscribeKey() string {
	return subscribeKey
}

func (lock *Lock) subscribeData() string {
	return subscribeKey + lock.resource
}

func (lock *Lock) tryLock() (ok bool, err error) {
	status, err := redis.String(lock.conn.Do("SET", lock.key(), lock.token, "EX", int64(lock.timeout/time.Second), "NX"))
	if err == redis.ErrNil {
		// The lock was not successful, it already exists.
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return status == "OK", nil
}

func (lock *Lock) unlock(spin ...bool) (err error) {
	_, err = unlockScript.Do(lock.conn, lock.key(), lock.token)
	if lock != nil && lock.conn != nil {
		if len(spin) > 0 && spin[0] {
			if _, err := lock.conn.Do("PUBLISH", lock.subscribeKey(), lock.subscribeData()); err != nil {
				fmt.Println("unlock publish failed:", err)
			}
		}
		lock.conn.Close()
	}
	return
}

func (self *RedisManager) getLockWithTimeout(conn redis.Conn, resource string, expSecond time.Duration, spin ...bool) (lock *Lock, ok bool, err error) {
	lock = &Lock{resource, util.GetSnowFlakeStrID(), conn, expSecond}
	ok, err = lock.tryLock()
	if !ok || err != nil {
		if len(spin) > 0 && spin[0] {
			return
		}
		conn.Close()
		lock = nil
	}
	return
}

func (self *RedisManager) SpinLockWithTimeout(resource string, expSecond int, call func() error) error {
	client := self.Pool.Get()
	lock, ok, err := self.getLockWithTimeout(client, resource, time.Duration(expSecond)*time.Second, true)
	if err != nil || !ok {
		if err := self.Subscribe(lock.subscribeKey(), expSecond, func(msg string) (bool, error) {
			if msg != lock.subscribeData() {
				return false, nil
			}
			lock, ok, err = self.getLockWithTimeout(client, resource, time.Duration(expSecond)*time.Second, true)
			if err != nil || !ok {
				return false, err
			}
			return true, nil
		}); err != nil {
			lock.conn.Close()
			lock = nil
			return ex.Throw{Code: ex.REDIS_LOCK_TIMEOUT, Msg: "spin locker acquire timeout", Err: err}
		}
	}
	err = call()
	lock.unlock(true)
	return err
}

func (self *RedisManager) TryLockWithTimeout(resource string, expSecond int, call func() error) error {
	if expSecond <= 0 {
		expSecond = 60
	}
	client := self.Pool.Get()
	lock, ok, err := self.getLockWithTimeout(client, resource, time.Duration(expSecond)*time.Second)
	if err != nil {
		return ex.Throw{Code: ex.REDIS_LOCK_ACQUIRE, Msg: "locker acquire failed", Err: err}
	}
	if !ok {
		return ex.Throw{Code: ex.REDIS_LOCK_PENDING, Msg: "locker pending"}
	}
	err = call()
	lock.unlock()
	return err
}

func SpinLocker(lockObj, errorMsg string, expSecond int, callObj func() error) error {
	redis, err := new(RedisManager).Client()
	if err != nil {
		return ex.Throw{Code: ex.CACHE, Msg: ex.CACHE_ERR, Err: err}
	}
	if err := redis.SpinLockWithTimeout(lockObj, expSecond, callObj); err != nil {
		if len(errorMsg) > 0 {
			r := ex.Catch(err)
			if r.Code == ex.REDIS_LOCK_ACQUIRE {
				return err
			} else if r.Code == ex.REDIS_LOCK_PENDING {
				return ex.Throw{Code: r.Code, Msg: errorMsg}
			} else if r.Code == ex.REDIS_LOCK_TIMEOUT {
				return err
			}
		}
		return err
	}
	return nil
}

func TryLocker(lockObj, errorMsg string, expSecond int, callObj func() error) error {
	redis, err := new(RedisManager).Client()
	if err != nil {
		return ex.Throw{Code: ex.CACHE, Msg: ex.CACHE_ERR, Err: err}
	}
	if err := redis.TryLockWithTimeout(lockObj, expSecond, callObj); err != nil {
		if len(errorMsg) > 0 {
			r := ex.Catch(err)
			if r.Code == ex.REDIS_LOCK_ACQUIRE {
				return err
			} else if r.Code == ex.REDIS_LOCK_PENDING {
				return ex.Throw{Code: r.Code, Msg: errorMsg}
			}
		}
		return err
	}
	return nil
}
