package rabbitmq

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	DIC "github.com/godaddy-x/freego/common"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/zlog"
	"github.com/streadway/amqp"
)

// PublishError 发布错误类型，提供错误分类和重试建议
type PublishError struct {
	Code      string // 错误代码
	Message   string // 错误消息
	Retryable bool   // 是否可以重试
}

func (e *PublishError) Error() string {
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// classifyError 分类错误，返回结构化的错误信息
func classifyError(err error) *PublishError {
	if err == nil {
		return nil
	}

	errStr := err.Error()

	// 连接相关错误
	switch {
	case strings.Contains(errStr, "channel is not available"),
		strings.Contains(errStr, "connection closed"),
		strings.Contains(errStr, "connection is not available"),
		strings.Contains(errStr, "channel closed"),
		strings.Contains(errStr, "connection reset"),
		strings.Contains(errStr, "broken pipe"):
		return &PublishError{
			Code:      "CONNECTION_ERROR",
			Message:   errStr,
			Retryable: true,
		}

	// 超时相关错误
	case strings.Contains(errStr, "timeout"),
		strings.Contains(errStr, "deadline exceeded"),
		strings.Contains(errStr, "context canceled"):
		return &PublishError{
			Code:      "TIMEOUT_ERROR",
			Message:   errStr,
			Retryable: true,
		}

	// 认证相关错误
	case strings.Contains(errStr, "access refused"),
		strings.Contains(errStr, "not authorized"),
		strings.Contains(errStr, "authentication failure"):
		return &PublishError{
			Code:      "AUTHENTICATION_ERROR",
			Message:   errStr,
			Retryable: false,
		}

	// 资源相关错误
	case strings.Contains(errStr, "resource limit exceeded"),
		strings.Contains(errStr, "out of memory"),
		strings.Contains(errStr, "too many channels"):
		return &PublishError{
			Code:      "RESOURCE_ERROR",
			Message:   errStr,
			Retryable: true,
		}

	// 参数相关错误
	case strings.Contains(errStr, "invalid argument"),
		strings.Contains(errStr, "precondition failed"),
		strings.Contains(errStr, "exchange not found"),
		strings.Contains(errStr, "queue not found"):
		return &PublishError{
			Code:      "PARAMETER_ERROR",
			Message:   errStr,
			Retryable: false,
		}

	// 服务器内部错误
	case strings.Contains(errStr, "internal error"),
		strings.Contains(errStr, "server error"),
		strings.Contains(errStr, "unexpected server response"):
		return &PublishError{
			Code:      "SERVER_ERROR",
			Message:   errStr,
			Retryable: true,
		}

	// 默认未知错误
	default:
		return &PublishError{
			Code:      "UNKNOWN_ERROR",
			Message:   errStr,
			Retryable: false,
		}
	}
}

// wrapPublishError 统一错误包装
func wrapPublishError(err error, code, message string, retryable bool) error {
	if err == nil {
		return nil
	}

	if pe, ok := err.(*PublishError); ok {
		return pe
	}

	return &PublishError{
		Code:      code,
		Message:   message + ": " + err.Error(),
		Retryable: retryable,
	}
}

var (
	publishMgrs = make(map[string]*PublishManager)
	mgrMu       sync.RWMutex // 保护publishMgrs的并发访问
)

// PublishManager 发布者管理器（管理连接和通道池）
type PublishManager struct {
	mu              sync.RWMutex          // 保护conn和channels的读写
	conf            AmqpConfig            // 连接配置
	conn            *amqp.Connection      // AMQP连接
	channels        map[string]*PublishMQ // 通道池（key：exchange+router+queue）
	closeChan       chan struct{}         // 关闭通知通道
	closed          bool                  // 关闭状态标记（防止重复关闭和关闭后的操作）
	closeOnce       sync.Once             // 确保只关闭一次
	initialized     bool                  // 初始化状态标记（确保Connect已成功调用）
	semaphore       chan struct{}         // 信号量控制通道并发数量，避免资源耗尽
	monitorWg       sync.WaitGroup        // 监控goroutine等待组
	channelMonitors sync.Map              // 通道监控器管理
	rebuildCtx      context.Context       // 重建上下文
	rebuildCancel   context.CancelFunc    // 重建取消函数
}

// PublishMQ 单个通道的发布实例
type PublishMQ struct {
	mu          sync.Mutex    // 保护通道操作（线程安全）
	ready       bool          // 通道是否就绪
	readyCond   *sync.Cond    // 通道就绪条件变量（优化等待效率）
	option      *Option       // 队列配置
	channel     *amqp.Channel // AMQP通道
	queue       *amqp.Queue   // 队列信息
	closeChan   chan struct{} // 通道关闭通知
	closed      bool          // 关闭状态标记（防止关闭后的操作）
	monitorStop chan struct{} // 监控停止信号
	rebuilding  bool          // 重建状态标记
}

// 初始化默认配置（内部使用）
func (conf *AmqpConfig) setDefaults() {
	if conf.Port <= 0 {
		conf.Port = 5672
	}
	if conf.Vhost == "" {
		conf.Vhost = "/"
	}
	if conf.Heartbeat <= 0 {
		conf.Heartbeat = 10 * time.Second
	}
	if conf.ConnectionTimeout <= 0 {
		conf.ConnectionTimeout = 30 * time.Second
	}
	if conf.ChannelMax < 0 {
		conf.ChannelMax = 0
	}
	if conf.FrameSize < 0 {
		conf.FrameSize = 0
	}
}

// 验证AmqpConfig配置有效性
func (conf *AmqpConfig) Validate() error {
	if conf.Host == "" {
		return utils.Error("rabbitmq host is required")
	}
	if conf.Username == "" {
		return utils.Error("rabbitmq username is required")
	}
	if conf.Password == "" {
		return utils.Error("rabbitmq password is required")
	}
	if conf.Port < 1 || conf.Port > 65535 {
		return utils.Error("rabbitmq port must be between 1 and 65535")
	}
	return nil
}

// 验证Option配置有效性，并设置默认值
func (opt *Option) Validate() error {
	if opt.Exchange == "" {
		return utils.Error("exchange is required in option")
	}
	if opt.Queue == "" {
		return utils.Error("queue is required in option")
	}
	if opt.SigTyp < 0 || opt.SigTyp > 1 {
		return utils.Error("invalid SigTyp: must be 0 (plain) or 1 (AES)")
	}

	// 设置默认交换机类型
	if opt.Kind == "" {
		opt.Kind = ExchangeDirect
	}

	// 设置默认Confirm超时时间
	if opt.ConfirmTimeout <= 0 {
		opt.ConfirmTimeout = 30 * time.Second
	}

	// 验证交换机类型
	switch opt.Kind {
	case ExchangeDirect, ExchangeTopic, ExchangeHeaders, ExchangeFanout:
	default:
		return utils.Error("invalid exchange kind: ", opt.Kind, ". Valid values are: direct, topic, headers, fanout")
	}
	return nil
}

// NewPublishManager 创建发布者管理器（单例模式，双重检查锁定）
func NewPublishManager(conf AmqpConfig) (*PublishManager, error) {
	if err := conf.Validate(); err != nil {
		return nil, wrapPublishError(err, "VALIDATION_ERROR", "config validation failed", false)
	}
	conf.setDefaults()

	// 第一重检查（读锁）
	mgrMu.RLock()
	if existMgr, ok := publishMgrs[conf.DsName]; ok {
		mgrMu.RUnlock()
		return existMgr, nil
	}
	mgrMu.RUnlock()

	// 创建管理器
	maxConcurrentCreates := 10
	if conf.ChannelMax > 0 && conf.ChannelMax < 50 {
		maxConcurrentCreates = conf.ChannelMax / 5
		if maxConcurrentCreates < 2 {
			maxConcurrentCreates = 2
		}
	} else if conf.ChannelMax >= 50 {
		maxConcurrentCreates = 20
	}

	rebuildCtx, rebuildCancel := context.WithCancel(context.Background())
	mgr := &PublishManager{
		conf:          conf,
		channels:      make(map[string]*PublishMQ),
		closeChan:     make(chan struct{}),
		semaphore:     make(chan struct{}, maxConcurrentCreates),
		rebuildCtx:    rebuildCtx,
		rebuildCancel: rebuildCancel,
	}

	// 第二重检查（写锁）
	mgrMu.Lock()
	defer mgrMu.Unlock()

	if existMgr, ok := publishMgrs[conf.DsName]; ok {
		// 如果已经存在，直接返回已存在的实例
		zlog.Info("returning existing publish manager", 0, zlog.String("ds_name", conf.DsName))
		rebuildCancel()
		return existMgr, nil
	}

	// 建立初始连接（在写锁保护下）
	if _, err := mgr.Connect(); err != nil {
		rebuildCancel()
		return nil, wrapPublishError(err, "INIT_ERROR", "init connection failed", false)
	}

	publishMgrs[conf.DsName] = mgr
	zlog.Info("rabbitmq publish manager started successfully", 0, zlog.String("ds_name", conf.DsName))
	return mgr, nil
}

// GetPublishManager 获取已创建的发布者管理器
func GetPublishManager(dsName ...string) (*PublishManager, error) {
	ds := DIC.MASTER
	if len(dsName) > 0 && dsName[0] != "" {
		ds = dsName[0]
	}

	mgrMu.RLock()
	mgr, ok := publishMgrs[ds]
	mgrMu.RUnlock()

	if !ok {
		return nil, &PublishError{
			Code:      "MANAGER_NOT_FOUND",
			Message:   "publish manager not found for dsName: " + ds,
			Retryable: false,
		}
	}
	return mgr, nil
}

// Connect 建立/重建RabbitMQ连接
func (m *PublishManager) Connect() (*amqp.Connection, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 关闭现有连接
	if m.conn != nil && !m.conn.IsClosed() {
		if err := m.conn.Close(); err != nil {
			zlog.Warn("close old connection warning", 0, zlog.AddError(err), zlog.String("ds_name", m.conf.DsName))
		}
	}

	// 构建AMQP URI
	username := url.QueryEscape(m.conf.Username)
	password := url.QueryEscape(m.conf.Password)
	vhost := url.QueryEscape(m.conf.Vhost)
	amqpURI := fmt.Sprintf("amqp://%s:%s@%s:%d/%s",
		username, password, m.conf.Host, m.conf.Port, vhost)

	// 配置连接参数
	amqpConfig := amqp.Config{
		Heartbeat:  m.conf.Heartbeat,
		ChannelMax: m.conf.ChannelMax,
		FrameSize:  m.conf.FrameSize,
		Properties: amqp.Table{
			"connection_name": m.conf.DsName,
		},
		Dial: func(network, addr string) (net.Conn, error) {
			return net.DialTimeout(network, addr, m.conf.ConnectionTimeout)
		},
	}

	// 建立连接
	conn, err := amqp.DialConfig(amqpURI, amqpConfig)
	if err != nil {
		return nil, wrapPublishError(err, "CONNECTION_FAILED",
			fmt.Sprintf("failed to connect to RabbitMQ at %s:%d (vhost: %s)", m.conf.Host, m.conf.Port, m.conf.Vhost), true)
	}

	m.conn = conn
	zlog.Info("rabbitmq connection established", 0, zlog.String("ds_name", m.conf.DsName), zlog.String("uri", amqpURI))

	// 启动连接监控
	m.monitorWg.Add(1)
	go m.monitorConnection()

	m.initialized = true
	return conn, nil
}

// monitorConnection 监听连接状态，异常时自动重连
func (m *PublishManager) monitorConnection() {
	defer m.monitorWg.Done()
	defer func() {
		if r := recover(); r != nil {
			zlog.Error("monitorConnection panicked", 0,
				zlog.Any("recover", r),
				zlog.String("ds_name", m.conf.DsName))
		}
	}()

	m.mu.RLock()
	if m.conn == nil || m.closed {
		m.mu.RUnlock()
		return
	}

	conn := m.conn
	closeChan := conn.NotifyClose(make(chan *amqp.Error, 1))
	m.mu.RUnlock()

	defer func() {
		// 安全关闭监控通道
		if closeChan != nil {
			select {
			case _, ok := <-closeChan:
				if !ok {
					return
				}
			default:
			}
		}
	}()

	select {
	case <-m.closeChan:
		// 主动关闭，不重连
		return
	case err, ok := <-closeChan:
		if !ok {
			return
		}

		// 验证连接是否仍然是我们在监控的那个
		m.mu.RLock()
		isSameConnection := (m.conn == conn)
		m.mu.RUnlock()

		if !isSameConnection {
			zlog.Debug("monitorConnection: connection has been replaced, ignoring close event", 0,
				zlog.String("ds_name", m.conf.DsName))
			return
		}

		if err != nil {
			zlog.Error("rabbitmq connection closed unexpectedly", 0,
				zlog.AddError(err),
				zlog.String("ds_name", m.conf.DsName),
				zlog.Int("code", err.Code),
				zlog.String("reason", err.Reason))
		} else {
			zlog.Error("rabbitmq connection closed with nil error", 0,
				zlog.String("ds_name", m.conf.DsName))
		}

		// 指数退避重连
		const maxRetries = 10
		baseDelay := 500 * time.Millisecond

		for i := 0; i < maxRetries; i++ {
			select {
			case <-m.closeChan:
				return
			default:
			}

			delay := time.Duration(int64(baseDelay) * (1 << uint(i)))
			if delay > 10*time.Second {
				delay = 10 * time.Second
			}

			zlog.Info("waiting before reconnect attempt", 0,
				zlog.Duration("delay", delay),
				zlog.Int("attempt", i+1),
				zlog.String("ds_name", m.conf.DsName))

			time.Sleep(delay)

			if _, err := m.Connect(); err == nil {
				zlog.Info("reconnection successful", 0, zlog.String("ds_name", m.conf.DsName))
				// 连接重建成功，重建所有通道
				m.rebuildChannels()
				return
			}
			zlog.Warn("reconnect failed, retry later", 0,
				zlog.Int("retry_count", i+1), zlog.String("ds_name", m.conf.DsName))
		}

		zlog.Error("max reconnect retries exceeded", 0, zlog.String("ds_name", m.conf.DsName))
	}
}

// rebuildChannels 重建所有通道（连接重连后）- 修复竞态条件版本
func (m *PublishManager) rebuildChannels() {
	m.mu.Lock()
	defer m.mu.Unlock()

	zlog.Info("starting channel rebuild", 0, zlog.Int("channel_count", len(m.channels)), zlog.String("ds_name", m.conf.DsName))

	for key, pub := range m.channels {
		pub.mu.Lock()
		if pub.closed {
			pub.mu.Unlock()
			continue
		}

		// 标记重建状态
		pub.ready = false
		pub.rebuilding = true
		oldChannel := pub.channel
		pub.channel = nil
		pub.mu.Unlock()

		// 关闭旧通道（在锁外）
		if oldChannel != nil {
			_ = oldChannel.Close()
		}

		// 使用安全的重建方式
		m.monitorWg.Add(1)
		go m.rebuildChannelSafe(key)
	}
}

// rebuildChannelSafe 安全重建单个通道
func (m *PublishManager) rebuildChannelSafe(chanKey string) {
	defer m.monitorWg.Done()

	// 获取通道实例
	m.mu.RLock()
	pub, exists := m.channels[chanKey]
	m.mu.RUnlock()

	if !exists {
		return
	}

	const maxRetries = 3
	for i := 0; i < maxRetries; i++ {
		select {
		case <-m.closeChan:
			return
		case <-m.rebuildCtx.Done():
			return
		case <-pub.closeChan:
			return
		default:
		}

		if err := m.rebuildPublishMQ(pub); err != nil {
			zlog.Error("rebuild channel failed, retry later", 0, zlog.AddError(err),
				zlog.String("exchange", pub.option.Exchange), zlog.String("queue", pub.option.Queue),
				zlog.String("key", chanKey), zlog.Int("retry", i+1))
			if i < maxRetries-1 {
				time.Sleep(time.Duration(i+1) * 500 * time.Millisecond)
			}
		} else {
			zlog.Info("channel rebuild successful", 0,
				zlog.String("exchange", pub.option.Exchange), zlog.String("queue", pub.option.Queue))
			break
		}
	}
}

// rebuildPublishMQ 重建单个PublishMQ通道
func (m *PublishManager) rebuildPublishMQ(pub *PublishMQ) error {
	// 检查关闭状态（不持有任何锁）
	select {
	case <-m.closeChan:
		return &PublishError{Code: "MANAGER_CLOSED", Message: "publish manager is closed", Retryable: false}
	case <-pub.closeChan:
		return &PublishError{Code: "CHANNEL_CLOSED", Message: "publish channel is closed", Retryable: false}
	default:
	}

	pub.mu.Lock()
	defer pub.mu.Unlock()

	// 再次检查关闭状态（获取锁后）
	select {
	case <-pub.closeChan:
		return &PublishError{Code: "CHANNEL_CLOSED", Message: "publish channel is closed", Retryable: false}
	default:
	}

	// 获取新通道
	channel, err := m.getChannel()
	if err != nil {
		return wrapPublishError(err, "CHANNEL_CREATION_FAILED", "get channel failed", true)
	}
	pub.channel = channel

	// 重新声明交换机和队列
	if err := pub.prepareExchange(); err != nil {
		return wrapPublishError(err, "EXCHANGE_PREPARE_FAILED", "prepare exchange failed", true)
	}
	if err := pub.prepareQueue(); err != nil {
		return wrapPublishError(err, "QUEUE_PREPARE_FAILED", "prepare queue failed", true)
	}

	pub.ready = true
	pub.rebuilding = false
	pub.readyCond.Broadcast()

	zlog.Info("channel rebuilt successfully", 0,
		zlog.String("exchange", pub.option.Exchange), zlog.String("queue", pub.option.Queue))
	return nil
}

// getChannel 创建新通道（带重试）
func (m *PublishManager) getChannel() (*amqp.Channel, error) {
	m.mu.RLock()
	select {
	case <-m.closeChan:
		m.mu.RUnlock()
		return nil, &PublishError{Code: "MANAGER_CLOSED", Message: "publish manager is closed", Retryable: false}
	default:
	}

	conn := m.conn
	closed := m.closed
	m.mu.RUnlock()

	if closed {
		return nil, &PublishError{Code: "MANAGER_CLOSED", Message: "publish manager is closed", Retryable: false}
	}

	if conn == nil || conn.IsClosed() {
		if _, err := m.Connect(); err != nil {
			return nil, wrapPublishError(err, "CONNECTION_FAILED", "reconnect failed", true)
		}
		m.mu.RLock()
		conn = m.conn
		m.mu.RUnlock()
	}

	const maxRetries = 3
	for i := 0; i < maxRetries; i++ {
		channel, err := conn.Channel()
		if err == nil {
			if err := channel.Qos(1, 0, false); err != nil {
				zlog.Warn("set channel QoS warning", 0, zlog.AddError(err))
			}
			return channel, nil
		}

		zlog.Warn("failed to create channel, retrying", 0, zlog.AddError(err), zlog.Int("retry", i+1))
		time.Sleep(time.Duration(i+1) * 500 * time.Millisecond)
	}

	return nil, &PublishError{
		Code:      "CHANNEL_CREATION_FAILED",
		Message:   "failed to create AMQP channel after 3 retries - connection may be unstable",
		Retryable: true,
	}
}

// initQueue 初始化队列和通道（双重检查锁定）
func (m *PublishManager) initQueue(data *MsgData) (*PublishMQ, error) {
	// 补全Option默认值
	opt := &data.Option
	if opt.Router == "" {
		opt.Router = opt.Queue
	}
	if opt.SigKey == "" {
		opt.SigKey = m.conf.SecretKey
	}
	if opt.SigKey == "" {
		return nil, &PublishError{
			Code:      "SIGNATURE_KEY_REQUIRED",
			Message:   "signature key is required (global or per-message)",
			Retryable: false,
		}
	}
	if err := opt.Validate(); err != nil {
		return nil, wrapPublishError(err, "OPTION_VALIDATION_FAILED", "option validation failed", false)
	}

	// 生成通道唯一键（包含发布模式以避免模式冲突）
	modeStr := "confirm"
	if opt.UseTransaction {
		modeStr = "transaction"
	}
	chanKey := utils.AddStr(opt.Exchange, opt.Router, opt.Queue, modeStr)

	// 第一重检查（读锁）
	m.mu.RLock()
	pub, ok := m.channels[chanKey]
	m.mu.RUnlock()
	if ok {
		return pub, nil
	}

	// 第二重检查（写锁）
	m.mu.Lock()
	pub, ok = m.channels[chanKey]
	if ok {
		m.mu.Unlock()
		return pub, nil
	}
	m.mu.Unlock()

	// 获取信号量许可
	acquired := false
	defer func() {
		if acquired {
			<-m.semaphore
		}
	}()

	select {
	case m.semaphore <- struct{}{}:
		acquired = true
	case <-m.closeChan:
		return nil, &PublishError{Code: "MANAGER_CLOSED", Message: "publish manager is closed", Retryable: false}
	case <-time.After(5 * time.Second):
		return nil, &PublishError{
			Code:      "SEMAPHORE_TIMEOUT",
			Message:   "channel creation timeout, too many concurrent creations",
			Retryable: true,
		}
	}

	// 第三次检查：在获取信号量后再次检查
	m.mu.RLock()
	if pub, ok := m.channels[chanKey]; ok {
		m.mu.RUnlock()
		return pub, nil
	}
	m.mu.RUnlock()

	// 创建通道实例
	channel, err := m.getChannel()
	if err != nil {
		return nil, err
	}

	// 重新获取写锁来存储通道
	m.mu.Lock()
	defer m.mu.Unlock()

	// 初始化PublishMQ
	pub = &PublishMQ{
		option:      opt,
		channel:     channel,
		closeChan:   make(chan struct{}),
		monitorStop: make(chan struct{}),
	}
	pub.readyCond = sync.NewCond(&pub.mu)

	// 准备交换机和队列
	if err := pub.prepareExchange(); err != nil {
		return nil, wrapPublishError(err, "EXCHANGE_PREPARE_FAILED", "prepare exchange failed", false)
	}
	if err := pub.prepareQueue(); err != nil {
		return nil, wrapPublishError(err, "QUEUE_PREPARE_FAILED", "prepare queue failed", false)
	}

	// 设置就绪状态
	pub.mu.Lock()
	pub.ready = true
	pub.readyCond.Broadcast()
	pub.mu.Unlock()

	m.channels[chanKey] = pub

	// 启动通道监控
	m.monitorWg.Add(1)
	go pub.monitorChannel(m)

	zlog.Info("new publish channel initialized", 0,
		zlog.String("exchange", opt.Exchange), zlog.String("queue", opt.Queue), zlog.String("router", opt.Router))
	return pub, nil
}

// prepareExchange 声明交换机
func (p *PublishMQ) prepareExchange() error {
	opt := p.option
	err := p.channel.ExchangeDeclare(
		opt.Exchange,
		opt.Kind,
		opt.Durable,
		opt.AutoDelete,
		false,
		false,
		nil,
	)
	if err != nil {
		return wrapPublishError(err, "EXCHANGE_DECLARE_FAILED", "declare exchange failed", false)
	}
	return nil
}

// prepareQueue 声明队列并绑定（支持死信队列）
func (p *PublishMQ) prepareQueue() error {
	opt := p.option
	args := amqp.Table{}

	// 配置死信队列
	if opt.DLXConfig != nil {
		args["x-dead-letter-exchange"] = opt.DLXConfig.DlxExchange
		args["x-dead-letter-routing-key"] = opt.DLXConfig.DlxRouter

		if err := p.channel.ExchangeDeclare(
			opt.DLXConfig.DlxExchange,
			ExchangeDirect,
			opt.Durable,
			opt.AutoDelete,
			false,
			false,
			nil,
		); err != nil {
			return wrapPublishError(err, "DLX_EXCHANGE_FAILED", "declare dlx exchange failed", false)
		}

		if _, err := p.channel.QueueDeclare(
			opt.DLXConfig.DlxQueue,
			opt.Durable,
			opt.AutoDelete,
			opt.Exclusive,
			false,
			nil,
		); err != nil {
			return wrapPublishError(err, "DLX_QUEUE_FAILED", "declare dlx queue failed", false)
		}

		if err := p.channel.QueueBind(
			opt.DLXConfig.DlxQueue,
			opt.DLXConfig.DlxRouter,
			opt.DLXConfig.DlxExchange,
			false,
			nil,
		); err != nil {
			return wrapPublishError(err, "DLX_BIND_FAILED", "bind dlx queue failed", false)
		}
	}

	// 声明主队列
	queue, err := p.channel.QueueDeclare(
		opt.Queue,
		opt.Durable,
		opt.AutoDelete,
		opt.Exclusive,
		false,
		args,
	)
	if err != nil {
		return wrapPublishError(err, "QUEUE_DECLARE_FAILED", "declare queue failed", false)
	}

	// 绑定队列到交换机
	if err := p.channel.QueueBind(
		queue.Name,
		opt.Router,
		opt.Exchange,
		false,
		nil,
	); err != nil {
		return wrapPublishError(err, "QUEUE_BIND_FAILED", "bind queue failed", false)
	}

	p.queue = &queue
	return nil
}

// monitorChannel 监听通道状态，异常时触发重建
func (p *PublishMQ) monitorChannel(m *PublishManager) {
	defer m.monitorWg.Done()
	defer func() {
		if r := recover(); r != nil {
			zlog.Error("monitorChannel panicked", 0,
				zlog.Any("recover", r),
				zlog.String("exchange", p.option.Exchange),
				zlog.String("queue", p.option.Queue))
		}
	}()

	p.mu.Lock()
	if p.channel == nil || p.closed {
		p.mu.Unlock()
		return
	}

	closeChan := make(chan *amqp.Error, 1)
	p.channel.NotifyClose(closeChan)
	p.mu.Unlock()

	defer func() {
		// 安全关闭监控通道
		select {
		case _, ok := <-closeChan:
			if !ok {
				return
			}
		default:
		}
	}()

	select {
	case <-p.monitorStop:
		// 主动停止监控
		return
	case <-p.closeChan:
		// 通道关闭
		return
	case err, ok := <-closeChan:
		if !ok {
			return
		}

		zlog.Error("publish channel closed", 0, zlog.AddError(err),
			zlog.String("exchange", p.option.Exchange), zlog.String("queue", p.option.Queue))

		// 标记通道未就绪
		p.mu.Lock()
		p.ready = false
		p.readyCond.Broadcast()
		p.mu.Unlock()

		// 触发重建
		m.monitorWg.Add(1)
		go func() {
			defer m.monitorWg.Done()
			const maxRetries = 5
			for i := 0; i < maxRetries; i++ {
				select {
				case <-m.closeChan:
					return
				case <-p.closeChan:
					return
				case <-p.monitorStop:
					return
				default:
				}

				if err := m.rebuildPublishMQ(p); err != nil {
					zlog.Error("rebuild channel failed, retry later", 0, zlog.AddError(err),
						zlog.String("exchange", p.option.Exchange), zlog.String("queue", p.option.Queue),
						zlog.Int("retry", i+1))
					if i < maxRetries-1 {
						time.Sleep(time.Duration(i+1) * time.Second)
					}
				} else {
					zlog.Info("channel rebuild successful", 0,
						zlog.String("exchange", p.option.Exchange), zlog.String("queue", p.option.Queue))
					break
				}
			}
		}()
	}
}

// waitReady 等待通道就绪（简化版本）
func (p *PublishMQ) waitReady(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// 快速检查
	if p.ready && !p.closed {
		return nil
	}

	// 设置超时检查
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for !p.ready && !p.closed {
		select {
		case <-ctx.Done():
			return wrapPublishError(ctx.Err(), "CONTEXT_CANCELED", "wait for channel ready timeout", true)
		case <-ticker.C:
			// 定期检查状态
			if p.ready && !p.closed {
				return nil
			}
		}
	}

	if p.closed {
		return &PublishError{
			Code:      "CHANNEL_CLOSED",
			Message:   "channel is closed",
			Retryable: false,
		}
	}
	return nil
}

// Publish 发布单条消息（简化接口）
func (m *PublishManager) Publish(ctx context.Context, exchange, queue string, dataType int64, content string, opts ...PublishOption) error {
	msg := &MsgData{
		Option: Option{
			Exchange: exchange,
			Queue:    queue,
			Durable:  true,
		},
		Type:    dataType,
		Content: content,
	}
	for _, opt := range opts {
		opt(msg)
	}
	return m.PublishMsgData(ctx, msg)
}

// PublishOption 发布消息可选配置
type PublishOption func(*MsgData)

// WithSigType 设置签名类型
func WithSigType(sigType int) PublishOption {
	return func(msg *MsgData) {
		msg.Option.SigTyp = sigType
	}
}

// WithRouter 设置路由键
func WithRouter(router string) PublishOption {
	return func(msg *MsgData) {
		msg.Option.Router = router
	}
}

// WithDelay 设置延时时间（秒）
func WithDelay(seconds int64) PublishOption {
	return func(msg *MsgData) {
		msg.Delay = seconds
	}
}

// WithDLXConfig 设置死信队列配置
func WithDLXConfig(dlx *DLXConfig) PublishOption {
	return func(msg *MsgData) {
		msg.Option.DLXConfig = dlx
	}
}

// WithUseTransaction 设置是否使用事务模式批量发布
func WithUseTransaction(useTx bool) PublishOption {
	return func(msg *MsgData) {
		msg.Option.UseTransaction = useTx
	}
}

// WithConfirmTimeout 设置Confirm模式确认超时时间
func WithConfirmTimeout(timeout time.Duration) PublishOption {
	return func(msg *MsgData) {
		msg.Option.ConfirmTimeout = timeout
	}
}

// WithDurable 设置持久化选项
func WithDurable(durable bool) PublishOption {
	return func(msg *MsgData) {
		msg.Option.Durable = durable
	}
}

// BatchPublishWithOptions 批量发布消息（支持自定义选项）
func (m *PublishManager) BatchPublishWithOptions(ctx context.Context, msgs []*MsgData, opts ...PublishOption) error {
	for _, msg := range msgs {
		for _, opt := range opts {
			opt(msg)
		}
	}
	return m.BatchPublish(ctx, msgs)
}

// PublishMsgData 发布完整消息结构体（带重试机制）
func (m *PublishManager) PublishMsgData(ctx context.Context, data *MsgData) error {
	const maxRetries = 3
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		err := m.publishMsgDataOnce(ctx, data)
		if err == nil {
			return nil
		}

		lastErr = err
		classifiedErr := classifyError(err)

		if classifiedErr != nil {
			zlog.Warn("publish attempt failed", 0,
				zlog.AddError(err),
				zlog.String("error_code", classifiedErr.Code),
				zlog.Bool("retryable", classifiedErr.Retryable),
				zlog.Int("attempt", i+1),
				zlog.Int("max_retries", maxRetries))

			if classifiedErr.Retryable {
				select {
				case <-ctx.Done():
					return wrapPublishError(ctx.Err(), "CONTEXT_CANCELED", "publish canceled", false)
				default:
					retryDelay := time.Duration(i+1) * 100 * time.Millisecond
					time.Sleep(retryDelay)
					continue
				}
			}
		}
		break
	}

	if lastErr != nil {
		return wrapPublishError(lastErr, "PUBLISH_FAILED",
			fmt.Sprintf("publish failed after %d retries", maxRetries), false)
	}

	return &PublishError{
		Code:      "PUBLISH_FAILED",
		Message:   fmt.Sprintf("publish failed after %d retries", maxRetries),
		Retryable: false,
	}
}

