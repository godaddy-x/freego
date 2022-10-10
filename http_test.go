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

const access_token = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxNTc5MjcxODA2MjczOTc4MzY5IiwiYXVkIjoiIiwiaXNzIjoiIiwiaWF0IjowLCJleHAiOjE2NjY1NzIzMTQsImRldiI6IkFQUCIsImp0aSI6Ild1WUVaaDBGVDlSUXVJVWdldkw3dFE9PSIsImV4dCI6IiJ9.HyS0aGor8nkFFEbg7UVNYs/Xo3jlJ8On8UN5W/SH0QM="
const token_secret = "kdXYjuhYNk/arEgHy*kT^j#lKwY+TI/9rG1gDuYW#lK!ZC@diQhTKljulBIOg78="

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
	var randomCode string
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
		output("RSA加密原文: ", pubkey)
		newRsa := &gorsa.RsaObj{}
		if err := newRsa.LoadRsaPemFileBase64(srvPubkeyBase64); err != nil {
			panic(err)
		}
		rsaData, err := newRsa.Encrypt(utils.Str2Bytes(pubkey))
		if err != nil {
			panic(err)
		}
		randomCode = rsaData
		output("RSA加密数据: ", randomCode)
		d, err := utils.AesEncrypt(req.Data.([]byte), pubkey, pubkey)
		if err != nil {
			panic(err)
		}
		req.Data = d
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
	reqcli.Header.Set("RandomCode", randomCode)
	reqcli.Header.SetMethod("POST")
	reqcli.SetRequestURI(domain + path)
	reqcli.SetBody(bytesData)
	defer fasthttp.ReleaseRequest(reqcli)

	respcli := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(respcli)

	if err := fasthttp.DoTimeout(reqcli, respcli, 30*time.Second); err != nil {
		panic(err)
	}

	respBytes := respcli.Body()

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
	data, _ := utils.JsonMarshal(map[string]string{"username": "1234567890123456", "password": "1234567890123456"})
	path := "/login"
	req := &node.JsonBody{
		Data:  data,
		Time:  utils.UnixSecond(),
		Nonce: utils.RandNonce(),
		Plan:  int64(2),
	}
	ToPostBy(path, req)
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
	ToPostBy(path, req)
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
	ToPostBy(path, req)
}

func TestGetUser(t *testing.T) {
	data, _ := utils.JsonMarshal(map[string]interface{}{"uid": 123, "name": "我爱中国/+_=/1df", "limit": 20, "offset": 5})
	path := "/test2"
	req := &node.JsonBody{
		Data:  data,
		Time:  utils.UnixSecond(),
		Nonce: utils.RandNonce(),
		Plan:  int64(0),
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
		data, _ := utils.JsonMarshal(map[string]string{"username": "1234567890123456", "password": "1234567890123456"})
		path := "/login"
		req := &node.JsonBody{
			Data:  data,
			Time:  utils.UnixSecond(),
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
			Time:  utils.UnixSecond(),
			Nonce: utils.RandNonce(),
			Plan:  int64(1),
		}
		ToPostBy(path, req)
	}
}

// go test http_test.go -bench=BenchmarkPubkey  -benchmem -count=10 -cpuprofile cpuprofile.out -memprofile memprofile.out
// go test http_test.go -bench=BenchmarkPubkey  -benchmem -count=10
func BenchmarkPubkey(b *testing.B) {
	b.StopTimer()
	b.StartTimer()
	for i := 0; i < b.N; i++ { //use b.N for looping
		initSrvPubkey()
	}
}
