package rabbitmq

import (
	"fmt"
	"github.com/godaddy-x/freego/util"
	"github.com/streadway/amqp"
	"net"
	"time"
)

const (
	MASTER     = "MASTER"
	DIRECT     = "direct"
	TOPIC      = "topic"
	FANOUT     = "fanout"
	MD5        = 1
	SHA256     = 2
	MD5_AES    = 11
	SHA256_AES = 21
)

// Amqp配置参数
type AmqpConfig struct {
	DsName    string
	Host      string
	Port      int
	Username  string
	Password  string
	SecretKey string
}

type Option struct {
	Exchange string `json:"ex"`
	Queue    string `json:"qe"`
	Kind     string `json:"kd"`
	Router   string `json:"ru"`
	SigTyp   int    `json:"st"` // 验签类型 1.MD5 2.SHA256 11.MD5+AES 21.SHA256+AES
	SigKey   string `json:"-"`  // 验签密钥
}

// Amqp消息参数
type MsgData struct {
	Option    Option      `json:"op"`
	Durable   bool        `json:"du"`
	Content   interface{} `json:"co"`
	Type      int64       `json:"ty"`
	Delay     int64       `json:"dy"`
	Retries   int64       `json:"rt"`
	Nonce     string      `json:"no"`
	Signature string      `json:"sg"`
}

// Amqp延迟发送配置
type DLX struct {
	DlxExchange string                                 // 死信交换机
	DlxQueue    string                                 // 死信队列
	DlkExchange string                                 // 重读交换机
	DlkQueue    string                                 // 重读队列
	DlkCallFunc func(message MsgData) (MsgData, error) // 回调函数
}

// Amqp监听配置参数
type Config struct {
	Option        Option
	Durable       bool
	PrefetchCount int
	PrefetchSize  int
	IsNack        bool
	AutoAck       bool
}

func ConnectRabbitMQ(conf AmqpConfig) (*amqp.Connection, error) {
	c, err := amqp.DialConfig(fmt.Sprintf("amqp://%s:%s@%s:%d/", conf.Username, conf.Password, conf.Host, conf.Port), amqp.Config{
		Dial: func(network, addr string) (net.Conn, error) {
			return net.DialTimeout(network, addr, 3*time.Second)
		},
	})
	if err != nil {
		return nil, util.Error("rabbitmq init failed: ", err)
	}
	return c, nil
}