// publishMsgDataOnce 单次发布消息的实现
func (m *PublishManager) publishMsgDataOnce(ctx context.Context, data *MsgData) error {
	select {
	case <-m.closeChan:
		return &PublishError{Code: "MANAGER_CLOSED", Message: "publish manager is closed", Retryable: false}
	default:
	}

	// 检查初始化状态
	m.mu.RLock()
	initialized := m.initialized
	closed := m.closed
	m.mu.RUnlock()

	if closed {
		return &PublishError{Code: "MANAGER_CLOSED", Message: "publish manager is closed", Retryable: false}
	}
	if !initialized {
		return &PublishError{Code: "MANAGER_NOT_INITIALIZED", Message: "publish manager not initialized", Retryable: false}
	}

	// 参数验证
	if data == nil {
		return &PublishError{Code: "INVALID_MESSAGE", Message: "msg data cannot be nil", Retryable: false}
	}
	if data.Option.Exchange == "" {
		return &PublishError{Code: "INVALID_EXCHANGE", Message: "exchange name cannot be empty", Retryable: false}
	}
	if data.Option.Queue == "" {
		return &PublishError{Code: "INVALID_QUEUE", Message: "queue name cannot be empty", Retryable: false}
	}
	if len(data.Content) == 0 {
		return &PublishError{Code: "INVALID_CONTENT", Message: "message content cannot be nil", Retryable: false}
	}

	// 初始化队列和通道
	pub, err := m.initQueue(data)
	if err != nil {
		return wrapPublishError(err, "CHANNEL_INIT_FAILED", "failed to initialize queue and channel", true)
	}

	// 等待通道就绪
	if err := pub.waitReady(ctx); err != nil {
		return wrapPublishError(err, "CHANNEL_NOT_READY",
			fmt.Sprintf("channel not ready for exchange '%s' queue '%s'", data.Option.Exchange, data.Option.Queue), true)
	}

	// 预处理消息
	if err := m.preprocessMsg(data); err != nil {
		return wrapPublishError(err, "MESSAGE_PREPROCESS_FAILED", "failed to preprocess message", false)
	}

	// 发送消息
	if err := pub.sendMessage(ctx, data); err != nil {
		return wrapPublishError(err, "SEND_MESSAGE_FAILED",
			fmt.Sprintf("failed to send message to exchange '%s' queue '%s'", data.Option.Exchange, data.Option.Queue), true)
	}

	return nil
}

