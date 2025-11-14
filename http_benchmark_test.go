package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/valyala/fasthttp"

	"github.com/godaddy-x/freego/utils/sdk"
)

const benchmarkDomain = "http://localhost:8090"
const benchmarkAccessToken = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxOTg0NTQyMTQyMzY5ODkwMzA1IiwiYXVkIjoiIiwiaXNzIjoiIiwiaWF0IjowLCJleHAiOjE3NjMxOTYyOTIsImRldiI6IkFQUCIsImp0aSI6IlMrQjh0ZDh4ZGErRFVGeFliemxWNWc9PSIsImV4dCI6IiJ9.IDMBqkgRgl5cA0EOurLr/9ZdTFv7T6ACGLMN0cwZUT8="
const benchmarkTokenSecret = "WZlK3jp1GNdXXi2lWM/DnfFkRbMSbO7JP/I+MhdblfLJZf6cZCzKsBi5i7pMfrFZuLnNj1Qf2cZIym1V/ti/LA=="
const benchmarkTokenExpire = 1763196292

var client = &sdk.HttpSDK{
	Debug:     false,
	Domain:    benchmarkDomain,
	KeyPath:   "/key",
	LoginPath: "/login",
}

func init() {
	initClient()
}

func initClient() {
	client.SetPublicKey("BKNoaVapAlKywv5sXfag/LHa8mp6sdGFX6QHzfXIjBojkoCfCgZg6RPBXwLUUpPDzOC3uhDC60ECz2i1EbITsGY=")
	client.AuthToken(sdk.AuthToken{Token: benchmarkAccessToken, Secret: benchmarkTokenSecret, Expired: int64(benchmarkTokenExpire)})
}

func BenchmarkPublicKey(b *testing.B) {
	b.StopTimer()
	b.StartTimer()
	for i := 0; i < b.N; i++ { //use b.N for looping
		_, err := client.GetPublicKey()
		if err != nil {
			fmt.Println(err)
		}
	}
}

//go test -v http_test.go -bench=BenchmarkPubkey -benchmem -count=10

// go tool pprof cpuprofile.out (采集完成后调用命令: web)
// go tool pprof http://localhost:8849/debug/pprof/profile?seconds=30 (采集完成后调用命令: web)
// go test http_benchmark_test.go -bench=BenchmarkECCLogin  -benchmem -count=10 -cpuprofile acpu.out -memprofile amem.out
// go test http_benchmark_test.go -bench=BenchmarkECCLogin  -benchmem -count=10
func BenchmarkECCLogin(b *testing.B) {
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			requestData := sdk.AuthToken{Token: "1234567890123456", Secret: "1234567890123456"}
			responseData := sdk.AuthToken{}
			if err := client.PostByECC("/login", &requestData, &responseData); err != nil {
				panic(err)
			}
		}
	})
}

func BenchmarkOnlyServerECCLogin(b *testing.B) {
	randomCode := `BPV/OyjWh6bkMrtinSdAj0Uq1OVqGkLuZH5t6OVgwllaEny5+AjD0Hk0GsB926UzhdtIUnCr6+2fe+6C0BHz34DxoY1KowhoUsWuROnwG+Ste2Hu67OYcPxEEQBlOaG/rO36ZFZW121nAIBB2prBgH02J7kKsuDmi3mFzl18dxusLIKr5Gb+bfW5x63GJ8ro17oTQAG9gAh6mrF320OAKTI=`
	requestData := []byte(`{"d":"fkVuWG2whxNOlmi2ovxsDRPWgcUeaYEu9af/QyOxyeES6L/pDcc5P7GWjp6e6ILsJc9uhY4djNoCTZdkTe0ITSIKTo69tQgRoKd6Z1Icai2mLEZ84t8mLIMzLEHXIhDYoTSo","n":"OIuCkQNq60CPJLSp06IL+g==","s":"xRqaWc/r2f2jt3papz/ToT4FJIjgKzuYjReiblPgMtQ=","t":1762159032,"p":2}`)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			request := fasthttp.AcquireRequest()
			request.Header.SetContentType("application/json;charset=UTF-8")
			request.Header.Set("Authorization", "")
			request.Header.Set("RandomCode", randomCode)
			request.Header.SetMethod("POST")
			request.SetRequestURI(benchmarkDomain + "/login")
			request.SetBody(requestData)
			defer fasthttp.ReleaseRequest(request)
			response := fasthttp.AcquireResponse()
			defer fasthttp.ReleaseResponse(response)
			if err := fasthttp.DoTimeout(request, response, time.Second*20); err != nil {
				panic(err)
			}
			//fmt.Println(string(response.Body()))
		}
	})
}

