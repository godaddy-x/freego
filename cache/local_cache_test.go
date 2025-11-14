package cache

import (
	"strings"
	"testing"
	"time"
)

func TestLocalCache_BasicOperations(t *testing.T) {
	// 创建缓存实例
	cache := NewLocalCache(5, 10) // 5分钟默认过期，10分钟清理间隔

	// 测试Put操作
	err := cache.Put("test_key", "test_value")
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// 测试Get操作
	val, exists, err := cache.Get("test_key", nil)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if !exists {
		t.Fatal("Key should exist")
	}
	if val != "test_value" {
		t.Fatalf("Expected 'test_value', got %v", val)
	}

	// 测试GetString
	str, err := cache.GetString("test_key")
	if err != nil {
		t.Fatalf("GetString failed: %v", err)
	}
	if str != "test_value" {
		t.Fatalf("Expected 'test_value', got %s", str)
	}

	// 测试Exists
	exists, err = cache.Exists("test_key")
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if !exists {
		t.Fatal("Key should exist")
	}

	// 测试Del操作
	err = cache.Del("test_key")
	if err != nil {
		t.Fatalf("Del failed: %v", err)
	}

	// 验证删除后不存在
	_, exists, err = cache.Get("test_key", nil)
	if err != nil {
		t.Fatalf("Get after delete failed: %v", err)
	}
	if exists {
		t.Fatal("Key should not exist after deletion")
	}
}

func TestLocalCache_Expiration(t *testing.T) {
	cache := NewLocalCache(5, 10)

	// 测试带过期时间的Put
	err := cache.Put("expire_key", "expire_value", 1) // 1秒过期
	if err != nil {
		t.Fatalf("Put with expiration failed: %v", err)
	}

	// 立即获取应该存在
	_, exists, err := cache.Get("expire_key", nil)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if !exists {
		t.Fatal("Key should exist immediately after put")
	}

	// 等待过期
	time.Sleep(2 * time.Second)

	// 现在应该不存在
	_, exists, err = cache.Get("expire_key", nil)
	if err != nil {
		t.Fatalf("Get after expiration failed: %v", err)
	}
	if exists {
		t.Fatal("Key should not exist after expiration")
	}
}

func TestLocalCache_Size(t *testing.T) {
	cache := NewLocalCache(5, 10)

	// 添加一些数据
	cache.Put("key1", "value1")
	cache.Put("key2", "value2")
	cache.Put("key3", "value3")

	size, err := cache.Size()
	if err != nil {
		t.Fatalf("Size failed: %v", err)
	}
	if size <= 0 {
		t.Fatal("Size should be greater than 0")
	}
	t.Logf("Cache size: %d", size)
}

func TestLocalCache_Keys(t *testing.T) {
	// 创建缓存实例
	cache := NewLocalCache(5, 10)

	// 添加测试数据
	testKeys := []string{"user:1", "user:2", "cache:temp", "session:abc", "config:app"}
	for _, key := range testKeys {
		err := cache.Put(key, "test_value")
		if err != nil {
			t.Fatalf("Put failed for key %s: %v", key, err)
		}
	}

	// 测试获取所有键
	allKeys, err := cache.Keys()
	if err != nil {
		t.Fatalf("Keys() failed: %v", err)
	}

	// 验证所有键都被返回
	if len(allKeys) < len(testKeys) {
		t.Fatalf("Expected at least %d keys, got %d", len(testKeys), len(allKeys))
	}

	// 验证所有测试键都存在
	keyMap := make(map[string]bool)
	for _, key := range allKeys {
		keyMap[key] = true
	}

	for _, testKey := range testKeys {
		if !keyMap[testKey] {
			t.Fatalf("Test key %s not found in result", testKey)
		}
	}

	t.Logf("All keys: %v", allKeys)

	// 测试模式匹配 - 前缀匹配
	userKeys, err := cache.Keys("user:*")
	if err != nil {
		t.Fatalf("Keys with pattern 'user:*' failed: %v", err)
	}

	expectedUserKeys := 2 // user:1, user:2
	if len(userKeys) != expectedUserKeys {
		t.Fatalf("Expected %d user keys, got %d: %v", expectedUserKeys, len(userKeys), userKeys)
	}

	// 验证返回的键都以user:开头
	for _, key := range userKeys {
		if !strings.HasPrefix(key, "user:") {
			t.Fatalf("Key %s doesn't match pattern 'user:*'", key)
		}
	}

	t.Logf("User keys: %v", userKeys)

	// 测试精确匹配
	configKeys, err := cache.Keys("config:app")
	if err != nil {
		t.Fatalf("Keys with pattern 'config:app' failed: %v", err)
	}

	if len(configKeys) != 1 || configKeys[0] != "config:app" {
		t.Fatalf("Expected ['config:app'], got %v", configKeys)
	}

	t.Logf("Config keys: %v", configKeys)

	// 测试无匹配模式
	emptyKeys, err := cache.Keys("nonexistent:*")
	if err != nil {
		t.Fatalf("Keys with pattern 'nonexistent:*' failed: %v", err)
	}

	if len(emptyKeys) != 0 {
		t.Fatalf("Expected empty result for nonexistent pattern, got %v", emptyKeys)
	}
}
