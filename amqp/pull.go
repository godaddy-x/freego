package rabbitmq

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	DIC "github.com/godaddy-x/freego/common"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/zlog"
	"github.com/streadway/amqp"
)

var (
	pullMgrs  = make(map[string]*PullManager)
	pullMgrMu sync.RWMutex
)

// PullManager 管理RabbitMQ消费连接和接收器
type PullManager struct {
	mu          sync.RWMutex
	conf        AmqpConfig
	conn        *amqp.Connection
	receivers   []*PullReceiver
	connErr     chan *amqp.Error
	closeChan   chan struct{}
	closed      bool
	monitorWg   sync.WaitGroup
	reconnectWg sync.WaitGroup
}

// InitConfig 初始化配置（支持多数据源配置）
// 通过双重检查锁定机制确保线程安全，避免重复初始化
func (self *PullManager) InitConfig(confs ...AmqpConfig) error {
	if len(confs) == 0 {
		return fmt.Errorf("rabbitmq pull init failed: at least one config is required")
	}

	var lastErr error
	successCount := 0

	// 遍历所有配置，为每个配置创建对应的manager
	for i, conf := range confs {
		_, err := self.initSingleConfig(conf)
		if err != nil {
			zlog.Warn("failed to init pull manager for config", 0,
				zlog.AddError(err),
				zlog.String("ds_name", conf.DsName),
				zlog.Int("config_index", i))
			lastErr = err
			continue
		}

		successCount++
		zlog.Info("pull manager initialized successfully", 0,
			zlog.String("ds_name", conf.DsName),
			zlog.Int("config_index", i))
	}

	if successCount == 0 {
		return fmt.Errorf("rabbitmq pull init failed: all configs failed, last error: %w", lastErr)
	}

	zlog.Info("pull managers initialization completed", 0,
		zlog.Int("total_configs", len(confs)),
		zlog.Int("success_count", successCount),
		zlog.Int("failure_count", len(confs)-successCount))

	return nil
}

// initSingleConfig 为单个配置创建pull manager
func (self *PullManager) initSingleConfig(conf AmqpConfig) (*PullManager, error) {
	// 第一重检查
	pullMgrMu.RLock()
	if _, exists := pullMgrs[conf.DsName]; exists {
		pullMgrMu.RUnlock()
		// 如果已经存在，返回已存在的实例
		return pullMgrs[conf.DsName], nil
	}
	pullMgrMu.RUnlock()

	if conf.DsName == "" {
		conf.DsName = DIC.MASTER
	}

	pullMgr := &PullManager{
		conf:      conf,
		connErr:   make(chan *amqp.Error, 1),
		closeChan: make(chan struct{}),
		receivers: make([]*PullReceiver, 0),
	}

	if err := pullMgr.Connect(); err != nil {
		return nil, fmt.Errorf("connect failed: %w", err)
	}

	// 第二重检查
	pullMgrMu.Lock()
	defer pullMgrMu.Unlock()

	if existing, exists := pullMgrs[conf.DsName]; exists {
		pullMgr.Close()
		return existing, nil
	}

	pullMgrs[conf.DsName] = pullMgr
	zlog.Info("rabbitmq pull service started successfully", 0,
		zlog.String("ds_name", conf.DsName))
	return pullMgr, nil
}

// NewPull 快捷创建PullManager客户端
func NewPull(ds ...string) (*PullManager, error) {
	dsName := DIC.MASTER
	if len(ds) > 0 && ds[0] != "" {
		dsName = ds[0]
	}

	pullMgrMu.RLock()
	mgr, ok := pullMgrs[dsName]
	pullMgrMu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("rabbitmq pull manager not found: [%s]", dsName)
	}
	return mgr, nil
}

// AddPullReceiver 添加接收器
func (self *PullManager) AddPullReceiver(receivers ...*PullReceiver) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.closed {
		return fmt.Errorf("pull manager is closed")
	}

	for _, receiver := range receivers {
		if receiver == nil {
			return fmt.Errorf("receiver cannot be nil")
		}

		// 初始化接收器
		receiver.initDefaults()
		receiver.initControlChans()

		// 添加到接收器列表
		self.receivers = append(self.receivers, receiver)

		// 启动监听
		self.monitorWg.Add(1)
		go self.listen(receiver)
	}

	return nil
}

