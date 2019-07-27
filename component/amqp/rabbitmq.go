package rabbitmq

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
	DsName   string
	Host     string
	Port     int
	Username string
	Password string
}

type Option struct {
	Exchange string `json:"exchange"`
	Queue    string `json:"queue"`
	Kind     string `json:"kind"`
	Router   string `json:"router"`
	SigTyp   int    `json:"sigTyp"` // 1.MD5 2.SHA256 11.MD5+AES 21.SHA256+AES
	SigKey   string `json:"-"`
}

// Amqp消息参数
type MsgData struct {
	Option    Option      `json:"option"`
	Durable   bool        `json:"durable"`
	Content   interface{} `json:"content"`
	Type      int64       `json:"type"`
	Delay     int64       `json:"delay"`
	Retries   int64       `json:"retries"`
	Signature string      `json:"signature"`
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
type LisData struct {
	Option        Option
	Durable       bool
	PrefetchCount int
	PrefetchSize  int
	IsNack        bool
	AutoAck       bool
}
