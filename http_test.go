package main

import (
	"fmt"
	"github.com/godaddy-x/freego/utils/sdk"
	"testing"
)

//go test -v http_test.go -bench=BenchmarkPubkey -benchmem -count=10

const domain = "http://localhost:8090"

const access_token = "eyJhbGciOiIiLCJ0eXAiOiIifQ==.eyJzdWIiOiIxODI5MDI5MjA2NjE3NDg5NDA5IiwiYXVkIjoiIiwiaXNzIjoiIiwiaWF0IjowLCJleHAiOjE3MjYxMTkxMTksImRldiI6IldFQiIsImp0aSI6IlRqeEdHRFNXajVrVnlWalM2dGhKdWc9PSIsImV4dCI6IiJ9.eXWi1BqMihY654FVoSOWZsiAyy41VMIsHWUIBk+gzIs="
const token_secret = "0b324d948d0aac4581ede07b23967be48bfd9a55a44bbb1e8484992619f1d928b50e1fbe3d6183fa6cc161cfb56d3360f1fbc7f075c744dc9bdd9a0b2a2a6dc13c8d843dbdba95f2b0b490cf8b4880d170a5ecfe3326904e20347866829df0f5"
const token_expire = 1726119119

var httpSDK = &sdk.HttpSDK{
	Debug:      false,
	Domain:     domain,
	AuthDomain: domain,
	KeyPath:    "/key",
	LoginPath:  "/login",
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
	fmt.Println("token: ", responseData.Token)
	fmt.Println("secret: ", responseData.Secret)
	fmt.Println("expired: ", responseData.Expired)
}

func TestGetUser(t *testing.T) {
	httpSDK.AuthObject(&map[string]string{"username": "1234567890123456", "password": "1234567890123456"})
	httpSDK.AuthToken(sdk.AuthToken{Token: access_token, Secret: token_secret, Expired: token_expire})
	requestObj := map[string]interface{}{"uid": 123, "name": "我爱中国/+_=/1df", "limit": 20, "offset": 5}
	responseData := map[string]string{}
	if err := httpSDK.PostByAuth("/getUser", &requestObj, &responseData, false); err != nil {
		fmt.Println(err)
	}
	fmt.Println(responseData)
}

func BenchmarkGetUser(b *testing.B) {
	b.StopTimer()
	b.StartTimer()
	for i := 0; i < b.N; i++ { //use b.N for looping
		httpSDK.AuthToken(sdk.AuthToken{Token: access_token, Secret: token_secret, Expired: token_expire})
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
