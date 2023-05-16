package main

import (
	"encoding/base64"
	"fmt"
	"github.com/godaddy-x/eccrypto"
	"github.com/godaddy-x/freego/node"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/utils/crypto"
	"github.com/valyala/fasthttp"
	"testing"
	"time"
)

//go test -v http_test.go -bench=BenchmarkPubkey -benchmem -count=10

const domain = "http://localhost:8090"

const access_token = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxNjU4MDk1NzI3NjY4MTAxMTIxIiwiYXVkIjoiIiwiaXNzIjoiIiwiaWF0IjowLCJleHAiOjE2ODUzNjU0MDIsImRldiI6IkFQUCIsImp0aSI6InRaMnlrR1V3MmU1MlFCTjFZMVlFckE9PSIsImV4dCI6IiJ9.rVNUWR01MTLIrCec8R0B2ZQRh4O5mqU3FOeSyS1g4mI="
const token_secret = "1f13RxJdVL53xlyHy*kT^j#lKw8yCvvf6yF0Dctm#lK!ZC@diQmpRKsYovkDWs0="

//const access_token = ""
//const token_secret = ""

// 客户端自定义密钥
var clientSecretKey = utils.MD5(utils.RandStr(16), true)

// 服务端公钥
var serverPublicKey string

func output(a ...interface{}) {
	fmt.Println(a...)
}

func getServerPublicKey() string {
	request := fasthttp.AcquireRequest()
	request.Header.SetMethod("GET")
	request.SetRequestURI(domain + "/publicKey")
	defer fasthttp.ReleaseRequest(request)
	response := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(response)
	if _, b, err := fasthttp.Get(nil, domain+"/publicKey"); err != nil {
		panic(err)
	} else {
		//output(utils.Bytes2Str(b))
		return utils.Bytes2Str(b)
	}
}

// 登录状态使用Token+Secret模式交互
func PostByTokenSecret(path string, req *node.JsonBody) {
	if req.Plan == 0 || req.Plan == 2 {
		d := utils.Base64Encode(req.Data.([]byte))
		req.Data = d
		output("请求数据Base64结果: ", req.Data)
	} else if req.Plan == 1 {
		d, err := utils.AesEncrypt(req.Data.([]byte), token_secret, utils.AddStr(req.Nonce, req.Time))
		if err != nil {
			panic(err)
		}
		req.Data = d
		output("请求数据AES加密结果: ", req.Data)
	}
	req.Sign = utils.HMAC_SHA256(utils.AddStr(path, req.Data, req.Nonce, req.Time, req.Plan), token_secret, true)
	bytesData, err := utils.JsonMarshal(req)
	if err != nil {
		panic(err)
	}
	output("请求示例: ")
	output(utils.Bytes2Str(bytesData))
	request := fasthttp.AcquireRequest()
	request.Header.SetContentType("application/json;charset=UTF-8")
	request.Header.Set("Authorization", access_token)
	request.Header.SetMethod("POST")
	request.SetRequestURI(domain + path)
	request.SetBody(bytesData)
	defer fasthttp.ReleaseRequest(request)
	response := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(response)
	if err := fasthttp.DoTimeout(request, response, 30*time.Second); err != nil {
		panic(err)
	}
	respBytes := response.Body()
	output("响应示例: ")
	output(utils.Bytes2Str(respBytes))
	respData := &node.JsonResp{
		Code:  utils.GetJsonInt(respBytes, "c"),
		Data:  utils.GetJsonString(respBytes, "d"),
		Nonce: utils.GetJsonString(respBytes, "n"),
		Time:  int64(utils.GetJsonInt(respBytes, "t")),
		Plan:  int64(utils.GetJsonInt(respBytes, "p")),
		Sign:  utils.GetJsonString(respBytes, "s"),
	}
	if respData.Code == 200 {
		s := utils.HMAC_SHA256(utils.AddStr(path, respData.Data, respData.Nonce, respData.Time, respData.Plan), token_secret, true)
		output("****************** Response Signature Verify:", s == respData.Sign, "******************")
		if respData.Plan == 0 {
			dec := utils.Base64Decode(respData.Data)
			output("响应数据Base64明文: ", string(dec))
		}
		if respData.Plan == 1 {
			dec, err := utils.AesDecrypt(respData.Data.(string), token_secret, utils.AddStr(respData.Nonce, respData.Time))
			if err != nil {
				panic(err)
			}
			output("响应数据AES解密明文: ", utils.Bytes2Str(dec))
		}
	}
}

