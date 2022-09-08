package main

import (
	"fmt"
	"github.com/godaddy-x/freego/node"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/utils/gorsa"
	"github.com/valyala/fasthttp"
	"testing"
	"time"
)

//go test -v http_test.go -bench=BenchmarkPubkey -benchmem -count=10

const domain = "http://localhost:8090"

const access_token = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxNTY3MDEzODI3NDkxMzk3NjMzIiwiYXVkIjoiIiwiaXNzIjoiIiwiaWF0IjowLCJleHAiOjE2NjM2NDk3ODQsImRldiI6IkFQUCIsImp0aSI6Img0YWplQVlSMXFFZlRLVjQyNVFVR3c9PSIsImV4dCI6e319.7xTHZFW0rE5fNO+COJh2N8iUcWS/0FDZWgUuX2JCLZQ="
const token_secret = "eXgL3uQw62UGEjeHy*kT^j#lKah2rQGIofcknEKz#lK!ZC@diQrWHDX0wxio2Nc="

//const access_token = ""
//const token_secret = ""

var pubkey = utils.MD5(utils.RandStr(16), true)
var srvPubkeyBase64 string

func initSrvPubkey() string {
	reqcli := fasthttp.AcquireRequest()
	reqcli.Header.SetMethod("GET")
	reqcli.SetRequestURI(domain + "/pubkey")
	defer fasthttp.ReleaseRequest(reqcli)

	respcli := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(respcli)
	if _, b, err := fasthttp.Get(nil, domain+"/pubkey"); err != nil {
		panic(err)
	} else {
		//output(string(respBytes))
		return utils.Bytes2Str(b)
	}
}

func output(a ...interface{}) {
	fmt.Println(a...)
}

// 测试使用的http post示例方法
func ToPostBy(path string, req *node.JsonBody) {
	if len(srvPubkeyBase64) == 0 {
		srvPubkeyBase64 = initSrvPubkey()
	}
	if req.Plan == 0 {
		d := utils.Base64URLEncode(req.Data.([]byte))
		req.Data = d
		output("Base64数据: ", req.Data)
	} else if req.Plan == 1 {
		d, err := utils.AesEncrypt(req.Data.([]byte), token_secret, utils.AddStr(req.Nonce, req.Time))
		if err != nil {
			panic(err)
		}
		req.Data = d
		output("AES加密数据: ", req.Data)
	} else if req.Plan == 2 {
		newRsa := &gorsa.RsaObj{}
		if err := newRsa.LoadRsaPemFileBase64(srvPubkeyBase64); err != nil {
			panic(err)
		}
		rsaData, err := newRsa.Encrypt(req.Data.([]byte))
		if err != nil {
			panic(err)
		}
		req.Data = rsaData
		output("RSA加密数据: ", req.Data)
	}
	secret := token_secret
	if req.Plan == 2 {
		secret = srvPubkeyBase64
		output("nonce secret:", pubkey)
	}
	req.Sign = utils.HMAC_SHA256(utils.AddStr(path, req.Data.(string), req.Nonce, req.Time, req.Plan), secret, true)
	bytesData, err := utils.JsonMarshal(req)
	if err != nil {
		panic(err)
	}
	output("请求示例: ")
	output(utils.Bytes2Str(bytesData))

	reqcli := fasthttp.AcquireRequest()
	reqcli.Header.SetContentType("application/json;charset=UTF-8")
	reqcli.Header.Set("Authorization", access_token)
	reqcli.Header.SetMethod("POST")
	reqcli.SetRequestURI(domain + path)
	reqcli.SetBody(bytesData)
	defer fasthttp.ReleaseRequest(reqcli)

	respcli := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(respcli)

	if err := fasthttp.DoTimeout(reqcli, respcli, 5*time.Second); err != nil {
		panic(err)
	}

	respBytes := respcli.Body()

	output("响应示例: ")
	output(utils.Bytes2Str(respBytes))
	respData := &node.JsonResp{}
	if err := utils.JsonUnmarshal(respBytes, &respData); err != nil {
		panic(err)
	}
	if respData.Code == 200 {
		key := token_secret
		if respData.Plan == 2 {
			key = pubkey
		}
		s := utils.HMAC_SHA256(utils.AddStr(path, respData.Data, respData.Nonce, respData.Time, respData.Plan), key, true)
		output("****************** Response Signature Verify:", s == respData.Sign, "******************")
		if respData.Plan == 0 {
			dec := utils.Base64URLDecode(respData.Data)
			output("Base64数据明文: ", string(dec))
		}
		if respData.Plan == 1 {
			dec, err := utils.AesDecrypt(respData.Data.(string), key, utils.AddStr(respData.Nonce, respData.Time))
			if err != nil {
				panic(err)
			}
			respData.Data = dec
			output("AES数据明文: ", respData.Data)
		}
		if respData.Plan == 2 {
			dec, err := utils.AesDecrypt(respData.Data.(string), pubkey, pubkey)
			if err != nil {
				panic(err)
			}
			output("LOGIN数据明文: ", dec)
		}
	}
}

func TestRSALogin(t *testing.T) {
	data, _ := utils.JsonMarshal(map[string]string{"username": "1234567890123456", "password": "1234567890123456", "pubkey": pubkey})
	path := "/login"
	req := &node.JsonBody{
		Data:  data,
		Time:  utils.TimeSecond(),
		Nonce: utils.RandNonce(),
		Plan:  int64(2),
	}
	ToPostBy(path, req)
}

func TestGetUser(t *testing.T) {
	data, _ := utils.JsonMarshal(map[string]interface{}{"uid": 123, "name": "我爱中国/+_=/1df", "limit": 20, "offset": 5})
	path := "/test2"
	req := &node.JsonBody{
		Data:  data,
		Time:  utils.TimeSecond(),
		Nonce: utils.RandNonce(),
		Plan:  int64(1),
	}
	ToPostBy(path, req)
}

func TestPubkey(t *testing.T) {
	initSrvPubkey()
}

func BenchmarkRSALogin(b *testing.B) {
	b.StopTimer()
	b.StartTimer()
	for i := 0; i < b.N; i++ { //use b.N for looping
		data, _ := utils.JsonMarshal(map[string]string{"username": "1234567890123456", "password": "1234567890123456", "pubkey": pubkey})
		path := "/login"
		req := &node.JsonBody{
			Data:  data,
			Time:  utils.TimeSecond(),
			Nonce: utils.RandNonce(),
			Plan:  int64(2),
		}
		ToPostBy(path, req)
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
			Time:  utils.TimeSecond(),
			Nonce: utils.RandNonce(),
			Plan:  int64(1),
		}
		ToPostBy(path, req)
	}
}

// go test http_test.go -bench=BenchmarkPubkey  -benchmem -count=10
func BenchmarkPubkey(b *testing.B) {
	b.StopTimer()
	b.StartTimer()
	for i := 0; i < b.N; i++ { //use b.N for looping
		initSrvPubkey()
	}
}