// BatchPublish 批量发布消息
func (m *PublishManager) BatchPublish(ctx context.Context, msgs []*MsgData) error {
	select {
	case <-m.closeChan:
		return &PublishError{Code: "MANAGER_CLOSED", Message: "publish manager is closed", Retryable: false}
	default:
	}

	m.mu.RLock()
	initialized := m.initialized
	closed := m.closed
	m.mu.RUnlock()

	if closed {
		return &PublishError{Code: "MANAGER_CLOSED", Message: "publish manager is closed", Retryable: false}
	}
	if !initialized {
		return &PublishError{Code: "MANAGER_NOT_INITIALIZED", Message: "publish manager not initialized", Retryable: false}
	}

	if len(msgs) == 0 {
		return &PublishError{Code: "EMPTY_BATCH", Message: "batch messages cannot be empty", Retryable: false}
	}
	if msgs[0] == nil {
		return &PublishError{Code: "NIL_MESSAGE", Message: "first message in batch cannot be nil", Retryable: false}
	}

	firstOpt := msgs[0].Option
	for _, msg := range msgs {
		if msg.Option.Exchange != firstOpt.Exchange || msg.Option.Queue != firstOpt.Queue {
			return &PublishError{Code: "INCONSISTENT_BATCH", Message: "batch messages must have same exchange and queue", Retryable: false}
		}
		if len(msg.Content) == 0 {
			return &PublishError{Code: "INVALID_CONTENT", Message: "batch message content cannot be nil", Retryable: false}
		}
	}

	// 初始化队列和通道
	pub, err := m.initQueue(msgs[0])
	if err != nil {
		return err
	}

	// 等待通道就绪
	if err := pub.waitReady(ctx); err != nil {
		return wrapPublishError(err, "CHANNEL_NOT_READY", "channel not ready", true)
	}

	// 预处理所有消息
	for _, msg := range msgs {
		if err := m.preprocessMsg(msg); err != nil {
			return wrapPublishError(err, "BATCH_PREPROCESS_FAILED", "preprocess batch msg failed", false)
		}
	}

	// 根据配置选择发布模式
	if firstOpt.UseTransaction {
		return pub.batchSendWithTransaction(ctx, msgs)
	} else {
		return pub.batchSendWithConfirm(ctx, msgs)
	}
}

