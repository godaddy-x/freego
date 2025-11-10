// Package rabbitmq 提供RabbitMQ消息队列的Go实现
//
// 功能特性:
//   - 生产者: 支持消息发布、批量发布、延时消息
//   - 消费者: 支持消息消费、自动重试、死信队列
//   - 安全: 支持消息加密和数字签名验证
//   - 连接: 支持连接池、自动重连、健康检查
//   - 监控: 提供详细的连接和队列统计信息
//
// 交换机类型常量:
//   - ExchangeDirect: 直接交换机，根据路由键精确匹配
//   - ExchangeTopic: 主题交换机，支持通配符匹配
//   - ExchangeHeaders: 头交换机，根据消息头匹配
//   - ExchangeFanout: 扇出交换机，广播到所有绑定队列
//
// 使用示例:
//
//	config := AmqpConfig{
//	    Host:     "localhost",
//	    Port:     5672,
//	    Username: "guest",
//	    Password: "guest",
//	}
//	conn, err := ConnectRabbitMQ(config)
//
//	// 使用交换机常量
//	option := Option{
//	    Exchange: "my.exchange",
//	    Queue:    "my.queue",
//	    Kind:     ExchangeDirect, // 使用常量避免拼写错误
//	}
package rabbitmq

import (
	"fmt"
	"net"
	"time"

	"github.com/godaddy-x/freego/utils"
	"github.com/streadway/amqp"
)

// 交换机类型常量定义（公开常量，增强类型安全）
const (
	ExchangeDirect  = "direct"  // 直接交换机: 根据路由键精确匹配
	ExchangeTopic   = "topic"   // 主题交换机: 支持通配符匹配
	ExchangeHeaders = "headers" // 头交换机: 根据消息头匹配
	ExchangeFanout  = "fanout"  // 扇出交换机: 广播到所有绑定队列
)

const (
	// RabbitMQ交换机类型定义（内部使用）
	direct  = ExchangeDirect  // 直接交换机: 根据路由键精确匹配
	topic   = ExchangeTopic   // 主题交换机: 支持通配符匹配
	headers = ExchangeHeaders // 头交换机: 根据消息头匹配
	fanout  = ExchangeFanout  // 扇出交换机: 广播到所有绑定队列

	// 默认配置常量
	defaultPort              = 5672             // RabbitMQ默认端口
	defaultVhost             = "/"              // 默认虚拟主机
	defaultHeartbeat         = 10 * time.Second // 默认心跳间隔
	defaultConnectionTimeout = 30 * time.Second // 默认连接超时时间
	defaultChannelMax        = 0                // 最大通道数，0表示无限制
	defaultFrameSize         = 0                // 帧大小，0表示使用服务器默认值
	defaultPrefetchCount     = 1                // 默认预取数量
	defaultPrefetchSize      = 0                // 默认预取大小，0表示不限制
)

// AmqpConfig RabbitMQ连接配置结构体
//
// 包含连接RabbitMQ服务器所需的所有配置参数，
// 支持详细的连接参数配置和安全验证。
type AmqpConfig struct {
	// 字符串字段（16字节，8字节对齐）
	DsName    string // 数据源名称，用于标识不同的RabbitMQ实例
	Host      string // RabbitMQ服务器主机地址（必需）
	Username  string // RabbitMQ用户名（必需）
	Password  string // RabbitMQ密码，建议加密存储（必需）
	SecretKey string // 消息签名和加密使用的密钥
	Vhost     string // 虚拟主机路径，默认"/"

	// time.Duration字段（8字节，8字节对齐）
	Heartbeat         time.Duration // 连接心跳间隔，默认10秒
	ConnectionTimeout time.Duration // 连接超时时间，默认30秒

	// int字段（8字节，8字节对齐）
	Port       int // RabbitMQ服务器端口号，默认5672
	ChannelMax int // 最大通道数，0表示无限制
	FrameSize  int // AMQP帧大小，0表示使用服务器默认值
}

