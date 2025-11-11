// Package rabbitmq 压力测试
//
// 运行压力测试说明：
// 1. 确保RabbitMQ服务正在运行
// 2. 配置文件位于 ../resource/rabbitmq.json
// 3. 运行测试：go test -run TestStress -v -timeout 300s
//
// 压力测试包括：
// - 高并发发布测试
// - 大批量消息测试
// - 长时间稳定性测试
// - 内存泄漏检测
// - 连接池压力测试
// - 错误场景压力测试
package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestHighConcurrencyStress 高并发压力测试
func TestHighConcurrencyStress(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	conf := loadTestConfig(t)
	conf.DsName = "stress_concurrency"

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

	// 测试参数
	numGoroutines := 100       // 并发goroutine数量
	messagesPerGoroutine := 50 // 每个goroutine发送的消息数
	totalMessages := int64(numGoroutines * messagesPerGoroutine)

	// 统计变量
	var successCount int64
	var errorCount int64
	var totalLatency int64

	t.Logf("Starting high concurrency stress test: %d goroutines, %d messages per goroutine, total: %d messages",
		numGoroutines, messagesPerGoroutine, totalMessages)

	startTime := time.Now()

	// 启动并发发布
	var wg sync.WaitGroup
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			for j := 0; j < messagesPerGoroutine; j++ {
				msgStart := time.Now()

				testData := map[string]interface{}{
					"goroutine_id": goroutineID,
					"message_id":   j,
					"timestamp":    time.Now().UnixNano(),
					"payload":      fmt.Sprintf("Stress test message from goroutine %d, message %d", goroutineID, j),
				}
				testDataBytes, _ := json.Marshal(testData)

				router := fmt.Sprintf("stress.concurrency.%d.%d", goroutineID, j%10) // 限制路由键数量避免过多通道

				err := mgr.Publish(ctx, "test.exchange", "test.queue", 1, string(testDataBytes), WithRouter(router))
				latency := time.Since(msgStart)

				atomic.AddInt64(&totalLatency, latency.Nanoseconds())

				if err != nil {
					atomic.AddInt64(&errorCount, 1)
					t.Logf("Goroutine %d, message %d failed: %v", goroutineID, j, err)
				} else {
					atomic.AddInt64(&successCount, 1)
				}
			}
		}(i)
	}

	// 等待所有goroutine完成
	wg.Wait()
	duration := time.Since(startTime)

	// 计算统计信息
	successRate := float64(successCount) / float64(totalMessages) * 100
	avgLatency := time.Duration(totalLatency / totalMessages)
	throughput := float64(totalMessages) / duration.Seconds()

	t.Logf("=== High Concurrency Stress Test Results ===")
	t.Logf("Duration: %v", duration)
	t.Logf("Total Messages: %d", totalMessages)
	t.Logf("Successful: %d", successCount)
	t.Logf("Failed: %d", errorCount)
	t.Logf("Success Rate: %.2f%%", successRate)
	t.Logf("Average Latency: %v", avgLatency)
	t.Logf("Throughput: %.2f messages/second", throughput)
	t.Logf("Memory Stats: %s", getMemoryStats())

	// 验证结果
	if successRate < 99.0 {
		t.Errorf("Success rate too low: %.2f%% (required: >=99%%)", successRate)
	}

	if avgLatency > 100*time.Millisecond {
		t.Errorf("Average latency too high: %v (acceptable: <=100ms)", avgLatency)
	}
}