// preprocessMsg 预处理消息（序列化、加密、签名）
func (m *PublishManager) preprocessMsg(data *MsgData) error {
	opt := data.Option

	// 生成Nonce（防重放）
	if data.Nonce == "" {
		data.Nonce = utils.Base64Encode(utils.GetAesIVSecure())
	}

	// 序列化消息内容
	if len(data.Content) == 0 {
		return &PublishError{Code: "EMPTY_CONTENT", Message: "serialized content is empty", Retryable: false}
	}

	// AES密钥长度校验
	if opt.SigTyp == 1 {
		if err := m.validateAESKeyLength(opt.SigKey); err != nil {
			return wrapPublishError(err, "INVALID_AES_KEY", "AES key validation failed", false)
		}
	}

	// 加密（如果需要）
	if opt.SigTyp == 1 {
		encryptedContent, err := utils.AesGCMEncrypt(utils.Str2Bytes(data.Content), opt.SigKey)
		if err != nil {
			return wrapPublishError(err, "ENCRYPTION_FAILED", "AES encrypt failed", false)
		}
		data.Content = encryptedContent
	}

	// 签名（基于最终内容，可能是原文或密文）
	data.Signature = utils.HMAC_SHA256(utils.AddStr(data.Content, data.Nonce), opt.SigKey, true)

	// 清除密钥
	opt.SigKey = ""

	// 处理延时消息
	if data.Delay > 0 {
		data.Expiration = fmt.Sprintf("%d", data.Delay*1000)
	}

	return nil
}

