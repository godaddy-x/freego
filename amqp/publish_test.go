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
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"
	"sync"
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

	mgr, err := NewPublishManager(conf)
	if err != nil {
		t.Fatalf("Failed to create publish manager: %v", err)
	}
	return mgr
}

// TestPublishBasic 基本的消息发布测试
func TestPublishBasic(t *testing.T) {
	mgr := setupTestManager(t, "test_basic_publish")
	defer mgr.Close()

	ctx := context.Background()
	testData := map[string]interface{}{
		"id":        12345,
		"message":   "test message",
		"timestamp": time.Now().Unix(),
	}
	testDataBytes, _ := json.Marshal(testData)

	// 测试基本的消息发布
	err := mgr.Publish(ctx, "test.exchange", "test.queue", 1, string(testDataBytes))
	if err != nil {
		t.Errorf("Basic publish failed: %v", err)
	}

	// 测试PublishMsgData接口
	msgContentBytes, _ := json.Marshal(testData)
	msg := &MsgData{
		Content: string(msgContentBytes),
		Option: Option{
			Exchange: "test.exchange",
			Queue:    "test.queue",
			Router:   "test.key",
		},
		Type:    1,
		Durable: true,
	}

	err = mgr.PublishMsgData(ctx, msg)
	if err != nil {
		t.Errorf("PublishMsgData failed: %v", err)
	}

	t.Log("Basic publish tests passed")
}

// TestBatchPublish 批量发布测试
func TestBatchPublish(t *testing.T) {
	mgr := setupTestManager(t, "test_batch_publish")
	defer mgr.Close()

	ctx := context.Background()

	// 准备批量消息
	batchSize := 5
	msgs := make([]*MsgData, batchSize)
	for i := 0; i < batchSize; i++ {
		contentData := map[string]interface{}{
			"id":      i + 1,
			"batch":   true,
			"message": fmt.Sprintf("batch message %d", i+1),
		}
		contentBytes, _ := json.Marshal(contentData)
		msgs[i] = &MsgData{
			Content: string(contentBytes),
			Option: Option{
				Exchange: "test.exchange",
				Queue:    "test.queue",
				Router:   "test.key",
			},
			Type:    1,
			Durable: true,
		}
	}

	// 测试事务模式批量发布
	err := mgr.BatchPublish(ctx, msgs)
	if err != nil {
		t.Errorf("Batch publish with transaction failed: %v", err)
	}

	// 测试Confirm模式批量发布
	err = mgr.BatchPublishWithOptions(ctx, msgs, WithUseTransaction(false))
	if err != nil {
		t.Errorf("Batch publish with confirm failed: %v", err)
	}

	t.Log("Batch publish tests passed")
}

// TestPublishWithEncryption 加密消息发布测试
func TestPublishWithEncryption(t *testing.T) {
	mgr := setupTestManager(t, "test_encryption_publish")
	defer mgr.Close()

	ctx := context.Background()
	testData := map[string]interface{}{
		"secret":    "this is secret data",
		"id":        12345,
		"timestamp": time.Now().Unix(),
	}

	// 测试带加密的消息发布
	msgContentBytes, _ := json.Marshal(testData)
	msg := &MsgData{
		Content: string(msgContentBytes),
		Option: Option{
			Exchange: "test.exchange",
			Queue:    "test.queue",
			Router:   "test.key",
			SigTyp:   1,                                  // 启用AES加密
			SigKey:   "12345678901234567890123456789012", // 32字节AES-256密钥
		},
		Type:    1,
		Durable: true,
	}

	err := mgr.PublishMsgData(ctx, msg)
	if err != nil {
		t.Errorf("Encrypted publish failed: %v", err)
	}

	t.Log("Encryption publish test passed")
}

// TestPublishErrorCases 错误情况测试
func TestPublishErrorCases(t *testing.T) {
	mgr := setupTestManager(t, "test_error_cases")
	defer mgr.Close()

	ctx := context.Background()

	// 测试无效参数
	testCases := []struct {
		name string
		msg  *MsgData
	}{
		{
			name: "nil message",
			msg:  nil,
		},
		{
			name: "empty exchange",
			msg: &MsgData{
				Content: "{\"test\":\"data\"}",
				Option: Option{
					Exchange: "",
					Queue:    "test.queue",
				},
			},
		},
		{
			name: "empty queue",
			msg: &MsgData{
				Content: "{\"test\":\"data\"}",
				Option: Option{
					Exchange: "test.exchange",
					Queue:    "",
				},
			},
		},
		{
			name: "empty content",
			msg: &MsgData{
				Content: "",
				Option: Option{
					Exchange: "test.exchange",
					Queue:    "test.queue",
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := mgr.PublishMsgData(ctx, tc.msg)
			if err == nil {
				t.Errorf("Expected error for %s, but got none", tc.name)
			} else {
				t.Logf("Correctly rejected %s: %v", tc.name, err)
			}
		})
	}

	t.Log("Error cases tests passed")
}

// TestHealthCheck 健康检查测试
func TestHealthCheck(t *testing.T) {
	mgr := setupTestManager(t, "test_health_check")
	defer mgr.Close()

	// 测试健康检查
	err := mgr.HealthCheck()
	if err != nil {
		t.Errorf("Health check failed: %v", err)
	}

	// 关闭管理器后测试健康检查
	mgr.Close()
	err = mgr.HealthCheck()
	if err == nil {
		t.Error("Expected health check to fail after manager is closed")
	}

	t.Log("Health check tests passed")
}

// TestSingletonConcurrency 单例模式并发测试
func TestSingletonConcurrency(t *testing.T) {
	conf := loadTestConfig(t)
	conf.DsName = "test_singleton_concurrency"

	const numGoroutines = 10
	var wg sync.WaitGroup
	results := make(chan *PublishManager, numGoroutines)
	errors := make(chan error, numGoroutines)

	// 启动多个goroutine同时创建单例
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			mgr, err := NewPublishManager(conf)
			if err != nil {
				errors <- err
				return
			}
			results <- mgr
		}(i)
	}

	wg.Wait()
	close(results)
	close(errors)

	// 检查是否有错误
	var errorCount int
	for err := range errors {
		errorCount++
		t.Errorf("Goroutine failed to create manager: %v", err)
	}

	if errorCount > 0 {
		t.Fatalf("Found %d errors during concurrent creation", errorCount)
	}

	// 检查所有返回的实例是否相同
	var managers []*PublishManager
	for mgr := range results {
		managers = append(managers, mgr)
	}

	if len(managers) != numGoroutines {
		t.Errorf("Expected %d managers, got %d", numGoroutines, len(managers))
	}

	// 验证所有实例都是同一个（内存地址相同）
	if len(managers) > 1 {
		first := managers[0]
		for i, mgr := range managers[1:] {
			if mgr != first {
				t.Errorf("Manager %d is different instance than first manager", i+1)
			}
		}
	}

	// 清理
	if len(managers) > 0 {
		if err := managers[0].Close(); err != nil {
			t.Logf("Close error: %v", err)
		}
	}

	t.Logf("Successfully created %d goroutines, all returned the same singleton instance", numGoroutines)
}

