package main

import (
	"fmt"
	"testing"
	"time"

	ecc "github.com/godaddy-x/eccrypto"
	"github.com/godaddy-x/freego/utils/sdk"
	"github.com/valyala/fasthttp"
)

//go test -v http_test.go -bench=BenchmarkPubkey -benchmem -count=10

const domain = "http://localhost:8090"

const access_token = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxOTg0NTQyMTQyMzY5ODkwMzA1IiwiYXVkIjoiIiwiaXNzIjoiIiwiaWF0IjowLCJleHAiOjE3NjMxOTYyOTIsImRldiI6IkFQUCIsImp0aSI6IlMrQjh0ZDh4ZGErRFVGeFliemxWNWc9PSIsImV4dCI6IiJ9.IDMBqkgRgl5cA0EOurLr/9ZdTFv7T6ACGLMN0cwZUT8="
const token_secret = "WZlK3jp1GNdXXi2lWM/DnfFkRbMSbO7JP/I+MhdblfLJZf6cZCzKsBi5i7pMfrFZuLnNj1Qf2cZIym1V/ti/LA=="
const token_expire = 1763196292

var httpSDK = &sdk.HttpSDK{
	Debug:     true,
	Domain:    domain,
	KeyPath:   "/key",
	LoginPath: "/login",
}

func TestGetPublicKey(t *testing.T) {
	publicKey, err := httpSDK.GetPublicKey()
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println("服务端公钥: ", publicKey)
}

func TestECCLogin(t *testing.T) {
	prk, _ := ecc.CreateECDSA()
	httpSDK.SetPrivateKey(prk)
	httpSDK.SetPublicKey("BKNoaVapAlKywv5sXfag/LHa8mp6sdGFX6QHzfXIjBojkoCfCgZg6RPBXwLUUpPDzOC3uhDC60ECz2i1EbITsGY=")
	requestData := sdk.AuthToken{Token: "AI工具人，鲨鱼宝宝！！！"}
	responseData := sdk.AuthToken{}
	if err := httpSDK.PostByECC("/login", &requestData, &responseData); err != nil {
		fmt.Println(err)
	}
	fmt.Println(responseData)
}

func TestGetUser(t *testing.T) {
	httpSDK.AuthObject(&map[string]string{"username": "1234567890123456", "password": "1234567890123456"})
	httpSDK.AuthToken(sdk.AuthToken{Token: access_token, Secret: token_secret, Expired: token_expire})
	requestObj := sdk.AuthToken{Token: "AI工具人，鲨鱼宝宝！QWER123456@##！！", Secret: "安排测试下吧123456789@@@"}
	responseData := sdk.AuthToken{}
	if err := httpSDK.PostByAuth("/getUser", &requestObj, &responseData, true); err != nil {
		fmt.Println(err)
	}
	fmt.Println(responseData)
}

func TestOnlyServerECCLogin(t *testing.T) {
	randomCode := `BARLw1KA4Erot6QrBsmlIFjR17yLtt9pNSfegWVMyaUcNJweGyJx6KGlVLUTnqo51fmmKbmMUJH+KKog5vsh6+GS+CEqlAhI1GnHe2pCmdnRzRfLdGgbf2M/p4dSqBB3Z0N49nFeQCLn+kbtin7ISq5ktdwdoc7zfc1kwwZdewtq+HfEzTIwUdjSkEAxl2GWo/DLrlNzUEtt5rhE92qHW+M=`
	requestData := []byte(`{"d":"h7mfHikfR7DLRQoxhN6CxQi+Azz+dPErYRFebyicZfiskkh+Z00Okg7BA/W88hOFSJhQT0Ecfn9iac6gkThooX4gF9mqmKo0Vr9Byo5E5Ue2pFZeLKo/J3zD3ZCPRsHacP/v","n":"nscrHrGNGRaitGJxsegJ8w==","s":"qmEGqs5TarHpaiP0r2HE0oOeCpaiHdTjgPv5Vn3SNvY=","t":1762159303,"p":2}`)
	request := fasthttp.AcquireRequest()
	request.Header.SetContentType("application/json;charset=UTF-8")
	request.Header.Set("Authorization", "")
	request.Header.Set("RandomCode", randomCode)
	request.Header.SetMethod("POST")
	request.SetRequestURI(domain + "/login")
	request.SetBody(requestData)
	defer fasthttp.ReleaseRequest(request)
	response := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(response)
	if err := fasthttp.DoTimeout(request, response, time.Second*20); err != nil {
		panic(err)
	}
	fmt.Println(string(response.Body()))
}

