package main

import (
	"fmt"
	"testing"
	"time"

	rabbitmq "github.com/godaddy-x/freego/amqp"
)

var exchange = "test.exchange"
var queue = "test.monitor"
var input = rabbitmq.AmqpConfig{
	Username: "admin",
	Password: "123456",
	Host:     "172.31.25.1",
	Port:     5672,
}

// 单元测试
func TestMQPull(t *testing.T) {
	mq, err := rabbitmq.NewPull()
	if err != nil {
		panic(err)
	}
	cli, _ := mq.Client()
	receiver := &rabbitmq.PullReceiver{
		Config: &rabbitmq.Config{Option: rabbitmq.Option{
			Exchange: exchange,
			Queue:    queue,
		}},
		Callback: func(msg *rabbitmq.MsgData) error {
			fmt.Println("receive msg: ", msg)
			return nil
		},
	}
	cli.AddPullReceiver(receiver)
	time.Sleep(10000 * time.Second)
}

func TestMQPublish(t *testing.T) {
	mq, err := rabbitmq.NewPublish()
	if err != nil {
		panic(err)
	}
	cli, _ := mq.Client()
	content := map[string]interface{}{"test": 1234}
	for {
		err := cli.Publish(exchange, queue, 1, content)
		if err != nil {
			fmt.Println("send msg failed: ", err)
			break
		} else {
			fmt.Println("send msg success: ", content)
		}
		time.Sleep(5 * time.Second)
	}
}
