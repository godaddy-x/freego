package node

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/fasthttp/websocket"
	fasthttpWs "github.com/fasthttp/websocket"
	rate "github.com/godaddy-x/freego/cache/limiter"
	"github.com/godaddy-x/freego/ex"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/utils/jwt"
	"github.com/valyala/fasthttp"
	"go.uber.org/zap"
)

// WebSocket服务器实现

// WebSocket专用常量
const (
	pingCmd         = "ws-health-check" // 心跳检测命令
	WS_MAX_BODY_LEN = 1024 * 1024       // 1MB
)

// 核心类型定义
type Handle func(ctx context.Context, connCtx *ConnectionContext, body []byte) (interface{}, error) // 业务处理函数，返回nil则不回复

// ConnectionContext 每个WebSocket连接的上下文，包含连接相关的所有信息
type ConnectionContext struct {
	Subject  *jwt.Subject
	JsonBody *JsonBody
	WsConn   *fasthttpWs.Conn
	DevConn  *DevConn
	Server   *WsServer
	Logger   *zap.Logger // 每个连接独立的logger，便于追踪
	ctx      context.Context
	cancel   context.CancelFunc
}

// ConnectionManager 连接管理器：线程安全的连接管理，支持广播、房间、过期清理
type ConnectionManager struct {
	mu        sync.RWMutex
	conns     map[string]map[string]*DevConn // subject -> deviceKey -> connection
	max       int                            // 最大连接数
	totalConn int32                          // 原子计数器：当前总连接数（性能优化）
}

// MessageHandler 消息处理器：统一处理消息校验、解码、路由
type MessageHandler struct {
	handle Handle
	debug  bool
}

// HeartbeatService 心跳服务：维护连接活性，清理过期连接
type HeartbeatService struct {
	interval time.Duration
	timeout  time.Duration
	manager  *ConnectionManager
	stopCh   chan struct{}
	running  bool
	mu       sync.Mutex
}

// DevConn 设备连接实体：存储单连接的核心信息
type DevConn struct {
	Sub    string
	Dev    string
	Life   int64            // 连接生命周期（时间戳）
	Last   int64            // 最后心跳时间（时间戳）
	Conn   *fasthttpWs.Conn // WebSocket连接
	sendMu sync.Mutex       // 发送消息互斥锁（避免并发写冲突）
	ctx    context.Context  // 用于取消该连接的相关goroutine
}

// WsServer WebSocket服务器核心结构体
type WsServer struct {
	Debug        bool
	server       *fasthttp.Server
	upgrader     *fasthttpWs.FastHTTPUpgrader
	routes       map[string]Handle // 路由映射：path -> 业务处理器
	routesMu     sync.RWMutex      // 保护routes的读写
	connManager  *ConnectionManager
	heartbeatSvc *HeartbeatService

	// 配置项
	ping         int           // 心跳间隔（秒）
	maxConn      int           // 最大连接数
	limiter      *rate.Limiter // 连接限流器
	globalCtx    context.Context
	globalCancel context.CancelFunc

	// 依赖组件
	logger          *zap.Logger
	errorHandler    *ErrorHandler
	configValidator *ConfigValidator
}

// ErrorHandler WebSocket错误处理器（统一错误处理）
type ErrorHandler struct {
	logger *zap.Logger
}

func (eh *ErrorHandler) handleConnectionError(connCtx *ConnectionContext, err error, operation string) {
	connCtx.Logger.Error(operation+"_failed", zap.Error(err))

	// 尝试发送错误响应
	if ws := connCtx.WsConn; ws != nil {
		resp := &JsonResp{
			Code:    ex.WS_SEND,
			Message: "websocket error: " + operation,
			Time:    utils.UnixMilli(),
		}

		if len(connCtx.Subject.Payload.Sub) == 0 {
			resp.Nonce = utils.RandNonce()
		} else if connCtx.JsonBody != nil {
			resp.Nonce = connCtx.JsonBody.Nonce
		}

		if result, marshalErr := utils.JsonMarshal(resp); marshalErr == nil {
			// 使用带超时的写控制，避免阻塞
			ws.WriteControl(fasthttpWs.TextMessage, result, time.Now().Add(1*time.Second))
		}
	}
}

// ConfigValidator 配置验证器（统一配置检查）
type ConfigValidator struct{}