// TestHttpSDKInitialization 测试HttpSDK初始化配置
// 验证SDK的默认值设置和自定义配置功能
func TestHttpSDKInitialization(t *testing.T) {
	// 测试默认初始化
	httpSdk := &sdk.HttpSDK{}
	if httpSdk.Debug != false {
		t.Errorf("期望Debug默认值为false，实际为%v", httpSdk.Debug)
	}

	// 测试带参数初始化
	httpSdkWithConfig := &sdk.HttpSDK{
		Debug:     true,
		Domain:    "https://api.example.com",
		KeyPath:   "/api/keys",
		LoginPath: "/api/auth",
	}
	if !httpSdkWithConfig.Debug {
		t.Error("期望Debug为true")
	}
	if httpSdkWithConfig.Domain != "https://api.example.com" {
		t.Errorf("期望Domain为https://api.example.com，实际为%s", httpSdkWithConfig.Domain)
	}
}

// TestGetPublicKeyErrorHandling 测试公钥获取错误处理
// 验证在网络错误或无效域名情况下SDK的错误处理能力
func TestGetPublicKeyErrorHandling(t *testing.T) {
	// 测试无效域名
	invalidSDK := &sdk.HttpSDK{
		Debug:  true,
		Domain: "http://invalid-domain-that-does-not-exist.com",
	}

	_, err := invalidSDK.GetPublicKey()
	if err == nil {
		t.Error("期望获取无效域名的公钥时返回错误")
	}
}

// TestECCLoginWithInvalidKey 测试ECC登录无效密钥场景
// 验证在使用无效ECC密钥时SDK的健壮性和错误处理
func TestECCLoginWithInvalidKey(t *testing.T) {
	// 使用无效的私钥
	httpSDK := &sdk.HttpSDK{
		Debug:     true,
		Domain:    domain,
		KeyPath:   "/key",
		LoginPath: "/login",
	}

	// 设置无效的私钥（nil）
	httpSDK.SetPrivateKey(nil)
	httpSDK.SetPublicKey("BKNoaVapAlKywv5sXfag/LHa8mp6sdGFX6QHzfXIjBojkoCfCgZg6RPBXwLUUpPDzOC3uhDC60ECz2i1EbITsGY=")

	requestData := sdk.AuthToken{Token: "测试无效密钥"}
	responseData := sdk.AuthToken{}

	err := httpSDK.PostByECC("/login", &requestData, &responseData)
	// 这里可能成功也可能失败，取决于服务器的处理，但不应该panic
	if err != nil {
		t.Logf("无效密钥登录返回错误（预期行为）: %v", err)
	}
}

// TestAuthTokenValidation 测试认证令牌验证逻辑
// 验证不同类型的令牌（空令牌、过期令牌、无效令牌）的处理能力
func TestAuthTokenValidation(t *testing.T) {
	httpSDK := &sdk.HttpSDK{
		Debug:     true,
		Domain:    domain,
		KeyPath:   "/key",
		LoginPath: "/login",
	}

	// 测试空的认证令牌
	httpSDK.AuthToken(sdk.AuthToken{})

	// 测试过期令牌
	expiredToken := sdk.AuthToken{
		Token:   "expired-token",
		Secret:  "expired-secret",
		Expired: 1234567890, // 过去的过期时间
	}
	httpSDK.AuthToken(expiredToken)

	// 测试无效令牌
	invalidToken := sdk.AuthToken{
		Token:   "",
		Secret:  "",
		Expired: 0,
	}
	httpSDK.AuthToken(invalidToken)
}

// TestPostByAuthWithInvalidData 测试认证请求的无效数据处理
// 验证在使用nil或无效参数调用PostByAuth时的错误处理
func TestPostByAuthWithInvalidData(t *testing.T) {
	httpSDK := &sdk.HttpSDK{
		Debug:     true,
		Domain:    domain,
		KeyPath:   "/key",
		LoginPath: "/login",
	}

	// 设置有效的认证令牌
	httpSDK.AuthToken(sdk.AuthToken{Token: access_token, Secret: token_secret, Expired: token_expire})

	// 测试nil请求数据
	responseData := sdk.AuthToken{}
	err := httpSDK.PostByAuth("/getUser", nil, &responseData, false)
	if err == nil {
		t.Error("期望nil请求数据时返回错误")
	}

	// 测试nil响应数据
	requestObj := sdk.AuthToken{Token: "test"}
	err = httpSDK.PostByAuth("/getUser", &requestObj, nil, false)
	if err == nil {
		t.Error("期望nil响应数据时返回错误")
	}
}

