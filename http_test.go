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

//const access_token = "eyJzdWIiOjEyMzQ1NiwiYXVkIjoiMjIyMjIiLCJpc3MiOiIxMTExIiwiaWF0IjoxNjQ4Njk3OTgyOTczLCJleHAiOjE2NDk5MDc1ODI5NzMsImRldiI6IkFQUCIsImp0aSI6ImMzNmVmOGQyYTBiMmM4YTUxMWY4NGFjODI2YzcyNmI4MzRmZDA2MzIzYTcwZGNkYWY5NGE4MTFlN2I0MGYwYWUiLCJuc3IiOiJjYWE1MDk1OTU1NTQyNjA5OTM0MDYiLCJleHQiOnsidGVzdCI6IjExIiwidGVzdDIiOiIyMjIifX0=.0d456a9baae05a459ec2121f7cab3e23d8b8ee278416a9d238773db5039d80fb"
//const token_secret = "65a8218cdafd13841c3afc952e2c5bb3f11e37eebd68d66bf32bacd337eed407"

const access_token = ""
const token_secret = ""

// 测试使用的http post示例方法
func ToPostBy(path string, req *node.ReqDto) {
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
	d := data
	x := util.TimeSecond()
	n := util.GetSnowFlakeStrID()
	p := int64(0)
	s := util.HMAC_SHA256(util.AddStr(path, d, n, x, p), token_secret)
	req := &node.ReqDto{
		Data:  d,
		Time:  x,
		Nonce: n,
		Plan:  p,
		Sign:  s,
	}
	fmt.Println(req)
	ToPostBy(path, req)
}

func TestGetUser(t *testing.T) {
	data, _ := util.ToJsonBase64(map[string]string{"test": "1234566"})
	path := "/test1"
	d := data
	x := util.TimeSecond()
	n := util.GetSnowFlakeStrID()
	p := int64(0)
	s := util.HMAC_SHA256(util.AddStr(path, d, n, x, p), token_secret)
	req := &node.ReqDto{
		Data:  d,
		Time:  x,
		Nonce: n,
		Plan:  p,
		Sign:  s,
	}
	fmt.Println(req)
	ToPostBy(path, req)
}

//var (
//	publish_option = rabbitmq.Option{Exchange: "topic.test.exchange", Router: "topic.test.router", Kind: rabbitmq.DIRECT}
//	pull_option1   = rabbitmq.Option{Exchange: "topic.test.exchange", Router: "topic.test.router", Queue: "topic.test.queue1", Kind: rabbitmq.DIRECT}
//	pull_option2   = rabbitmq.Option{Exchange: "topic.test.exchange", Router: "topic.test.#", Queue: "topic.test.queue2", Kind: rabbitmq.DIRECT}
//	amqp           = rabbitmq.AmqpConfig{
//		Host:     "192.168.27.160",
//		Port:     5672,
//		Username: "admin",
//		Password: "admin",
//	}
//)
//
//func TestPublish(t *testing.T) {
//	v, _ := new(rabbitmq.PublishManager).InitConfig(amqp)
//	client, _ := v.Client()
//	client.Publish(&rabbitmq.MsgData{
//		Option:  publish_option,
//		Content: map[string]string{"username": "张三"},
//	})
//	// new(rabbitmq.PullManager).InitConfig(amqp)
//}
//
//func TestPull1(t *testing.T) {
//	v, _ := new(rabbitmq.PullManager).InitConfig(amqp)
//	client, _ := v.Client()
//	pull := &rabbitmq.PullReceiver{
//		LisData: &rabbitmq.LisData{Option: pull_option1},
//		Callback: func(msg *rabbitmq.MsgData) error {
//			b, _ := util.JsonMarshal(msg)
//			fmt.Println("消费读取数据1: ", util.Bytes2Str(b))
//			return nil
//		},
//	}
//	client.AddPullReceiver(pull)
//	time.Sleep(1 * time.Hour)
//}
//
//func TestPull2(t *testing.T) {
//	v, _ := new(rabbitmq.PullManager).InitConfig(amqp)
//	client, _ := v.Client()
//	pull := &rabbitmq.PullReceiver{
//		LisData: &rabbitmq.LisData{Option: pull_option2},
//		Callback: func(msg *rabbitmq.MsgData) error {
//			b, _ := util.JsonMarshal(msg)
//			fmt.Println("消费读取数据2: ", util.Bytes2Str(b))
//			return nil
//		},
//	}
//	client.AddPullReceiver(pull)
//	time.Sleep(1 * time.Hour)
//}