// QueueData 队列状态数据
//
// 包含队列的实时状态信息，用于监控和管理。
type QueueData struct {
	Name      string `json:"name"`      // 队列名称
	Consumers int    `json:"consumers"` // 当前消费者数量
	Messages  int    `json:"messages"`  // 队列中待消费的消息数量
}

// DLX 死信队列配置
//
// 配置消息消费失败后的死信处理机制，
// 支持消息重试和最终失败处理。
type DLX struct {
	// 字符串字段（16字节，8字节对齐）
	DlxExchange string // 死信交换机名称
	DlxQueue    string // 死信队列名称，存储最终失败的消息
	DlkExchange string // 重试交换机名称
	DlkQueue    string // 重试队列名称，存储待重试的消息

	// 函数字段（8字节对齐）
	DlkCallFunc func(message MsgData) (MsgData, error) // 重试处理函数，返回处理后的消息或错误
}

// Config AMQP消费者配置参数
//
// 定义消费者连接和消费行为的所有配置选项，
// 包括QoS参数、确认模式、重试策略等。
type Config struct {
	// amqp.Table字段（16字节，8字节对齐）
	Args amqp.Table `json:"args"` // 额外的队列声明参数，如死信队列配置

	// int字段（8字节，8字节对齐）
	PrefetchCount int `json:"prefetch_count"` // 预取消息数量，控制消费者并发处理能力
	PrefetchSize  int `json:"prefetch_size"`  // 预取消息总大小，0表示不限制

	// bool字段（1字节对齐）
	Durable   bool `json:"durable"`   // 队列是否持久化存储
	IsNack    bool `json:"is_nack"`   // 是否支持消息否定确认，用于重试机制, true是必须确认消息，false是每次消费掉数据
	AutoAck   bool `json:"auto_ack"`  // 是否自动确认消息，false需要手动确认
	Exclusive bool `json:"exclusive"` // 是否为独占队列，只允许一个消费者
	NoWait    bool `json:"no_wait"`   // 是否不等待服务器确认，异步操作

	// 结构体字段（8字节对齐）
	Option Option `json:"option"` // 消息队列基础配置选项
}

// ValidateAndSetDefaults 验证AmqpConfig配置并设置默认值
//
// 执行以下验证和设置:
// 1. 检查必需参数（Host、Username、Password）
// 2. 设置默认端口号（5672）
// 3. 设置默认虚拟主机（"/"）
// 4. 设置默认心跳间隔（10秒）
// 5. 设置默认连接超时（30秒）
// 6. 验证端口号范围（1-65535）
//
// 返回错误时会包含详细的错误信息，便于调试。
func (conf *AmqpConfig) ValidateAndSetDefaults() error {
	// 验证必需参数
	if conf.Host == "" {
		return utils.Error("rabbitmq host is required")
	}
	if conf.Username == "" {
		return utils.Error("rabbitmq username is required")
	}
	if conf.Password == "" {
		return utils.Error("rabbitmq password is required")
	}

	// 设置默认值
	if conf.Port <= 0 {
		conf.Port = defaultPort
	}
	if conf.Vhost == "" {
		conf.Vhost = defaultVhost
	}
	if conf.Heartbeat <= 0 {
		conf.Heartbeat = defaultHeartbeat
	}
	if conf.ConnectionTimeout <= 0 {
		conf.ConnectionTimeout = defaultConnectionTimeout
	}
	if conf.ChannelMax < 0 {
		conf.ChannelMax = defaultChannelMax
	}
	if conf.FrameSize == 0 {
		conf.FrameSize = defaultFrameSize
	}

	// 验证端口范围
	if conf.Port < 1 || conf.Port > 65535 {
		return utils.Error("rabbitmq port must be between 1 and 65535")
	}

	return nil
}