// validateAESKeyLength 校验AES密钥长度是否符合要求
func (m *PublishManager) validateAESKeyLength(key string) error {
	if key == "" {
		return &PublishError{Code: "EMPTY_AES_KEY", Message: "AES key cannot be empty", Retryable: false}
	}

	keyLen := len(key)
	if keyLen < 8 {
		return &PublishError{
			Code:      "AES_KEY_TOO_SHORT",
			Message:   fmt.Sprintf("AES key is too short: minimum 8 characters recommended, got %d", keyLen),
			Retryable: false,
		}
	}

	if keyLen > 128 {
		return &PublishError{
			Code:      "AES_KEY_TOO_LONG",
			Message:   fmt.Sprintf("AES key is too long: maximum 128 characters allowed, got %d", keyLen),
			Retryable: false,
		}
	}

	if keyLen != 16 && keyLen != 24 && keyLen != 32 {
		zlog.Warn("AES key length is not standard AES size (16/24/32 bytes)", 0,
			zlog.Int("key_length", keyLen),
			zlog.String("recommendation", "use 32 characters for AES-256"))
	}

	return nil
}

// sendMessage 发送单条消息（线程安全）
func (p *PublishMQ) sendMessage(ctx context.Context, msg *MsgData) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.ready || p.channel == nil {
		return &PublishError{Code: "CHANNEL_UNAVAILABLE", Message: "channel is not available", Retryable: true}
	}

	body, err := utils.JsonMarshal(msg)
	if err != nil {
		return wrapPublishError(err, "MARSHAL_FAILED", "marshal msg failed", false)
	}

	publishing := amqp.Publishing{
		ContentType:   "application/json",
		Body:          body,
		DeliveryMode:  amqp.Persistent,
		Timestamp:     time.Now(),
		MessageId:     utils.NextSID(),
		CorrelationId: msg.Nonce,
		Priority:      msg.Priority,
		Expiration:    msg.Expiration,
	}

	done := make(chan error, 1)
	go func() {
		done <- p.channel.Publish(
			p.option.Exchange,
			p.option.Router,
			true,
			false,
			publishing,
		)
	}()

	select {
	case <-ctx.Done():
		return wrapPublishError(ctx.Err(), "PUBLISH_TIMEOUT", "publish timeout", true)
	case err := <-done:
		if err != nil {
			return wrapPublishError(err, "PUBLISH_FAILED", "publish failed", true)
		}
	}

	zlog.Info("msg published successfully", 0,
		zlog.String("msg_id", publishing.MessageId),
		zlog.String("exchange", p.option.Exchange),
		zlog.String("queue", p.option.Queue))
	return nil
}