func (cv *ConfigValidator) validateServerConfig(addr string, server *fasthttp.Server, heartbeatSvc *HeartbeatService) error {
	if addr == "" {
		return utils.Error("server address cannot be empty")
	}
	if server == nil {
		return utils.Error("server not initialized, call AddRouter first")
	}
	if heartbeatSvc == nil {
		return utils.Error("heartbeat service not initialized")
	}
	return nil
}

func (cv *ConfigValidator) validateRouterConfig(path string, handle Handle) error {
	if path == "" || path[0] != '/' {
		return utils.Error("router path must start with '/'")
	}
	if handle == nil {
		return utils.Error("router handle function cannot be nil")
	}
	return nil
}

func (cv *ConfigValidator) validatePoolConfig(maxConn, limit, bucket, ping int) error {
	if maxConn <= 0 || limit <= 0 || bucket <= 0 || ping <= 0 {
		return utils.Error("pool config error: maxConn/limit/bucket/ping must be > 0")
	}
	return nil
}

// 假设这些类型在其他文件中定义，如果没有请取消注释
// type JsonResp struct { Code int; Message, Data, Nonce string; Time, Plan int64; Sign string }
// type JsonBody struct { Data, Nonce, Sign string; Time, Plan int64 }
// type HookNode struct{} // 如果未使用，可以移除

// -------------------------- ConnectionManager 实现 --------------------------

// NewConnectionManager 创建连接管理器
func NewConnectionManager(maxConn int) *ConnectionManager {
	return &ConnectionManager{
		conns: make(map[string]map[string]*DevConn),
		max:   maxConn,
	}
}

// Add 添加连接
func (cm *ConnectionManager) Add(conn *DevConn) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	currentTotal := atomic.LoadInt32(&cm.totalConn)
	if currentTotal >= int32(cm.max) {
		return utils.Error("connection pool full", currentTotal)
	}

	deviceKey := utils.AddStr(conn.Dev, "_", conn.Sub) // 使用 Sub + Dev 作为唯一键

	if cm.conns[conn.Sub] == nil {
		cm.conns[conn.Sub] = make(map[string]*DevConn)
	}

	// 替换旧连接
	if oldConn, exists := cm.conns[conn.Sub][deviceKey]; exists {
		closeConn(oldConn, "replace old connection")
	}

	cm.conns[conn.Sub][deviceKey] = conn
	atomic.AddInt32(&cm.totalConn, 1)
	return nil
}

// Remove 移除连接
func (cm *ConnectionManager) Remove(subject, deviceKey string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if subjectConns, exists := cm.conns[subject]; exists {
		if conn, exists := subjectConns[deviceKey]; exists {
			closeConn(conn, "remove connection")
			delete(subjectConns, deviceKey)
			atomic.AddInt32(&cm.totalConn, -1)

			if len(subjectConns) == 0 {
				delete(cm.conns, subject)
			}
		}
	}
}

// Get 获取指定连接
func (cm *ConnectionManager) Get(subject, deviceKey string) *DevConn {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if subjectConns, exists := cm.conns[subject]; exists {
		return subjectConns[deviceKey]
	}
	return nil
}

// Broadcast 广播消息到所有连接
func (cm *ConnectionManager) Broadcast(data []byte) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	for _, subjectConns := range cm.conns {
		for _, conn := range subjectConns {
			conn.Send(data)
		}
	}
}

// SendToSubject 发送消息到指定主题的所有连接
func (cm *ConnectionManager) SendToSubject(subject string, data []byte) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if subjectConns, exists := cm.conns[subject]; exists {
		for _, conn := range subjectConns {
			conn.Send(data)
		}
	}
}

// UpdateHeartbeat 更新连接心跳时间
func (cm *ConnectionManager) UpdateHeartbeat(subject, deviceKey string) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if subjectConns, exists := cm.conns[subject]; exists {
		if conn, exists := subjectConns[deviceKey]; exists {
			atomic.StoreInt64(&conn.Last, utils.UnixSecond())
		}
	}
}

// CleanupExpired 清理过期连接
func (cm *ConnectionManager) CleanupExpired(timeoutSeconds int64) int {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cleaned := 0
	currentTime := utils.UnixSecond()

	for subject, subjectConns := range cm.conns {
		for deviceKey, conn := range subjectConns {
			if currentTime-atomic.LoadInt64(&conn.Last) > timeoutSeconds || currentTime > conn.Life {
				closeConn(conn, "cleanup expired connection")
				delete(subjectConns, deviceKey)
				atomic.AddInt32(&cm.totalConn, -1)
				cleaned++
			}
		}
		if len(subjectConns) == 0 {
			delete(cm.conns, subject)
		}
	}
	return cleaned
}