// 非登录状态使用公钥验签模式交互
func PostByPublicKeyHAX(path string, req *node.JsonBody) {
	if req.Plan != 3 {
		panic("plan invalid")
	}
	serverPublicKey := getServerPublicKey()
	d := utils.Base64Encode(req.Data.([]byte))
	req.Data = d
	output("请求数据Base64结果: ", req.Data)
	req.Sign = utils.HMAC_SHA256(utils.AddStr(path, req.Data, req.Nonce, req.Time, req.Plan), serverPublicKey, true)
	bytesData, err := utils.JsonMarshal(req)
	if err != nil {
		panic(err)
	}
	output("请求示例: ")
	output(utils.Bytes2Str(bytesData))
	request := fasthttp.AcquireRequest()
	request.Header.SetContentType("application/json;charset=UTF-8")
	request.Header.Set("Authorization", access_token)
	request.Header.SetMethod("POST")
	request.SetRequestURI(domain + path)
	request.SetBody(bytesData)
	defer fasthttp.ReleaseRequest(request)
	response := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(response)
	if err := fasthttp.DoTimeout(request, response, 30*time.Second); err != nil {
		panic(err)
	}
	respBytes := response.Body()
	output("响应示例: ")
	output(utils.Bytes2Str(respBytes))
	respData := &node.JsonResp{
		Code:  utils.GetJsonInt(respBytes, "c"),
		Data:  utils.GetJsonString(respBytes, "d"),
		Nonce: utils.GetJsonString(respBytes, "n"),
		Time:  int64(utils.GetJsonInt(respBytes, "t")),
		Plan:  int64(utils.GetJsonInt(respBytes, "p")),
		Sign:  utils.GetJsonString(respBytes, "s"),
	}
	if respData.Code == 200 {
		s := utils.HMAC_SHA256(utils.AddStr(path, respData.Data, respData.Nonce, respData.Time, respData.Plan), serverPublicKey, true)
		output("****************** Response Signature Verify:", s == respData.Sign, "******************")
		if respData.Plan == 0 {
			dec := utils.Base64Decode(respData.Data)
			output("响应数据Base64明文: ", string(dec))
		}
	}
}

// 非登录状态使用RSA+AES模式交互
func PostByRSA(path string, req *node.JsonBody, useECC bool) {
	if req.Plan != 2 {
		panic("plan invalid")
	}
	var err error
	var randomCode string
	serverPublicKey := getServerPublicKey()
	if useECC {
		_, pubBs, err := ecc.LoadBase64PublicKey(serverPublicKey)
		if err != nil {
			panic(err)
		}
		r, err := ecc.Encrypt(pubBs, utils.Str2Bytes(clientSecretKey))
		if err != nil {
			panic(err)
		}
		randomCode = base64.StdEncoding.EncodeToString(r)
	} else {
		newRsa := &crypto.RsaObj{}
		if err := newRsa.LoadRsaPemFileBase64(serverPublicKey); err != nil {
			panic(err)
		}
		randomCode, err = newRsa.Encrypt(utils.Str2Bytes(clientSecretKey))
	}
	if err != nil {
		panic(err)
	}
	output("服务端公钥: ", serverPublicKey)
	output("RSA加密客户端密钥原文: ", clientSecretKey)
	output("RSA加密客户端密钥密文: ", randomCode)
	d, err := utils.AesEncrypt(req.Data.([]byte), clientSecretKey, clientSecretKey)
	if err != nil {
		panic(err)
	}
	req.Data = d
	req.Sign = utils.HMAC_SHA256(utils.AddStr(path, req.Data.(string), req.Nonce, req.Time, req.Plan), serverPublicKey, true)
	bytesData, err := utils.JsonMarshal(req)
	if err != nil {
		panic(err)
	}
	output("请求示例: ")
	output(utils.Bytes2Str(bytesData))
	request := fasthttp.AcquireRequest()
	request.Header.SetContentType("application/json;charset=UTF-8")
	request.Header.Set("Authorization", "")
	request.Header.Set("RandomCode", randomCode)
	request.Header.SetMethod("POST")
	request.SetRequestURI(domain + path)
	request.SetBody(bytesData)
	defer fasthttp.ReleaseRequest(request)
	response := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(response)
	if err := fasthttp.DoTimeout(request, response, 30*time.Second); err != nil {
		panic(err)
	}
	respBytes := response.Body()
	output("响应示例: ")
	output(utils.Bytes2Str(respBytes))
	respData := &node.JsonResp{
		Code:  utils.GetJsonInt(respBytes, "c"),
		Data:  utils.GetJsonString(respBytes, "d"),
		Nonce: utils.GetJsonString(respBytes, "n"),
		Time:  int64(utils.GetJsonInt(respBytes, "t")),
		Plan:  int64(utils.GetJsonInt(respBytes, "p")),
		Sign:  utils.GetJsonString(respBytes, "s"),
	}
	if respData.Code == 200 {
		s := utils.HMAC_SHA256(utils.AddStr(path, respData.Data, respData.Nonce, respData.Time, respData.Plan), clientSecretKey, true)
		output("****************** Response Signature Verify:", s == respData.Sign, "******************")
		dec, err := utils.AesDecrypt(respData.Data.(string), clientSecretKey, clientSecretKey)
		if err != nil {
			panic(err)
		}
		output("Plain2数据明文: ", utils.Bytes2Str(dec))
	}
}

