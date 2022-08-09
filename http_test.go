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

const access_token = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOjEyMzQ1NiwiYXVkIjoiIiwiaXNzIjoiIiwiaWF0IjowLCJleHAiOjE2NjEyMzY2NzIsImRldiI6IkFQUCIsImp0aSI6IisxQkxQQVZFSjdTcWpyZXVma0l1ODlhNTRaakRhVDRQT3grNHU0aWtUYzQ9IiwiZXh0Ijp7fX0=.fV3PoyChfaRLS4DfzR4d5OAeT/f7KqfY4fYxTv+hldk="
const token_secret = "eDi3MbyznGsaN748X5rlwrZy4FAzMyNLJ+iGbLnqhKk="

//const access_token = ""
//const token_secret = ""

// 测试使用的http post示例方法
func ToPostBy(path string, req *node.ReqDto) string {
	if req.Plan == 1 {
		d, _ := util.AesEncrypt(req.Data.(string), token_secret, util.AddStr(req.Nonce, req.Time))
		req.Data = d
		fmt.Println("加密数据: ", req.Data, util.AddStr(req.Nonce, req.Time))
	}
	if len(req.Sign) == 0 {
		req.Sign = util.HMAC_SHA256(util.AddStr(path, req.Data.(string), req.Nonce, req.Time, req.Plan), token_secret, true)
	}
	bytesData, err := util.JsonMarshal(req)
	if err != nil {
		fmt.Println(err)
		return ""
	}
	fmt.Println("请求示例: ")
	fmt.Println(string(bytesData))
	reader := bytes.NewReader(bytesData)
	request, err := http.NewRequest("POST", domain+path, reader)
	if err != nil {
		fmt.Println(err)
		return ""
	}
	request.Header.Set("Content-Type", "application/json;charset=UTF-8")
	request.Header.Set("Authorization", access_token)
	client := http.Client{}
	resp, err := client.Do(request)
	if err != nil {
		fmt.Println(err)
		return ""
	}
	respBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println(err)
		return ""
	}
	fmt.Println("响应示例: ")
	fmt.Println(util.Bytes2Str(respBytes))
	respData := &node.RespDto{}
	if err := util.JsonUnmarshal(respBytes, &respData); err != nil {
		fmt.Println(err)
		return ""
	}
	if respData.Code == 200 {
		s := util.HMAC_SHA256(util.AddStr(path, respData.Data, respData.Nonce, respData.Time, respData.Plan), token_secret, true)
		fmt.Println("数据验签: ", s == respData.Sign)
		if respData.Plan == 1 {
			dec, err := util.AesDecrypt(respData.Data.(string), token_secret, util.AddStr(respData.Nonce, respData.Time))
			if err != nil {
				panic(err)
			}
			respData.Data = dec
			fmt.Println("数据明文: ", util.Bytes2Str(util.Base64Decode(respData.Data)))
		}
	}
	return ""
}

func ToPostByLogin(path, loginData, servePubkey string) string {
	//fmt.Println("pubkey len: ", len(clientPubkey), " pubkey sign len: ", len(clientSign))
	fmt.Println("login data length: ", len(loginData))
	fmt.Println("请求示例: ")
	fmt.Println(loginData)
	reader := bytes.NewReader(util.Str2Bytes(loginData))
	request, err := http.NewRequest("POST", domain+path, reader)
	if err != nil {
		fmt.Println(err)
		return ""
	}
	request.Header.Set("Content-Type", "application/json;charset=UTF-8")
	//request.Header.Set("ClientPubkey", clientPubkey)
	//request.Header.Set("ClientPubkeySign", clientSign)
	client := http.Client{}
	resp, err := client.Do(request)
	if err != nil {
		fmt.Println(err)
		return ""
	}
	respBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println(err)
		return ""
	}
	fmt.Println("响应示例: ")
	fmt.Println(string(respBytes))
	respData := &node.RespDto{}
	if err := util.JsonUnmarshal(respBytes, &respData); err != nil {
		fmt.Println(err)
		return ""
	}
	if respData.Code == 200 {
		s := util.AddStr(path, respData.Data, respData.Nonce, respData.Time, respData.Plan)
		rsaObj := &gorsa.RsaObj{}
		rsaObj.LoadRsaPemFileBase64(servePubkey)
		fmt.Println("RSA数据验签: ", rsaObj.VerifyBySHA256(util.Str2Bytes(s), respData.Sign) == nil)
		a, _ := respData.Data.(string)
		m := map[string]string{}
		util.ParseJsonBase64(a, &m)
		return m["secret"]
	}
	return ""
}

func TestRsaLogin(t *testing.T) {
	resp, err := http.Get(domain + "/pubkey")
	if err != nil {
		panic(err)
	}
	respBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println(err)
		return
	}
	servePubkey := string(respBytes)
	cliRsa := &gorsa.RsaObj{}
	// 1024 pubkey len:  689  pubkey sign len:  172
	// 2048 pubkey len:  1034  pubkey sign len:  344
	cliRsa.CreateRsaFileBase64(1024)
	data, _ := util.ToJsonBase64(map[string]string{"username": "1234567890123456", "password": "1234567890123456"})
	path := "/login2"
	req := &node.ReqDto{
		Data:  data,
		Time:  util.TimeSecond(),
		Nonce: util.RandNonce(),
		Plan:  int64(0),
		Sign:  cliRsa.PubkeyBase64,
	}
	srvRsa := &gorsa.RsaObj{}
	if err := srvRsa.LoadRsaPemFileBase64(servePubkey); err != nil {
		panic(err)
	}
	loginData, _ := util.ToJsonBase64(req)
	loginDataRes, err := srvRsa.EncryptPlanText(loginData)
	if err != nil {
		panic(err)
	}
	secret := ToPostByLogin(path, loginDataRes, servePubkey)
	bs, err := cliRsa.Decrypt(util.Base64Decode(secret))
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("login secret: ", string(bs))
}

func TestGetUser(t *testing.T) {
	data, _ := util.ToJsonBase64(map[string]interface{}{"uid": 123, "name": "我爱中国/+_=/1df", "limit": 20, "offset": 5})
	path := "/test2"
	req := &node.ReqDto{
		Data:  data,
		Time:  util.TimeSecond(),
		Nonce: util.RandNonce(),
		Plan:  int64(1),
		Sign:  "",
	}
	fmt.Println(req)
	ToPostBy(path, req)
}

func BenchmarkLogin(b *testing.B) {
	data, _ := util.ToJsonBase64(map[string]string{"test": "1234566"})
	path := "/login1"
	req := &node.ReqDto{
		Data:  data,
		Time:  util.TimeSecond(),
		Nonce: util.RandNonce(),
		Plan:  int64(1),
		Sign:  "",
	}
	fmt.Println(req)
	ToPostBy(path, req)
}

func BenchmarkGetUser(b *testing.B) {
	data, _ := util.ToJsonBase64(map[string]string{"test": "1234566"})
	path := "/test1"
	req := &node.ReqDto{
		Data:  data,
		Time:  util.TimeSecond(),
		Nonce: util.RandNonce(),
		Plan:  int64(0),
		Sign:  "",
	}
	fmt.Println(req)
	ToPostBy(path, req)
}
