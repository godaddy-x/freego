package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/godaddy-x/freego/node"
	"github.com/godaddy-x/freego/util"
	"io/ioutil"
	"net/http"
)

func ToPost(url, token, access_key string, req interface{}) {
	content, _ := util.JsonMarshal(req)
	data := util.Base64URLEncode(content)
	nonce := util.GetSnowFlakeStrID()
	time := util.Time()
	sign_str := util.AddStr("a=", token, "&", "d=", data, "&", "k=", access_key, "&", "n=", nonce, "&", "t=", time)
	sign := util.MD5(sign_str)
	ToPostBy(url,
		&node.ReqDto{
			Token: token,
			Nonce: nonce,
			Time:  time,
			Data:  data,
			Sign:  sign,
		})
}

// 测试使用的http post示例方法
func ToPostBy(url string, data interface{}) {
	bytesData, err := json.Marshal(&data)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("请求示例: ")
	fmt.Println(string(bytesData))
	reader := bytes.NewReader(bytesData)
	request, err := http.NewRequest("POST", url, reader)
	if err != nil {
		fmt.Println(err)
		return
	}
	request.Header.Set("Content-Type", "application/json;charset=UTF-8")
	request.Header.Set("a", "123456")
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
}

