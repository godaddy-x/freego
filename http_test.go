package main

import (
	"fmt"
	"github.com/godaddy-x/freego/utils/sdk"
	"testing"
)

//go test -v http_test.go -bench=BenchmarkPubkey -benchmem -count=10

const domain = "http://localhost:8090"

const access_token = "eyJhbGciOiIiLCJ0eXAiOiIifQ==.eyJzdWIiOiIxODMwMTcyOTQ5MzQ1MjA2MjczIiwiYXVkIjoiIiwiaXNzIjoiIiwiaWF0IjowLCJleHAiOjE3MjYzOTE4MDgsImRldiI6IldFQiIsImp0aSI6IlhLVXVIR29RdEN2cmdYWnBVVFd5SGc9PSIsImV4dCI6IiJ9.zYROucLgbx0ALjuHZJFKXasdhni9E2lQuRo60PrDq88="
const token_secret = "XP8NC4cXJ6UdO9pfjSkcb5kLH4e7xgq+p6HvO8a2kSAuiV9EbUeTt7mYftItzj7su2qNvqZWDVtMH9SFH3Wg/w=="
const token_expire = 1726391808

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
	httpSDK.SetPublicKey("BCpGBnLcwqQq3R4zw54XRCFF+eglX/UX0aZGuDw2xHvV0ru8zmDZ+WAFLA8uBNmbcx+VHOE9jdnUMEDoOaFTWAE=")
	if err := httpSDK.PostByECC("/login", &requestData, &responseData); err != nil {
		fmt.Println(err)
	}
	fmt.Println("token: ", responseData.Token)
	fmt.Println("secret: ", responseData.Secret)
	fmt.Println("expired: ", responseData.Expired)
}

func TestGetUser(t *testing.T) {
	httpSDK.SetAuthObject(func() interface{} {
		return &map[string]string{"username": "1234567890123456", "password": "1234567890123456"}
	})
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
		//fmt.Println(responseData)
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
		httpSDK.SetPublicKey("BPrRMrc3nv9SGVsj0eMwgPnLfKr6HTWLVJ2f9QcHH6qOEpsgpUkBKhNY6G4J7LmdD9l+ruLMn3Zn/Fwi+h80dM0=")
		if err := httpSDK.PostByECC("/login", &requestData, &responseData); err != nil {
			fmt.Println(err)
		}
	}
}