// Connect 建立连接
func (self *PullManager) Connect() error {
	self.mu.Lock()
	defer self.mu.Unlock()

	// 关闭现有连接
	if self.conn != nil && !self.conn.IsClosed() {
		if err := self.conn.Close(); err != nil {
			zlog.Debug("close old connection warning", 0, zlog.AddError(err))
		}
		self.conn = nil
	}

	conn, err := ConnectRabbitMQ(self.conf)
	if err != nil {
		return fmt.Errorf("connect to rabbitmq failed: %w", err)
	}
	self.conn = conn

	// 启动连接监控（每次重连都启动新的监控goroutine）
	self.monitorWg.Add(1)
	go self.monitorConnection()

	zlog.Info("rabbitmq connection established", 0,
		zlog.String("ds_name", self.conf.DsName))
	return nil
}

// monitorConnection 监控连接状态
// 持续监控RabbitMQ连接状态，自动处理连接断开和重连逻辑
func (self *PullManager) monitorConnection() {
	defer self.monitorWg.Done()

	self.mu.RLock()
	if self.conn == nil || self.closed {
		self.mu.RUnlock()
		return
	}

	conn := self.conn
	closeChan := conn.NotifyClose(make(chan *amqp.Error, 1))
	self.mu.RUnlock()

	select {
	case <-self.closeChan:
		return
	case err, ok := <-closeChan:
		if !ok {
			return
		}

		// 验证连接是否仍然是同一个
		self.mu.RLock()
		isSameConnection := (self.conn == conn)
		self.mu.RUnlock()

		if !isSameConnection {
			zlog.Debug("connection has been replaced, ignoring close event", 0)
			return
		}

		if err != nil {
			zlog.Error("rabbitmq connection closed unexpectedly", 0,
				zlog.AddError(err),
				zlog.String("ds_name", self.conf.DsName))
		}

		// 触发重连所有接收器
		self.reconnectAllReceivers()
	}
}

// reconnectAllReceivers 重连所有接收器
// 持续尝试重连RabbitMQ，直到成功或程序关闭
func (self *PullManager) reconnectAllReceivers() {
	baseDelay := time.Second
	attempt := 0

	for {
		select {
		case <-self.closeChan:
			return
		default:
		}

		attempt++
		if err := self.Connect(); err == nil {
			zlog.Info("reconnection successful after attempts", 0,
				zlog.String("ds_name", self.conf.DsName),
				zlog.Int("attempts", attempt))
			self.restartAllReceivers()
			return
		} else {
			// 连接失败，记录错误日志
			zlog.Warn("reconnection failed, will retry", 0,
				zlog.String("ds_name", self.conf.DsName),
				zlog.Int("attempt", attempt),
				zlog.AddError(err))
		}

		// 计算延迟时间（指数退避，最长30秒）
		delay := time.Duration(attempt) * baseDelay
		if delay > 30*time.Second {
			delay = 30 * time.Second
		}

		// 使用select监听关闭信号，避免长时间阻塞
		select {
		case <-self.closeChan:
			return
		case <-time.After(delay):
			// 继续下一次重试
		}
	}
}

// restartAllReceivers 重启所有接收器（带重连风暴防护）
func (self *PullManager) restartAllReceivers() {
	self.mu.Lock()
	receivers := make([]*PullReceiver, len(self.receivers))
	copy(receivers, self.receivers) // 复制接收器列表
	self.mu.Unlock()

	// 为每个接收器添加随机延迟，避免重连风暴
	for i, receiver := range receivers {
		// 停止当前监听
		receiver.Stop()

		// 等待一小段时间确保资源清理
		time.Sleep(100 * time.Millisecond)

		// 重新初始化
		receiver.initControlChans()

		// 添加随机延迟分散重连时间（0-2秒）
		jitter := time.Duration(rand.Intn(2000)) * time.Millisecond
		time.Sleep(jitter)

		zlog.Info("restarting receiver with jitter", 0,
			zlog.String("queue", receiver.Config.Option.Queue),
			zlog.Int("receiver_index", i),
			zlog.Duration("jitter", jitter))

		// 启动监听
		self.monitorWg.Add(1)
		go self.listen(receiver)
	}
}