func TestRSALogin(t *testing.T) {
	data, _ := utils.JsonMarshal(map[string]string{"username": "1234567890123456", "password": "1234567890123456"})
	path := "/login"
	req := &node.JsonBody{
		Data:  data,
		Time:  utils.UnixSecond(),
		Nonce: utils.RandNonce(),
		Plan:  int64(2),
	}
	PostByRSA(path, req, true)
}

func TestGeetestRegister(t *testing.T) {
	data, _ := utils.JsonMarshal(map[string]string{"filterObject": "123456", "filterMethod": "/test"})
	path := "/geetest/register"
	req := &node.JsonBody{
		Data:  data,
		Time:  utils.UnixSecond(),
		Nonce: utils.RandNonce(),
		Plan:  int64(2),
	}
	PostByRSA(path, req, true)
}

func TestGeetestValidate(t *testing.T) {
	data, _ := utils.JsonMarshal(map[string]string{
		"filterObject":      "123456",
		"filterMethod":      "/test",
		"geetest_challenge": "123",
		"geetest_validate":  "123",
		"geetest_seccode":   "123",
	})
	path := "/geetest/validate"
	req := &node.JsonBody{
		Data:  data,
		Time:  utils.UnixSecond(),
		Nonce: utils.RandNonce(),
		Plan:  int64(2),
	}
	PostByTokenSecret(path, req)
}

func TestGetUser(t *testing.T) {
	data, _ := utils.JsonMarshal(map[string]interface{}{"uid": 123, "name": "我爱中国/+_=/1df", "limit": 20, "offset": 5})
	path := "/getUser"
	req := &node.JsonBody{
		Data:  data,
		Time:  utils.UnixSecond(),
		Nonce: utils.RandNonce(),
		Plan:  int64(0),
	}
	PostByTokenSecret(path, req)
}

func TestHAX(t *testing.T) {
	data, _ := utils.JsonMarshal(map[string]interface{}{"uid": 123, "name": "我爱中国/+_=/1df", "limit": 20, "offset": 5})
	path := "/testHAX"
	req := &node.JsonBody{
		Data:  data,
		Time:  utils.UnixSecond(),
		Nonce: utils.RandNonce(),
		Plan:  int64(3),
	}
	PostByPublicKeyHAX(path, req)
}

func TestPubkey(t *testing.T) {
	getServerPublicKey()
}

func TestGuestPost(t *testing.T) {
	guestBody, _ := utils.JsonMarshal(map[string]string{"test": "shabi", "name": "unknown"})
	output("请求示例: ")
	output(utils.Bytes2Str(guestBody))
	request := fasthttp.AcquireRequest()
	request.Header.SetContentType("application/json;charset=UTF-8")
	request.Header.SetMethod("POST")
	request.SetRequestURI(domain + "/testGuestPost")
	request.SetBody(guestBody)
	defer fasthttp.ReleaseRequest(request)
	response := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(response)
	if err := fasthttp.DoTimeout(request, response, 30*time.Second); err != nil {
		panic(err)
	}
	respBytes := response.Body()
	output("响应示例: ")
	output(utils.Bytes2Str(respBytes))
}

func BenchmarkRSALogin(b *testing.B) {
	b.StopTimer()
	b.StartTimer()
	for i := 0; i < b.N; i++ { //use b.N for looping
		data, _ := utils.JsonMarshal(map[string]string{"username": "1234567890123456", "password": "1234567890123456"})
		path := "/login"
		req := &node.JsonBody{
			Data:  data,
			Time:  utils.UnixSecond(),
			Nonce: utils.RandNonce(),
			Plan:  int64(2),
		}
		PostByRSA(path, req, true)
	}
}

func BenchmarkGetUser(b *testing.B) {
	b.StopTimer()
	b.StartTimer()
	for i := 0; i < b.N; i++ { //use b.N for looping
		data, _ := utils.JsonMarshal(map[string]interface{}{"uid": 123, "name": "我爱中国/+_=/1df", "limit": 20, "offset": 5})
		path := "/test2"
		req := &node.JsonBody{
			Data:  data,
			Time:  utils.UnixSecond(),
			Nonce: utils.RandNonce(),
			Plan:  int64(1),
		}
		PostByTokenSecret(path, req)
	}
}

// go test http_test.go -bench=BenchmarkPubkey  -benchmem -count=10 -cpuprofile cpuprofile.out -memprofile memprofile.out
// go test http_test.go -bench=BenchmarkPubkey  -benchmem -count=10
func BenchmarkPublicKey(b *testing.B) {
	b.StopTimer()
	b.StartTimer()
	for i := 0; i < b.N; i++ { //use b.N for looping
		getServerPublicKey()
	}
}
