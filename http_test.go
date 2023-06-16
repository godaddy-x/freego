package main

import (
	"fmt"
	"github.com/godaddy-x/freego/utils/sdk"
	"testing"
)

//go test -v http_test.go -bench=BenchmarkPubkey -benchmem -count=10

const domain = "http://localhost:8090"

const access_token = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxNjY3MzYwOTMyMzMxNzgyMTQ1IiwiYXVkIjoiIiwiaXNzIjoiIiwiaWF0IjowLCJleHAiOjE2ODc1NzQzOTgsImRldiI6IkFQUCIsImp0aSI6IlRPVDBmUVY1bCtnL2JRQ3JlQmNrTEE9PSIsImV4dCI6IiJ9.jXXBaG/ne0oDyTzRwM39k2ivCTZGMxLp1SmBxJT0flQ="
const token_secret = "eM5wygtspL1AVqpHy*kT^j#lKIUQc7g22ifj4myS#lK!ZC@diQbx5OGpau6KXGA="

var httpSDK = &sdk.HttpSDK{
	Debug:     true,
	Domain:    domain,
	KeyPath:   "/key",
	LoginPath: "/login",
}

func TestGetPublicKey(t *testing.T) {
	//publicKey, err := httpSDK.GetPublicKey()
	//if err != nil {
	//	fmt.Println(err)
	//}
	//fmt.Println("服务端公钥: ", publicKey)
}

func TestECCLogin(t *testing.T) {
	requestData := map[string]string{"username": "1234567890123456", "password": "1234567890123456"}
	responseData := sdk.AuthToken{}
	if err := httpSDK.PostByECC("/login", &requestData, &responseData); err != nil {
		fmt.Println(err)
	}
	fmt.Println(responseData)
}

func TestGetUser(t *testing.T) {
	httpSDK.AuthObject(&map[string]string{"username": "1234567890123456", "password": "1234567890123456"})
	//httpSDK.AuthToken(sdk.AuthToken{Token: access_token, Secret: token_secret})
	requestObj := map[string]interface{}{"uid": 123, "name": "我爱中国/+_=/1df", "limit": 20, "offset": 5}
	responseData := map[string]string{}
	if err := httpSDK.PostByAuth("/getUser", &requestObj, &responseData, true); err != nil {
		fmt.Println(err)
	}
	fmt.Println(responseData)
}

func TestHAX(t *testing.T) {
	//httpSDK.AuthObject(&map[string]string{"username": "1234567890123456", "password": "1234567890123456"})
	//httpSDK.AuthToken(sdk.AuthToken{Token: access_token, Secret: token_secret})
	requestObj := map[string]interface{}{"uid": 123, "name": "我爱中国/+_=/1df", "limit": 20, "offset": 5}
	responseData := map[string]string{}
	if err := httpSDK.PostByHAX("/login", &requestObj, &responseData); err != nil {
		fmt.Println(err)
	}
	fmt.Println(responseData)
}

func BenchmarkGetUser(b *testing.B) {
	b.StopTimer()
	b.StartTimer()
	for i := 0; i < b.N; i++ { //use b.N for looping
		httpSDK.AuthToken(sdk.AuthToken{Token: access_token, Secret: token_secret})
		requestObj := map[string]interface{}{"uid": 123, "name": "我爱中国/+_=/1df", "limit": 20, "offset": 5}
		responseData := map[string]string{}
		if err := httpSDK.PostByAuth("/getUser", &requestObj, &responseData, false); err != nil {
			fmt.Println(err)
		}
	}
}

// go test http_test.go -bench=BenchmarkPubkey  -benchmem -count=10 -cpuprofile cpuprofile.out -memprofile memprofile.out
// go test http_test.go -bench=BenchmarkGetUser  -benchmem -count=10
func BenchmarkECCLogin(b *testing.B) {
	b.StopTimer()
	b.StartTimer()
	for i := 0; i < b.N; i++ { //use b.N for looping
		requestData := map[string]string{"username": "1234567890123456", "password": "1234567890123456"}
		responseData := sdk.AuthToken{}
		if err := httpSDK.PostByECC("/login", &requestData, &responseData); err != nil {
			fmt.Println(err)
		}
	}
}
