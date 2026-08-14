// Package rabbitmq 发布功能测试
//
// 运行实际环境测试说明：
// 1. 确保RabbitMQ服务正在运行
// 2. 配置文件位于 ../resource/rabbitmq.json
// 3. 运行测试：go test -run TestRealEnvironmentPublish -v
// 4. 测试会创建测试交换机和队列，发送各种类型的消息
//
// 如果没有真实的RabbitMQ环境，可以使用Docker启动临时实例：
// docker run -d --name rabbitmq-test -p 5672:5672 -p 15672:15672 rabbitmq:3-management
package rabbitmq

import (
	"encoding/json"
	"io/ioutil"
	"testing"
	"time"
)

// loadTestConfig 加载测试配置
func loadTestConfig(t *testing.T) AmqpConfig {
	configData, err := ioutil.ReadFile("../resource/rabbitmq.json")
	if err != nil {
		t.Fatalf("Failed to read RabbitMQ config file: %v", err)
	}

	var conf AmqpConfig
	if err := json.Unmarshal(configData, &conf); err != nil {
		t.Fatalf("Failed to parse RabbitMQ config: %v", err)
	}
	return conf
}

// setupTestManager 创建测试用的发布管理器
func setupTestManager(t *testing.T, dsName string) *PublishManager {
	conf := loadTestConfig(t)
	conf.DsName = dsName

	_ = new(PublishManager).InitConfig(conf)

	mgr, err := NewPublish()
	if err != nil {
		t.Fatalf("Failed to create publish manager: %v", err)
	}
	return mgr
}

// TestPublishObject 对象序列化发布测试
func TestPublishObject(t *testing.T) {
	mgr := setupTestManager(t, "")
	defer func() {
		if err := mgr.Close(); err != nil {
			t.Errorf("Failed to close publish manager: %v", err)
		}
	}()

	for i := 0; i < 100000000; i++ {
		// 测试对象 - 将被自动序列化为JSON
		testObject := map[string]interface{}{
			"id":        12345,
			"message":   "test object message",
			"timestamp": time.Now().Unix(),
			"type":      "object_test",
			"nested": map[string]interface{}{
				"key":   "value",
				"count": 42,
				"index": i,
			},
		}

		// 使用PublishObject方法发布对象
		err := mgr.Publish("test.exchange", "test.queue", 1, testObject)
		if err != nil {
			t.Errorf("PublishObject failed: %v", err)
		}

		t.Log("PublishObject tests passed: ", i)

		time.Sleep(time.Duration(100) * time.Millisecond)
	}

}
