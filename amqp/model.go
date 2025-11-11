package rabbitmq

import "time"

// MsgData 消息数据结构体
//
// 包含完整的消息信息，支持消息加密、签名、
// 延时投递、重试等高级功能。
// easyjson:json
type MsgData struct {
	// 字符串字段（16字节对齐）- 按声明顺序排列以优化对齐
	Content    string `json:"co"` // 消息内容，可以是字符串或其他类型
	Nonce      string `json:"no"` // 随机数，用于防重放攻击
	Signature  string `json:"sg"` // 消息数字签名
	Expiration string `json:"ex"` // 消息过期时间（RabbitMQ格式）

	// int64字段（8字节对齐）
	Type    int64 `json:"ty"` // 消息类型标识
	Delay   int64 `json:"dy"` // 延时投递时间(秒)，0表示立即投递
	Retries int64 `json:"rt"` // 已重试次数

	// 结构体字段（8字节对齐）
	Option Option `json:"op"` // 消息队列配置选项

	// 小字段分组（1字节对齐）- 放在最后减少填充
	Priority uint8 `json:"pr"` // 消息优先级(0-255)
}

// Option 消息队列选项配置
//
// 定义交换机、队列和路由等消息队列基础配置，
// 以及消息安全相关的配置参数。
// easyjson:json
type Option struct {
	// 字符串字段（16字节对齐）- 按声明顺序排列以优化对齐
	Exchange string `json:"ex"` // 交换机名称
	Queue    string `json:"qe"` // 队列名称
	Kind     string `json:"kd"` // 交换机类型: direct/topic/headers/fanout
	Router   string `json:"ru"` // 路由键，用于消息路由
	SigKey   string `json:"-"`  // 签名密钥，用于消息验证和解密

	// 8字节对齐字段
	SigTyp         int           `json:"st"`                   // 签名类型: 0=明文签名, 1=AES加密签名
	ConfirmTimeout time.Duration `json:"ct"`                   // Confirm模式确认超时，默认30秒
	DLXConfig      *DLXConfig    `json:"dlx_config,omitempty"` // 死信队列配置，可选

	// 小字段分组（1字节对齐）- 放在最后减少填充
	Durable        bool `json:"du"` // 是否持久化交换机和队列
	AutoDelete     bool `json:"ad"` // 是否自动删除
	Exclusive      bool `json:"ev"` // 是否排他队列
	UseTransaction bool `json:"ut"` // 是否使用事务模式（默认true）批量发布时有效
}

// DLXConfig 死信队列配置
//
// 当消息无法被正常消费时，会被路由到死信队列，
// 用于消息重试、异常处理等场景。
// easyjson:json
type DLXConfig struct {
	DlxExchange string `json:"dlx_exchange"` // 死信交换机名称
	DlxQueue    string `json:"dlx_queue"`    // 死信队列名称
	DlxRouter   string `json:"dlx_router"`   // 死信路由键
}