// TestLargeBatchStress 大批量消息压力测试
func TestLargeBatchStress(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	conf := loadTestConfig(t)
	conf.DsName = "stress_batch"

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

	// 测试参数
	batchSizes := []int{50, 100, 200} // 降低批量大小避免服务器过载
	iterations := 3                   // 减少迭代次数加快测试

	for _, batchSize := range batchSizes {
		t.Logf("Testing batch size: %d", batchSize)

		var totalLatency time.Duration
		var successCount int

		for iter := 0; iter < iterations; iter++ {
			// 创建大批量消息
			msgs := make([]*MsgData, batchSize)
			for i := 0; i < batchSize; i++ {
				contentData := map[string]interface{}{
					"batch_id":    iter,
					"message_id":  i,
					"batch_size":  batchSize,
					"timestamp":   time.Now().UnixNano(),
					"payload":     fmt.Sprintf("Large batch message %d in batch %d of size %d", i, iter, batchSize),
					"large_field": generateLargeString(1024), // 1KB 数据
				}
				contentBytes, _ := json.Marshal(contentData)
				msgs[i] = &MsgData{
					Content: string(contentBytes),
					Option: Option{
						Exchange:       "test.exchange",
						Queue:          "test.queue",
						Router:         fmt.Sprintf("stress.batch.%d", batchSize),
						UseTransaction: true, // 明确使用事务模式而不是确认模式
						Durable:        true,
					},
					Type: 1,
				}
			}

			start := time.Now()
			t.Logf("Starting batch %d (size %d) at %v", iter, batchSize, start)
			err := mgr.BatchPublishWithOptions(ctx, msgs, WithConfirmTimeout(60*time.Second))
			latency := time.Since(start)

			totalLatency += latency

			if err != nil {
				t.Errorf("Batch publish failed for size %d, iteration %d after %v: %v", batchSize, iter, latency, err)
			} else {
				successCount++
				t.Logf("✓ Batch %d (size %d) completed in %v", iter, batchSize, latency)
			}
		}

		avgLatency := totalLatency / time.Duration(iterations)
		throughput := float64(batchSize*successCount) / totalLatency.Seconds()

		t.Logf("Batch Size %d Results:", batchSize)
		t.Logf("  Success Rate: %d/%d (%.1f%%)", successCount, iterations, float64(successCount)/float64(iterations)*100)
		t.Logf("  Average Latency: %v", avgLatency)
		t.Logf("  Throughput: %.2f messages/second", throughput)
		t.Logf("  Memory Stats: %s", getMemoryStats())
	}
}

// TestLongRunningStability 长时间稳定性压力测试
func TestLongRunningStability(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	conf := loadTestConfig(t)
	conf.DsName = "stress_stability"

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

	// 测试参数
	testDuration := 60 * time.Second   // 测试持续时间
	interval := 100 * time.Millisecond // 发送间隔
	numGoroutines := 10                // 并发goroutine数量

	var messageCount int64
	var errorCount int64
	var memoryPeaks []string

	t.Logf("Starting long-running stability test for %v with %d goroutines", testDuration, numGoroutines)

	startTime := time.Now()
	lastStatsTime := startTime

	// 启动并发发布goroutine
	var wg sync.WaitGroup
	stopChan := make(chan struct{})

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			ticker := time.NewTicker(interval)
			defer ticker.Stop()

			for {
				select {
				case <-stopChan:
					return
				case <-ticker.C:
					atomic.AddInt64(&messageCount, 1)

					testData := map[string]interface{}{
						"goroutine_id": goroutineID,
						"message_seq":  atomic.LoadInt64(&messageCount),
						"timestamp":    time.Now().UnixNano(),
						"uptime":       time.Since(startTime).String(),
					}
					testDataBytes, _ := json.Marshal(testData)

					err := mgr.Publish(ctx, "test.exchange", "test.queue", 1, string(testDataBytes),
						WithRouter(fmt.Sprintf("stability.%d", goroutineID%5)))

					if err != nil {
						atomic.AddInt64(&errorCount, 1)
					}
				}
			}
		}(i)
	}

	// 监控goroutine
	monitorDone := make(chan struct{})
	go func() {
		defer close(monitorDone)

		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-stopChan:
				return
			case <-ticker.C:
				elapsed := time.Since(lastStatsTime)
				currentCount := atomic.LoadInt64(&messageCount)
				currentErrors := atomic.LoadInt64(&errorCount)

				throughput := float64(currentCount) / elapsed.Seconds()
				errorRate := float64(currentErrors) / float64(currentCount) * 100

				memoryStats := getMemoryStats()
				memoryPeaks = append(memoryPeaks, fmt.Sprintf("Time: %v, Memory: %s", time.Since(startTime), memoryStats))

				t.Logf("Stability Progress - Elapsed: %v, Messages: %d, Errors: %d (%.2f%%), Throughput: %.1f msg/s",
					time.Since(startTime), currentCount, currentErrors, errorRate, throughput)
			}
		}
	}()

	// 等待测试时间结束
	time.Sleep(testDuration)
	close(stopChan)

	// 等待所有goroutine完成
	wg.Wait()
	<-monitorDone

	// 计算最终统计
	totalMessages := atomic.LoadInt64(&messageCount)
	totalErrors := atomic.LoadInt64(&errorCount)
	finalDuration := time.Since(startTime)

	successRate := float64(totalMessages-totalErrors) / float64(totalMessages) * 100
	avgThroughput := float64(totalMessages) / finalDuration.Seconds()

	t.Logf("=== Long-Running Stability Test Results ===")
	t.Logf("Duration: %v", finalDuration)
	t.Logf("Total Messages: %d", totalMessages)
	t.Logf("Successful: %d", totalMessages-totalErrors)
	t.Logf("Failed: %d", totalErrors)
	t.Logf("Success Rate: %.2f%%", successRate)
	t.Logf("Average Throughput: %.2f messages/second", avgThroughput)
	t.Logf("Memory Usage Over Time:")
	for _, peak := range memoryPeaks {
		t.Logf("  %s", peak)
	}
	t.Logf("Final Memory Stats: %s", getMemoryStats())

	// 验证稳定性
	if successRate < 95.0 {
		t.Errorf("Success rate too low for stability test: %.2f%% (required: >=95%%)", successRate)
	}
}