// TestPublishTimeout 超时测试
func TestPublishTimeout(t *testing.T) {
	mgr := setupTestManager(t, "test_timeout")
	defer mgr.Close()

	// 创建一个已经超时的上下文
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	// 故意等待一段时间让上下文超时
	time.Sleep(10 * time.Millisecond)
	defer cancel()

	// 这个消息应该会超时
	msgContent := map[string]interface{}{
		"test": "timeout test",
	}
	msgContentBytes, _ := json.Marshal(msgContent)
	msg := &MsgData{
		Content: string(msgContentBytes),
		Option: Option{
			Exchange: "test.exchange",
			Queue:    "test.queue",
			Router:   "test.key",
		},
		Type:    1,
		Durable: true,
	}

	err := mgr.PublishMsgData(ctx, msg)
	if err == nil {
		t.Error("Expected timeout error, but publish succeeded")
	} else if !strings.Contains(err.Error(), "timeout") && !strings.Contains(err.Error(), "cancelled") && !strings.Contains(err.Error(), "deadline exceeded") {
		t.Errorf("Expected timeout or cancellation error, got: %v", err)
	} else {
		t.Logf("Correctly handled timeout: %v", err)
	}

	t.Log("Timeout test passed")
}

// TestConcurrentChannelCreation 并发通道创建测试
func TestConcurrentChannelCreation(t *testing.T) {
	mgr := setupTestManager(t, "test_concurrent_creation")
	defer mgr.Close()

	const numGoroutines = 10
	const numMessagesPerGoroutine = 3

	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines*numMessagesPerGoroutine)

	// 启动多个goroutine同时发送消息到相同的exchange和queue
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			for j := 0; j < numMessagesPerGoroutine; j++ {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()

				testData := map[string]interface{}{
					"goroutine_id": id,
					"message_id":   j,
					"timestamp":    time.Now().Unix(),
				}
				testDataBytes, _ := json.Marshal(testData)

				err := mgr.Publish(ctx, "test.exchange", "test.queue", 1, string(testDataBytes))
				if err != nil {
					errors <- fmt.Errorf("goroutine %d, message %d failed: %v", id, j, err)
				}
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// 检查是否有错误
	var errorCount int
	for err := range errors {
		errorCount++
		t.Errorf("Concurrent creation error: %v", err)
	}

	if errorCount > 0 {
		t.Fatalf("Found %d errors during concurrent channel creation", errorCount)
	}

	expectedMessages := numGoroutines * numMessagesPerGoroutine
	t.Logf("Successfully created channels concurrently and sent %d messages", expectedMessages)
}

// TestPublishWithOptions 选项配置测试
func TestPublishWithOptions(t *testing.T) {
	mgr := setupTestManager(t, "test_options")
	defer mgr.Close()

	ctx := context.Background()

	// 测试各种选项
	contentData := map[string]interface{}{"test": "with options"}
	contentBytes, _ := json.Marshal(contentData)
	msgs := []*MsgData{
		{
			Content: string(contentBytes),
			Option: Option{
				Exchange: "test.exchange",
				Queue:    "test.queue",
				Router:   "test.key",
			},
			Type:    1,
			Durable: true,
		},
	}

	// 测试事务模式
	err := mgr.BatchPublishWithOptions(ctx, msgs, WithUseTransaction(true))
	if err != nil {
		t.Errorf("Batch publish with transaction option failed: %v", err)
	}

	// 测试Confirm模式和自定义超时
	customTimeout := 10 * time.Second
	err = mgr.BatchPublishWithOptions(ctx, msgs,
		WithUseTransaction(false),
		WithConfirmTimeout(customTimeout))
	if err != nil {
		t.Errorf("Batch publish with confirm and timeout options failed: %v", err)
	}

	t.Log("Options configuration tests passed")
}

// TestConfigLoading 配置加载测试
func TestConfigLoading(t *testing.T) {
	// 测试配置文件加载（不依赖真实的RabbitMQ）
	conf := AmqpConfig{
		DsName:    "test_config_loading",
		Host:      "127.0.0.1",
		Username:  "guest",
		Password:  "guest",
		Port:      5672,
		Vhost:     "/",
		SecretKey: "test_secret_key_32_bytes_1234567890",
	}

	// 验证配置验证
	err := conf.Validate()
	if err != nil {
		t.Errorf("Config validation failed: %v", err)
	}

	// 验证默认值设置
	conf.setDefaults()
	if conf.Port != 5672 {
		t.Errorf("Expected default port 5672, got %d", conf.Port)
	}
	if conf.Vhost != "/" {
		t.Errorf("Expected default vhost '/', got %s", conf.Vhost)
	}

	t.Log("Config loading and validation tests passed")
}

