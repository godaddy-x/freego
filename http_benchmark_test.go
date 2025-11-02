package main

import (
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	ecc "github.com/godaddy-x/eccrypto"

	"github.com/godaddy-x/freego/ex"
	"github.com/godaddy-x/freego/utils/sdk"
)

var client = &sdk.HttpSDK{
	Debug:     false,
	Domain:    "http://localhost:8090",
	KeyPath:   "/key",
	LoginPath: "/login",
}

func init() {
	initClient()
}

func initClient() {
	var prk, _ = ecc.CreateECDSA()
	client.SetPrivateKey(prk)
	client.SetPublicKey("BKNoaVapAlKywv5sXfag/LHa8mp6sdGFX6QHzfXIjBojkoCfCgZg6RPBXwLUUpPDzOC3uhDC60ECz2i1EbITsGY=")
	token := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxOTg0NTQyMTQyMzY5ODkwMzA1IiwiYXVkIjoiIiwiaXNzIjoiIiwiaWF0IjowLCJleHAiOjE3NjMxOTYyOTIsImRldiI6IkFQUCIsImp0aSI6IlMrQjh0ZDh4ZGErRFVGeFliemxWNWc9PSIsImV4dCI6IiJ9.IDMBqkgRgl5cA0EOurLr/9ZdTFv7T6ACGLMN0cwZUT8="
	secret := "WZlK3jp1GNdXXi2lWM/DnfFkRbMSbO7JP/I+MhdblfLJZf6cZCzKsBi5i7pMfrFZuLnNj1Qf2cZIym1V/ti/LA=="
	expire := 1763196292
	client.AuthToken(sdk.AuthToken{Token: token, Secret: secret, Expired: int64(expire)})
}

//go test -v http_test.go -bench=BenchmarkPubkey -benchmem -count=10

// go test http_test.go -bench=BenchmarkPubkey  -benchmem -count=10 -cpuprofile cpuprofile.out -memprofile memprofile.out
// go test http_test.go -bench=BenchmarkECCLogin  -benchmem -count=10
func BenchmarkECCLogin(b *testing.B) {
	var successCount, errorCount, non200StatusCount, businessErrorCount int64

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			requestData := sdk.AuthToken{Token: "1234567890123456", Secret: "1234567890123456"}
			responseData := sdk.AuthToken{}
			if err := client.PostByECC("/login", &requestData, &responseData); err != nil {
				atomic.AddInt64(&errorCount, 1)

				// 分析错误类型：区分HTTP状态码错误和业务错误
				if throw, ok := err.(ex.Throw); ok {
					errorMsg := throw.Msg

					// 检查是否为HTTP网络/连接错误（通常表示非200状态码）
					if strings.Contains(errorMsg, "post request failed") ||
						strings.Contains(errorMsg, "lookup ") ||
						strings.Contains(errorMsg, "connection refused") ||
						strings.Contains(errorMsg, "timeout") ||
						strings.Contains(errorMsg, "network") {
						atomic.AddInt64(&non200StatusCount, 1)
						b.Errorf("HTTP状态码错误 #%d: %v", atomic.LoadInt64(&non200StatusCount), err)
					} else if throw.Code > 0 && throw.Code != 200 {
						// 业务逻辑错误（服务器返回的错误状态码）
						atomic.AddInt64(&businessErrorCount, 1)
						b.Errorf("业务错误(状态码%d) #%d: %s",
							throw.Code, atomic.LoadInt64(&businessErrorCount), throw.Msg)
					} else {
						// 其他协议错误（如签名验证失败、解密失败等）
						b.Errorf("协议错误 #%d: %v", atomic.LoadInt64(&errorCount), err)
					}
				} else {
					b.Errorf("未知错误类型 #%d: %v", atomic.LoadInt64(&errorCount), err)
				}
				continue
			}
			atomic.AddInt64(&successCount, 1)
		}
	})

	// 报告测试结果
	totalRequests := successCount + errorCount
	if totalRequests > 0 {
		errorRate := float64(errorCount) / float64(totalRequests) * 100
		b.Logf("ECC登录测试完成:")
		b.Logf("  总请求: %d", totalRequests)
		b.Logf("  成功请求: %d", successCount)
		b.Logf("  失败请求: %d", errorCount)
		b.Logf("  ├── HTTP状态码错误: %d", non200StatusCount)
		b.Logf("  ├── 业务逻辑错误: %d", businessErrorCount)
		b.Logf("  └── 协议/其他错误: %d", errorCount-non200StatusCount-businessErrorCount)
		b.Logf("  错误率: %.2f%%", errorRate)

		// 如果错误率过高，标记测试为失败
		if errorRate > 10.0 { // 允许最多10%的错误率
			b.Fatalf("ECC登录错误率过高: %.2f%% (HTTP错误: %d, 业务错误: %d)",
				errorRate, non200StatusCount, businessErrorCount)
		}
	}
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

// go test http_test.go -bench=BenchmarkPubkey  -benchmem -count=10 -cpuprofile cpuprofile.out -memprofile memprofile.out
// go test http_test.go -bench=BenchmarkGetUser  -benchmem -count=10
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
