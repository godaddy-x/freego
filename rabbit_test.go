package main

import (
	"fmt"
	"github.com/godaddy-x/freego/component/amqp"
	"testing"
	"time"
)

var exchange = "test.exchange"
var queue = "test.monitor"
var input = rabbitmq.AmqpConfig{
	Username: "root",
	Password: "123456",
	Host:     "127.0.0.1",
	Port:     5672,
}

// 单元测试
func TestMQPull(t *testing.T) {
	mq, _ := new(rabbitmq.PullManager).Client()
	mq.AddPullReceiver(
		&rabbitmq.PullReceiver{
			LisData: &rabbitmq.LisData{Option: rabbitmq.Option{
				Exchange: exchange,
				Queue:    queue,
			}},
			Callback: func(msg *rabbitmq.MsgData) error {
				fmt.Println("msg: ", msg)
				return nil
			},
		},
	)
	time.Sleep(10000 * time.Second)
}

func TestMQPublish(t *testing.T) {
	mq, err := new(rabbitmq.PublishManager).InitConfig(input)
	if err != nil {
		panic(err)
	}
	cli, _ := mq.Client()
	v := map[string]interface{}{"test": 1234}
	cli.Publish(&rabbitmq.MsgData{
		Option: rabbitmq.Option{
			Exchange: exchange,
			Queue:    queue,
		},
		Content: &v,
	})
}