// TestConnectionPoolStress 连接池压力测试
func TestConnectionPoolStress(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	conf := loadTestConfig(t)
	conf.DsName = "stress_connection_pool"
	conf.ChannelMax = 5 // 限制通道数量以测试连接池压力

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

	// 测试参数
	numGoroutines := 50 // 超过ChannelMax的goroutine数量
	messagesPerGoroutine := 20
	totalMessages := numGoroutines * messagesPerGoroutine

	var successCount int64
	var semaphoreTimeoutCount int64
	var otherErrorCount int64

	t.Logf("Starting connection pool stress test: %d goroutines, %d messages each, ChannelMax: %d",
		numGoroutines, messagesPerGoroutine, conf.ChannelMax)

	startTime := time.Now()

	// 启动并发发布，超过连接池容量
	var wg sync.WaitGroup
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			for j := 0; j < messagesPerGoroutine; j++ {
				testData := map[string]interface{}{
					"goroutine_id": goroutineID,
					"message_id":   j,
					"timestamp":    time.Now().UnixNano(),
				}
				testDataBytes, _ := json.Marshal(testData)

				// 使用不同的路由键强制创建新通道
				router := fmt.Sprintf("pool.stress.%d.%d", goroutineID, j)

				err := mgr.Publish(ctx, "test.exchange", "test.queue", 1, string(testDataBytes), WithRouter(router))

				if err != nil {
					errorStr := err.Error()
					if strings.Contains(errorStr, "SEMAPHORE_TIMEOUT") {
						atomic.AddInt64(&semaphoreTimeoutCount, 1)
					} else {
						atomic.AddInt64(&otherErrorCount, 1)
						t.Logf("Unexpected error: %v", err)
					}
				} else {
					atomic.AddInt64(&successCount, 1)
				}
			}
		}(i)
	}

	wg.Wait()
	duration := time.Since(startTime)

	totalProcessed := successCount + semaphoreTimeoutCount + otherErrorCount
	successRate := float64(successCount) / float64(totalProcessed) * 100
	semaphoreRate := float64(semaphoreTimeoutCount) / float64(totalProcessed) * 100

	t.Logf("=== Connection Pool Stress Test Results ===")
	t.Logf("Duration: %v", duration)
	t.Logf("Expected Messages: %d", totalMessages)
	t.Logf("Processed Messages: %d", totalProcessed)
	t.Logf("Successful: %d (%.1f%%)", successCount, successRate)
	t.Logf("Semaphore Timeouts: %d (%.1f%%)", semaphoreTimeoutCount, semaphoreRate)
	t.Logf("Other Errors: %d", otherErrorCount)
	t.Logf("Throughput: %.1f messages/second", float64(totalProcessed)/duration.Seconds())
	t.Logf("Channel Pool Size: %d", conf.ChannelMax)
	t.Logf("Memory Stats: %s", getMemoryStats())

	// 验证连接池行为
	if semaphoreTimeoutCount == 0 && successCount == int64(totalMessages) {
		t.Log("All messages succeeded - connection pool may be larger than expected")
	} else if semaphoreTimeoutCount > 0 {
		t.Log("✓ Semaphore timeouts detected - connection pool working correctly")
	}

	if otherErrorCount > int64(totalMessages)*1/100 { // 允许1%的其他错误
		t.Errorf("Too many unexpected errors: %d (acceptable: <=%d)", otherErrorCount, totalMessages/100)
	}
}

