package main

import (
	"bytes"
	"fmt"
	"github.com/godaddy-x/freego/component/jwt"
	"github.com/godaddy-x/freego/node"
	"github.com/godaddy-x/freego/util"
	"io/ioutil"
	"net/http"
	"testing"
)

const domain = "http://localhost:8090"

// 测试使用的http post示例方法
func ToPostBy(token, path string, data interface{}) {
	bytesData, err := util.JsonMarshal(&data)
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
	request.Header.Set("Authorization", token)
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

func TestLogin(t *testing.T) {
	subject := &jwt.Subject{}
	data, _ := util.ToJsonBase64(map[string]string{"test": "1234566"})
	path := "/login1"
	token := "eyJzdWIiOjEyMzQ1NiwiYXVkIjoiMjIyMjIiLCJpc3MiOiIxMTExIiwiaWF0IjoxNjQ4NjI0NzQ4Nzk1LCJleHAiOjE2NDk4MzQzNDg3OTUsImRldiI6IkFQUCIsImp0aSI6ImMxMTA2ZmY3ZTQ5NzFkNTI5ZGU5Yjc0YzNhNzhlNGYyNzhiOTdlODc2NWM5MDA1ZGQ1ODM3YmM2MjBjM2ZlZjgiLCJuc3IiOiJlN2Q1OTFiMjIyNWM0NjU2NzkzNGIiLCJleHQiOnsidGVzdCI6IjExIiwidGVzdDIiOiIyMjIifX0=.b73f8329a96c8d68267ba1cd77015f73d112487e23ee70b7adbf6b1a75f68608"
	d := data
	x := util.Time()
	n := util.GetSnowFlakeStrID()
	p := int64(0)
	s := util.HMAC256(util.AddStr(path, d, n, x, p), subject.GetTokenSecret(token))
	req := &node.ReqDto{
		Data:  d,
		Time:  x,
		Nonce: n,
		Plan:  p,
		Sign:  s,
	}
	fmt.Println(req)
	//ToPostBy(token, path, req)
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
