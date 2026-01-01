package rabbitmq

import (
	"fmt"
	"sync"
	"time"

	DIC "github.com/godaddy-x/freego/common"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/zlog"
	"github.com/streadway/amqp"
)

var (
	publishMgrs  = make(map[string]*PublishManager)
	publishMgrMu sync.RWMutex
)

type PublishManager struct {
	mu0      sync.Mutex
	mu       sync.Mutex
	conf     AmqpConfig
	conn     *amqp.Connection
	channels map[string]*PublishMQ
	closeCh  chan struct{}
	closed   bool
}

type PublishMQ struct {
	mu        sync.Mutex
	ready     bool
	listening bool // 标记是否已启动监听器，避免重复启动
	option    *Option
	channel   *amqp.Channel
	queue     *amqp.Queue
}

// InitConfig 初始化配置（支持多数据源配置）
// 通过双重检查锁定机制确保线程安全，避免重复初始化
func (self *PublishManager) InitConfig(confs ...AmqpConfig) error {
	if len(confs) == 0 {
		return fmt.Errorf("rabbitmq publish init failed: at least one config is required")
	}

	var lastErr error
	successCount := 0

	// 遍历所有配置，为每个配置创建对应的manager
	for i, conf := range confs {
		_, err := self.initSingleConfig(conf)
		if err != nil {
			zlog.Warn("failed to init publish manager for config", 0,
				zlog.AddError(err),
				zlog.String("ds_name", conf.DsName),
				zlog.Int("config_index", i))
			lastErr = err
			continue
		}

		successCount++
		zlog.Info("publish manager initialized successfully", 0,
			zlog.String("ds_name", conf.DsName),
			zlog.Int("config_index", i))
	}

	if successCount == 0 {
		return fmt.Errorf("rabbitmq publish init failed: all configs failed, last error: %w", lastErr)
	}

	zlog.Info("publish managers initialization completed", 0,
		zlog.Int("total_configs", len(confs)),
		zlog.Int("success_count", successCount),
		zlog.Int("failure_count", len(confs)-successCount))

	return nil
}

// initSingleConfig 为单个配置创建publish manager
func (self *PublishManager) initSingleConfig(conf AmqpConfig) (*PublishManager, error) {
	if conf.DsName == "" {
		conf.DsName = DIC.MASTER
	}

	// 使用写锁进行双重检查锁定，确保线程安全
	publishMgrMu.Lock()
	defer publishMgrMu.Unlock()

	// 第二重检查 - 在锁内进行
	if existing, exists := publishMgrs[conf.DsName]; exists {
		return existing, nil
	}

	// 创建新的实例
	publishMgr := &PublishManager{
		conf:     conf,
		channels: make(map[string]*PublishMQ),
		closeCh:  make(chan struct{}),
	}

	// 连接到RabbitMQ
	if err := publishMgr.Connect(); err != nil {
		return nil, fmt.Errorf("connect failed: %w", err)
	}

	// 存储到全局映射
	publishMgrs[conf.DsName] = publishMgr
	zlog.Info("rabbitmq publish service started successfully", 0,
		zlog.String("ds_name", conf.DsName))
	return publishMgr, nil
}

// Client 获取已初始化的PublishManager
func (self *PublishManager) Client(ds ...string) (*PublishManager, error) {
	dsName := DIC.MASTER
	if len(ds) > 0 && ds[0] != "" {
		dsName = ds[0]
	}

	publishMgrMu.RLock()
	mgr, ok := publishMgrs[dsName]
	publishMgrMu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("rabbitmq publish manager not found: [%s]", dsName)
	}
	return mgr, nil
}

func NewPublish(ds ...string) (*PublishManager, error) {
	return new(PublishManager).Client(ds...)
}

// Connect 建立连接
func (self *PublishManager) Connect() error {
	conn, err := ConnectRabbitMQ(self.conf)
	if err != nil {
		return fmt.Errorf("connect to rabbitmq failed: %w", err)
	}
	self.conn = conn
	return nil
}

func (self *PublishManager) openChannel() (*amqp.Channel, error) {
	self.mu.Lock()
	defer self.mu.Unlock()
	channel, err := self.conn.Channel()
	if err != nil {
		e, b := err.(*amqp.Error)
		if b && e.Code == 504 { // 重连connection
			if err := self.Connect(); err != nil {
				return nil, err
			}
		}
		return nil, err
	}
	return channel, nil
}