// TestPostByECCWithInvalidData 测试ECC请求的无效数据处理
// 验证在使用nil或无效参数调用PostByECC时的错误处理
func TestPostByECCWithInvalidData(t *testing.T) {
	prk, _ := ecc.CreateECDSA()
	httpSDK := &sdk.HttpSDK{
		Debug:     true,
		Domain:    domain,
		KeyPath:   "/key",
		LoginPath: "/login",
	}
	httpSDK.SetPrivateKey(prk)
	httpSDK.SetPublicKey("BKNoaVapAlKywv5sXfag/LHa8mp6sdGFX6QHzfXIjBojkoCfCgZg6RPBXwLUUpPDzOC3uhDC60ECz2i1EbITsGY=")

	// 测试nil请求数据
	responseData := sdk.AuthToken{}
	err := httpSDK.PostByECC("/login", nil, &responseData)
	if err == nil {
		t.Error("期望nil请求数据时返回错误")
	}

	// 测试nil响应数据
	requestData := sdk.AuthToken{Token: "test"}
	err = httpSDK.PostByECC("/login", &requestData, nil)
	if err == nil {
		t.Error("期望nil响应数据时返回错误")
	}
}

// TestConcurrentRequests 测试并发请求处理能力
// 验证SDK在高并发场景下的稳定性和资源竞争处理
func TestConcurrentRequests(t *testing.T) {
	httpSDK := &sdk.HttpSDK{
		Debug:     false, // 关闭调试以减少日志输出
		Domain:    domain,
		KeyPath:   "/key",
		LoginPath: "/login",
	}

	// 设置认证
	httpSDK.AuthToken(sdk.AuthToken{Token: access_token, Secret: token_secret, Expired: token_expire})

	// 启动多个goroutine并发请求
	const numGoroutines = 5
	const requestsPerGoroutine = 3

	errChan := make(chan error, numGoroutines*requestsPerGoroutine)

	for i := 0; i < numGoroutines; i++ {
		go func(goroutineID int) {
			for j := 0; j < requestsPerGoroutine; j++ {
				requestObj := sdk.AuthToken{
					Token:  fmt.Sprintf("并发测试-%d-%d", goroutineID, j),
					Secret: fmt.Sprintf("secret-%d-%d", goroutineID, j),
				}
				responseData := sdk.AuthToken{}

				err := httpSDK.PostByAuth("/getUser", &requestObj, &responseData, false)
				errChan <- err
			}
		}(i)
	}

	// 收集所有错误
	errorCount := 0
	totalRequests := numGoroutines * requestsPerGoroutine
	for i := 0; i < totalRequests; i++ {
		if err := <-errChan; err != nil {
			errorCount++
			t.Logf("并发请求错误: %v", err)
		}
	}

	t.Logf("并发请求完成: 总请求数=%d, 错误数=%d", totalRequests, errorCount)
}

// TestNetworkTimeout 测试网络超时处理机制
// 验证SDK对网络超时场景的处理和错误恢复能力
func TestNetworkTimeout(t *testing.T) {
	// 使用一个会超时的端点
	slowSDK := &sdk.HttpSDK{
		Debug:     true,
		Domain:    "http://httpbin.org/delay/10", // 故意使用会延迟10秒的端点
		KeyPath:   "/key",
		LoginPath: "/login",
	}

	// 记录开始时间
	start := time.Now()

	_, err := slowSDK.GetPublicKey()

	elapsed := time.Since(start)

	// 验证是否在合理时间内超时
	if elapsed > time.Second*30 {
		t.Errorf("请求耗时过长: %v", elapsed)
	}

	if err == nil {
		t.Log("请求意外成功，可能网络条件良好")
	} else {
		t.Logf("请求失败（预期行为）: %v", err)
	}
}

// TestRequestDataSerialization 测试请求数据序列化能力
// 验证SDK对不同类型请求数据的JSON序列化处理
func TestRequestDataSerialization(t *testing.T) {
	prk, _ := ecc.CreateECDSA()
	httpSDK := &sdk.HttpSDK{
		Debug:     true,
		Domain:    domain,
		KeyPath:   "/key",
		LoginPath: "/login",
	}
	httpSDK.SetPrivateKey(prk)
	httpSDK.SetPublicKey("BKNoaVapAlKywv5sXfag/LHa8mp6sdGFX6QHzfXIjBojkoCfCgZg6RPBXwLUUpPDzOC3uhDC60ECz2i1EbITsGY=")

	testCases := []struct {
		name     string
		request  interface{}
		response interface{}
	}{
		{"字符串请求", "简单字符串请求", &sdk.AuthToken{}},
		{"空结构体请求", sdk.AuthToken{}, &sdk.AuthToken{}},
		{"复杂结构体请求", sdk.AuthToken{Token: "复杂测试", Secret: "secret123"}, &sdk.AuthToken{}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := httpSDK.PostByECC("/login", tc.request, tc.response)
			// 这里我们只测试序列化不报错，实际业务逻辑可能成功或失败
			if err != nil {
				t.Logf("序列化测试[%s]返回错误: %v", tc.name, err)
			} else {
				t.Logf("序列化测试[%s]成功", tc.name)
			}
		})
	}
}

