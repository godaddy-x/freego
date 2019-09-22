package main

import (
	"bytes"
	"fmt"
	"github.com/godaddy-x/freego/component/amqp"
	"github.com/godaddy-x/freego/node"
	"github.com/godaddy-x/freego/util"
	"io/ioutil"
	"net/http"
	"testing"
	"time"
)

func ToPost(url, token, access_key string, req interface{}) {
	content, _ := util.JsonMarshal(req)
	data := util.Base64Encode(content)
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
	bytesData, err := util.JsonMarshal(&data)
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

var (
	publish_option = rabbitmq.Option{Exchange: "topic.test.exchange", Router: "topic.test.router", Kind: rabbitmq.DIRECT}
	pull_option1    = rabbitmq.Option{Exchange: "topic.test.exchange", Router: "topic.test.router", Queue: "topic.test.queue1", Kind: rabbitmq.DIRECT}
	pull_option2    = rabbitmq.Option{Exchange: "topic.test.exchange", Router: "topic.test.#", Queue: "topic.test.queue2", Kind: rabbitmq.DIRECT}
	amqp           = rabbitmq.AmqpConfig{
		Host:     "192.168.27.160",
		Port:     5672,
		Username: "admin",
		Password: "admin",
	}
)

func TestPublish(t *testing.T) {
	v, _ := new(rabbitmq.PublishManager).InitConfig(amqp)
	client, _ := v.Client()
	client.Publish(&rabbitmq.MsgData{
		Option:  publish_option,
		Content: map[string]string{"username": "张三"},
	})
	// new(rabbitmq.PullManager).InitConfig(amqp)
}

func TestPull1(t *testing.T) {
	v, _ := new(rabbitmq.PullManager).InitConfig(amqp)
	client, _ := v.Client()
	pull := &rabbitmq.PullReceiver{
		LisData: &rabbitmq.LisData{Option: pull_option1},
		Callback: func(msg *rabbitmq.MsgData) error {
			b, _ := util.JsonMarshal(msg)
			fmt.Println("消费读取数据1: ", util.Bytes2Str(b))
			return nil
		},
	}
	client.AddPullReceiver(pull)
	time.Sleep(1 * time.Hour)
}

func TestPull2(t *testing.T) {
	v, _ := new(rabbitmq.PullManager).InitConfig(amqp)
	client, _ := v.Client()
	pull := &rabbitmq.PullReceiver{
		LisData: &rabbitmq.LisData{Option: pull_option2},
		Callback: func(msg *rabbitmq.MsgData) error {
			b, _ := util.JsonMarshal(msg)
			fmt.Println("消费读取数据2: ", util.Bytes2Str(b))
			return nil
		},
	}
	client.AddPullReceiver(pull)
	time.Sleep(1 * time.Hour)
}
