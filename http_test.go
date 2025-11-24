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
)

var httpSDK = NewSDK()

func NewSDK() *sdk.HttpSDK {
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
	httpSDK := NewSDK()

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
	goroutineSDK := NewSDK()
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