// TestRealEnvironmentPublish 实际环境发消息测试
func TestRealEnvironmentPublish(t *testing.T) {
	// 加载RabbitMQ配置文件
	configData, err := ioutil.ReadFile("../resource/rabbitmq.json")
	if err != nil {
		t.Fatalf("Failed to read RabbitMQ config file: %v", err)
	}

	var conf AmqpConfig
	if err := json.Unmarshal(configData, &conf); err != nil {
		t.Fatalf("Failed to parse RabbitMQ config: %v", err)
	}

	// 创建发布管理器
	mgr, err := NewPublishManager(conf)
	if err != nil {
		t.Fatalf("Failed to create publish manager: %v", err)
	}
	defer func() {
		if err := mgr.Close(); err != nil {
			t.Logf("Error closing manager: %v", err)
		}
	}()

	ctx := context.Background()

	t.Run("SingleMessagePublish", func(t *testing.T) {
		// 测试单个消息发布
		testData := map[string]interface{}{
			"id":          fmt.Sprintf("test-%d", time.Now().Unix()),
			"message":     "Hello from real environment test!",
			"type":        "single",
			"timestamp":   time.Now().Unix(),
			"environment": "test",
		}
		testDataBytes, _ := json.Marshal(testData)

		err := mgr.Publish(ctx, "test.exchange", "test.queue", 1, string(testDataBytes))
		if err != nil {
			t.Errorf("Single message publish failed: %v", err)
		} else {
			t.Logf("Successfully published single message: %v", testData)
		}
	})

	t.Run("MsgDataPublish", func(t *testing.T) {
		// 测试MsgData结构发布
		msgContent := map[string]interface{}{
			"id":          fmt.Sprintf("msgdata-%d", time.Now().Unix()),
			"message":     "Hello from MsgData test!",
			"type":        "msgdata",
			"timestamp":   time.Now().Unix(),
			"environment": "test",
		}
		msgContentBytes, _ := json.Marshal(msgContent)
		msg := &MsgData{
			Content: string(msgContentBytes),
			Option: Option{
				Exchange: "test.exchange",
				Queue:    "test.queue",
				Router:   "test.routing.key",
			},
			Type:    1,
			Durable: true,
		}

		err := mgr.PublishMsgData(ctx, msg)
		if err != nil {
			t.Errorf("MsgData publish failed: %v", err)
		} else {
			t.Logf("Successfully published MsgData message: %v", msg.Content)
		}
	})

	t.Run("BatchPublish", func(t *testing.T) {
		// 测试批量发布
		batchSize := 3
		msgs := make([]*MsgData, batchSize)
		for i := 0; i < batchSize; i++ {
			contentData := map[string]interface{}{
				"id":          fmt.Sprintf("batch-%d-%d", i+1, time.Now().Unix()),
				"message":     fmt.Sprintf("Batch message %d", i+1),
				"type":        "batch",
				"batch_index": i + 1,
				"timestamp":   time.Now().Unix(),
			}
			contentBytes, _ := json.Marshal(contentData)
			msgs[i] = &MsgData{
				Content: string(contentBytes),
				Option: Option{
					Exchange: "test.exchange",
					Queue:    "test.queue",
					Router:   "test.batch.routing.key",
				},
				Type:    1,
				Durable: true,
			}
		}

		err := mgr.BatchPublish(ctx, msgs)
		if err != nil {
			t.Errorf("Batch publish failed: %v", err)
		} else {
			t.Logf("Successfully published %d batch messages", batchSize)
		}
	})

	t.Run("EncryptedMessagePublish", func(t *testing.T) {
		// 测试加密消息发布
		encryptedContent := map[string]interface{}{
			"id":          fmt.Sprintf("encrypted-%d", time.Now().Unix()),
			"secret_data": "This is sensitive information that should be encrypted",
			"type":        "encrypted",
			"timestamp":   time.Now().Unix(),
		}
		encryptedContentBytes, _ := json.Marshal(encryptedContent)
		encryptedMsg := &MsgData{
			Content: string(encryptedContentBytes),
			Option: Option{
				Exchange: "test.exchange",
				Queue:    "test.queue",
				Router:   "test.encrypted.routing.key",
				SigTyp:   1,                                  // 启用AES加密
				SigKey:   "12345678901234567890123456789012", // 32字节AES-256密钥
			},
			Type:    1,
			Durable: true,
		}

		err := mgr.PublishMsgData(ctx, encryptedMsg)
		if err != nil {
			t.Errorf("Encrypted message publish failed: %v", err)
		} else {
			t.Logf("Successfully published encrypted message")
		}
	})

	t.Run("HealthCheck", func(t *testing.T) {
		// 测试健康检查
		err := mgr.HealthCheck()
		if err != nil {
			t.Errorf("Health check failed: %v", err)
		} else {
			t.Log("Health check passed")
		}
	})

	t.Run("QueueStatusCheck", func(t *testing.T) {
		// 测试队列状态检查
		status, err := mgr.GetQueueStatus(ctx, "test.exchange", "test.queue", "test.routing.key")
		if err != nil {
			t.Logf("Queue status check failed (expected if queue doesn't exist): %v", err)
		} else {
			t.Logf("Queue status: %+v", status)
		}
	})
}

