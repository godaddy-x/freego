package main

import (
	"fmt"
	"github.com/garyburd/redigo/redis"
	"github.com/godaddy-x/freego/cache"
	"reflect"
	"testing"
	"time"
)

var subkey = "test.subkey"

func init() {
	initRedis()
}

func expectPushed(t *testing.T, c redis.PubSubConn, message string, expected interface{}) {
	actual := c.Receive()
	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("%s = %v, want %v", message, actual, expected)
	}
}

func TestRedisPublish(t *testing.T) {
	mgr, err := cache.NewRedis()
	if err != nil {
		panic(err)
	}
	b, err := mgr.Publish("test_123456_uid", "uid_orderNo_success")
	if err != nil {
		panic(err)
	}
	fmt.Println("send success: ", b)
}

func TestRedisSubscribe(t *testing.T) {
	mgr, err := cache.NewRedis()
	if err != nil {
		panic(err)
	}
	mgr.Subscribe("test_123456_uid", 5, func(msg string) (bool, error) {
		if err != nil {
			fmt.Println("read error: ", err)
			return false, err
		}
		fmt.Println("read msg:", msg)
		return true, nil
	})
}

func TestRedisSubscribe2(t *testing.T) {
	mgr, err := new(cache.RedisManager).Client()
	if err != nil {
		panic(err)
	}
	mgr.Subscribe(subkey, 0, func(msg string) (bool, error) {
		fmt.Println("subscribe:", msg)
		return false, nil
	})
}

func TestRedisSpinLocker(t *testing.T) {
	mgr, err := new(cache.RedisManager).Client()
	if err != nil {
		panic(err)
	}
	if err := mgr.SpinLockWithTimeout("spinlock", 20, 20, func() error {
		fmt.Println("test1 spin lock successful")
		time.Sleep(15 * time.Second)
		return nil
	}); err != nil {
		fmt.Println(err)
	}
}

func TestRedisSpinLocker2(t *testing.T) {
	mgr, err := new(cache.RedisManager).Client()
	if err != nil {
		panic(err)
	}
	if err := mgr.SpinLockWithTimeout("spinlock", 20, 20, func() error {
		fmt.Println("test2 spin lock successful")
		return nil
	}); err != nil {
		fmt.Println(err)
	}
}

func TestRedisSpinLocker3(t *testing.T) {
	mgr, err := new(cache.RedisManager).Client()
	if err != nil {
		panic(err)
	}
	if err := mgr.SpinLockWithTimeout("spinlock", 20, 20, func() error {
		fmt.Println("test3 spin lock successful")
		return nil
	}); err != nil {
		fmt.Println(err)
	}
}

func TestRedisTryLocker1(t *testing.T) {
	mgr, err := new(cache.RedisManager).Client()
	if err != nil {
		panic(err)
	}
	if err := mgr.TryLockWithTimeout("trylock", 20, func() error {
		fmt.Println("test1 try lock successful")
		time.Sleep(15 * time.Second)
		return nil
	}); err != nil {
		fmt.Println(err)
	}
}

func TestRedisTryLocker2(t *testing.T) {
	mgr, err := new(cache.RedisManager).Client()
	if err != nil {
		panic(err)
	}
	if err := mgr.TryLockWithTimeout("trylock", 20, func() error {
		fmt.Println("test2 try lock successful")
		return nil
	}); err != nil {
		fmt.Println(err)
	}
}