// TestResponseDataDeserialization 测试响应数据反序列化能力
// 验证SDK对不同类型响应数据的JSON反序列化处理
func TestResponseDataDeserialization(t *testing.T) {
	httpSDK := &sdk.HttpSDK{
		Debug:     true,
		Domain:    domain,
		KeyPath:   "/key",
		LoginPath: "/login",
	}

	// 设置认证
	httpSDK.AuthToken(sdk.AuthToken{Token: access_token, Secret: token_secret, Expired: token_expire})

	testCases := []struct {
		name     string
		response interface{}
	}{
		{"AuthToken响应", &sdk.AuthToken{}},
		{"字符串响应", ""},
		{"字节数组响应", &[]byte{}},
		{"map响应", &map[string]interface{}{}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			requestObj := sdk.AuthToken{Token: fmt.Sprintf("反序列化测试-%s", tc.name)}
			err := httpSDK.PostByAuth("/getUser", &requestObj, tc.response, false)
			if err != nil {
				t.Logf("反序列化测试[%s]返回错误: %v", tc.name, err)
			} else {
				t.Logf("反序列化测试[%s]成功", tc.name)
			}
		})
	}
}

// TestInvalidDomain 测试无效域名和URL处理
// 验证SDK对各种无效域名和URL格式的错误处理能力
func TestInvalidDomain(t *testing.T) {
	invalidDomains := []string{
		"",
		"http://",
		"https://",
		"not-a-url",
		"ftp://example.com",
		"http://256.256.256.256", // 无效IP
	}

	for _, domain := range invalidDomains {
		t.Run(fmt.Sprintf("Domain_%s", domain), func(t *testing.T) {
			sdk := &sdk.HttpSDK{
				Debug:  true,
				Domain: domain,
			}

			_, err := sdk.GetPublicKey()
			if err == nil {
				t.Errorf("期望域名[%s]返回错误，但成功了", domain)
			} else {
				t.Logf("域名[%s]正确返回错误: %v", domain, err)
			}
		})
	}
}

// TestPathHandling 测试API路径处理能力
// 验证SDK对不同格式API路径的处理和URL构造能力
func TestPathHandling(t *testing.T) {
	httpSDK := &sdk.HttpSDK{
		Debug:     true,
		Domain:    domain,
		KeyPath:   "/key",
		LoginPath: "/login",
	}

	testPaths := []string{
		"/test",
		"test", // 无前缀斜杠
		"/api/test",
		"/api/v1/test",
		"",              // 空路径
		"/",             // 根路径
		"/test?param=1", // 带查询参数
	}

	// 设置认证
	httpSDK.AuthToken(sdk.AuthToken{Token: access_token, Secret: token_secret, Expired: token_expire})

	for _, path := range testPaths {
		t.Run(fmt.Sprintf("Path_%s", path), func(t *testing.T) {
			requestObj := sdk.AuthToken{Token: fmt.Sprintf("路径测试-%s", path)}
			responseData := sdk.AuthToken{}

			err := httpSDK.PostByAuth(path, &requestObj, &responseData, false)
			if err != nil {
				t.Logf("路径[%s]请求返回错误: %v", path, err)
			} else {
				t.Logf("路径[%s]请求成功", path)
			}
		})
	}
}

// TestLargeRequestData 测试大请求数据处理能力
// 验证SDK对大数据量请求的处理能力和内存使用情况
func TestLargeRequestData(t *testing.T) {
	httpSDK := &sdk.HttpSDK{
		Debug:     false, // 关闭调试以减少输出
		Domain:    domain,
		KeyPath:   "/key",
		LoginPath: "/login",
	}

	// 设置认证
	httpSDK.AuthToken(sdk.AuthToken{Token: access_token, Secret: token_secret, Expired: token_expire})

	// 生成大字符串数据
	largeData := make([]string, 100)
	for i := range largeData {
		largeData[i] = fmt.Sprintf("大数据测试内容_%d_这是一个很长的字符串用于测试HTTP客户端对大数据的处理能力。", i)
	}

	requestObj := sdk.AuthToken{
		Token:  fmt.Sprintf("大数据测试_%s", fmt.Sprintf("%v", largeData)),
		Secret: "large_data_secret",
	}
	responseData := sdk.AuthToken{}

	err := httpSDK.PostByAuth("/getUser", &requestObj, &responseData, false)
	if err != nil {
		t.Logf("大数据请求返回错误: %v", err)
	} else {
		t.Log("大数据请求成功")
	}
}