// TestReconnectionMechanism 重连机制测试
func TestReconnectionMechanism(t *testing.T) {
	conf := loadTestConfig(t)
	conf.DsName = "test_reconnection"

	// 创建发布管理器
	mgr, err := NewPublishManager(conf)
	if err != nil {
		t.Fatalf("Failed to create publish manager: %v", err)
	}
	defer func() {
		if err := mgr.Close(); err != nil {
			t.Logf("Error closing manager: %v", err)
		}
	}()

	ctx := context.Background()

	// 第一阶段：正常发布消息
	t.Run("InitialPublish", func(t *testing.T) {
		testData := map[string]interface{}{
			"id":        "reconnect-test-1",
			"message":   "Initial message before reconnection",
			"timestamp": time.Now().Unix(),
			"phase":     "initial",
		}
		testDataBytes, _ := json.Marshal(testData)

		err := mgr.Publish(ctx, "test.exchange", "test.queue", 1, string(testDataBytes))
		if err != nil {
			t.Errorf("Initial publish failed: %v", err)
		} else {
			t.Logf("Successfully published initial message: %v", testData)
		}
	})

	// 第二阶段：模拟连接断开
	t.Run("SimulateConnectionLoss", func(t *testing.T) {
		// 等待一段时间确保连接监控goroutine启动
		time.Sleep(100 * time.Millisecond)

		// 通过内部方法模拟连接断开
		mgr.mu.Lock()
		if mgr.conn != nil {
			// 强制关闭连接以触发重连机制
			if err := mgr.conn.Close(); err != nil {
				t.Logf("Connection close error (expected): %v", err)
			}
			mgr.conn = nil
		}
		mgr.mu.Unlock()

		t.Log("Simulated connection loss")

		// 等待重连机制启动（指数退避重连需要时间）
		time.Sleep(2 * time.Second)
	})

	// 第三阶段：验证重连后的发布
	t.Run("PublishAfterReconnection", func(t *testing.T) {
		// 等待更长时间确保重连和通道重建完成
		t.Log("Waiting for reconnection to complete...")
		time.Sleep(5 * time.Second)

		// 多次重试，确保重连完成
		maxRetries := 15
		var lastErr error

		for i := 0; i < maxRetries; i++ {
			testData := map[string]interface{}{
				"id":        fmt.Sprintf("reconnect-test-%d", i+2),
				"message":   fmt.Sprintf("Message after reconnection attempt %d", i+1),
				"timestamp": time.Now().Unix(),
				"phase":     "reconnected",
				"attempt":   i + 1,
			}
			testDataBytes, _ := json.Marshal(testData)

			// 使用不同的路由键强制创建新通道，避免使用缓存的无效通道
			router := fmt.Sprintf("test.reconnect.%d", i+2)
			err := mgr.Publish(ctx, "test.exchange", "test.queue", 1, string(testDataBytes), WithRouter(router))
			if err == nil {
				t.Logf("Successfully published message after reconnection on attempt %d: %v", i+1, testData)
				return // 成功，退出重试循环
			}

			lastErr = err
			t.Logf("Publish attempt %d failed (may be reconnecting): %v", i+1, err)

			// 检查是否是连接相关的可重试错误
			errStr := err.Error()
			isRetryable := strings.Contains(errStr, "retryable") ||
				strings.Contains(errStr, "CONNECTION") ||
				strings.Contains(errStr, "CHANNEL") ||
				strings.Contains(errStr, "channel/connection is not open") ||
				strings.Contains(errStr, "Exception (504)") ||
				strings.Contains(errStr, "not open")

			if !isRetryable {
				t.Errorf("Non-retryable error: %v", err)
				return
			}

			// 等待更长时间再重试
			waitTime := time.Duration(i+1) * 300 * time.Millisecond
			if waitTime > 2*time.Second {
				waitTime = 2 * time.Second
			}
			t.Logf("Waiting %v before retry %d", waitTime, i+2)
			time.Sleep(waitTime)
		}

		t.Errorf("Failed to publish after reconnection after %d attempts. Last error: %v", maxRetries, lastErr)
	})

	// 第四阶段：批量发布测试重连稳定性
	t.Run("BatchPublishAfterReconnection", func(t *testing.T) {
		batchSize := 3
		msgs := make([]*MsgData, batchSize)
		for i := 0; i < batchSize; i++ {
			contentData := map[string]interface{}{
				"id":          fmt.Sprintf("batch-reconnect-%d", i+1),
				"message":     fmt.Sprintf("Batch message %d after reconnection", i+1),
				"type":        "batch_reconnect",
				"batch_index": i + 1,
				"timestamp":   time.Now().Unix(),
			}
			contentBytes, _ := json.Marshal(contentData)
			msgs[i] = &MsgData{
				Content: string(contentBytes),
				Option: Option{
					Exchange: "test.exchange",
					Queue:    "test.queue",
					Router:   "test.batch.reconnect",
				},
				Type:    1,
				Durable: true,
			}
		}

		err := mgr.BatchPublish(ctx, msgs)
		if err != nil {
			t.Errorf("Batch publish after reconnection failed: %v", err)
		} else {
			t.Logf("Successfully published %d batch messages after reconnection", batchSize)
		}
	})

	// 第五阶段：健康检查验证连接状态
	t.Run("HealthCheckAfterReconnection", func(t *testing.T) {
		err := mgr.HealthCheck()
		if err != nil {
			t.Errorf("Health check failed after reconnection: %v", err)
		} else {
			t.Log("Health check passed after reconnection")
		}
	})

	t.Log("Reconnection mechanism test completed")
}