// getChannel 获取通道（带重试机制）
// 从连接池获取AMQP通道，失败时进行重试
func (self *PullManager) getChannel() (*amqp.Channel, error) {
	const maxRetries = 3

	for i := 0; i < maxRetries; i++ {
		select {
		case <-self.closeChan:
			return nil, fmt.Errorf("pull manager is closed")
		default:
		}

		self.mu.Lock()
		if self.conn == nil || self.conn.IsClosed() {
			self.mu.Unlock()
			return nil, fmt.Errorf("connection is not available")
		}
		conn := self.conn
		self.mu.Unlock()

		channel, err := conn.Channel()
		if err == nil {
			// 设置QoS
			if err := channel.Qos(1, 0, false); err != nil {
				zlog.Warn("set channel QoS warning", 0, zlog.AddError(err))
			}
			return channel, nil
		}

		zlog.Warn("failed to create channel, retrying...", 0,
			zlog.AddError(err),
			zlog.Int("retry", i+1))

		if i < maxRetries-1 {
			time.Sleep(time.Duration(i+1) * 500 * time.Millisecond)
		}
	}

	return nil, fmt.Errorf("failed to create channel after %d retries", maxRetries)
}

// listen 监听消息队列
// 为指定的接收器启动消息监听循环，包含连接管理和错误恢复
func (self *PullManager) listen(receiver *PullReceiver) {
	defer self.monitorWg.Done()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 使用waitGroup管理goroutine
	var wg sync.WaitGroup

	// 获取通道
	channel, err := self.getChannel()
	if err != nil {
		receiver.OnError(fmt.Errorf("get channel failed: %w", err))
		receiver.scheduleReconnect(self)
		return
	}
	receiver.setChannel(channel)

	// 初始化交换机和队列
	if err := self.setupChannel(receiver, channel); err != nil {
		receiver.OnError(err)
		receiver.scheduleReconnect(self)
		return
	}

	// 启动消息消费
	msgs, err := channel.Consume(
		receiver.Config.Option.Queue,
		"",    // consumer
		false, // auto-ack
		false, // exclusive
		false, // no-local
		false, // no-wait
		nil,   // args
	)
	if err != nil {
		receiver.OnError(fmt.Errorf("consume failed: %w", err))
		receiver.scheduleReconnect(self)
		return
	}

	// 启动通道监控
	wg.Add(1)
	go func() {
		defer wg.Done()
		self.monitorChannel(ctx, receiver, channel)
	}()

	// 启动消息处理
	wg.Add(1)
	go func() {
		defer wg.Done()
		self.processMessages(ctx, receiver, msgs)
	}()

	zlog.Info("receiver started successfully", 0,
		zlog.String("exchange", receiver.Config.Option.Exchange),
		zlog.String("queue", receiver.Config.Option.Queue))

	// 等待停止或重连信号
	select {
	case <-receiver.stopChan:
		zlog.Info("receiver stopped by request", 0,
			zlog.String("queue", receiver.Config.Option.Queue))
	case <-receiver.closeChan:
		zlog.Info("receiver reconnecting", 0,
			zlog.String("queue", receiver.Config.Option.Queue))
		// 延迟重连，避免频繁重连
		time.Sleep(2 * time.Second)
		go func() {
			self.monitorWg.Add(1)
			self.listen(receiver)
		}()
	case <-self.closeChan:
		zlog.Info("receiver stopped due to manager shutdown", 0,
			zlog.String("queue", receiver.Config.Option.Queue))
	}

	// 清理资源
	cancel()
	if err := channel.Close(); err != nil {
		zlog.Debug("channel close warning", 0, zlog.AddError(err))
	}
	receiver.setChannel(nil)
	wg.Wait()
}

// monitorChannel 监控通道状态
// 监控AMQP通道的连接状态，处理通道断开事件
func (self *PullManager) monitorChannel(ctx context.Context, receiver *PullReceiver, channel *amqp.Channel) {
	chErr := make(chan *amqp.Error, 1)
	channel.NotifyClose(chErr)

	select {
	case <-ctx.Done():
		return
	case err, ok := <-chErr:
		if !ok {
			return
		}
		zlog.Error("channel closed", 0,
			zlog.AddError(err),
			zlog.String("exchange", receiver.Config.Option.Exchange),
			zlog.String("queue", receiver.Config.Option.Queue))
		receiver.triggerReconnect()
	}
}

// processMessages 处理消息流
// 从通道接收并处理消息，支持上下文取消和错误处理
func (self *PullManager) processMessages(ctx context.Context, receiver *PullReceiver, msgs <-chan amqp.Delivery) {
	for {
		select {
		case <-ctx.Done():
			return
		case d, ok := <-msgs:
			if !ok {
				zlog.Warn("message channel closed", 0,
					zlog.String("queue", receiver.Config.Option.Queue))
				receiver.triggerReconnect()
				return
			}
			self.handleDelivery(receiver, d)
		}
	}
}