// batchSendWithTransaction 批量发送消息（事务保证原子性）
func (p *PublishMQ) batchSendWithTransaction(ctx context.Context, msgs []*MsgData) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.ready || p.channel == nil {
		return &PublishError{Code: "CHANNEL_UNAVAILABLE", Message: "channel is not available", Retryable: true}
	}

	if err := p.channel.Tx(); err != nil {
		return wrapPublishError(err, "TX_START_FAILED", "start tx failed", true)
	}

	var rollbackErr error
	defer func() {
		if rollbackErr != nil {
			_ = p.channel.TxRollback()
		}
	}()

	for _, msg := range msgs {
		body, err := utils.JsonMarshal(msg)
		if err != nil {
			rollbackErr = err
			return wrapPublishError(err, "BATCH_MARSHAL_FAILED", "marshal batch msg failed", false)
		}

		publishing := amqp.Publishing{
			ContentType:   "application/json",
			Body:          body,
			DeliveryMode:  amqp.Persistent,
			Timestamp:     time.Now(),
			MessageId:     utils.NextSID(),
			CorrelationId: msg.Nonce,
			Priority:      msg.Priority,
			Expiration:    msg.Expiration,
		}

		if err := p.channel.Publish(
			p.option.Exchange,
			p.option.Router,
			true,
			false,
			publishing,
		); err != nil {
			rollbackErr = err
			return wrapPublishError(err, "BATCH_PUBLISH_FAILED", "batch publish failed", true)
		}
	}

	if err := p.channel.TxCommit(); err != nil {
		rollbackErr = err
		return wrapPublishError(err, "TX_COMMIT_FAILED", "tx commit failed", true)
	}

	zlog.Info("batch msg published with transaction successfully", 0,
		zlog.Int("count", len(msgs)),
		zlog.String("exchange", p.option.Exchange),
		zlog.String("queue", p.option.Queue))
	return nil
}