// CleanupAll 清理所有连接
func (cm *ConnectionManager) CleanupAll() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	for _, subjectConns := range cm.conns {
		for _, conn := range subjectConns {
			closeConn(conn, "server shutdown cleanup")
			atomic.AddInt32(&cm.totalConn, -1)
		}
	}
	cm.conns = make(map[string]map[string]*DevConn) // 重置map
}

// Count 获取当前连接数
func (cm *ConnectionManager) Count() int {
	return int(atomic.LoadInt32(&cm.totalConn))
}

// -------------------------- MessageHandler 实现 --------------------------

func NewMessageHandler(handle Handle, debug bool) *MessageHandler {
	return &MessageHandler{
		handle: handle,
		debug:  debug,
	}
}

func (mh *MessageHandler) Process(connCtx *ConnectionContext, body []byte) (interface{}, error) {
	if mh.debug {
		connCtx.Logger.Debug("message_received", zap.ByteString("raw_data", body))
	}

	jsonBody := &JsonBody{}
	if err := utils.JsonUnmarshal(body, jsonBody); err != nil {
		return nil, ex.Throw{Code: fasthttp.StatusBadRequest, Msg: "invalid JSON format"}
	}
	connCtx.JsonBody = jsonBody

	if jsonBody.Data == pingCmd { // 简化心跳检测
		connCtx.Server.connManager.UpdateHeartbeat(connCtx.Subject.GetSub(nil), utils.AddStr(connCtx.Subject.GetDev(nil), "_", connCtx.Subject.GetSub(nil)))
		return nil, nil
	}

	// 简化处理：直接使用data字段作为业务数据（可根据需要添加base64解码）
	bizData := []byte(jsonBody.Data)
	return mh.handle(connCtx.ctx, connCtx, bizData)
}

// -------------------------- HeartbeatService 实现 --------------------------

func NewHeartbeatService(interval, timeout time.Duration, manager *ConnectionManager) *HeartbeatService {
	return &HeartbeatService{
		interval: interval,
		timeout:  timeout,
		manager:  manager,
		stopCh:   make(chan struct{}),
	}
}

func (hs *HeartbeatService) Start(logger *zap.Logger) {
	hs.mu.Lock()
	defer hs.mu.Unlock()

	if hs.running {
		logger.Warn("heartbeat_service_already_running")
		return
	}

	hs.running = true
	logger.Info("heartbeat_service_started", zap.Duration("interval", hs.interval), zap.Duration("timeout", hs.timeout))

	go hs.run(logger)
}

func (hs *HeartbeatService) Stop() {
	hs.mu.Lock()
	defer hs.mu.Unlock()

	if !hs.running {
		return
	}

	hs.running = false
	close(hs.stopCh)
}

func (hs *HeartbeatService) run(logger *zap.Logger) {
	ticker := time.NewTicker(hs.interval)
	defer ticker.Stop()

	for {
		select {
		case <-hs.stopCh:
			logger.Info("heartbeat_service_stopped")
			return
		case <-ticker.C:
			startTime := time.Now()
			cleaned := hs.manager.CleanupExpired(int64(hs.timeout.Seconds()))
			if cleaned > 0 {
				logger.Info("cleanup_expired_connections", zap.Int("cleaned", cleaned), zap.Int("remaining", hs.manager.Count()), zap.Duration("cost", time.Since(startTime)))
			}
		}
	}
}

// -------------------------- DevConn 实现 --------------------------

func (dc *DevConn) Send(data []byte) error {
	dc.sendMu.Lock()
	defer dc.sendMu.Unlock()

	if dc.Conn == nil {
		return utils.Error("connection is closed")
	}

	// 使用非阻塞的Send，fasthttpWs内部会处理缓冲区
	return dc.Conn.WriteMessage(fasthttpWs.TextMessage, data)
}

// -------------------------- WsServer 实现 --------------------------

