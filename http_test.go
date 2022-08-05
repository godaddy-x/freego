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

//const access_token = "eyJzdWIiOjEyMzQ1NiwiYXVkIjoiMjIyMjIiLCJpc3MiOiIxMTExIiwiaWF0IjoxNjU5NTc2NDExLCJleHAiOjE2NjA3ODYwMTEsImRldiI6IkFQUCIsImp0aSI6IjM0MTdlNGQ1YmJkMmQ5YWNkYzg4MzBmNmQ5NTE4MmI5ZjQ3YjhhNDBiNWI3YzQ5NDJkYzMwMGRlNGQ4YTIyZjgiLCJuc3IiOiJiZWI5ZmVkYmMzNDgzZDAzIiwiZXh0Ijp7InRlc3QiOiIxMSIsInRlc3QyIjoiMjIyIn19.fd12a7bfa21d66567ec8c6b4252279db21ad3662ac12970b24a5aa2a087239fe"
//const token_secret = "79ed7b4447b43c6110b3031065e771e23b8c1798b1e9ff42933eb983a68301ed"

const access_token = ""
const token_secret = ""

// 测试使用的http post示例方法
func ToPostBy(path string, req *node.ReqDto) {
	if req.Plan == 1 {
		d, _ := util.AesEncrypt(req.Data.(string), token_secret, util.AddStr(req.Nonce, req.Time))
		req.Data = d
		fmt.Println("加密数据: ", req.Data, util.AddStr(req.Nonce, req.Time))
	}
	if len(req.Sign) == 0 {
		req.Sign = util.HMAC_SHA256(util.AddStr(path, req.Data.(string), req.Nonce, req.Time, req.Plan), token_secret)
	}
	bytesData, err := util.JsonMarshal(req)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("请求示例: ")
	fmt.Println(string(bytesData))
	reader := bytes.NewReader(bytesData)
	request, err := http.NewRequest("POST", domain+path, reader)
	if err != nil {
		fmt.Println(err)
		return
	}
	request.Header.Set("Content-Type", "application/json;charset=UTF-8")
	request.Header.Set("Authorization", access_token)
	client := http.Client{}
	resp, err := client.Do(request)
	if err != nil {
		fmt.Println(err)
		return
	}
	respBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("响应示例: ")
	fmt.Println(util.Bytes2Str(respBytes))
	respData := &node.RespDto{}
	if err := util.JsonUnmarshal(respBytes, &respData); err != nil {
		fmt.Println("响应数据解析失败: ", err)
		return
	}
	if respData.Code == 200 {
		s := util.HMAC_SHA256(util.AddStr(path, respData.Data, respData.Nonce, respData.Time, respData.Plan), token_secret)
		fmt.Println("数据验签: ", s == respData.Sign)
		if respData.Plan == 1 {
			dec, err := util.AesDecrypt(respData.Data.(string), token_secret, util.AddStr(respData.Nonce, respData.Time))
			if err != nil {
				fmt.Println("数据解密失败: ", err)
				return
			}
			respData.Data = dec
			fmt.Println("数据明文: ", util.Bytes2Str(util.Base64Decode(respData.Data)))
		}
	}
}

func TestLogin(t *testing.T) {
	data, _ := util.ToJsonBase64(map[string]string{"test": "1234566"})
	path := "/login1"
	req := &node.ReqDto{
		Data:  data,
		Time:  util.TimeSecond(),
		Nonce: util.GetSnowFlakeStrID(),
		Plan:  int64(1),
		Sign:  "",
	}
	fmt.Println(req)
	ToPostBy(path, req)
}

func TestRsaLogin(t *testing.T) {
	resp, err := http.Get(domain + "/keyfile")
	if err != nil {
		panic(err)
	}
	respBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println(err)
		return
	}
	pubkey := string(respBytes)
	rsaObj := &gorsa.RsaObj{}
	rsaObj.CreateRsaFileHex()
	data, _ := util.ToJsonBase64(map[string]string{"username": "1234567890123456", "password": "1234567890123456"})
	path := "/login2"
	req := &node.ReqDto{
		Data:  data,
		Time:  util.TimeSecond(),
		Nonce: util.GetSnowFlakeStrID(),
		Plan:  int64(2),
		Sign:  "",
	}
	loginReq, _ := util.ToJsonBase64(req)
	srvRsa := &gorsa.RsaObj{}
	if err := srvRsa.LoadRsaPemFileHex(pubkey); err != nil {
		panic(err)
	}
	res, err := srvRsa.Encrypt(loginReq)
	if err != nil {
		panic(err)
	}
	req2 := &node.ReqDto{
		Data: res,
		Sign: rsaObj.PubkeyHex,
	}
	fmt.Println(req2)
	ToPostBy(path, req2)
}

func BenchmarkLogin(b *testing.B) {
	data, _ := util.ToJsonBase64(map[string]string{"test": "1234566"})
	path := "/login1"
	req := &node.ReqDto{
		Data:  data,
		Time:  util.TimeSecond(),
		Nonce: util.GetSnowFlakeStrID(),
		Plan:  int64(1),
		Sign:  "",
	}
	fmt.Println(req)
	ToPostBy(path, req)
}

func TestGetUser(t *testing.T) {
	data, _ := util.ToJsonBase64(map[string]string{"test": "1234566"})
	path := "/test1"
	req := &node.ReqDto{
		Data:  data,
		Time:  util.TimeSecond(),
		Nonce: util.GetSnowFlakeStrID(),
		Plan:  int64(0),
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
		Nonce: util.GetSnowFlakeStrID(),
		Plan:  int64(0),
		Sign:  "",
	}
	fmt.Println(req)
	ToPostBy(path, req)
}