// batchSendWithConfirm 批量发送消息（Confirm模式，高性能但无原子性保证）
func (p *PublishMQ) batchSendWithConfirm(ctx context.Context, msgs []*MsgData) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.ready || p.channel == nil {
		return &PublishError{
			Code:      "CHANNEL_UNAVAILABLE",
			Message:   "channel is not available",
			Retryable: true,
		}
	}

	if err := p.channel.Confirm(false); err != nil {
		return &PublishError{
			Code:      "CONFIRM_MODE_ERROR",
			Message:   "enable confirm mode failed: " + err.Error(),
			Retryable: true,
		}
	}

	confirms := p.channel.NotifyPublish(make(chan amqp.Confirmation, len(msgs)))
	pendingMessages := make(map[uint64]*MsgData, len(msgs))
	var deliveryTag uint64

	for _, msg := range msgs {
		body, err := utils.JsonMarshal(msg)
		if err != nil {
			return &PublishError{
				Code:      "MARSHAL_ERROR",
				Message:   "marshal batch msg failed: " + err.Error(),
				Retryable: false,
			}
		}

		msgId := utils.NextSID()
		publishing := amqp.Publishing{
			ContentType:   "application/json",
			Body:          body,
			DeliveryMode:  amqp.Persistent,
			Timestamp:     time.Now(),
			MessageId:     msgId,
			CorrelationId: msg.Nonce,
			Priority:      msg.Priority,
			Expiration:    msg.Expiration,
		}

		err = p.channel.Publish(
			p.option.Exchange,
			p.option.Router,
			true,
			false,
			publishing,
		)
		if err != nil {
			return &PublishError{
				Code:      "PUBLISH_ERROR",
				Message:   "batch publish failed: " + err.Error(),
				Retryable: true,
			}
		}

		deliveryTag++
		pendingMessages[deliveryTag] = msg
	}

	confirmCtx, cancel := context.WithTimeout(ctx, p.option.ConfirmTimeout)
	defer cancel()

	confirmedCount := 0
	expectedConfirmations := len(msgs)

	for confirmedCount < expectedConfirmations {
		select {
		case confirm, ok := <-confirms:
			if !ok {
				remaining := expectedConfirmations - confirmedCount
				return &PublishError{
					Code:      "CONFIRM_CHANNEL_CLOSED",
					Message:   fmt.Sprintf("confirm channel closed unexpectedly, %d messages still pending", remaining),
					Retryable: true,
				}
			}

			if confirm.Ack {
				confirmedCount++
				delete(pendingMessages, confirm.DeliveryTag)
			} else {
				remaining := expectedConfirmations - confirmedCount
				return &PublishError{
					Code:      "MESSAGE_REJECTED",
					Message:   fmt.Sprintf("message not acknowledged by server, deliveryTag: %d - %d messages still pending", confirm.DeliveryTag, remaining),
					Retryable: true,
				}
			}

		case <-confirmCtx.Done():
			remaining := expectedConfirmations - confirmedCount
			return &PublishError{
				Code:      "CONFIRM_TIMEOUT",
				Message:   fmt.Sprintf("confirm timeout after %v, confirmed %d/%d messages (%d pending)", p.option.ConfirmTimeout, confirmedCount, expectedConfirmations, remaining),
				Retryable: true,
			}
		}
	}

	zlog.Info("batch msg published with confirm successfully", 0,
		zlog.Int("count", len(msgs)),
		zlog.String("exchange", p.option.Exchange),
		zlog.String("queue", p.option.Queue))
	return nil
}