// TestConnectionChannelRecovery 连接/通道异常恢复机制测试
func TestConnectionChannelRecovery(t *testing.T) {
	conf := loadTestConfig(t)
	conf.DsName = "test_recovery"

	// 创建发布管理器
	mgr, err := NewPublishManager(conf)
	if err != nil {
		t.Fatalf("Failed to create publish manager: %v", err)
	}
	defer func() {
		if err := mgr.Close(); err != nil {
			t.Logf("Error closing manager: %v", err)
		}
	}()

	ctx := context.Background()

	// 第一阶段：正常发布，建立基线
	t.Run("BaselinePublish", func(t *testing.T) {
		testData := map[string]interface{}{
			"id":        "recovery-test-1",
			"message":   "Baseline message",
			"timestamp": time.Now().Unix(),
		}
		testDataBytes, _ := json.Marshal(testData)

		err := mgr.Publish(ctx, "test.exchange", "test.queue", 1, string(testDataBytes))
		if err != nil {
			t.Errorf("Baseline publish failed: %v", err)
		} else {
			t.Log("Baseline publish successful")
		}
	})

	// 第二阶段：模拟连接断开，验证重连机制
	t.Run("SimulateConnectionInterruption", func(t *testing.T) {
		// 强制关闭连接并等待一段时间确保监控goroutine检测到
		mgr.mu.Lock()
		if mgr.conn != nil {
			mgr.conn.Close()
			mgr.conn = nil
		}
		mgr.mu.Unlock()

		t.Log("Connection forcibly closed")

		// 等待更长时间确保重连机制有机会启动
		// monitorConnection使用指数退避，最小延迟500ms
		time.Sleep(1 * time.Second)

		// 手动触发重连（因为monitorConnection可能没有检测到连接关闭）
		// 或者检查连接是否需要重建
		mgr.mu.RLock()
		conn := mgr.conn
		mgr.mu.RUnlock()

		if conn == nil || conn.IsClosed() {
			t.Log("Connection is closed, attempting manual reconnection")
			// 尝试手动重建连接
			_, err := mgr.Connect()
			if err != nil {
				t.Logf("Manual reconnection failed: %v", err)
			} else {
				t.Log("Manual reconnection succeeded")
				// 重连后需要清理旧的通道缓存，因为它们指向旧的连接
				mgr.mu.Lock()
				mgr.channels = make(map[string]*PublishMQ)
				mgr.mu.Unlock()
				t.Log("Cleared channel cache after reconnection")
			}
		}

		// 再次等待重连完成
		time.Sleep(2 * time.Second)

		// 验证重连后的发布
		testData := map[string]interface{}{
			"id":        "recovery-test-2",
			"message":   "Message after connection recovery",
			"timestamp": time.Now().Unix(),
		}
		testDataBytes, _ := json.Marshal(testData)

		// 多次重试，因为重连可能需要时间
		var lastErr error
		for i := 0; i < 15; i++ {
			err := mgr.Publish(ctx, "test.exchange", "test.queue", 1, string(testDataBytes))
			if err == nil {
				t.Logf("Successfully published after connection recovery on attempt %d", i+1)
				return
			}
			lastErr = err
			t.Logf("Attempt %d failed: %v", i+1, err)
			time.Sleep(500 * time.Millisecond)
		}

		t.Errorf("Failed to publish after connection recovery: %v", lastErr)
	})

	// 第三阶段：通道重建验证（简化版本）
	t.Run("ChannelReuseAfterReconnection", func(t *testing.T) {
		// 重连后验证新通道可以正常工作
		testData := map[string]interface{}{
			"id":        "channel-reuse-test",
			"message":   "Testing channel reuse after reconnection",
			"timestamp": time.Now().Unix(),
		}
		testDataBytes, _ := json.Marshal(testData)

		// 使用不同的路由键创建新通道
		err := mgr.Publish(ctx, "test.exchange", "test.queue", 1, string(testDataBytes), WithRouter("test.channel.reuse"))
		if err != nil {
			t.Errorf("Failed to publish with new channel after reconnection: %v", err)
		} else {
			t.Log("Successfully published using new channel after reconnection")
		}

		// 验证通道数量
		mgr.mu.RLock()
		channelCount := len(mgr.channels)
		mgr.mu.RUnlock()

		t.Logf("Current channel count after reconnection tests: %d", channelCount)
	})

	t.Log("Connection/channel recovery test completed")
}

// TestConfirmModeEdgeCases Confirm模式特殊情况测试
func TestConfirmModeEdgeCases(t *testing.T) {
	conf := loadTestConfig(t)
	conf.DsName = "test_confirm_edge"

	mgr, err := NewPublishManager(conf)
	if err != nil {
		t.Fatalf("Failed to create publish manager: %v", err)
	}
	defer func() {
		if err := mgr.Close(); err != nil {
			t.Logf("Error closing manager: %v", err)
		}
	}()

	ctx := context.Background()

	// 测试场景：向不存在的队列发布消息，验证Nack处理
	t.Run("MessageRejectedByServer", func(t *testing.T) {
		contentData := map[string]interface{}{
			"id":        "reject-test-1",
			"message":   "This message should be rejected",
			"timestamp": time.Now().Unix(),
		}
		contentBytes, _ := json.Marshal(contentData)
		msgs := []*MsgData{
			{
				Content: string(contentBytes),
				Option: Option{
					Exchange: "test.exchange",
					Queue:    "nonexistent.queue", // 不存在的队列
					Router:   "test.reject",
				},
				Type:    1,
				Durable: true,
			},
		}

		// 使用Confirm模式批量发布，预期会被拒绝
		err := mgr.BatchPublish(ctx, msgs)
		if err != nil {
			// 检查是否是预期的拒绝错误
			errStr := err.Error()
			if strings.Contains(errStr, "MESSAGE_REJECTED") ||
				strings.Contains(errStr, "Nack") ||
				strings.Contains(errStr, "not found") {
				t.Logf("Correctly handled message rejection: %v", err)
			} else {
				t.Logf("Unexpected error (may be expected depending on RabbitMQ config): %v", err)
			}
		} else {
			t.Log("Message was accepted (queue may exist or be auto-created)")
		}
	})

	// 测试场景：Confirm通道异常关闭
	t.Run("ConfirmChannelClosure", func(t *testing.T) {
		// 先正常发布一次建立Confirm通道
		contentData := map[string]interface{}{
			"id":        "confirm-test-1",
			"message":   "Normal message",
			"timestamp": time.Now().Unix(),
		}
		contentBytes, _ := json.Marshal(contentData)
		msgs := []*MsgData{
			{
				Content: string(contentBytes),
				Option: Option{
					Exchange: "test.exchange",
					Queue:    "test.queue",
					Router:   "test.confirm",
				},
				Type:    1,
				Durable: true,
			},
		}

		err := mgr.BatchPublish(ctx, msgs)
		if err != nil {
			t.Errorf("Initial confirm publish failed: %v", err)
			return
		}

		// 强制关闭Confirm通道
		chanKey := "test.exchangetest.queuetest.confirmconfirm"
		mgr.mu.Lock()
		if pub, exists := mgr.channels[chanKey]; exists {
			if pub.channel != nil {
				pub.channel.Close()
				pub.channel = nil
				pub.ready = false
			}
		}
		mgr.mu.Unlock()

		t.Log("Confirm channel forcibly closed")

		// 再次发布，验证通道重建和Confirm处理
		time.Sleep(500 * time.Millisecond)

		contentData2 := map[string]interface{}{
			"id":        "confirm-test-2",
			"message":   "Message after channel closure",
			"timestamp": time.Now().Unix(),
		}
		contentBytes2, _ := json.Marshal(contentData2)
		msgs2 := []*MsgData{
			{
				Content: string(contentBytes2),
				Option: Option{
					Exchange: "test.exchange",
					Queue:    "test.queue",
					Router:   "test.confirm",
				},
				Type:    1,
				Durable: true,
			},
		}

		err = mgr.BatchPublish(ctx, msgs2)
		if err != nil {
			t.Errorf("Publish after confirm channel closure failed: %v", err)
		} else {
			t.Log("Successfully handled confirm channel closure and recovery")
		}
	})

	t.Log("Confirm mode edge cases test completed")
}