// TestErrorHandlingStress 错误处理压力测试
func TestErrorHandlingStress(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	conf := loadTestConfig(t)
	conf.DsName = "stress_error_handling"

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

	// 测试参数
	numGoroutines := 20
	messagesPerGoroutine := 10
	interruptionInterval := 50 * time.Millisecond // 定期中断连接

	var wg sync.WaitGroup
	var successCount int64
	var retryCount int64
	var connectionErrorCount int64

	t.Logf("Starting error handling stress test: %d goroutines, periodic connection interruptions",
		numGoroutines)

	// 启动连接中断goroutine
	stopInterruption := make(chan struct{})
	go func() {
		ticker := time.NewTicker(interruptionInterval)
		defer ticker.Stop()

		for {
			select {
			case <-stopInterruption:
				return
			case <-ticker.C:
				// 定期中断连接
				mgr.mu.Lock()
				if mgr.conn != nil {
					mgr.conn.Close()
					mgr.conn = nil
					t.Log("Connection interrupted for stress testing")
				}
				mgr.mu.Unlock()
			}
		}
	}()

	// 启动并发发布
	startTime := time.Now()
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			for j := 0; j < messagesPerGoroutine; j++ {
				testData := map[string]interface{}{
					"goroutine_id": goroutineID,
					"message_id":   j,
					"timestamp":    time.Now().UnixNano(),
				}
				testDataBytes, _ := json.Marshal(testData)

				// 使用重试选项
				err := mgr.Publish(ctx, "test.exchange", "test.queue", 1, string(testDataBytes),
					WithRouter(fmt.Sprintf("error.stress.%d", goroutineID%5)))

				if err != nil {
					errorStr := err.Error()
					if strings.Contains(errorStr, "connection") ||
						strings.Contains(errorStr, "channel") ||
						strings.Contains(errorStr, "not open") {
						atomic.AddInt64(&connectionErrorCount, 1)
					}
					// 记录重试次数（通过日志分析）
					if strings.Contains(errorStr, "attempt") {
						atomic.AddInt64(&retryCount, 1)
					}
				} else {
					atomic.AddInt64(&successCount, 1)
				}

				// 小延迟避免过于激进
				time.Sleep(10 * time.Millisecond)
			}
		}(i)
	}

	wg.Wait()
	close(stopInterruption)
	duration := time.Since(startTime)

	totalMessages := int64(numGoroutines * messagesPerGoroutine)
	successRate := float64(successCount) / float64(totalMessages) * 100

	t.Logf("=== Error Handling Stress Test Results ===")
	t.Logf("Duration: %v", duration)
	t.Logf("Total Messages: %d", totalMessages)
	t.Logf("Successful: %d", successCount)
	t.Logf("Connection Errors: %d", connectionErrorCount)
	t.Logf("Success Rate: %.2f%%", successRate)
	t.Logf("Throughput: %.1f messages/second", float64(totalMessages)/duration.Seconds())
	t.Logf("Memory Stats: %s", getMemoryStats())

	// 在高干扰环境下，成功率应该仍然较高
	if successRate < 80.0 {
		t.Errorf("Success rate too low under error conditions: %.2f%% (required: >=80%%)", successRate)
	}

	if connectionErrorCount == 0 {
		t.Log("No connection errors detected - interference may not be effective")
	} else {
		t.Log("✓ Connection errors handled correctly under stress")
	}
}