// go test http_benchmark_test.go -bench=BenchmarkAesGetUser -benchmem -count=10 -cpuprofile acpu.out -memprofile amem.out
func BenchmarkAesGetUser(b *testing.B) {
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			requestObj := sdk.AuthToken{Token: "AI工具人，鲨鱼宝宝！QWER123456@##！", Secret: "安排测试下吧123456789@@@"}
			responseData := sdk.AuthToken{}
			if err := client.PostByAuth("/getUser", &requestObj, &responseData, true); err != nil {
				fmt.Println(err)
			}
			check := responseData.Token
			if len(check) == 0 {
				b.Logf("getUser no result")
			}
		}
	})
}

// BenchmarkConcurrentLoad 不同并发度下的性能对比测试
// 测试在不同并发负载下的HTTP客户端性能表现
func BenchmarkConcurrentLoad(b *testing.B) {
	loadLevels := []int{1, 10, 50, 100}

	for _, load := range loadLevels {
		b.Run(fmt.Sprintf("Concurrency_%d", load), func(b *testing.B) {
			b.ResetTimer()
			b.SetParallelism(load)

			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					requestObj := sdk.AuthToken{
						Token:  fmt.Sprintf("并发测试_%d_%d", load, time.Now().UnixNano()),
						Secret: fmt.Sprintf("secret_%d", load),
					}
					responseData := sdk.AuthToken{}
					err := client.PostByAuth("/getUser", &requestObj, &responseData, false)
					if err != nil {
						b.Logf("并发请求失败: %v", err)
					}
				}
			})
		})
	}
}

// BenchmarkLargePayload 大数据量请求性能测试
// 测试处理不同大小请求数据的性能表现
func BenchmarkLargePayload(b *testing.B) {
	// 准备不同大小的测试数据
	payloadSizes := []int{1 * 1024, 10 * 1024, 100 * 1024} // 1KB, 10KB, 100KB

	for _, size := range payloadSizes {
		b.Run(fmt.Sprintf("Payload_%dKB", size/1024), func(b *testing.B) {
			// 生成指定大小的测试数据
			largeToken := make([]byte, size)
			for i := range largeToken {
				largeToken[i] = byte(65 + (i % 26)) // A-Z循环
			}

			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					requestObj := sdk.AuthToken{
						Token:  string(largeToken),
						Secret: "large_payload_secret",
					}
					responseData := sdk.AuthToken{}
					err := client.PostByAuth("/getUser", &requestObj, &responseData, false)
					if err != nil {
						b.Logf("大数据请求失败: %v", err)
					}
				}
			})
		})
	}
}

// BenchmarkAuthMethods 不同认证方式性能对比
// 对比ECC、AES和无认证方式的性能差异
func BenchmarkAuthMethods(b *testing.B) {
	// 测试不同的认证方法
	authMethods := []string{"ECC", "AES", "NoAuth"}

	for _, method := range authMethods {
		b.Run(fmt.Sprintf("Auth_%s", method), func(b *testing.B) {
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					requestData := sdk.AuthToken{
						Token:  fmt.Sprintf("%s认证测试_%d", method, time.Now().UnixNano()),
						Secret: fmt.Sprintf("%s_secret", method),
					}
					responseData := sdk.AuthToken{}

					var err error
					switch method {
					case "ECC":
						err = client.PostByECC("/login", &requestData, &responseData)
					case "AES":
						err = client.PostByAuth("/getUser", &requestData, &responseData, true)
					case "NoAuth":
						// 创建无认证的客户端进行测试
						tempClient := &sdk.HttpSDK{
							Debug:     false,
							Domain:    "http://localhost:8090",
							KeyPath:   "/key",
							LoginPath: "/login",
						}
						err = tempClient.PostByAuth("/getUser", &requestData, &responseData, false)
					}

					if err != nil {
						b.Logf("%s认证请求失败: %v", method, err)
					}
				}
			})
		})
	}
}

