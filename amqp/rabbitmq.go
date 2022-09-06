package rabbitmq

import (
	"fmt"
	"github.com/godaddy-x/freego/utils"
	"github.com/streadway/amqp"
	"net"
	"time"
)

const (
	direct = "direct"
)

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
	SigTyp   int    `json:"st"` // 是否加密 0.明文签名 1.AES加密签名
	SigKey   string `json:"-"`  // 验签密钥
}

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
		return nil, utils.Error("rabbitmq init failed: ", err)
	}
	return c, nil
}
