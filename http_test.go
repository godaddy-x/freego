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
	Debug:     false,
	Domain:    domain,
	KeyPath:   "/publicKey",
	LoginPath: "/login",
}

//func TestGetPublicKey(t *testing.T) {
//	publicKey, err := httpSDK.GetPublicKey()
//	if err != nil {
//		fmt.Println(err)
//	}
//	fmt.Println("服务端公钥: ", publicKey)
//}
//
//func TestECCLogin(t *testing.T) {
//	requestData := map[string]string{"username": "1234567890123456", "password": "1234567890123456"}
//	responseData := sdk.AuthToken{}
//	if err := httpSDK.PostByECC("/login", &requestData, &responseData); err != nil {
//		fmt.Println(err)
//	}
//	fmt.Println(responseData)
//}
//
func TestGetUser(t *testing.T) {
	//httpSDK.AuthObject(&map[string]string{"username": "1234567890123456", "password": "1234567890123456"})
	httpSDK.AuthToken(sdk.AuthToken{Token: access_token, Secret: token_secret})
	requestObj := map[string]interface{}{"uid": 123, "name": "我爱中国/+_=/1df", "limit": 20, "offset": 5}
	responseData := map[string]string{}
	if err := httpSDK.PostByAuth("/getUser", &requestObj, &responseData, true); err != nil {
		fmt.Println(err)
	}
	fmt.Println(responseData)
}

//
//func TestGeetestRegister(t *testing.T) {
//	data, _ := utils.JsonMarshal(map[string]string{"filterObject": "123456", "filterMethod": "/test"})
//	path := "/geetest/register"
//	req := &node.JsonBody{
//		Data:  data,
//		Time:  utils.UnixSecond(),
//		Nonce: utils.RandNonce(),
//		Plan:  int64(2),
//	}
//	PostByRSA(path, req, true)
//}
//
//func TestGeetestValidate(t *testing.T) {
//	data, _ := utils.JsonMarshal(map[string]string{
//		"filterObject":      "123456",
//		"filterMethod":      "/test",
//		"geetest_challenge": "123",
//		"geetest_validate":  "123",
//		"geetest_seccode":   "123",
//	})
//	path := "/geetest/validate"
//	req := &node.JsonBody{
//		Data:  data,
//		Time:  utils.UnixSecond(),
//		Nonce: utils.RandNonce(),
//		Plan:  int64(2),
//	}
//	PostByTokenSecret(path, req)
//}

//
//func TestHAX(t *testing.T) {
//	data, _ := utils.JsonMarshal(map[string]interface{}{"uid": 123, "name": "我爱中国/+_=/1df", "limit": 20, "offset": 5})
//	path := "/testHAX"
//	req := &node.JsonBody{
//		Data:  data,
//		Time:  utils.UnixSecond(),
//		Nonce: utils.RandNonce(),
//		Plan:  int64(3),
//	}
//	PostByPublicKeyHAX(path, req)
//}

//
//func TestGuestPost(t *testing.T) {
//	guestBody, _ := utils.JsonMarshal(map[string]string{"test": "shabi", "name": "unknown"})
//	output("请求示例: ")
//	output(utils.Bytes2Str(guestBody))
//	request := fasthttp.AcquireRequest()
//	request.Header.SetContentType("application/json;charset=UTF-8")
//	request.Header.SetMethod("POST")
//	request.SetRequestURI(domain + "/testGuestPost")
//	request.SetBody(guestBody)
//	defer fasthttp.ReleaseRequest(request)
//	response := fasthttp.AcquireResponse()
//	defer fasthttp.ReleaseResponse(response)
//	if err := fasthttp.DoTimeout(request, response, 30*time.Second); err != nil {
//		panic(err)
//	}
//	respBytes := response.Body()
//	output("响应示例: ")
//	output(utils.Bytes2Str(respBytes))
//}

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