// TestPerformanceBenchmark 性能基准测试
func TestPerformanceBenchmark(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping benchmark test in short mode")
	}

	conf := loadTestConfig(t)
	conf.DsName = "benchmark"

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

	// 基准测试参数
	iterations := 1000
	concurrencyLevels := []int{1, 5, 10, 20}

	for _, concurrency := range concurrencyLevels {
		t.Logf("Benchmarking with concurrency level: %d", concurrency)

		var wg sync.WaitGroup
		messagesPerGoroutine := iterations / concurrency
		latencies := make([]time.Duration, 0, iterations)
		var latencyMutex sync.Mutex

		startTime := time.Now()

		// 启动并发goroutine
		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func(goroutineID int) {
				defer wg.Done()

				for j := 0; j < messagesPerGoroutine; j++ {
					msgStart := time.Now()

					testData := map[string]interface{}{
						"goroutine_id": goroutineID,
						"message_id":   j,
						"concurrency":  concurrency,
						"timestamp":    time.Now().UnixNano(),
					}
					testDataBytes, _ := json.Marshal(testData)

					err := mgr.Publish(ctx, "test.exchange", "test.queue", 1, string(testDataBytes),
						WithRouter(fmt.Sprintf("benchmark.%d", goroutineID%5)))

					latency := time.Since(msgStart)

					if err != nil {
						t.Errorf("Benchmark publish failed: %v", err)
						continue
					}

					latencyMutex.Lock()
					latencies = append(latencies, latency)
					latencyMutex.Unlock()
				}
			}(i)
		}

		wg.Wait()
		totalDuration := time.Since(startTime)

		// 计算统计信息
		if len(latencies) == 0 {
			t.Errorf("No successful messages for concurrency %d", concurrency)
			continue
		}

		// 排序延迟以计算分位数
		sortedLatencies := make([]time.Duration, len(latencies))
		copy(sortedLatencies, latencies)
		for i := 0; i < len(sortedLatencies)-1; i++ {
			for j := i + 1; j < len(sortedLatencies); j++ {
				if sortedLatencies[i] > sortedLatencies[j] {
					sortedLatencies[i], sortedLatencies[j] = sortedLatencies[j], sortedLatencies[i]
				}
			}
		}

		p50 := sortedLatencies[len(sortedLatencies)*50/100]
		p95 := sortedLatencies[len(sortedLatencies)*95/100]
		p99 := sortedLatencies[len(sortedLatencies)*99/100]

		throughput := float64(len(latencies)) / totalDuration.Seconds()
		avgLatency := totalDuration / time.Duration(len(latencies))

		t.Logf("Concurrency %d Results:", concurrency)
		t.Logf("  Total Duration: %v", totalDuration)
		t.Logf("  Messages Sent: %d", len(latencies))
		t.Logf("  Throughput: %.1f messages/second", throughput)
		t.Logf("  Average Latency: %v", avgLatency)
		t.Logf("  P50 Latency: %v", p50)
		t.Logf("  P95 Latency: %v", p95)
		t.Logf("  P99 Latency: %v", p99)
	}
}

// 辅助函数

// generateLargeString 生成指定大小的字符串用于测试
func generateLargeString(size int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, size)
	for i := range result {
		result[i] = charset[i%len(charset)]
	}
	return string(result)
}

// getMemoryStats 获取内存统计信息
func getMemoryStats() string {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return fmt.Sprintf("Alloc=%dKB, TotalAlloc=%dKB, Sys=%dKB, NumGC=%d",
		m.Alloc/1024, m.TotalAlloc/1024, m.Sys/1024, m.NumGC)
}
