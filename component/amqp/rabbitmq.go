package rabbitmq

const (
	MASTER        = "MASTER"
	DIRECT        = "direct"
	PrefetchCount = 50
)

// Amqp配置参数
type AmqpConfig struct {
	DsName   string
	Host     string
	Port     int
	Username string
	Password string
}

// Amqp消息参数
type MsgData struct {
	Exchange string      `json:"exchange"`
	Queue    string      `json:"queue"`
	Durable  bool        `json:"durable"`
	Kind     string      `json:"kind"`
	Content  interface{} `json:"content"`
	Type     int64       `json:"type"`
	Delay    int64       `json:"delay"`
	Retries  int64       `json:"retries"`
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
	Exchange      string
	Queue         string
	Kind          string
	Durable       bool
	PrefetchCount int
	PrefetchSize  int
	IsNack        bool
	AutoAck       bool
}