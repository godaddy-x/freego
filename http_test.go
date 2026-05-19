package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/godaddy-x/freego/utils/sdk"
	"github.com/valyala/fasthttp"
)

//go test -v http_test.go -bench=BenchmarkPubkey -benchmem -count=10

const (
	domain       = "http://localhost:8090"
	access_token = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIyMDU2NTMyMDAwMTM4ODU0NDAxIiwiYXVkIjoiIiwiaXNzIjoiIiwiZGV2IjoiQVBQIiwianRpIjoiOTU3NzdmZjc3ZjM4NDk3NWI0ZGI2N2M0ODdjZjE2ZTUiLCJleHQiOiIiLCJpYXQiOjAsImV4cCI6MTc5MTI0NjQxMn0=.uf8PPESiid09MguWrDCeqVAjE/nT/Is2hrP4Nl7CvWs="
	token_secret = "Ncvt4HENsQv++HWUgsvEDxLAUjfZgyChdy6RUOZHcSc="
	token_expire = 1791246412
)

var httpSDK = NewSDK()

func NewSDK() *sdk.HttpSDK {
	newObject := &sdk.HttpSDK{
		Domain:    domain,
		KeyPath:   "/key",
		LoginPath: "/login",
	}
	newObject.SetClientNo(1)
	_ = newObject.SetMLDSA87Object(newObject.ClientNo, pqClientPrk, pqServerPub)
	return newObject
}

// NewPlan2SDK Plan2（/login 等）使用 ML-DSA + ML-KEM，与 node/test/webapp AddPQCipher 配对。
func NewPlan2SDK() *sdk.HttpSDK {
	s := NewSDK()
	_ = s.SetMLDSA87Object(s.ClientNo, pqClientPrk, pqServerPub)
	return s
}

func TestGetPublicKey(t *testing.T) {
	_, publicKey, _, err := httpSDK.GetPublicKey()
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println("服务端公钥: ", publicKey)
}

func TestECCLogin(t *testing.T) {
	plan2SDK := NewPlan2SDK()
	requestData := sdk.AuthToken{Token: "AI工具人，鲨鱼宝宝！！！"}
	responseData := sdk.AuthToken{}
	if err := plan2SDK.PostByPlan2("/login", &requestData, &responseData); err != nil {
		fmt.Println(err)
	}
	fmt.Println(responseData)
}

func TestGetUser(t *testing.T) {
	httpSDK.AuthObject(func() (interface{}, error) {
		return &map[string]string{"username": "1234567890123456", "password": "1234567890123456"}, nil
	})
	httpSDK.AuthToken(sdk.AuthToken{Token: access_token, Secret: token_secret, Expired: token_expire})
	requestObj := sdk.AuthToken{Token: "AI工具人，鲨鱼宝宝！QWER123456@##！！", Secret: "安排测试下吧123456789@@@"}
	responseData := sdk.AuthToken{}
	if err := httpSDK.PostByPlan01("/getUser", &requestObj, &responseData, true); err != nil {
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

// BenchmarkHttpSDK_PostByPlan01 认证请求性能基准测试
// 测量PostByPlan01方法在并发场景下的性能表现和吞吐量
func BenchmarkHttpSDK_PostByPlan01(b *testing.B) {
	httpSDK := NewSDK()

	// 设置认证
	httpSDK.AuthToken(sdk.AuthToken{Token: access_token, Secret: token_secret, Expired: token_expire})

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			requestObj := sdk.AuthToken{Token: "基准测试请求"}
			responseData := sdk.AuthToken{}

			err := httpSDK.PostByPlan01("/getUser", &requestObj, &responseData, false)
			if err != nil {
				b.Logf("认证请求失败: %v", err)
			}
			if len(responseData.Token) == 0 {
				b.Fatal("token is nil")
			}
		}
	})
}

// BenchmarkHttpSDK_PostByPlan2 Plan2 压测（ML-KEM + ML-DSA）
// HttpSDK 的 plan2SharedB64/plan2KemCtB64 非并发安全，须每 goroutine 独立实例。
func BenchmarkHttpSDK_PostByPlan2(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		goroutineSDK := NewPlan2SDK()
		localCounter := 0
		for pb.Next() {
			localCounter++

			// 使用goroutine ID + 计数器生成唯一token，避免重放攻击检测
			token := fmt.Sprintf("Plan2并发测试_g%d_%d_%d", b.N, localCounter, time.Now().UnixNano())
			requestData := sdk.AuthToken{Token: token}
			responseData := sdk.AuthToken{}

			// 处理时间戳过期重试逻辑
			maxRetries := 2
			var err error

			for retry := 0; retry <= maxRetries; retry++ {
				err = goroutineSDK.PostByPlan2("/login", &requestData, &responseData)
				if responseData.Token == "" {
					panic("invalid token")
				}
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
				b.Logf("Plan2并发请求失败 (goroutine counter: %d, retries: %d): %v", localCounter, maxRetries, err)
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