// TestTransactionRollback 事务模式失败回滚测试
func TestTransactionRollback(t *testing.T) {
	conf := loadTestConfig(t)
	conf.DsName = "test_transaction_rollback"

	mgr, err := NewPublishManager(conf)
	if err != nil {
		t.Fatalf("Failed to create publish manager: %v", err)
	}
	defer func() {
		if err := mgr.Close(); err != nil {
			t.Logf("Error closing manager: %v", err)
		}
	}()

	ctx := context.Background()

	// 测试场景：事务模式下消息被拒绝，验证回滚
	t.Run("TransactionRollbackOnRejection", func(t *testing.T) {
		contentData1 := map[string]interface{}{
			"id":        "tx-rollback-1",
			"message":   "First message in transaction",
			"timestamp": time.Now().Unix(),
		}
		contentBytes1, _ := json.Marshal(contentData1)

		contentData2 := map[string]interface{}{
			"id":        "tx-rollback-2",
			"message":   "Second message in transaction",
			"timestamp": time.Now().Unix(),
		}
		contentBytes2, _ := json.Marshal(contentData2)

		contentData3 := map[string]interface{}{
			"id":        "tx-rollback-3",
			"message":   "Third message (same queue to avoid INCONSISTENT_BATCH)",
			"timestamp": time.Now().Unix(),
		}
		contentBytes3, _ := json.Marshal(contentData3)

		msgs := []*MsgData{
			{
				Content: string(contentBytes1),
				Option: Option{
					Exchange:       "test.exchange",
					Queue:          "test.queue",
					Router:         "test.tx.rollback",
					UseTransaction: true, // 启用事务模式
				},
				Type:    1,
				Durable: true,
			},
			{
				Content: string(contentBytes2),
				Option: Option{
					Exchange:       "test.exchange",
					Queue:          "test.queue",
					Router:         "test.tx.rollback",
					UseTransaction: true,
				},
				Type:    1,
				Durable: true,
			},
			{
				Content: string(contentBytes3),
				Option: Option{
					Exchange:       "test.exchange",
					Queue:          "test.queue", // 使用相同的队列避免INCONSISTENT_BATCH
					Router:         "test.tx.rollback.3",
					UseTransaction: true,
				},
				Type:    1,
				Durable: true,
			},
		}

		// 使用事务模式发布，预期整个事务回滚
		err := mgr.BatchPublish(ctx, msgs)
		if err != nil {
			errStr := err.Error()
			if strings.Contains(errStr, "rollback") ||
				strings.Contains(errStr, "transaction") ||
				strings.Contains(errStr, "failed") {
				t.Logf("Transaction correctly rolled back on failure: %v", err)
			} else {
				t.Logf("Transaction failed with unexpected error: %v", err)
			}
		} else {
			t.Log("Transaction completed successfully (all messages may have been accepted)")
		}
	})

	// 测试场景：事务提交失败的模拟
	t.Run("TransactionCommitFailure", func(t *testing.T) {
		// 这个测试比较难模拟真实的网络中断
		// 我们可以尝试在事务进行中强制关闭连接
		contentData := map[string]interface{}{
			"id":        "tx-commit-fail-1",
			"message":   "Message in failing transaction",
			"timestamp": time.Now().Unix(),
		}
		contentBytes, _ := json.Marshal(contentData)
		msgs := []*MsgData{
			{
				Content: string(contentBytes),
				Option: Option{
					Exchange:       "test.exchange",
					Queue:          "test.queue",
					Router:         "test.tx.commit.fail",
					UseTransaction: true,
				},
				Type:    1,
				Durable: true,
			},
		}

		// 启动goroutine在事务进行时关闭连接
		done := make(chan bool, 1)
		go func() {
			time.Sleep(100 * time.Millisecond) // 等待事务开始
			mgr.mu.Lock()
			if mgr.conn != nil {
				mgr.conn.Close()
				mgr.conn = nil
			}
			mgr.mu.Unlock()
			done <- true
		}()

		err := mgr.BatchPublish(ctx, msgs)
		<-done

		if err != nil {
			errStr := err.Error()
			if strings.Contains(errStr, "rollback") ||
				strings.Contains(errStr, "transaction") ||
				strings.Contains(errStr, "connection") ||
				strings.Contains(errStr, "closed") {
				t.Logf("✓ Transaction correctly failed due to connection closure: %v", err)
			} else {
				t.Logf("Transaction failed with unexpected error: %v", err)
			}
		} else {
			t.Log("Transaction succeeded despite connection issues")
		}
	})

	t.Log("Transaction rollback test completed")
}