func NewWsServer(debug bool) *WsServer {
	globalCtx, globalCancel := context.WithCancel(context.Background())

	// 创建一个简单的logger，避免复杂的初始化依赖
	logger := zap.NewNop()

	s := &WsServer{
		Debug:           debug,
		routes:          make(map[string]Handle),
		globalCtx:       globalCtx,
		globalCancel:    globalCancel,
		logger:          logger,
		errorHandler:    &ErrorHandler{logger: logger},
		configValidator: &ConfigValidator{},
		upgrader: &fasthttpWs.FastHTTPUpgrader{
			ReadBufferSize:  1024 * 4,
			WriteBufferSize: 1024 * 4,
			CheckOrigin: func(ctx *fasthttp.RequestCtx) bool {
				// 生产环境应配置具体的Origin白名单
				return true
			},
		},
	}

	s.server = &fasthttp.Server{
		Handler:            s.serveHTTP,
		Concurrency:        100000,
		ReadTimeout:        30 * time.Second,
		WriteTimeout:       30 * time.Second,
		IdleTimeout:        60 * time.Second,
		MaxRequestBodySize: WS_MAX_BODY_LEN,
	}

	return s
}

func (s *WsServer) AddRouter(path string, handle Handle) error {
	if err := s.configValidator.validateRouterConfig(path, handle); err != nil {
		return err
	}

	s.routesMu.Lock()
	defer s.routesMu.Unlock()
	s.routes[path] = handle
	return nil
}

func (s *WsServer) NewPool(maxConn, limit, bucket, ping int) error {
	if err := s.configValidator.validatePoolConfig(maxConn, limit, bucket, ping); err != nil {
		return err
	}

	s.maxConn = maxConn
	s.ping = ping

	s.connManager = NewConnectionManager(maxConn)
	s.limiter = rate.NewLimiter(rate.Limit(limit), bucket)

	pingDuration := time.Duration(ping) * time.Second
	s.heartbeatSvc = NewHeartbeatService(pingDuration, pingDuration*2, s.connManager)
	return nil
}

func (s *WsServer) StartWebsocket(addr string) error {
	if err := s.configValidator.validateServerConfig(addr, s.server, s.heartbeatSvc); err != nil {
		return err
	}

	s.logger.Info("server_starting", zap.String("address", addr))

	// 启动心跳服务
	s.heartbeatSvc.Start(s.logger)

	// 监听中断信号
	go s.gracefulShutdown()

	// 启动HTTP服务器
	if err := s.server.ListenAndServe(addr); err != nil {
		s.logger.Error("server_start_failed", zap.Error(err))
		return err
	}

	s.logger.Info("server_stopped_gracefully")
	return nil
}

func (s *WsServer) serveHTTP(ctx *fasthttp.RequestCtx) {
	path := string(ctx.Path())

	s.routesMu.RLock()
	handle, exists := s.routes[path]
	s.routesMu.RUnlock()

	if !exists {
		ctx.SetStatusCode(fasthttp.StatusNotFound)
		return
	}

	if s.limiter != nil && !s.limiter.Allow() {
		ctx.SetStatusCode(fasthttp.StatusTooManyRequests)
		ctx.SetBodyString("too many connections")
		return
	}

	if err := s.upgrader.Upgrade(ctx, func(ws *fasthttpWs.Conn) {
		connCtx, err := s.initializeConnection(ctx, ws, path)
		if err != nil {
			s.logger.Error("failed to initialize connection", zap.Error(err))
			ws.Close() // 关闭WebSocket连接
			return
		}
		defer s.cleanupConnection(connCtx)

		s.handleConnectionLoop(connCtx, handle)
	}); err != nil {
		s.logger.Error("websocket_upgrade_failed", zap.String("path", path), zap.Error(err))
	}
}

func (s *WsServer) initializeConnection(httpCtx *fasthttp.RequestCtx, ws *fasthttpWs.Conn, path string) (*ConnectionContext, error) {
	authHeader := string(httpCtx.Request.Header.Peek("Authorization"))
	if authHeader == "" {
		return nil, utils.Error("missing authorization header")
	}

	subject := &jwt.Subject{}
	if err := subject.Verify(nil, ""); err != nil { // 假设jwt.Subject有ParseToken方法
		return nil, utils.Error("invalid or expired token: %v", err)
	}
	if len(subject.Payload.Sub) == 0 {
		return nil, utils.Error("token authentication failed")
	}

	connCtx, devConn := s.createConnectionContext(subject, ws, path)

	if err := s.connManager.Add(devConn); err != nil {
		connCtx.cancel()
		return nil, err
	}

	return connCtx, nil
}