// GetQueueStatus 获取队列状态
func (m *PublishManager) GetQueueStatus(ctx context.Context, exchange, queue, router string) (*QueueData, error) {
	msg := &MsgData{
		Option: Option{
			Exchange: exchange,
			Queue:    queue,
			Router:   router,
		},
	}
	pub, err := m.initQueue(msg)
	if err != nil {
		return nil, err
	}

	if err := pub.waitReady(ctx); err != nil {
		return nil, err
	}

	pub.mu.Lock()
	defer pub.mu.Unlock()

	if pub.queue == nil {
		return nil, &PublishError{
			Code:      "QUEUE_NOT_DECLARED",
			Message:   "queue not declared",
			Retryable: false,
		}
	}

	return &QueueData{
		Name:      pub.queue.Name,
		Consumers: pub.queue.Consumers,
		Messages:  pub.queue.Messages,
	}, nil
}

// Close 安全关闭发布者管理器
func (m *PublishManager) Close() error {
	var closeErr error
	m.closeOnce.Do(func() {
		zlog.Info("starting publish manager shutdown", 0, zlog.String("ds_name", m.conf.DsName))

		// 第一步：通知关闭
		close(m.closeChan)
		m.rebuildCancel()

		// 第二步：停止所有监控goroutine
		m.mu.Lock()
		m.closed = true

		// 通知所有通道停止监控
		for key, pub := range m.channels {
			pub.mu.Lock()
			pub.closed = true
			pub.ready = false

			select {
			case <-pub.closeChan:
			default:
				close(pub.closeChan)
			}

			select {
			case <-pub.monitorStop:
			default:
				close(pub.monitorStop)
			}

			pub.readyCond.Broadcast()

			if pub.channel != nil {
				if err := pub.channel.Close(); err != nil {
					zlog.Warn("close channel warning", 0, zlog.AddError(err),
						zlog.String("exchange", pub.option.Exchange),
						zlog.String("queue", pub.option.Queue))
				}
				pub.channel = nil
			}
			pub.mu.Unlock()
			delete(m.channels, key)
		}

		channelCount := len(m.channels)
		m.mu.Unlock()

		zlog.Info("closed all channels", 0,
			zlog.Int("channel_count", channelCount),
			zlog.String("ds_name", m.conf.DsName))

		// 第三步：等待所有监控goroutine退出
		done := make(chan struct{})
		go func() {
			m.monitorWg.Wait()
			close(done)
		}()

		select {
		case <-done:
			zlog.Debug("all monitor goroutines stopped", 0, zlog.String("ds_name", m.conf.DsName))
		case <-time.After(5 * time.Second):
			zlog.Warn("timeout waiting for monitor goroutines to stop", 0, zlog.String("ds_name", m.conf.DsName))
		}

		// 第四步：关闭连接
		if m.conn != nil {
			if !m.conn.IsClosed() {
				if err := m.conn.Close(); err != nil {
					zlog.Error("close connection failed", 0, zlog.AddError(err), zlog.String("ds_name", m.conf.DsName))
					closeErr = err
				}
			}
			m.conn = nil
		}

		// 从单例中移除
		mgrMu.Lock()
		delete(publishMgrs, m.conf.DsName)
		mgrMu.Unlock()

		zlog.Info("rabbitmq publish manager closed successfully", 0, zlog.String("ds_name", m.conf.DsName))
	})
	return closeErr
}

// HealthCheck 检查RabbitMQ连接健康状态
func (m *PublishManager) HealthCheck() error {
	select {
	case <-m.closeChan:
		return &PublishError{Code: "MANAGER_CLOSED", Message: "publish manager is closed", Retryable: false}
	default:
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.initialized {
		return &PublishError{Code: "MANAGER_NOT_INITIALIZED", Message: "publish manager not initialized", Retryable: false}
	}

	if m.conn == nil || m.conn.IsClosed() {
		return &PublishError{Code: "CONNECTION_UNAVAILABLE", Message: "rabbitmq connection is not available", Retryable: true}
	}

	channel, err := m.conn.Channel()
	if err != nil {
		return wrapPublishError(err, "HEALTH_CHECK_FAILED", "connection health check failed", true)
	}
	defer func() {
		if err := channel.Close(); err != nil {
			zlog.Debug("health check channel close warning", 0, zlog.AddError(err))
		}
	}()

	if len(m.channels) > 0 {
		healthyChannels := 0
		totalChannels := len(m.channels)

		for _, pub := range m.channels {
			pub.mu.Lock()
			if pub.ready && pub.channel != nil {
				healthyChannels++
			}
			pub.mu.Unlock()
		}

		if healthyChannels == 0 {
			return &PublishError{
				Code:      "NO_HEALTHY_CHANNELS",
				Message:   fmt.Sprintf("no healthy channels available (%d total)", totalChannels),
				Retryable: true,
			}
		}

		zlog.Debug("health check summary", 0,
			zlog.Int("healthy_channels", healthyChannels),
			zlog.Int("total_channels", totalChannels))
	}

	return nil
}