func (self *PublishManager) getChannel() *amqp.Channel {
	index := 0
	for {
		if index > 0 {
			zlog.Warn("rabbitmq publish trying to connect again", 0, zlog.Int("tried", index))
		}
		channel, err := self.openChannel()
		if err != nil {
			zlog.Error("rabbitmq publish init Connection/Channel failed: ", 0, zlog.AddError(err))
			time.Sleep(2500 * time.Millisecond)
			index++
			continue
		}
		return channel
	}
}

func (self *PublishManager) Queue(data *MsgData) (*QueueData, error) {
	pub, err := self.initQueue(data)
	if err != nil {
		return nil, err
	}
	return &QueueData{
		Name:      pub.queue.Name,
		Consumers: pub.queue.Consumers,
		Messages:  pub.queue.Messages}, nil
}

func (self *PublishManager) initQueue(data *MsgData) (*PublishMQ, error) {
	if len(data.Option.Router) == 0 {
		data.Option.Router = data.Option.Queue
	}
	if !utils.CheckInt(data.Option.SigTyp, 0, 1) {
		data.Option.SigTyp = 1
	}
	//if len(data.Option.SigKey) < 32 {
	//	data.Option.SigKey = utils.AddStr(utils.GetLocalSecretKey(), self.conf.SecretKey)
	//}
	if len(data.Nonce) == 0 {
		data.Nonce = utils.RandStr(6)
	}
	chanKey := utils.AddStr(data.Option.Exchange, data.Option.Router, data.Option.Queue)

	// 双重检查锁定：先检查是否已存在
	if pub, ok := self.channels[chanKey]; ok {
		return pub, nil
	}

	// 获取锁进行原子操作
	self.mu0.Lock()
	defer self.mu0.Unlock()

	// 再次检查（在锁内）
	if pub, ok := self.channels[chanKey]; ok {
		return pub, nil
	}

	// 创建新的 PublishMQ 实例
	if len(data.Option.Kind) == 0 {
		data.Option.Kind = direct
	}
	if len(data.Option.Router) == 0 {
		data.Option.Router = data.Option.Queue
	}

	opt := &Option{
		Kind:     data.Option.Kind,
		Exchange: data.Option.Exchange,
		Queue:    data.Option.Queue,
		Router:   data.Option.Router,
		SigTyp:   data.Option.SigTyp,
	}

	// 创建 PublishMQ 实例
	pub := &PublishMQ{option: opt}

	// 获取专用通道（每个 PublishMQ 实例使用独立的通道）
	channel := self.getChannel()
	pub.channel = channel

	// 初始化 exchange 和 queue
	if err := pub.prepareExchange(); err != nil {
		// 清理失败的通道
		if closeErr := pub.channel.Close(); closeErr != nil {
			zlog.Warn("failed to close channel after exchange preparation error", 0, zlog.AddError(closeErr))
		}
		return nil, fmt.Errorf("failed to prepare exchange: %w", err)
	}

	if err := pub.prepareQueue(); err != nil {
		// 清理失败的通道
		if closeErr := pub.channel.Close(); closeErr != nil {
			zlog.Warn("failed to close channel after queue preparation error", 0, zlog.AddError(closeErr))
		}
		return nil, fmt.Errorf("failed to prepare queue: %w", err)
	}

	// 标记为就绪并存储
	pub.ready = true
	self.channels[chanKey] = pub

	return pub, nil
}

func (self *PublishManager) listen(pub *PublishMQ) {
	pub.mu.Lock()
	if !pub.ready { // 重新连接channel
		pub.channel = self.getChannel()
		pub.ready = true
	}
	pub.mu.Unlock()

	// 监控通道状态
	mqErr := make(chan *amqp.Error, 1)
	closeErr := pub.channel.NotifyClose(mqErr)

	select {
	case err := <-closeErr:
		if err != nil {
			zlog.Error("rabbitmq publish connection/channel receive failed", 0,
				zlog.String("exchange", pub.option.Exchange),
				zlog.String("queue", pub.option.Queue),
				zlog.AddError(err))
		}

		// 标记为不就绪并重新连接
		pub.mu.Lock()
		pub.ready = false
		pub.listening = false // 重置监听标志，允许重连时重新启动监听器
		pub.mu.Unlock()

		zlog.Warn("rabbitmq publish received channel exception, attempting reconnection", 0,
			zlog.String("exchange", pub.option.Exchange),
			zlog.String("queue", pub.option.Queue))

		// 重新启动监听（非递归，避免栈溢出）
		go self.listen(pub)
	case <-self.closeCh:
		// PublishManager 被关闭，退出监听
		zlog.Info("publish listener stopped due to manager shutdown", 0,
			zlog.String("exchange", pub.option.Exchange),
			zlog.String("queue", pub.option.Queue))
		return
	}
}