func (s *WsServer) createConnectionContext(subject *jwt.Subject, ws *fasthttpWs.Conn, path string) (*ConnectionContext, *DevConn) {
	connCtx, cancel := context.WithCancel(s.globalCtx)

	devID := subject.GetDev(nil)
	subID := subject.GetSub(nil)

	devConn := &DevConn{
		Sub:  subID,
		Dev:  devID,
		Life: utils.UnixSecond() + 3600, // 1小时生命周期，可配置
		Last: utils.UnixSecond(),
		Conn: ws,
		ctx:  connCtx,
	}

	// 为每个连接创建独立的logger，包含连接标识
	connLogger := s.logger.With(
		zap.String("sub", subID),
		zap.String("dev", devID),
		zap.String("path", path),
	)

	return &ConnectionContext{
		Subject: subject,
		WsConn:  ws,
		DevConn: devConn,
		Server:  s,
		Logger:  connLogger,
		ctx:     connCtx,
		cancel:  cancel,
	}, devConn
}

func (s *WsServer) handleConnectionLoop(connCtx *ConnectionContext, handle Handle) {
	connCtx.Logger.Info("client_connected")
	messageHandler := NewMessageHandler(handle, s.Debug)

	for {
		select {
		case <-connCtx.ctx.Done():
			connCtx.Logger.Info("connection context cancelled")
			return
		default:
			// 设置读取超时，防止死连接
			connCtx.WsConn.SetReadDeadline(time.Now().Add(time.Duration(s.ping) * 3 * time.Second))

			messageType, message, err := connCtx.WsConn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
					connCtx.Logger.Error("read_message_error", zap.Error(err))
				} else {
					connCtx.Logger.Info("connection_closed_by_client", zap.Error(err))
				}
				return
			}

			if messageType != fasthttpWs.TextMessage {
				connCtx.Logger.Warn("unsupported_message_type", zap.Int("type", messageType))
				continue
			}

			reply, err := messageHandler.Process(connCtx, message)
			if err != nil {
				s.errorHandler.handleConnectionError(connCtx, err, "process_message")
				continue
			}

			if reply != nil {
				replyBytes, err := utils.JsonMarshal(reply)
				if err != nil {
					connCtx.Logger.Error("failed_to_marshal_reply", zap.Error(err))
					continue
				}
				if err := connCtx.DevConn.Send(replyBytes); err != nil {
					connCtx.Logger.Error("failed_to_send_reply", zap.Error(err))
					return // 发送失败通常意味着连接已断开
				}
			}
		}
	}
}

func (s *WsServer) cleanupConnection(connCtx *ConnectionContext) {
	connCtx.cancel()
	deviceKey := utils.AddStr(connCtx.DevConn.Dev, "_", connCtx.DevConn.Sub)
	s.connManager.Remove(connCtx.DevConn.Sub, deviceKey)
	connCtx.Logger.Info("client_disconnected")
}

func (s *WsServer) gracefulShutdown() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	s.logger.Info("initiating graceful shutdown...")

	// 停止接受新连接
	if err := s.server.Shutdown(); err != nil {
		s.logger.Error("error shutting down server", zap.Error(err))
	}

	// 停止心跳服务
	if s.heartbeatSvc != nil {
		s.heartbeatSvc.Stop()
	}

	// 关闭所有现有连接
	if s.connManager != nil {
		s.connManager.CleanupAll()
	}

	// 触发全局取消
	s.globalCancel()
}

// closeConn 安全关闭连接
func closeConn(conn *DevConn, reason string) {
	if conn == nil {
		return
	}

	// 注意: 这里简化处理，实际应该通过context cancel来停止相关goroutine
	// 但由于DevConn没有持有cancel函数，这里只关闭WebSocket连接
	if conn.Conn != nil {
		closeMsg := fasthttpWs.FormatCloseMessage(fasthttpWs.CloseNormalClosure, reason)
		// 使用带超时的写控制
		conn.Conn.WriteControl(fasthttpWs.CloseMessage, closeMsg, time.Now().Add(1*time.Second))
		conn.Conn.Close()
	}
}

// 辅助函数，用于从token中提取信息，需要根据你的jwt.Subject实现进行调整
// 假设jwt.Subject有以下方法：
// func (s *jwt.Subject) ParseToken(tokenString string) error
// func (s *jwt.Subject) IsAuthenticated() bool
// func (s *jwt.Subject) GetSub(nil) string
// func (s *jwt.Subject) GetDev(nil) string
