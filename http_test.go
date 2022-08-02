package main

import (
	"bytes"
	"fmt"
	"github.com/godaddy-x/freego/node"
	"github.com/godaddy-x/freego/util"
	"io/ioutil"
	"net/http"
	"testing"
)

const domain = "http://localhost:8090"

const access_token = "eyJzdWIiOjEyMzQ1NiwiYXVkIjoiMjIyMjIiLCJpc3MiOiIxMTExIiwiaWF0IjoxNjU5NDI1MDYyLCJleHAiOjE2NjA2MzQ2NjIsImRldiI6IkFQUCIsImp0aSI6ImQyOWRkMjQwN2VhNzg2MWJhZDg1NGJiODJjYWIwMTU4NDQ4OWRmOThjMjA5MjgyODY4MjNmMjNiZDdiMzRmNTEiLCJuc3IiOiI4MzAyOGMyZDJhYTEwMDU2IiwiZXh0Ijp7InRlc3QiOiIxMSIsInRlc3QyIjoiMjIyIn19.d4b34ae8c0af2d1428574c79ac1570534163ca6f5a595f88b46c8b8b99397bc5"
const token_secret = "e4072518580445e17004d7717c175bbdb2d63aaca66d5d2500e3c873c4f163ae"

//const access_token = ""
//const token_secret = ""

// 测试使用的http post示例方法
func ToPostBy(path string, req *node.ReqDto) {
	if req.Plan == 1 {
		d, _ := util.AesEncrypt(req.Data.(string), token_secret, util.AddStr(req.Nonce, req.Time))
		req.Data = d
		fmt.Println("加密数据: ", req.Data, util.AddStr(req.Nonce, req.Time))
	}
	req.Sign = util.HMAC_SHA256(util.AddStr(path, req.Data.(string), req.Nonce, req.Time, req.Plan), token_secret)
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