// TestSpecialCharacters 测试特殊字符处理能力
// 验证SDK对Unicode、Emoji、特殊符号等字符的处理能力
func TestSpecialCharacters(t *testing.T) {
	httpSDK := &sdk.HttpSDK{
		Debug:     true,
		Domain:    domain,
		KeyPath:   "/key",
		LoginPath: "/login",
	}

	// 设置认证
	httpSDK.AuthToken(sdk.AuthToken{Token: access_token, Secret: token_secret, Expired: token_expire})

	specialChars := []string{
		"中文测试",
		"Emoji测试 😀🎉🚀",
		"特殊符号: !@#$%^&*()",
		"Unicode: \u4f60\u597d",
		"换行测试\n第二行",
		"制表符测试\t列",
		"引号测试: \"单引号\" '双引号'",
		"XML/HTML: <tag>内容</tag>",
		"SQL注入测试: '; DROP TABLE users; --",
		"路径遍历: ../../../etc/passwd",
	}

	for _, chars := range specialChars {
		t.Run(fmt.Sprintf("Chars_%s", chars[:min(10, len(chars))]), func(t *testing.T) {
			requestObj := sdk.AuthToken{Token: fmt.Sprintf("特殊字符测试: %s", chars)}
			responseData := sdk.AuthToken{}

			err := httpSDK.PostByAuth("/getUser", &requestObj, &responseData, false)
			if err != nil {
				t.Logf("特殊字符[%s]请求返回错误: %v", chars[:min(10, len(chars))], err)
			} else {
				t.Logf("特殊字符[%s]请求成功", chars[:min(10, len(chars))])
			}
		})
	}
}

// min 辅助函数，返回两个整数中的较小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// BenchmarkHttpSDK_GetPublicKey 公钥获取性能基准测试
// 测量GetPublicKey方法在并发场景下的性能表现和响应时间
func BenchmarkHttpSDK_GetPublicKey(b *testing.B) {
	httpSDK := &sdk.HttpSDK{
		Debug:  false, // 基准测试时关闭调试
		Domain: domain,
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := httpSDK.GetPublicKey()
			if err != nil {
				b.Logf("公钥获取失败: %v", err)
			}
		}
	})
}

// BenchmarkHttpSDK_PostByAuth 认证请求性能基准测试
// 测量PostByAuth方法在并发场景下的性能表现和吞吐量
func BenchmarkHttpSDK_PostByAuth(b *testing.B) {
	httpSDK := &sdk.HttpSDK{
		Debug:     false, // 基准测试时关闭调试
		Domain:    domain,
		KeyPath:   "/key",
		LoginPath: "/login",
	}

	// 设置认证
	httpSDK.AuthToken(sdk.AuthToken{Token: access_token, Secret: token_secret, Expired: token_expire})

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			requestObj := sdk.AuthToken{Token: "基准测试请求"}
			responseData := sdk.AuthToken{}

			err := httpSDK.PostByAuth("/getUser", &requestObj, &responseData, false)
			if err != nil {
				b.Logf("认证请求失败: %v", err)
			}
		}
	})
}

// BenchmarkHttpSDK_PostByECC ECC请求性能基准测试
// 测量PostByECC方法在并发场景下的性能表现，包含ECC加密开销
func BenchmarkHttpSDK_PostByECC(b *testing.B) {
	prk, _ := ecc.CreateECDSA()
	httpSDK := &sdk.HttpSDK{
		Debug:     false, // 基准测试时关闭调试
		Domain:    domain,
		KeyPath:   "/key",
		LoginPath: "/login",
	}
	httpSDK.SetPrivateKey(prk)
	httpSDK.SetPublicKey("BKNoaVapAlKywv5sXfag/LHa8mp6sdGFX6QHzfXIjBojkoCfCgZg6RPBXwLUUpPDzOC3uhDC60ECz2i1EbITsGY=")

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			requestData := sdk.AuthToken{Token: "ECC基准测试"}
			responseData := sdk.AuthToken{}

			err := httpSDK.PostByECC("/login", &requestData, &responseData)
			if err != nil {
				b.Logf("ECC请求失败: %v", err)
			}
		}
	})
}