// BenchmarkNetworkLatency 网络延迟和超时处理性能测试
// 测试在不同网络条件下的响应性能和超时处理效率
func BenchmarkNetworkLatency(b *testing.B) {
	// 测试不同网络条件下的性能
	networkConditions := []string{"Normal", "Timeout", "Slow"}

	for _, condition := range networkConditions {
		b.Run(fmt.Sprintf("Network_%s", condition), func(b *testing.B) {
			var testClient *sdk.HttpSDK

			switch condition {
			case "Normal":
				testClient = client
			case "Timeout":
				// 使用一个会超时的端点
				testClient = &sdk.HttpSDK{
					Debug:     false,
					Domain:    "http://httpbin.org/delay/5", // 5秒延迟
					KeyPath:   "/key",
					LoginPath: "/login",
				}
			case "Slow":
				// 使用慢响应端点
				testClient = &sdk.HttpSDK{
					Debug:     false,
					Domain:    "http://httpbin.org/delay/2", // 2秒延迟
					KeyPath:   "/key",
					LoginPath: "/login",
				}
			}

			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					if condition == "Normal" {
						_, err := testClient.GetPublicKey()
						if err != nil {
							b.Logf("网络测试[%s]失败: %v", condition, err)
						}
					} else {
						// 对于超时和慢响应测试，只执行一次获取公钥
						start := time.Now()
						_, err := testClient.GetPublicKey()
						elapsed := time.Since(start)

						if err != nil {
							b.Logf("网络测试[%s]失败，用时: %v, 错误: %v", condition, elapsed, err)
						} else {
							b.Logf("网络测试[%s]成功，用时: %v", condition, elapsed)
						}
					}
				}
			})
		})
	}
}

// BenchmarkErrorHandling 错误处理性能测试
// 测试各种错误场景下的处理性能和资源消耗
func BenchmarkErrorHandling(b *testing.B) {
	errorScenarios := []string{"InvalidURL", "ConnectionRefused", "InvalidAuth", "Timeout"}

	for _, scenario := range errorScenarios {
		b.Run(fmt.Sprintf("Error_%s", scenario), func(b *testing.B) {
			var testClient *sdk.HttpSDK

			switch scenario {
			case "InvalidURL":
				testClient = &sdk.HttpSDK{
					Debug:     false,
					Domain:    "http://invalid-domain-that-does-not-exist-12345.com",
					KeyPath:   "/key",
					LoginPath: "/login",
				}
			case "ConnectionRefused":
				testClient = &sdk.HttpSDK{
					Debug:     false,
					Domain:    "http://127.0.0.1:12345", // 不存在的端口
					KeyPath:   "/key",
					LoginPath: "/login",
				}
			case "InvalidAuth":
				testClient = &sdk.HttpSDK{
					Debug:     false,
					Domain:    "http://localhost:8090",
					KeyPath:   "/key",
					LoginPath: "/login",
				}
				// 设置无效的认证信息
				testClient.AuthToken(sdk.AuthToken{Token: "", Secret: "", Expired: 0})
			case "Timeout":
				testClient = &sdk.HttpSDK{
					Debug:     false,
					Domain:    "http://httpbin.org/delay/10", // 10秒超时
					KeyPath:   "/key",
					LoginPath: "/login",
				}
			}

			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					_, err := testClient.GetPublicKey()
					// 错误是预期的，我们只关心错误处理的性能
					if err == nil && scenario != "Normal" {
						b.Logf("错误场景[%s]意外成功", scenario)
					}
					// 不记录错误，只测试错误处理的性能开销
				}
			})
		})
	}
}

