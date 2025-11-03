package main

import (
	"fmt"
	"github.com/valyala/fasthttp"
	"testing"
	"time"

	ecc "github.com/godaddy-x/eccrypto"

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
			request.SetRequestURI(domain + "/login")
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