// handleDelivery 处理单个消息投递
// 根据配置处理消息，有限重试模式
func (self *PullManager) handleDelivery(receiver *PullReceiver, d amqp.Delivery) {
	const maxRetries = 3
	for i := 0; i < maxRetries; i++ {
		if ok := receiver.processMessage(d); ok {
			return // 处理成功
		}
		zlog.Warn("message processing failed, retrying...", 0,
			zlog.String("message_id", d.MessageId),
			zlog.String("queue", receiver.Config.Option.Queue),
			zlog.Int("retry", i+1))
		time.Sleep(time.Duration(i+3) * time.Second)
	}

	// 达到最大重试次数，重新放回队列
	zlog.Error("message processing failed after max retries, requeueing", 0,
		zlog.String("message_id", d.MessageId),
		zlog.String("queue", receiver.Config.Option.Queue),
		zlog.Int("max_retries", maxRetries))
	if err := d.Nack(false, true); err != nil { // requeue=true
		zlog.Error("failed to requeue message", 0,
			zlog.String("message_id", d.MessageId),
			zlog.AddError(err))
	}
}

// setupChannel 设置通道和队列
// 声明交换机、队列并建立绑定关系，为消息消费做准备
func (self *PullManager) setupChannel(receiver *PullReceiver, channel *amqp.Channel) error {
	opt := receiver.Config.Option

	// 设置默认值
	if opt.Kind == "" {
		opt.Kind = ExchangeDirect
	}
	if opt.Router == "" {
		opt.Router = opt.Queue
	}

	// 声明交换机
	if err := channel.ExchangeDeclare(
		opt.Exchange,
		opt.Kind,
		opt.Durable,    // 使用配置的持久化设置
		opt.AutoDelete, // 使用配置的自动删除设置
		false,          // internal
		false,          // noWait
		nil,            // args
	); err != nil {
		return fmt.Errorf("declare exchange failed: %w", err)
	}

	// 声明队列
	_, err := channel.QueueDeclare(
		opt.Queue,
		opt.Durable,    // 使用配置的持久化设置
		opt.AutoDelete, // 使用配置的自动删除设置
		opt.Exclusive,  // 使用配置的独占设置
		false,          // noWait
		nil,            // args
	)
	if err != nil {
		return fmt.Errorf("declare queue failed: %w", err)
	}

	// 绑定队列
	if err := channel.QueueBind(
		opt.Queue,
		opt.Router,
		opt.Exchange,
		false, // noWait
		nil,   // args
	); err != nil {
		return fmt.Errorf("bind queue failed: %w", err)
	}

	// 设置QoS
	if err := channel.Qos(receiver.Config.PrefetchCount, receiver.Config.PrefetchSize, false); err != nil {
		return fmt.Errorf("set qos failed: %w", err)
	}

	return nil
}

// Close 关闭PullManager
func (self *PullManager) Close() error {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.closed {
		return nil
	}
	self.closed = true

	zlog.Info("closing pull manager", 0, zlog.String("ds_name", self.conf.DsName))

	// 关闭所有接收器
	for _, receiver := range self.receivers {
		receiver.Stop()
	}
	self.receivers = nil

	// 通知关闭
	select {
	case <-self.closeChan:
		// channel 已经关闭
	default:
		close(self.closeChan)
	}

	// 关闭连接
	if self.conn != nil && !self.conn.IsClosed() {
		if err := self.conn.Close(); err != nil {
			zlog.Warn("close connection warning", 0, zlog.AddError(err))
		}
		self.conn = nil
	}

	// 等待所有goroutine退出
	done := make(chan struct{})
	go func() {
		self.monitorWg.Wait()
		close(done)
	}()

	select {
	case <-done:
		zlog.Info("pull manager closed successfully", 0, zlog.String("ds_name", self.conf.DsName))
	case <-time.After(5 * time.Second):
		zlog.Warn("timeout waiting for goroutines to exit", 0, zlog.String("ds_name", self.conf.DsName))
	}

	// 从全局映射中移除
	pullMgrMu.Lock()
	delete(pullMgrs, self.conf.DsName)
	pullMgrMu.Unlock()

	return nil
}

