package main

import (
	"bytes"
	"fmt"
	"github.com/godaddy-x/freego/component/gorsa"
	"github.com/godaddy-x/freego/node"
	"github.com/godaddy-x/freego/util"
	"io/ioutil"
	"net/http"
	"testing"
)

const domain = "http://localhost:8090"

const access_token = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOjEyMzQ1NiwiYXVkIjoiIiwiaXNzIjoiIiwiaWF0IjowLCJleHAiOjE2NjEzMjc2OTcsImRldiI6IkFQUCIsImp0aSI6InRKT1pEWG5PNzV6THc3MTl6RU0vejhHeXY5aXJpTEpKN1UwOXRtc3psNGs9IiwiZXh0Ijp7fX0=.d2hVmPghML9NOtuDCc1DEjD5zcZlIpuWimRE3MEsOzw="
const token_secret = "gaJ7/YrJBaBG62oHy*kT^j#lKDgUKV7Yv+Rj++QH#lK!ZC@diQEifttsdNRYrgg="

//const access_token = ""
//const token_secret = ""

var pubkey = util.MD5(util.RandStr(16), true)
var srvPubkeyBase64 = initSrvPubkey()

func init() {
	initSrvPubkey()
}

func initSrvPubkey() string {
	resp, err := http.Get(domain + "/pubkey")
	if err != nil {
		panic(err)
	}
	respBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}
	//fmt.Println(string(respBytes))
	return string(respBytes)
	//if err := srvRsa.LoadRsaPemFileBase64(string(respBytes)); err != nil {
	//	panic(err)
	//}
	//if err != nil {
	//	panic(err)
	//}
}

// 测试使用的http post示例方法
func ToPostBy(path string, req *node.ReqDto) {
	if req.Plan == 0 {
		d := util.Base64URLEncode(req.Data.([]byte))
		req.Data = d
		fmt.Println("Base64数据: ", req.Data)
	} else if req.Plan == 1 {
		d, err := util.AesEncrypt(req.Data.([]byte), token_secret, util.AddStr(req.Nonce, req.Time))
		if err != nil {
			panic(err)
		}
		req.Data = d
		fmt.Println("AES加密数据: ", req.Data)
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
		fmt.Println("RSA加密数据: ", req.Data)
	}
	secret := token_secret
	if req.Plan == 2 {
		secret = srvPubkeyBase64
		fmt.Println("nonce secret:", pubkey)
	}
	req.Sign = util.HMAC_SHA256(util.AddStr(path, req.Data.(string), req.Nonce, req.Time, req.Plan), secret, true)
	bytesData, err := util.JsonMarshal(req)
	if err != nil {
		panic(err)
	}
	fmt.Println("请求示例: ")
	fmt.Println(string(bytesData))
	reader := bytes.NewReader(bytesData)
	request, err := http.NewRequest("POST", domain+path, reader)
	if err != nil {
		panic(err)
	}
	request.Header.Set("Content-Type", "application/json;charset=UTF-8")
	request.Header.Set("Authorization", access_token)
	client := http.Client{}
	resp, err := client.Do(request)
	if err != nil {
		panic(err)
	}
	respBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}
	fmt.Println("响应示例: ")
	fmt.Println(util.Bytes2Str(respBytes))
	respData := &node.RespDto{}
	if err := util.JsonUnmarshal(respBytes, &respData); err != nil {
		panic(err)
	}
	if respData.Code == 200 {
		key := token_secret
		if respData.Plan == 2 {
			key = pubkey
		}
		s := util.HMAC_SHA256(util.AddStr(path, respData.Data, respData.Nonce, respData.Time, respData.Plan), key, true)
		fmt.Println("****************** Response Signature Verify:", s == respData.Sign, "******************")
		if respData.Plan == 0 {
			dec := util.Base64URLDecode(respData.Data)
			fmt.Println("Base64数据明文: ", string(dec))
		}
		if respData.Plan == 1 {
			dec, err := util.AesDecrypt(respData.Data.(string), key, util.AddStr(respData.Nonce, respData.Time))
			if err != nil {
				panic(err)
			}
			respData.Data = dec
			fmt.Println("AES数据明文: ", respData.Data)
		}
		if respData.Plan == 2 {
			dec, err := util.AesDecrypt(respData.Data.(string), pubkey, pubkey)
			if err != nil {
				panic(err)
			}
			fmt.Println("LOGIN数据明文: ", dec)
		}
	}
}

func TestRsaLogin(t *testing.T) {
	data, _ := util.JsonMarshal(map[string]string{"username": "1234567890123456", "password": "1234567890123456", "pubkey": pubkey})
	path := "/login2"
	req := &node.ReqDto{
		Data:  data,
		Time:  util.TimeSecond(),
		Nonce: util.RandNonce(),
		Plan:  int64(2),
	}
	ToPostBy(path, req)
}

func TestGetUser(t *testing.T) {
	data, _ := util.JsonMarshal(map[string]interface{}{"uid": 123, "name": "我爱中国/+_=/1df", "limit": 20, "offset": 5})
	path := "/test2"
	req := &node.ReqDto{
		Data:  data,
		Time:  util.TimeSecond(),
		Nonce: util.RandNonce(),
		Plan:  int64(1),
	}
	ToPostBy(path, req)
}

func BenchmarkLogin(b *testing.B) {
	b.StopTimer()
	b.StartTimer()
	for i := 0; i < b.N; i++ { //use b.N for looping
		data, _ := util.JsonMarshal(map[string]string{"username": "1234567890123456", "password": "1234567890123456", "pubkey": pubkey})
		path := "/login2"
		req := &node.ReqDto{
			Data:  data,
			Time:  util.TimeSecond(),
			Nonce: util.RandNonce(),
			Plan:  int64(2),
		}
		ToPostBy(path, req)
	}
}

func BenchmarkGetUser(b *testing.B) {
	b.StopTimer()
	b.StartTimer()
	for i := 0; i < b.N; i++ { //use b.N for looping
		data, _ := util.JsonMarshal(map[string]string{"test": "1234566"})
		path := "/test1"
		req := &node.ReqDto{
			Data:  data,
			Time:  util.TimeSecond(),
			Nonce: util.RandNonce(),
			Plan:  int64(1),
		}
		ToPostBy(path, req)
	}
}