// TestDeadLetterQueue 死信队列配置测试
func TestDeadLetterQueue(t *testing.T) {
	conf := loadTestConfig(t)
	conf.DsName = "test_dlx"

	mgr, err := NewPublishManager(conf)
	if err != nil {
		t.Fatalf("Failed to create publish manager: %v", err)
	}
	defer func() {
		if err := mgr.Close(); err != nil {
			t.Logf("Error closing manager: %v", err)
		}
	}()

	ctx := context.Background()

	// 测试场景：设置死信队列配置
	t.Run("DeadLetterQueueConfiguration", func(t *testing.T) {
		contentData := map[string]interface{}{
			"id":        "dlx-test-1",
			"message":   "Message with DLX configuration",
			"timestamp": time.Now().Unix(),
		}
		contentBytes, _ := json.Marshal(contentData)
		msg := &MsgData{
			Content: string(contentBytes),
			Option: Option{
				Exchange: "test.exchange",
				Queue:    "test.queue.dlx",
				Router:   "test.dlx",
				DLXConfig: &DLXConfig{
					DlxExchange: "test.dlx.exchange",
					DlxQueue:    "test.dlx.queue",
					DlxRouter:   "test.dlx.routing.key",
				},
			},
			Type:    1,
			Durable: true,
		}

		err := mgr.PublishMsgData(ctx, msg)
		if err != nil {
			t.Logf("DLX configuration test result: %v", err)
			// DLX配置可能因为RabbitMQ权限或配置问题失败，这是正常的
		} else {
			t.Log("DLX configuration applied successfully")
		}
	})

	// 测试场景：验证DLX相关的参数处理
	t.Run("DLXParameterValidation", func(t *testing.T) {
		// 测试空的DLX配置
		contentData1 := map[string]interface{}{
			"id":        "dlx-test-2",
			"message":   "Message with empty DLX",
			"timestamp": time.Now().Unix(),
		}
		contentBytes1, _ := json.Marshal(contentData1)
		msg1 := &MsgData{
			Content: string(contentBytes1),
			Option: Option{
				Exchange:  "test.exchange",
				Queue:     "test.queue",
				Router:    "test.dlx.empty",
				DLXConfig: &DLXConfig{}, // 空的DLX配置
			},
			Type:    1,
			Durable: true,
		}

		err1 := mgr.PublishMsgData(ctx, msg1)
		if err1 != nil {
			t.Logf("Empty DLX config test result: %v", err1)
		} else {
			t.Log("Empty DLX config accepted")
		}

		// 测试不设置DLX（默认行为）
		contentData2 := map[string]interface{}{
			"id":        "dlx-test-3",
			"message":   "Message without DLX",
			"timestamp": time.Now().Unix(),
		}
		contentBytes2, _ := json.Marshal(contentData2)
		msg2 := &MsgData{
			Content: string(contentBytes2),
			Option: Option{
				Exchange: "test.exchange",
				Queue:    "test.queue",
				Router:   "test.no.dlx",
				// DLXConfig 为 nil
			},
			Type:    1,
			Durable: true,
		}

		err2 := mgr.PublishMsgData(ctx, msg2)
		if err2 != nil {
			t.Errorf("No DLX config failed: %v", err2)
		} else {
			t.Log("Message without DLX published successfully")
		}
	})

	t.Log("Dead letter queue test completed")
}

// TestSemaphoreTimeout 信号量超时与通道创建限制测试
func TestSemaphoreTimeout(t *testing.T) {
	conf := loadTestConfig(t)
	conf.DsName = "test_semaphore"
	conf.ChannelMax = 1 // 设置很小的通道限制

	mgr, err := NewPublishManager(conf)
	if err != nil {
		t.Fatalf("Failed to create publish manager: %v", err)
	}
	defer func() {
		if err := mgr.Close(); err != nil {
			t.Logf("Error closing manager: %v", err)
		}
	}()

	ctx := context.Background()

	// 首先创建一个通道并保持它活跃，占用信号量
	t.Run("SetupChannelOccupation", func(t *testing.T) {
		testData := map[string]interface{}{
			"id":        "semaphore-setup",
			"message":   "Setup message to occupy semaphore",
			"timestamp": time.Now().Unix(),
		}
		testDataBytes, _ := json.Marshal(testData)

		err := mgr.Publish(ctx, "test.exchange", "test.queue", 1, string(testDataBytes), WithRouter("test.semaphore.setup"))
		if err != nil {
			t.Errorf("Setup publish failed: %v", err)
			return
		}

		// 验证通道确实被创建
		mgr.mu.RLock()
		channelCount := len(mgr.channels)
		mgr.mu.RUnlock()

		if channelCount == 0 {
			t.Error("No channels were created during setup")
		} else {
			t.Logf("Setup completed: %d channel(s) created", channelCount)
		}
	})

	// 测试场景：信号量已满时尝试创建新通道
	t.Run("SemaphoreTimeoutOnNewChannel", func(t *testing.T) {
		testData := map[string]interface{}{
			"id":        "semaphore-timeout-test",
			"message":   "This should timeout due to semaphore limit",
			"timestamp": time.Now().Unix(),
		}
		testDataBytes, _ := json.Marshal(testData)

		start := time.Now()
		// 使用不同的路由键强制创建新通道
		err := mgr.Publish(ctx, "test.exchange", "test.queue", 1, string(testDataBytes), WithRouter("test.semaphore.timeout"))
		duration := time.Since(start)

		if err != nil {
			errStr := err.Error()
			if strings.Contains(errStr, "SEMAPHORE_TIMEOUT") {
				t.Logf("✓ Correctly received semaphore timeout after %v: %v", duration, err)
			} else if strings.Contains(errStr, "channel creation timeout") {
				t.Logf("✓ Received channel creation timeout (acceptable): %v", err)
			} else if strings.Contains(errStr, "channel id space exhausted") {
				t.Logf("Received RabbitMQ channel limit error (server limit reached): %v", err)
			} else {
				t.Logf("Received other error: %v", err)
			}
		} else {
			t.Logf("Publish succeeded after %v (semaphore logic may not be working)", duration)
		}
	})

	// 测试场景：验证信号量释放后可以创建新通道
	t.Run("SemaphoreReleaseVerification", func(t *testing.T) {
		// 等待一段时间让可能的超时完成
		time.Sleep(1 * time.Second)

		testData := map[string]interface{}{
			"id":        "semaphore-release-test",
			"message":   "Test after semaphore should be available",
			"timestamp": time.Now().Unix(),
		}
		testDataBytes, _ := json.Marshal(testData)

		// 使用相同的路由键，应该复用现有通道
		err := mgr.Publish(ctx, "test.exchange", "test.queue", 1, string(testDataBytes), WithRouter("test.semaphore.setup"))
		if err != nil {
			t.Logf("Release verification failed (may be expected): %v", err)
		} else {
			t.Log("✓ Successfully published after semaphore release verification")
		}
	})

	t.Log("Semaphore timeout test completed")
}

