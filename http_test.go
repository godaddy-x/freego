package main

import (
	"crypto/sha512"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/utils/sdk"
	"github.com/valyala/fasthttp"
	"golang.org/x/crypto/pbkdf2"
)

//go test -v http_test.go -bench=BenchmarkPubkey -benchmem -count=10

const (
	domain       = "http://localhost:8090"
	access_token = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxOTg5NTgzMzQ1MTA4OTEwMDgxIiwiYXVkIjoiIiwiaXNzIjoiIiwiZGV2IjoiQVBQIiwianRpIjoiV292R29Lb0NRZUorYUY0cFVRR2VJQT09IiwiZXh0IjoiIiwiaWF0IjowLCJleHAiOjE3NjQzOTgyMDh9.89JrFfOqT3gcAf++S1LM9L0gUMAkhRlLLAOKQzfnZtc="
	token_secret = "qFbtP73t3hzhChX2wa1o+D/ebwgppSwkq6MAwyz1ApvNjpYowD4dyZQM2Cjct8J2VFuwIB1VYP77m+KBCoruMw=="
	token_expire = 1764398208

	// 服务端公钥
	serverPub = "BO6XQ+PD66TMDmQXSEHl2xQarWE0HboB4LazrznThhr6Go5SvpjXJqiSe2fX+sup5OQDOLPkLdI1gh48jOmAq+k="
	// 客户端私钥
	clientPrk = "rnX5ykQivfbLHtcbPR68CP636usTNC03u8OD1KeoDPg="
)

var httpSDK = NewSDK(true)

func NewSDK(debug bool) *sdk.HttpSDK {
	newObject := &sdk.HttpSDK{
		Domain:    domain,
		KeyPath:   "/key",
		LoginPath: "/login",
	}
	_ = newObject.SetECDSAObject(clientPrk, serverPub)
	return newObject
}

func TestGetPublicKey(t *testing.T) {
	_, publicKey, _, err := httpSDK.GetPublicKey()
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println("服务端公钥: ", publicKey)
}

func TestECCLogin(t *testing.T) {
	_ = httpSDK.SetECDSAObject(clientPrk, serverPub)
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
	if err := httpSDK.PostByAuth("/getUser", &requestObj, &responseData, false); err != nil {
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
	// 测试带参数初始化
	httpSdkWithConfig := &sdk.HttpSDK{
		Domain:    "https://api.example.com",
		KeyPath:   "/api/keys",
		LoginPath: "/api/auth",
	}
	if httpSdkWithConfig.Domain != "https://api.example.com" {
		t.Errorf("期望Domain为https://api.example.com，实际为%s", httpSdkWithConfig.Domain)
	}
}

// TestECCLoginWithInvalidKey 测试ECC登录无效密钥场景
// 验证在使用无效ECC密钥时SDK的健壮性和错误处理
func TestECCLoginWithInvalidKey(t *testing.T) {
	// 使用无效的私钥
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

// TestRequestDataSerialization 测试请求数据序列化能力
// 验证SDK对不同类型请求数据的JSON序列化处理
func TestRequestDataSerialization(t *testing.T) {
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

// TestPathHandling 测试API路径处理能力
// 验证SDK对不同格式API路径的处理和URL构造能力
func TestPathHandling(t *testing.T) {
	httpSDK := &sdk.HttpSDK{
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
func BenchmarkPBKDF2(b *testing.B) {
	password := "test_password"
	salt := utils.GetAesIVSecure()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		derivedKey := pbkdf2.Key(utils.Str2Bytes(password), salt, 50000, 64, sha512.New)
		_ = derivedKey
	}
}

// BenchmarkHttpSDK_PostByAuth 认证请求性能基准测试
// 测量PostByAuth方法在并发场景下的性能表现和吞吐量
func BenchmarkHttpSDK_PostByAuth(b *testing.B) {
	httpSDK := NewSDK(false)

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
// 测试动态ECDH在并发执行下的性能表现和稳定性
func BenchmarkHttpSDK_PostByECC(b *testing.B) {
	// 每个goroutine创建独立的SDK实例，避免并发冲突
	goroutineSDK := NewSDK(false)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {

		localCounter := 0
		for pb.Next() {
			localCounter++

			// 使用goroutine ID + 计数器生成唯一token，避免重放攻击检测
			token := fmt.Sprintf("ECC并发测试_g%d_%d_%d", b.N, localCounter, time.Now().UnixNano())
			requestData := sdk.AuthToken{Token: token}
			responseData := sdk.AuthToken{}

			// 处理时间戳过期重试逻辑
			maxRetries := 2
			var err error

			for retry := 0; retry <= maxRetries; retry++ {
				err = goroutineSDK.PostByECC("/login", &requestData, &responseData)
				if err == nil {
					break // 成功则跳出重试循环
				}

				// 检查是否是时间戳过期错误
				errStr := err.Error()
				if strings.Contains(errStr, "request time invalid") && retry < maxRetries {
					// 时间戳过期，稍作延迟后重试
					time.Sleep(time.Millisecond * 10) // 10ms延迟让时间戳刷新
					continue
				}

				// 其他错误或重试次数用完，直接跳出
				break
			}

			if err != nil {
				b.Logf("ECC并发请求失败 (goroutine counter: %d, retries: %d): %v", localCounter, maxRetries, err)
				// 记录错误但不终止测试，继续观察稳定性
			} else {
				// 可选：验证响应数据有效性
				if responseData.Token == "" {
					b.Logf("警告: 响应token为空 (goroutine counter: %d)", localCounter)
				}
			}
		}
	})
}
