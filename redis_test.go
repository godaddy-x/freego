package main

import (
	"fmt"
	"github.com/garyburd/redigo/redis"
	"github.com/godaddy-x/freego/cache"
	"github.com/godaddy-x/freego/utils"
	"reflect"
	"sync"
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
	for i := 0; i < 1000; i++ {
		go func(int2 int) {
			if err := cache.TryLocker("trylock", utils.AddStr("object: ", int2), 20, func() error {
				fmt.Println("test2 try lock successful: ", int2)
				time.Sleep(10 * time.Second)
				return nil
			}); err != nil {
				fmt.Println(err)
			}
		}(i)
	}
	select {}
}

func TestRedisGetAndSet(t *testing.T) {
	for i := 0; i < 2000; i++ {
		go func(int2 int) {
			rds, err := cache.NewRedis()
			if err != nil {
				panic(err)
			}
			key := utils.MD5("123456")
			if err := rds.Put(key, 1, 30); err != nil {
				fmt.Println(1, int2, err)
			}
			fmt.Println("success: ", int2)
		}(i)
	}

	select {}

}

func TestRedisGetAndSet1(t *testing.T) {
	rds, err := cache.NewRedis()
	if err != nil {
		t.Fatal(err)
	}

	// 预计算key避免重复计算
	key := utils.MD5("123456")

	// 使用sync.WaitGroup等待所有goroutine完成
	var wg sync.WaitGroup
	concurrency := 500 // 测试并发量

	// 可选：限制最大并发（推荐）
	sem := make(chan struct{}, 500) // 500并发上限

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// 限流控制（如果启用）
			sem <- struct{}{}
			defer func() { <-sem }()

			// 测试PUT操作
			if err := rds.Put(key, id, 30); err != nil {
				t.Logf("goroutine %d failed: %v", id, err)
			}
		}(i)
	}

	wg.Wait() // 等待所有goroutine完成
	t.Log("Stress test completed")
}

func TestLocalCacheGetAndSet(t *testing.T) {
	rds := cache.NewLocalCache(10, 10)
	key := utils.MD5("123456")
	if err := rds.Put(key, 1, 30); err != nil {
		panic(err)
	}
	value, err := rds.Exists(key)
	if err != nil {
		panic(err)
	}
	fmt.Println("---", value)
}

func BenchmarkRedisGetAndSet(b *testing.B) {
	b.StopTimer()
	b.StartTimer()
	rds, err := cache.NewRedis()
	if err != nil {
		panic(err)
	}
	for i := 0; i < b.N; i++ { //use b.N for looping
		key := utils.MD5(utils.NextSID())
		if err := rds.Put(key, 1, 30); err != nil {
			panic(err)
		}
		_, err := rds.Exists(key)
		if err != nil {
			panic(err)
		}
	}
}

func BenchmarkLocalCacheGetAndSet(b *testing.B) {
	b.StopTimer()
	b.StartTimer()
	rds := cache.NewLocalCache(10, 10)
	for i := 0; i < b.N; i++ { //use b.N for looping
		key := utils.MD5(utils.NextSID())
		if err := rds.Put(key, 1, 30); err != nil {
			panic(err)
		}
		_, err := rds.Exists(key)
		if err != nil {
			panic(err)
		}
	}
}