// BenchmarkMemoryEfficiency 内存使用效率测试
// 对比内存复用和频繁分配对性能的影响
func BenchmarkMemoryEfficiency(b *testing.B) {
	// 测试内存复用和垃圾回收效率
	b.Run("MemoryReuse", func(b *testing.B) {
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			// 预分配对象以减少GC压力
			requestObj := &sdk.AuthToken{
				Token:  "内存复用测试",
				Secret: "memory_efficiency_secret",
			}
			responseData := &sdk.AuthToken{}

			for pb.Next() {
				// 复用相同的对象
				requestObj.Token = fmt.Sprintf("内存复用测试_%d", time.Now().UnixNano())
				err := client.PostByAuth("/getUser", requestObj, responseData, false)
				if err != nil {
					b.Logf("内存复用请求失败: %v", err)
				}
			}
		})
	})

	b.Run("MemoryAllocation", func(b *testing.B) {
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				// 每次都创建新对象，增加GC压力
				requestObj := &sdk.AuthToken{
					Token:  fmt.Sprintf("内存分配测试_%d", time.Now().UnixNano()),
					Secret: "memory_allocation_secret",
				}
				responseData := &sdk.AuthToken{}

				err := client.PostByAuth("/getUser", requestObj, responseData, false)
				if err != nil {
					b.Logf("内存分配请求失败: %v", err)
				}
			}
		})
	})
}

// BenchmarkConnectionPooling 连接池和资源复用性能测试
// 对比不同连接管理策略的性能表现
func BenchmarkConnectionPooling(b *testing.B) {
	// 测试不同连接复用策略的性能
	connectionStrategies := []string{"ReuseConnection", "NewConnection", "KeepAlive"}

	for _, strategy := range connectionStrategies {
		b.Run(fmt.Sprintf("Connection_%s", strategy), func(b *testing.B) {
			var testClient *sdk.HttpSDK

			switch strategy {
			case "ReuseConnection":
				testClient = client // 复用已存在的连接
			case "NewConnection":
				// 每次创建新客户端（模拟无连接池）
				testClient = &sdk.HttpSDK{
					Debug:     false,
					Domain:    "http://localhost:8090",
					KeyPath:   "/key",
					LoginPath: "/login",
				}
				// 重新初始化认证
				testClient.SetPublicKey("BKNoaVapAlKywv5sXfag/LHa8mp6sdGFX6QHzfXIjBojkoCfCgZg6RPBXwLUUpPDzOC3uhDC60ECz2i1EbITsGY=")
				testClient.AuthToken(sdk.AuthToken{Token: benchmarkAccessToken, Secret: benchmarkTokenSecret, Expired: int64(benchmarkTokenExpire)})
			case "KeepAlive":
				testClient = client // 使用keep-alive的连接
			}

			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					requestObj := sdk.AuthToken{
						Token:  fmt.Sprintf("%s连接测试_%d", strategy, time.Now().UnixNano()),
						Secret: fmt.Sprintf("%s_secret", strategy),
					}
					responseData := sdk.AuthToken{}

					err := testClient.PostByAuth("/getUser", &requestObj, &responseData, false)
					if err != nil {
						b.Logf("%s连接请求失败: %v", strategy, err)
					}
				}
			})
		})
	}
}