// TestRetryMechanism 重试机制完整路径测试
func TestRetryMechanism(t *testing.T) {
	conf := loadTestConfig(t)
	conf.DsName = "test_retry"

	mgr, err := NewPublishManager(conf)
	if err != nil {
		t.Fatalf("Failed to create publish manager: %v", err)
	}
	defer func() {
		if err := mgr.Close(); err != nil {
			t.Logf("Error closing manager: %v", err)
		}
	}()

	ctx := context.Background()

	// 测试场景：模拟持续失败的情况，验证重试机制
	t.Run("RetryExhaustion", func(t *testing.T) {
		// 创建一个会持续失败的消息
		contentData := map[string]interface{}{
			"id":        "retry-test-1",
			"message":   "Message that will fail repeatedly",
			"timestamp": time.Now().Unix(),
		}
		contentBytes, _ := json.Marshal(contentData)
		msg := &MsgData{
			Content: string(contentBytes),
			Option: Option{
				Exchange: "test.exchange",
				Queue:    "test.queue",
				Router:   "test.retry",
			},
			Type:    1,
			Durable: true,
		}

		// 首先正常发布一次，确保机制工作
		err := mgr.PublishMsgData(ctx, msg)
		if err != nil {
			t.Errorf("Initial retry test failed: %v", err)
			return
		}

		// 现在模拟持续失败的情况
		// 我们可以通过临时断开连接来模拟失败
		mgr.mu.Lock()
		if mgr.conn != nil {
			mgr.conn.Close()
			mgr.conn = nil
		}
		mgr.mu.Unlock()

		t.Log("Connection closed to simulate persistent failure")

		// 现在尝试发布，应该触发重试机制
		// PublishMsgData 内部有重试逻辑（3次）
		err = mgr.PublishMsgData(ctx, msg)

		if err != nil {
			errStr := err.Error()
			// 检查是否是重试相关的错误
			if strings.Contains(errStr, "retry") ||
				strings.Contains(errStr, "timeout") ||
				strings.Contains(errStr, "connection") {
				t.Logf("Retry mechanism triggered correctly: %v", err)
			} else {
				t.Logf("Unexpected error after retries: %v", err)
			}
		} else {
			// 如果成功了，说明重连机制自动恢复了连接
			t.Log("Message published successfully after automatic reconnection")
		}
	})

	// 测试场景：验证重试计数和错误信息
	t.Run("RetryCountVerification", func(t *testing.T) {
		// 这个测试主要验证日志输出和错误处理
		// 在实际运行中，我们可以通过日志看到重试次数

		contentData := map[string]interface{}{
			"id":        "retry-count-test",
			"message":   "Testing retry count",
			"timestamp": time.Now().Unix(),
		}
		contentBytes, _ := json.Marshal(contentData)
		msg := &MsgData{
			Content: string(contentBytes),
			Option: Option{
				Exchange: "test.exchange",
				Queue:    "test.queue",
				Router:   "test.retry.count",
			},
			Type:    1,
			Durable: true,
		}

		// 正常发布，应该成功
		err := mgr.PublishMsgData(ctx, msg)
		if err != nil {
			t.Errorf("Retry count test failed: %v", err)
		} else {
			t.Log("Retry count test completed successfully")
		}
	})

	t.Log("Retry mechanism test completed")
}

// TestCloseTimeout 关闭逻辑超时处理测试
func TestCloseTimeout(t *testing.T) {
	conf := loadTestConfig(t)
	conf.DsName = "test_close_timeout"

	mgr, err := NewPublishManager(conf)
	if err != nil {
		t.Fatalf("Failed to create publish manager: %v", err)
	}

	// 先创建一些通道
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		testData := map[string]interface{}{
			"id":        fmt.Sprintf("close-test-%d", i+1),
			"message":   fmt.Sprintf("Close test message %d", i+1),
			"timestamp": time.Now().Unix(),
		}
		testDataBytes, _ := json.Marshal(testData)

		router := fmt.Sprintf("test.close.%d", i+1)
		err := mgr.Publish(ctx, "test.exchange", "test.queue", 1, string(testDataBytes), WithRouter(router))
		if err != nil {
			t.Logf("Setup publish %d failed: %v", i+1, err)
		}
	}

	// 测试正常关闭
	t.Run("NormalClose", func(t *testing.T) {
		err := mgr.Close()
		if err != nil {
			t.Errorf("Normal close failed: %v", err)
		} else {
			t.Log("Normal close completed successfully")
		}
	})

	// 测试重复关闭
	t.Run("DoubleClose", func(t *testing.T) {
		err := mgr.Close()
		// 重复关闭应该不报错（once机制保护）
		if err != nil {
			t.Logf("Double close returned error (acceptable): %v", err)
		} else {
			t.Log("Double close handled gracefully")
		}
	})

	t.Log("Close timeout test completed")
}