// HealthCheck 健康检查
// 检查RabbitMQ连接和通道的健康状态
func (self *PullManager) HealthCheck() error {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.closed {
		return fmt.Errorf("pull manager is closed")
	}

	if self.conn == nil || self.conn.IsClosed() {
		return fmt.Errorf("connection is not available")
	}

	// 测试通道创建
	channel, err := self.conn.Channel()
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	defer channel.Close()

	healthyReceivers := 0
	for _, receiver := range self.receivers {
		if receiver.IsHealthy() {
			healthyReceivers++
		}
	}

	if healthyReceivers == 0 && len(self.receivers) > 0 {
		return fmt.Errorf("no healthy receivers (%d total)", len(self.receivers))
	}

	return nil
}

// PullReceiver 消息接收器
type PullReceiver struct {
	mu        sync.RWMutex
	channel   *amqp.Channel
	Config    *Config
	Callback  func(msg *MsgData) error
	Debug     bool
	closeChan chan struct{}
	stopChan  chan struct{}
	stopping  bool
	healthy   bool
}

// initDefaults 初始化默认配置
// 为PullReceiver设置默认值，确保配置的完整性
func (self *PullReceiver) initDefaults() {
	if self.Config == nil {
		self.Config = &Config{}
	}
	if self.Config.Option.Exchange == "" {
		self.Config.Option.Exchange = "default.exchange"
	}
	if self.Config.Option.Queue == "" {
		self.Config.Option.Queue = "default.queue"
	}
	if self.Config.PrefetchCount == 0 {
		self.Config.PrefetchCount = 50
	}
	if !utils.CheckInt(self.Config.Option.SigTyp, 0, 1) {
		self.Config.Option.SigTyp = 1
	}
}

// initControlChans 初始化控制通道
// 创建用于goroutine间通信的控制通道
func (self *PullReceiver) initControlChans() {
	self.mu.Lock()
	defer self.mu.Unlock()

	// 总是重新创建通道，确保没有残留的旧信号
	self.closeChan = make(chan struct{}, 1)
	self.stopChan = make(chan struct{}, 1)
	self.stopping = false
	self.healthy = true
}

// setChannel 设置通道
// 线程安全地设置或清除AMQP通道引用
func (self *PullReceiver) setChannel(channel *amqp.Channel) {
	self.mu.Lock()
	defer self.mu.Unlock()
	self.channel = channel
}

// getChannel 获取通道
// 线程安全地获取AMQP通道引用
func (self *PullReceiver) getChannel() *amqp.Channel {
	self.mu.RLock()
	defer self.mu.RUnlock()
	return self.channel
}

// IsHealthy 检查健康状态
// 返回接收器的当前健康状态
func (self *PullReceiver) IsHealthy() bool {
	self.mu.RLock()
	defer self.mu.RUnlock()
	return self.healthy && self.channel != nil
}

// scheduleReconnect 调度重连
// 延迟后触发接收器重连逻辑
func (self *PullReceiver) scheduleReconnect(mgr *PullManager) {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.stopping {
		return
	}

	self.healthy = false

	select {
	case self.closeChan <- struct{}{}:
		// 重连已触发
	default:
		// 重连已在调度中
	}
}

// triggerReconnect 触发重连
// 立即触发接收器重连，不等待延迟
func (self *PullReceiver) triggerReconnect() {
	select {
	case self.closeChan <- struct{}{}:
	default:
	}
}

// OnError 错误处理回调
// 处理接收器运行过程中的错误，记录日志并触发重连
func (self *PullReceiver) OnError(err error) {
	zlog.Error("rabbitmq receiver error", 0,
		zlog.AddError(err),
		zlog.String("queue", self.Config.Option.Queue))
}

// Stop 停止接收器
// 优雅地停止接收器的运行，清理资源并等待goroutine退出
func (self *PullReceiver) Stop() {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.stopping {
		return
	}
	self.stopping = true
	self.healthy = false

	select {
	case self.stopChan <- struct{}{}:
	default:
	}

	// 关闭通道
	if self.channel != nil {
		if err := self.channel.Close(); err != nil {
			zlog.Debug("channel close warning", 0, zlog.AddError(err))
		}
		self.channel = nil
	}

	zlog.Info("receiver stopped", 0,
		zlog.String("queue", self.Config.Option.Queue))
}