// ValidateAndSetDefaults 验证Config配置并设置默认值
//
// 执行以下验证和设置:
// 1. 检查必需的Option参数（Exchange、Queue）
// 2. 设置默认交换机类型（direct）
// 3. 设置默认预取参数
// 4. 验证交换机类型有效性
// 5. 验证签名类型范围（0或1）
//
// 确保消费者配置在合理范围内，避免运行时错误。
func (conf *Config) ValidateAndSetDefaults() error {
	// 设置默认值
	if conf.PrefetchCount <= 0 {
		conf.PrefetchCount = defaultPrefetchCount
	}
	if conf.PrefetchSize < 0 {
		conf.PrefetchSize = defaultPrefetchSize
	}

	// 验证Option
	if conf.Option.Exchange == "" {
		return utils.Error("exchange is required in config option")
	}
	if conf.Option.Queue == "" {
		return utils.Error("queue is required in config option")
	}

	// 设置默认交换机类型
	if conf.Option.Kind == "" {
		conf.Option.Kind = direct
	}

	// 验证交换机类型
	switch conf.Option.Kind {
	case ExchangeDirect, ExchangeTopic, ExchangeHeaders, ExchangeFanout:
		// 有效的交换机类型
	default:
		return utils.Error("invalid exchange kind: ", conf.Option.Kind, ", must be one of: direct, topic, headers, fanout")
	}

	// 验证签名类型
	if !utils.CheckInt(conf.Option.SigTyp, 0, 1) {
		return utils.Error("invalid signature type: ", conf.Option.SigTyp, ", must be 0 (plain signature) or 1 (AES encrypted signature)")
	}

	return nil
}

// ConnectRabbitMQ 建立到RabbitMQ服务器的连接
//
// 使用提供的配置参数建立AMQP连接，支持高级连接配置:
// - 自定义心跳间隔
// - 连接超时控制
// - 通道数量限制
// - 帧大小配置
// - 连接属性标识
//
// 连接建立过程:
// 1. 验证并设置默认配置
// 2. 构建AMQP URI
// 3. 配置连接参数
// 4. 建立TCP连接并进行AMQP握手
//
// 返回:
//   - *amqp.Connection: 成功建立的连接对象
//   - error: 连接失败时的详细错误信息
//
// 注意: 调用者负责在适当时候关闭连接。
func ConnectRabbitMQ(conf AmqpConfig) (*amqp.Connection, error) {
	// 验证并设置默认配置
	if err := conf.ValidateAndSetDefaults(); err != nil {
		return nil, utils.Error("rabbitmq config validation failed: ", err)
	}

	// 构建AMQP URI
	amqpURI := fmt.Sprintf("amqp://%s:%s@%s:%d%s",
		conf.Username,
		conf.Password,
		conf.Host,
		conf.Port,
		conf.Vhost)

	// 配置连接参数
	amqpConfig := amqp.Config{
		Heartbeat:  conf.Heartbeat,
		ChannelMax: conf.ChannelMax,
		FrameSize:  conf.FrameSize,
		Properties: amqp.Table{
			"connection_name": conf.DsName, // 连接名称，便于调试
		},
		Dial: func(network, addr string) (net.Conn, error) {
			return net.DialTimeout(network, addr, conf.ConnectionTimeout)
		},
	}

	// 建立连接
	conn, err := amqp.DialConfig(amqpURI, amqpConfig)
	if err != nil {
		return nil, utils.Error("rabbitmq connection failed [", conf.DsName, "]: ", err)
	}

	return conn, nil
}

// GetConnectionInfo 获取RabbitMQ连接的详细信息
//
// 返回连接的当前状态信息，便于调试和监控:
// - 连接是否已关闭
// - 本地网络地址信息
//
// 参数:
//
//	conn: RabbitMQ连接对象，不能为nil
//
// 返回:
//   - map[string]interface{}: 包含连接状态信息的字典
//   - error: 获取信息失败时的错误
//
// 主要用于:
// - 连接健康检查
// - 调试连接问题
// - 监控连接状态
func GetConnectionInfo(conn *amqp.Connection) (map[string]interface{}, error) {
	if conn == nil {
		return nil, utils.Error("connection is nil")
	}

	info := map[string]interface{}{
		"is_closed":  conn.IsClosed(),
		"local_addr": conn.LocalAddr(),
	}

	return info, nil
}