func (self *PublishManager) Publish(exchange, queue string, dataType int64, content interface{}) error {
	jsonData, err := utils.JsonMarshal(content)
	if err != nil {
		return err
	}
	msg := GetMsgData()
	defer PutMsgData(msg)
	msg.Option.SigKey = self.conf.SecretKey
	msg.Option.Exchange = exchange
	msg.Option.Queue = queue
	msg.Type = dataType
	msg.CreatedAt = utils.UnixMilli()
	msg.Content = utils.Bytes2Str(jsonData)
	msg.Nonce = utils.Base64Encode(utils.GetRandomSecure(32))
	return self.PublishMsgData(msg)
}

func (self *PublishManager) PublishMsgData(data *MsgData) error {
	if data == nil {
		return fmt.Errorf("publish data empty")
	}
	pub, err := self.initQueue(data)
	if err != nil {
		return err
	}
	// 确保监听已启动（延迟启动，避免不必要的资源占用）
	// 使用原子操作确保只启动一次监听器
	pub.mu.Lock()
	shouldStart := pub.ready && pub.channel != nil && !pub.listening
	if shouldStart {
		pub.listening = true // 在锁内立即设置标志，避免竞态条件
	}
	pub.mu.Unlock()

	if shouldStart {
		go self.listen(pub)
	}
	// 数据加密模式
	sigTyp := data.Option.SigTyp
	sigKey := data.Option.SigKey
	if len(sigKey) == 0 {
		return fmt.Errorf("rabbitmq publish data key is nil")
	}
	if sigTyp == 1 {
		aesContent, err := utils.AesGCMEncrypt(utils.Str2Bytes(data.Content), sigKey)
		if err != nil {
			return fmt.Errorf("rabbitmq publish content aes encrypt failed: %w", err)
		}
		data.Content = aesContent
	}
	data.Signature = utils.HMAC_SHA256(utils.AddStr(data.Content, data.Nonce, data.CreatedAt), sigKey, true)
	data.Option.SigKey = ""
	if _, err := pub.sendMessage(data); err != nil {
		return err
	}
	return nil
}

func (self *PublishMQ) sendMessage(msg *MsgData) (bool, error) {
	body, err := utils.JsonMarshal(msg)
	if err != nil {
		return false, err
	}
	data := amqp.Publishing{ContentType: "text/plain", Body: body}
	if err := self.channel.Publish(self.option.Exchange, self.option.Router, false, false, data); err != nil {
		return false, err
	}
	return true, nil
}

func (self *PublishMQ) prepareExchange() error {
	zlog.Println(fmt.Sprintf("rabbitmq publish init [%s - %s] successful", self.option.Kind, self.option.Exchange))
	return self.channel.ExchangeDeclare(self.option.Exchange, self.option.Kind, true, false, false, false, nil)
}

func (self *PublishMQ) prepareQueue() error {
	if len(self.option.Queue) == 0 {
		return nil
	}
	if q, err := self.channel.QueueDeclare(self.option.Queue, true, false, false, false, nil); err != nil {
		return err
	} else {
		self.queue = &q
	}
	if err := self.channel.QueueBind(self.option.Queue, self.option.Router, self.option.Exchange, false, nil); err != nil {
		return err
	}
	return nil
}

// Close 关闭PublishManager
func (self *PublishManager) Close() error {
	self.mu0.Lock()
	if self.closed {
		self.mu0.Unlock()
		return nil
	}
	self.closed = true
	self.mu0.Unlock()

	zlog.Info("closing publish manager", 0, zlog.String("ds_name", self.conf.DsName))

	// 发送关闭信号
	select {
	case <-self.closeCh:
		// channel 已经关闭
	default:
		close(self.closeCh)
	}

	// 等待一小段时间让监听器退出
	time.Sleep(100 * time.Millisecond)

	// 关闭所有通道
	for key, pub := range self.channels {
		if pub.channel != nil {
			if err := pub.channel.Close(); err != nil {
				zlog.Warn("channel close warning", 0, zlog.AddError(err))
			}
		}
		delete(self.channels, key)
	}

	// 关闭连接
	if self.conn != nil && !self.conn.IsClosed() {
		if err := self.conn.Close(); err != nil {
			zlog.Warn("close connection warning", 0, zlog.AddError(err))
		}
		self.conn = nil
	}

	// 从全局映射中移除
	publishMgrMu.Lock()
	delete(publishMgrs, self.conf.DsName)
	publishMgrMu.Unlock()

	return nil
}