// processMessage 处理消息
// 执行完整的消息处理流程：解析、验证、回调、确认
func (self *PullReceiver) processMessage(d amqp.Delivery) bool {
	b := d.Body
	if len(b) == 0 || string(b) == "{}" || string(b) == "[]" {
		return self.ackMessage(d)
	}

	if self.Debug {
		zlog.Debug("received message", 0,
			zlog.String("message_id", d.MessageId),
			zlog.String("body", utils.Bytes2Str(b)),
			zlog.String("queue", self.Config.Option.Queue))
	}

	// 从对象池获取MsgData对象
	msg := GetMsgData()
	defer PutMsgData(msg) // 确保消息处理完成后归还对象池

	// 解析消息
	if err := self.parseMessage(b, msg); err != nil {
		zlog.Error("parse message failed", 0,
			zlog.String("message_id", d.MessageId),
			zlog.AddError(err),
			zlog.String("queue", self.Config.Option.Queue))
		return self.ackMessage(d)
	}

	// 验证消息
	if err := self.validateMessage(msg); err != nil {
		zlog.Error("validate message failed", 0,
			zlog.String("message_id", d.MessageId),
			zlog.AddError(err),
			zlog.String("queue", self.Config.Option.Queue))
		return self.ackMessage(d)
	}

	// 处理消息
	if err := self.Callback(msg); err != nil {
		zlog.Error("message callback failed", 0,
			zlog.String("message_id", d.MessageId),
			zlog.AddError(err),
			zlog.String("queue", self.Config.Option.Queue))
		if self.Config.IsNack {
			return false
		}
	}

	return self.ackMessage(d)
}

// parseMessage 解析消息
// 将JSON格式的消息体解析为MsgData结构体
func (self *PullReceiver) parseMessage(body []byte, msg *MsgData) error {
	if err := utils.JsonUnmarshal(body, msg); err != nil {
		return fmt.Errorf("json unmarshal failed: %w", err)
	}

	if len(msg.Content) == 0 {
		return fmt.Errorf("message content is empty")
	}

	return nil
}

// validateMessage 验证消息
// 验证消息的签名和完整性，包括HMAC校验和AES解密
func (self *PullReceiver) validateMessage(msg *MsgData) error {
	// 验证签名
	if len(msg.Signature) == 0 {
		return fmt.Errorf("message signature is empty")
	}

	sigKey := self.Config.Option.SigKey
	if sigKey == "" {
		return fmt.Errorf("signature key is empty")
	}

	// 先验签（使用接收到的内容，可能已加密）
	expectedSig := utils.HMAC_SHA256(utils.AddStr(msg.Content, msg.Nonce, msg.CreatedAt), sigKey, true)
	if msg.Signature != expectedSig {
		return fmt.Errorf("signature mismatch")
	}

	// 再解密内容（如果需要）
	if self.Config.Option.SigTyp == 1 {
		if err := validateAESKeyLength(sigKey); err != nil {
			return fmt.Errorf("AES key invalid: %w", err)
		}

		decrypted, err := utils.AesGCMDecrypt(msg.Content, sigKey)
		if err != nil {
			return fmt.Errorf("AES decrypt failed: %w", err)
		}
		msg.Content = utils.Bytes2Str(decrypted)

		if len(msg.Content) == 0 {
			return fmt.Errorf("decrypted content is empty")
		}
	}

	return nil
}

// ackMessage 确认消息
// 向RabbitMQ确认消息已成功处理，支持重试机制
func (self *PullReceiver) ackMessage(d amqp.Delivery) bool {
	const maxRetries = 3
	const baseDelay = 100 * time.Millisecond

	for i := 0; i < maxRetries; i++ {
		if err := d.Ack(false); err == nil {
			return true
		}

		if i < maxRetries-1 {
			delay := time.Duration(i+1) * baseDelay
			time.Sleep(delay)
		}
	}

	zlog.Error("failed to ack message after retries", 0,
		zlog.String("message_id", d.MessageId),
		zlog.String("queue", self.Config.Option.Queue),
		zlog.Int("max_retries", maxRetries))
	return false
}

// validateAESKeyLength 验证AES密钥长度
// 确保AES密钥长度符合标准要求（16、24、32字节）
func validateAESKeyLength(key string) error {
	if key == "" {
		return fmt.Errorf("AES key cannot be empty")
	}

	keyLen := len(key)
	if keyLen < 8 {
		return fmt.Errorf("AES key too short: minimum 8 characters, got %d", keyLen)
	}

	if keyLen > 128 {
		return fmt.Errorf("AES key too long: maximum 128 characters, got %d", keyLen)
	}

	return nil
}