// BenchmarkDataSerialization 数据序列化性能测试
// 测试不同数据结构的JSON序列化/反序列化性能
func BenchmarkDataSerialization(b *testing.B) {
	// 测试不同数据结构的序列化性能
	dataTypes := []string{"Simple", "Complex", "Large", "SpecialChars"}

	for _, dataType := range dataTypes {
		b.Run(fmt.Sprintf("Serialization_%s", dataType), func(b *testing.B) {
			var requestData interface{}

			switch dataType {
			case "Simple":
				requestData = &sdk.AuthToken{Token: "简单测试", Secret: "simple"}
			case "Complex":
				requestData = &sdk.AuthToken{
					Token:  "复杂测试数据包含更多信息",
					Secret: "complex_secret_with_more_data",
				}
			case "Large":
				largeData := make([]byte, 50*1024) // 50KB数据
				for i := range largeData {
					largeData[i] = byte(65 + (i % 26))
				}
				requestData = &sdk.AuthToken{Token: string(largeData), Secret: "large"}
			case "SpecialChars":
				requestData = &sdk.AuthToken{
					Token:  "特殊字符测试: 😀🎉🚀 中文测试 !@#$%^&*()",
					Secret: "special_chars_secret_123",
				}
			}

			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					responseData := &sdk.AuthToken{}
					err := client.PostByECC("/login", requestData, responseData)
					if err != nil {
						b.Logf("%s数据序列化请求失败: %v", dataType, err)
					}
				}
			})
		})
	}
}

// BenchmarkRequestFrequency 请求频率性能测试
// 测试不同请求频率下的性能表现和系统承载能力
func BenchmarkRequestFrequency(b *testing.B) {
	frequencies := []time.Duration{
		0,                      // 无延迟
		time.Millisecond * 10,  // 10ms
		time.Millisecond * 100, // 100ms
		time.Millisecond * 500, // 500ms
	}

	for _, freq := range frequencies {
		b.Run(fmt.Sprintf("Frequency_%v", freq), func(b *testing.B) {
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					requestObj := sdk.AuthToken{
						Token:  fmt.Sprintf("频率测试_%v_%d", freq, time.Now().UnixNano()),
						Secret: fmt.Sprintf("freq_secret_%v", freq),
					}
					responseData := sdk.AuthToken{}

					err := client.PostByAuth("/getUser", &requestObj, &responseData, false)
					if err != nil {
						b.Logf("频率[%v]请求失败: %v", freq, err)
					}

					// 模拟请求间隔
					if freq > 0 {
						time.Sleep(freq)
					}
				}
			})
		})
	}
}

// BenchmarkMixedOperations 混合操作性能测试
// 测试读写混合操作场景下的性能表现
func BenchmarkMixedOperations(b *testing.B) {
	operations := []string{"ReadHeavy", "WriteHeavy", "Mixed"}

	for _, opType := range operations {
		b.Run(fmt.Sprintf("Mixed_%s", opType), func(b *testing.B) {
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				localCounter := 0
				for pb.Next() {
					localCounter++

					switch opType {
					case "ReadHeavy":
						// 主要执行读取操作
						if localCounter%10 == 0 {
							// 偶尔执行写入
							requestObj := sdk.AuthToken{
								Token:  fmt.Sprintf("写入操作_%d", localCounter),
								Secret: "write_secret",
							}
							responseData := sdk.AuthToken{}
							client.PostByECC("/login", &requestObj, &responseData)
						} else {
							// 主要执行读取
							_, err := client.GetPublicKey()
							if err != nil {
								b.Logf("读取操作失败: %v", err)
							}
						}

					case "WriteHeavy":
						// 主要执行写入操作
						if localCounter%10 == 0 {
							// 偶尔执行读取
							_, err := client.GetPublicKey()
							if err != nil {
								b.Logf("读取操作失败: %v", err)
							}
						} else {
							// 主要执行写入
							requestObj := sdk.AuthToken{
								Token:  fmt.Sprintf("写入操作_%d", localCounter),
								Secret: "write_secret",
							}
							responseData := sdk.AuthToken{}
							client.PostByECC("/login", &requestObj, &responseData)
						}

					case "Mixed":
						// 均衡的读写操作
						if localCounter%2 == 0 {
							_, err := client.GetPublicKey()
							if err != nil {
								b.Logf("读取操作失败: %v", err)
							}
						} else {
							requestObj := sdk.AuthToken{
								Token:  fmt.Sprintf("混合操作_%d", localCounter),
								Secret: "mixed_secret",
							}
							responseData := sdk.AuthToken{}
							client.PostByECC("/login", &requestObj, &responseData)
						}
					}
				}
			})
		})
	}
}
