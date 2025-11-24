package node

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/godaddy-x/freego/zlog"

	"github.com/fasthttp/websocket"
	fasthttpWs "github.com/fasthttp/websocket"
	"github.com/godaddy-x/freego/cache"
	rate "github.com/godaddy-x/freego/cache/limiter"
	DIC "github.com/godaddy-x/freego/common"
	"github.com/godaddy-x/freego/ex"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/utils/crypto"
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
	Subject      *jwt.Subject
	JsonBody     *JsonBody
	WsConn       *fasthttpWs.Conn
	DevConn      *DevConn
	Server       *WsServer
	RouterConfig *RouterConfig // 路由配置
	Path         string        // WebSocket连接的路径，用于签名验证
	RawToken     []byte        // 原始JWT token字节，用于签名验证
	ctx          context.Context
	cancel       context.CancelFunc
}

// GetRawTokenBytes 获取原始JWT token字节
func (cc *ConnectionContext) GetRawTokenBytes() []byte {
	return cc.RawToken
}

// GetTokenSecret 获取WebSocket连接的token密钥
func (cc *ConnectionContext) GetTokenSecret() []byte {
	return cc.Subject.GetTokenSecret(utils.Bytes2Str(cc.GetRawTokenBytes()), cc.Server.jwtConfig.TokenKey)
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
	rsa    []crypto.Cipher
	handle Handle
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

// RouteInfo WebSocket路由信息结构体
type RouteInfo struct {
	Handle       Handle        // 业务处理器
	RouterConfig *RouterConfig // 路由配置
}

// WsServer WebSocket服务器核心结构体
type WsServer struct {
	server       *fasthttp.Server
	upgrader     *fasthttpWs.FastHTTPUpgrader
	routes       map[string]*RouteInfo // 路由映射：path -> 路由信息 (启动后只读)
	connManager  *ConnectionManager
	heartbeatSvc *HeartbeatService

	// 配置项
	ping         int           // 心跳间隔（秒）
	maxConn      int           // 最大连接数
	limiter      *rate.Limiter // 连接限流器
	globalCtx    context.Context
	globalCancel context.CancelFunc

	errorHandler    *ErrorHandler
	configValidator *ConfigValidator

	// JWT配置
	jwtConfig jwt.JwtConfig

	// ECC和缓存配置（用于Plan 2）
	// 8字节函数指针字段组 (5个字段，40字节)
	RSA             []crypto.Cipher                         // 8字节 - RSA/ECDSA加密解密对象列表
	RedisCacheAware func(ds ...string) (cache.Cache, error) // 8字节 - 函数指针
	LocalCacheAware func(ds ...string) (cache.Cache, error) // 8字节 - 函数指针
}

// ErrorHandler WebSocket错误处理器（统一错误处理）
type ErrorHandler struct {
}

func (eh *ErrorHandler) handleConnectionError(connCtx *ConnectionContext, err error, operation string) {
	zlog.Error(operation+"_failed", 0, zap.Error(err))

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

func NewMessageHandler(rsa []crypto.Cipher, handle Handle) *MessageHandler {
	return &MessageHandler{
		rsa:    rsa,
		handle: handle,
	}
}

func (mh *MessageHandler) Process(connCtx *ConnectionContext, body []byte) (crypto.Cipher, interface{}, error) {
	// 解析WebSocket消息体
	jsonBody := &JsonBody{}
	if err := utils.JsonUnmarshal(body, jsonBody); err != nil {
		zlog.Error("websocket message json unmarshal failed", 0, zlog.ByteString("raw_body", body), zlog.AddError(err))
		return nil, nil, ex.Throw{Code: fasthttp.StatusBadRequest, Msg: "invalid JSON format"}
	}
	connCtx.JsonBody = jsonBody

	// 心跳检测
	if jsonBody.Data == pingCmd {
		connCtx.Server.connManager.UpdateHeartbeat(connCtx.Subject.GetSub(nil), utils.AddStr(connCtx.Subject.GetDev(nil), "_", connCtx.Subject.GetSub(nil)))
		return nil, nil, nil
	}

	// 验证消息体（按照HTTP协议标准）
	cipher, err := mh.validWebSocketBody(connCtx)
	if err != nil {
		zlog.Error("websocket message validation failed", 0, zlog.AddError(err))
		return nil, nil, err
	}

	// 解密业务数据
	bizData, err := mh.decryptWebSocketData(connCtx)
	if err != nil {
		zlog.Error("websocket data decryption failed", 0, zap.Error(err))
		return nil, nil, err
	}
	result, err := mh.handle(connCtx.ctx, connCtx, bizData)
	return cipher, result, err
}

func (self *MessageHandler) CheckECDSASign(msg, sign []byte) (crypto.Cipher, error) {
	if len(self.rsa) == 0 {
		return nil, nil
	}
	for _, cip := range self.rsa {
		if err := cip.Verify(msg, sign); err == nil {
			return cip, nil
		}
	}
	return nil, ex.Throw{Code: http.StatusBadRequest, Msg: "request signature valid invalid"}
}

// validWebSocketBody 验证WebSocket消息体（参考HTTP协议）
func (mh *MessageHandler) validWebSocketBody(connCtx *ConnectionContext) (crypto.Cipher, error) {
	body := connCtx.JsonBody
	d := body.Data
	if len(d) == 0 {
		return nil, ex.Throw{Code: fasthttp.StatusBadRequest, Msg: "websocket data is nil"}
	}

	// 只支持Plan 0和1，不再支持Plan 2
	if !utils.CheckInt64(body.Plan, 0, 1) {
		return nil, ex.Throw{Code: fasthttp.StatusBadRequest, Msg: "websocket plan invalid"}
	}

	if !utils.CheckLen(body.Nonce, 8, 32) {
		return nil, ex.Throw{Code: fasthttp.StatusBadRequest, Msg: "websocket nonce invalid"}
	}
	if body.Time <= 0 {
		return nil, ex.Throw{Code: fasthttp.StatusBadRequest, Msg: "websocket time must be > 0"}
	}
	if utils.MathAbs(utils.UnixSecond()-body.Time) > jwt.FIVE_MINUTES {
		return nil, ex.Throw{Code: fasthttp.StatusBadRequest, Msg: "websocket time invalid"}
	}

	// 检查是否需要AES加密
	if connCtx.RouterConfig != nil && connCtx.RouterConfig.AesRequest && body.Plan != 1 {
		return nil, ex.Throw{Code: fasthttp.StatusBadRequest, Msg: "websocket parameters must use encryption"}
	}

	if !utils.CheckStrLen(body.Sign, 32, 64) {
		return nil, ex.Throw{Code: fasthttp.StatusBadRequest, Msg: "websocket signature length invalid"}
	}

	// 对于Plan 0/1，必须要有token
	if len(connCtx.GetRawTokenBytes()) == 0 {
		return nil, ex.Throw{Code: fasthttp.StatusBadRequest, Msg: "websocket header token is nil"}
	}

	var sharedKey []byte

	// Plan 0/1使用token secret
	sharedKey = connCtx.GetTokenSecret()
	if len(sharedKey) == 0 {
		return nil, ex.Throw{Code: fasthttp.StatusBadRequest, Msg: "websocket secret is nil"}
	}

	// 构建签名字符串
	signStr := utils.AddStr(connCtx.Path, d, body.Nonce, body.Time, body.Plan) // 使用实际的WebSocket路径
	sign := utils.HMAC_SHA256_BASE(utils.Str2Bytes(signStr), sharedKey)
	expectedSign := utils.Base64Encode(sign)
	if expectedSign != body.Sign {
		return nil, ex.Throw{Code: fasthttp.StatusUnauthorized, Msg: "websocket signature verify invalid"}
	}

	cipher, err := mh.CheckECDSASign(sign, utils.Base64Decode(body.Valid))
	if err != nil {
		return nil, err
	}

	return cipher, nil
}

// decryptWebSocketData 解密WebSocket数据（参考HTTP协议）
func (mh *MessageHandler) decryptWebSocketData(connCtx *ConnectionContext) ([]byte, error) {
	body := connCtx.JsonBody
	d := body.Data

	switch body.Plan {
	case 0: // 明文
		rawData := utils.Base64Decode(d)
		if len(rawData) == 0 {
			return nil, ex.Throw{Code: fasthttp.StatusBadRequest, Msg: "websocket base64 parsing failed"}
		}
		return rawData, nil
	case 1: // AES-GCM加密
		secret := connCtx.GetTokenSecret()
		if len(secret) == 0 {
			return nil, ex.Throw{Code: fasthttp.StatusBadRequest, Msg: "websocket aes secret is nil"}
		}
		defer DIC.ClearData(secret)
		// 使用与HTTP相同的GCM解密方式
		iv := utils.Str2Bytes(utils.AddStr(body.Time, body.Nonce, body.Plan, connCtx.Path))
		rawData, err := utils.AesGCMDecryptBase(d, secret[:32], iv)
		if err != nil {
			return nil, ex.Throw{Code: fasthttp.StatusBadRequest, Msg: "websocket aes decrypt failed", Err: err}
		}
		return rawData, nil
	default:
		return nil, ex.Throw{Code: fasthttp.StatusBadRequest, Msg: "websocket unsupported plan"}
	}
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

func (hs *HeartbeatService) Start() {
	hs.mu.Lock()
	defer hs.mu.Unlock()

	if hs.running {
		zlog.Warn("heartbeat_service_already_running", 0)
		return
	}

	hs.running = true
	if zlog.IsDebug() {
		zlog.Debug("heartbeat_service_started", 0, zlog.String("interval", hs.interval.String()), zlog.String("timeout", hs.timeout.String()))
	}

	go hs.run()
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

func (hs *HeartbeatService) run() {
	ticker := time.NewTicker(hs.interval)
	defer ticker.Stop()

	for {
		select {
		case <-hs.stopCh:
			if zlog.IsDebug() {
				zlog.Debug("heartbeat_service_stopped", 0)
			}
			return
		case <-ticker.C:
			cleaned := hs.manager.CleanupExpired(int64(hs.timeout.Seconds()))
			if cleaned > 0 {
				zlog.Info("cleanup_expired_connections", 0, zlog.Int("cleaned", cleaned), zlog.Int("remaining", hs.manager.Count()))
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

func NewWsServer() *WsServer {
	globalCtx, globalCancel := context.WithCancel(context.Background())

	s := &WsServer{
		routes:          make(map[string]*RouteInfo),
		globalCtx:       globalCtx,
		globalCancel:    globalCancel,
		errorHandler:    &ErrorHandler{},
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

func (s *WsServer) AddRouter(path string, handle Handle, routerConfig *RouterConfig) error {
	if err := s.configValidator.validateRouterConfig(path, handle); err != nil {
		return err
	}

	// 如果没有提供RouterConfig，使用默认配置
	if routerConfig == nil {
		routerConfig = &RouterConfig{}
	}

	s.routes[path] = &RouteInfo{
		Handle:       handle,
		RouterConfig: routerConfig,
	}
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

	zlog.Info("server_starting", 0, zlog.String("address", addr))

	// 启动心跳服务
	s.heartbeatSvc.Start()

	// 监听中断信号
	go s.gracefulShutdown()

	// 如无设定本地缓存，则使用默认缓存
	if s.LocalCacheAware == nil {
		s.AddLocalCache(nil)
	}

	// 启动HTTP服务器
	if err := s.server.ListenAndServe(addr); err != nil {
		zlog.Error("server_start_failed", 0, zlog.AddError(err))
		return err
	}

	zlog.Info("server_stopped_gracefully", 0)
	return nil
}

func (s *WsServer) serveHTTP(ctx *fasthttp.RequestCtx) {
	path := string(ctx.Path())
	method := string(ctx.Method())

	if zlog.IsDebug() {
		zlog.Debug("HTTP_REQUEST_RECEIVED", 0,
			zlog.String("method", method),
			zlog.String("path", path),
			zlog.String("client_ip", ctx.RemoteIP().String()),
			zlog.String("user_agent", string(ctx.UserAgent())))
	}

	routeInfo, exists := s.routes[path]
	if !exists {
		zlog.Warn("ROUTE_NOT_FOUND", 0,
			zlog.String("path", path),
			zlog.Int("available_routes", len(s.routes)))
		ctx.SetStatusCode(fasthttp.StatusNotFound)
		return
	}

	zlog.Info("WEBSOCKET_UPGRADE_ATTEMPT", 0,
		zlog.String("path", path))

	if s.limiter != nil && !s.limiter.Allow() {
		ctx.SetStatusCode(fasthttp.StatusTooManyRequests)
		ctx.SetBodyString("too many connections")
		return
	}

	if err := s.upgrader.Upgrade(ctx, func(ws *fasthttpWs.Conn) {
		connCtx, err := s.initializeConnection(ctx, ws, path, routeInfo.RouterConfig)
		if err != nil {
			zlog.Error("failed to initialize connection", 0, zlog.AddError(err))
			ws.Close() // 关闭WebSocket连接
			return
		}
		defer s.cleanupConnection(connCtx)

		s.handleConnectionLoop(connCtx, routeInfo.Handle)
	}); err != nil {
		zlog.Error("websocket_upgrade_failed", 0, zlog.String("path", path), zlog.AddError(err))
	}
}

func (s *WsServer) initializeConnection(httpCtx *fasthttp.RequestCtx, ws *fasthttpWs.Conn, path string, routerConfig *RouterConfig) (*ConnectionContext, error) {
	authHeader := httpCtx.Request.Header.Peek("Authorization")
	if zlog.IsDebug() {
		zlog.Debug("WEBSOCKET_UPGRADE_ATTEMPT", 0,
			zlog.String("path", path),
			zlog.String("client_ip", httpCtx.RemoteIP().String()),
			zlog.Bool("auth_header_present", len(authHeader) > 0))
	}

	// 检查认证头（非游客模式需要认证）
	if len(authHeader) == 0 {
		zlog.Error("MISSING_AUTH_HEADER", 0, zlog.String("client_ip", httpCtx.RemoteIP().String()))
		return nil, utils.Error("missing authorization header")
	}

	// 认证模式：验证JWT token
	subject := &jwt.Subject{Payload: &jwt.Payload{}}

	var c cache.Cache
	var err error
	// 设置Subject的cache字段，与HTTP流程保持一致
	if s.LocalCacheAware != nil {
		c, err = s.LocalCacheAware()
		if err != nil {
			return nil, utils.Error("missing cache object: ", err.Error())
		}
	}

	subject.SetCache(c)

	if len(authHeader) == 0 {
		return nil, utils.Error("empty authorization header")
	}

	// 验证token (使用配置的JWT密钥)
	if err := subject.Verify(authHeader, s.jwtConfig.TokenKey); err != nil {
		return nil, utils.Error("invalid or expired token: %v", err)
	}

	// 检查token是否有效
	if !subject.CheckReady() {
		return nil, utils.Error("token not ready")
	}

	connCtx, devConn := s.createConnectionContext(subject, ws, path, routerConfig, authHeader)

	if err := s.connManager.Add(devConn); err != nil {
		connCtx.cancel()
		return nil, err
	}

	return connCtx, nil
}

func (s *WsServer) createConnectionContext(subject *jwt.Subject, ws *fasthttpWs.Conn, path string, routerConfig *RouterConfig, rawToken []byte) (*ConnectionContext, *DevConn) {
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

	return &ConnectionContext{
		Subject:      subject,
		WsConn:       ws,
		DevConn:      devConn,
		Server:       s,
		RouterConfig: routerConfig, // 使用传入的路由配置
		Path:         path,         // 设置WebSocket连接路径
		RawToken:     rawToken,     // 设置原始token字节
		ctx:          connCtx,
		cancel:       cancel,
	}, devConn
}

func (s *WsServer) handleConnectionLoop(connCtx *ConnectionContext, handle Handle) {
	zlog.Info("CLIENT_CONNECTED", 0,
		zlog.String("client_address", connCtx.WsConn.RemoteAddr().String()),
		zlog.String("user_id", connCtx.Subject.GetSub(nil)),
		zlog.String("device_id", connCtx.Subject.GetDev(nil)))

	if zlog.IsDebug() {
		zlog.Debug("STARTING_MESSAGE_LOOP", 0, zlog.String("client", connCtx.WsConn.RemoteAddr().String()))
	}
	messageHandler := NewMessageHandler(s.RSA, handle)

	for {
		select {
		case <-connCtx.ctx.Done():
			if zlog.IsDebug() {
				zlog.Debug("connection context cancelled", 0)
			}
			return
		default:
			// 设置读取超时，防止死连接
			connCtx.WsConn.SetReadDeadline(time.Now().Add(time.Duration(s.ping) * 3 * time.Second))

			messageType, message, err := connCtx.WsConn.ReadMessage()
			if err != nil {
				zlog.Error("READ_MESSAGE_FAILED", 0,
					zlog.AddError(err),
					zlog.String("client", connCtx.WsConn.RemoteAddr().String()))
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
					zlog.Error("read_message_error", 0, zlog.AddError(err))
				} else {
					zlog.Info("connection_closed_by_client", 0, zlog.AddError(err))
				}
				return
			}

			if zlog.IsDebug() {
				zlog.Debug("MESSAGE_RECEIVED", 0,
					zlog.Int("message_type", messageType),
					zlog.Int("message_length", len(message)),
					zlog.String("raw_message", string(message)),
					zlog.String("client", connCtx.WsConn.RemoteAddr().String()))
			}

			if messageType != fasthttpWs.TextMessage {
				zlog.Warn("unsupported_message_type", 0, zlog.Int("type", messageType))
				continue
			}

			cipher, reply, err := messageHandler.Process(connCtx, message)
			if err != nil {
				s.errorHandler.handleConnectionError(connCtx, err, "process_message")
				continue
			}

			if reply != nil {
				// 构造JsonResp格式的响应，与HTTP流程保持一致
				jsonResp := &JsonResp{
					Code:    200,
					Message: "success",
					Data:    "",
					Nonce:   connCtx.JsonBody.Nonce, // 使用请求的nonce
					Time:    utils.UnixSecond(),
					Plan:    connCtx.JsonBody.Plan, // 使用请求的plan
				}

				// 序列化响应数据
				respData, err := utils.JsonMarshal(reply)
				if err != nil {
					zlog.Error("failed_to_marshal_reply_data", 0, zlog.AddError(err))
					continue
				}

				// 根据plan决定是否加密
				if connCtx.JsonBody.Plan == 1 {
					// AES加密响应数据
					secret := connCtx.GetTokenSecret()
					if len(secret) == 0 {
						zlog.Error("response_aes_secret_missing", 0)
						continue
					}
					defer DIC.ClearData(secret)

					encryptedData, err := utils.AesGCMEncryptBase(respData, secret[:32],
						utils.Str2Bytes(utils.AddStr(jsonResp.Time, jsonResp.Nonce, jsonResp.Plan, connCtx.Path)))
					if err != nil {
						zlog.Error("response_data_encrypt_failed", 0, zlog.AddError(err))
						continue
					}
					jsonResp.Data = encryptedData
				} else {
					jsonResp.Data = utils.Base64Encode(utils.Bytes2Str(respData))
				}

				// 生成响应签名
				signStr := utils.AddStr(connCtx.Path, jsonResp.Data, jsonResp.Nonce, jsonResp.Time, jsonResp.Plan)
				sign := utils.HMAC_SHA256_BASE(utils.Str2Bytes(signStr), connCtx.GetTokenSecret())
				jsonResp.Sign = utils.Base64Encode(sign)

				if cipher != nil {
					result, err := cipher.Sign(sign)
					if err != nil {
						zlog.Error("failed_to_ecdsa_sign_data", 0, zlog.AddError(err))
						return
					}
					jsonResp.Valid = utils.Base64Encode(result)
				}

				// 发送JsonResp格式的响应
				replyBytes, err := utils.JsonMarshal(jsonResp)
				if err != nil {
					zlog.Error("failed_to_marshal_jsonresp", 0, zlog.AddError(err))
					continue
				}

				if err := connCtx.DevConn.Send(replyBytes); err != nil {
					zlog.Error("failed_to_send_reply", 0, zlog.AddError(err))
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
	if zlog.IsDebug() {
		zlog.Debug("client_disconnected", 0)
	}
}

func (s *WsServer) gracefulShutdown() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	if zlog.IsDebug() {
		zlog.Debug("initiating graceful shutdown...", 0)
	}

	// 停止接受新连接
	if err := s.server.Shutdown(); err != nil {
		zlog.Error("error shutting down server", 0, zlog.AddError(err))
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

// GetCacheObject 获取缓存对象
func (self *WsServer) GetCacheObject() (cache.Cache, error) {
	var err error
	var c cache.Cache
	if self.RedisCacheAware != nil {
		c, err = self.RedisCacheAware()
		if err != nil {
			return nil, ex.Throw{Code: http.StatusBadRequest, Msg: "redis cache get error", Err: err}
		}
	} else if self.LocalCacheAware != nil {
		c, err = self.LocalCacheAware()
		if err != nil {
			return nil, ex.Throw{Code: http.StatusBadRequest, Msg: "local cache get error", Err: err}
		}
	} else {
		return nil, ex.Throw{Code: http.StatusBadRequest, Msg: "cache object not setting"}
	}
	return c, nil
}

// AddRedisCache 增加redis缓存实例
func (self *WsServer) AddRedisCache(cacheAware CacheAware) {
	if cacheAware != nil && self.RedisCacheAware == nil {
		self.RedisCacheAware = cacheAware
	}
}

// AddLocalCache 增加本地缓存实例
func (self *WsServer) AddLocalCache(cacheAware CacheAware) {
	if self.LocalCacheAware == nil {
		if cacheAware == nil {
			cacheAware = func(ds ...string) (cache.Cache, error) {
				return defaultCacheObject, nil
			}
		}
		self.LocalCacheAware = cacheAware
	}
}

// AddCipher 增加RSA/ECDSA加密解密对象
func (self *WsServer) AddCipher(cipher crypto.Cipher) error {
	if cipher == nil {
		return utils.Error("cipher is nil")
	}
	self.RSA = append(self.RSA, cipher)
	return nil
}

// AddJwtConfig 添加JWT配置
func (self *WsServer) AddJwtConfig(config jwt.JwtConfig) error {
	if len(config.TokenKey) == 0 {
		return utils.Error("jwt config key is nil")
	}
	if config.TokenExp < 0 {
		return utils.Error("jwt config exp invalid")
	}
	self.jwtConfig.TokenAlg = config.TokenAlg
	self.jwtConfig.TokenTyp = config.TokenTyp
	self.jwtConfig.TokenKey = config.TokenKey
	self.jwtConfig.TokenExp = config.TokenExp
	return nil
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
