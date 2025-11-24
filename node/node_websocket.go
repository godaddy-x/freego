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
)

// WebSocket服务器实现

// WebSocket专用常量
const (
	pingCmd         = "ws-health-check" // 心跳检测命令
	DefaultWsRoute  = "/ws"             // 默认WebSocket路由路径
	WS_MAX_BODY_LEN = 1024 * 1024       // 1MB
)

// ConnectionUniquenessMode 连接唯一性模式
// 用于控制WebSocket连接的唯一性策略
type ConnectionUniquenessMode int

const (
	// SubjectUnique 仅Subject唯一，一个用户只能有一个连接
	// 适用于单设备应用场景，如移动端App
	SubjectUnique ConnectionUniquenessMode = iota

	// SubjectDeviceUnique Subject+Device唯一，一个用户可以在多个设备上连接
	// 适用于多设备场景，如Web、App、PC同时在线
	SubjectDeviceUnique
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
	Path         string        // WebSocket连接的路径
	RawToken     []byte        // 原始JWT token字节，用于签名验证
	ctx          context.Context
	cancel       context.CancelFunc
}

// GetRawTokenBytes 获取原始JWT token字节
func (cc *ConnectionContext) GetRawTokenBytes() []byte {
	return cc.RawToken
}

// GetUserID 获取用户ID int64类型
func (cc *ConnectionContext) GetUserID() int64 {
	if cc.Subject == nil || cc.Subject.Payload == nil || len(cc.Subject.Payload.Sub) == 0 {
		return 0
	}
	userID, err := utils.StrToInt64(cc.Subject.Payload.Sub)
	if err != nil {
		return 0
	}
	return userID
}

// GetUserIDString 获取用户ID string类型
func (cc *ConnectionContext) GetUserIDString() string {
	if cc.Subject == nil || cc.Subject.Payload == nil || len(cc.Subject.Payload.Sub) == 0 {
		return ""
	}
	return cc.Subject.Payload.Sub
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
	mode      ConnectionUniquenessMode       // 连接唯一性模式
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
	Last   int64              // 最后心跳时间（时间戳）
	Conn   *fasthttpWs.Conn   // WebSocket连接
	sendMu sync.Mutex         // 发送消息互斥锁（避免并发写冲突）
	ctx    context.Context    // 用于取消该连接的相关goroutine
	cancel context.CancelFunc // 上下文取消函数，用于终止相关goroutine
}

// RouteInfo WebSocket路由信息结构体
type RouteInfo struct {
	Handle       Handle        // 业务处理器
	RouterConfig *RouterConfig // 路由配置
}

// WebSocketMetrics WebSocket监控指标
type WebSocketMetrics struct {
	// 连接相关指标
	connectionsTotal  int64 // 总连接数
	connectionsActive int64 // 当前活跃连接数
	connectionsPeak   int64 // 峰值连接数

	// 消息处理指标
	messagesTotal   int64 // 总消息数
	messagesSuccess int64 // 成功处理消息数
	messagesError   int64 // 错误消息数

	// 心跳相关指标
	heartbeatsTotal   int64 // 总心跳数
	heartbeatsSuccess int64 // 成功心跳数
	heartbeatsFailed  int64 // 失败心跳数

	// 性能指标
	uptimeSeconds int64     // 运行时间（秒）
	startTime     time.Time // 启动时间
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
	idleTimeout  time.Duration // 连接空闲超时时间
	globalCtx    context.Context
	globalCancel context.CancelFunc

	// 连接唯一性模式
	connUniquenessMode ConnectionUniquenessMode

	// 监控指标
	metrics *WebSocketMetrics

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
	zlog.Error(operation+"_failed", 0, zlog.AddError(err))

	// 尝试发送错误响应
	if ws := connCtx.WsConn; ws != nil {
		resp := &JsonResp{
			Code:    ex.WS_SEND,
			Message: "websocket error: " + operation,
			Time:    utils.UnixMilli(),
		}

		if len(connCtx.Subject.Payload.Sub) == 0 {
			resp.Nonce = utils.GetUUID(true)
		} else if connCtx.JsonBody != nil {
			resp.Nonce = connCtx.JsonBody.Nonce
		}

		if result, marshalErr := utils.JsonMarshal(resp); marshalErr == nil {
			// 捕获 WriteControl 错误，仅在 debug 级别记录，避免连接关闭时的无效错误日志
			if err := ws.WriteControl(fasthttpWs.TextMessage, result, time.Now().Add(1*time.Second)); err != nil {
				if zlog.IsDebug() {
					zlog.Debug("failed to send error response to closed connection", 0, zlog.AddError(err))
				}
			}
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
func NewConnectionManager(maxConn int, mode ConnectionUniquenessMode) *ConnectionManager {
	return &ConnectionManager{
		conns: make(map[string]map[string]*DevConn),
		max:   maxConn,
		mode:  mode,
	}
}

// Add 添加连接
// 根据连接唯一性模式决定连接的处理策略：
// - SubjectUnique: 替换同subject的所有连接，只保留一个
// - SubjectDeviceUnique: 替换同subject+device的连接，允许多设备同时在线
// 设备键格式：subject_device (如: user123_web, user123_app)
func (cm *ConnectionManager) Add(conn *DevConn) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	currentTotal := atomic.LoadInt32(&cm.totalConn)
	if currentTotal >= int32(cm.max) {
		return utils.Error("connection pool full", currentTotal)
	}

	var uniqueKey string
	if cm.mode == SubjectUnique {
		// Subject唯一模式：使用subject作为唯一键
		uniqueKey = conn.Sub
		// 检查是否已有连接，如果有则替换
		if cm.conns[conn.Sub] == nil {
			cm.conns[conn.Sub] = make(map[string]*DevConn)
		}
		if _, exists := cm.conns[conn.Sub][uniqueKey]; exists {
			cm.removeConnection(conn.Sub, uniqueKey) // 直接调用内部无锁方法，避免重复加锁
		}
		cm.conns[conn.Sub][uniqueKey] = conn
	} else {
		// Subject+Device唯一模式：使用subject+device作为唯一键
		uniqueKey = utils.AddStr(conn.Sub, "_", conn.Dev)
		if cm.conns[conn.Sub] == nil {
			cm.conns[conn.Sub] = make(map[string]*DevConn)
		}
		// 替换旧连接
		if _, exists := cm.conns[conn.Sub][uniqueKey]; exists {
			cm.removeConnection(conn.Sub, uniqueKey) // 直接调用内部无锁方法，避免重复加锁
		}
		cm.conns[conn.Sub][uniqueKey] = conn
	}

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

// removeConnection 内部方法：移除连接（不获取锁，调用方需要处理锁）
func (cm *ConnectionManager) removeConnection(subject, deviceKey string) {
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

// RemoveWithCallback 移除连接并执行回调（用于指标记录）
func (cm *ConnectionManager) RemoveWithCallback(subject, deviceKey string, callback func()) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if subjectConns, exists := cm.conns[subject]; exists {
		if _, exists := subjectConns[deviceKey]; exists {
			cm.removeConnection(subject, deviceKey)

			// 执行回调（通常用于指标记录）
			if callback != nil {
				callback()
			}
		}
	}
}

// CleanupExpired 清理过期连接
func (cm *ConnectionManager) CleanupExpired(timeoutSeconds int64) int {
	cleaned := 0
	currentTime := utils.UnixSecond()

	// 收集需要清理的连接（无锁操作，提高并发性）
	cm.mu.RLock()
	var toRemove []struct {
		subject   string
		deviceKey string
	}

	for subject, subjectConns := range cm.conns {
		for deviceKey, conn := range subjectConns {
			if currentTime-atomic.LoadInt64(&conn.Last) > timeoutSeconds {
				toRemove = append(toRemove, struct {
					subject   string
					deviceKey string
				}{subject, deviceKey})
			}
		}
	}
	cm.mu.RUnlock()

	// 执行清理（使用标准接口，保持设计一致性）
	for _, item := range toRemove {
		cm.RemoveWithCallback(item.subject, item.deviceKey, nil)
		cleaned++
	}

	return cleaned
}

// CleanupAll 清理所有连接
func (cm *ConnectionManager) CleanupAll() {
	// 收集所有连接信息（使用读锁，提高并发性）
	cm.mu.RLock()
	var toRemove []struct {
		subject   string
		deviceKey string
	}

	for subject, subjectConns := range cm.conns {
		for deviceKey := range subjectConns {
			toRemove = append(toRemove, struct {
				subject   string
				deviceKey string
			}{subject, deviceKey})
		}
	}
	cm.mu.RUnlock()

	// 执行清理（使用标准接口，保持设计一致性）
	for _, item := range toRemove {
		cm.RemoveWithCallback(item.subject, item.deviceKey, nil)
	}

	// 确保计数器清零
	atomic.StoreInt32(&cm.totalConn, 0)
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

	// 检查是否是心跳包（用于失败时的指标记录）
	isHeartbeat := jsonBody.Router == "/ws/ping"

	// 验证消息体（按照HTTP协议标准）
	cipher, err := mh.validWebSocketBody(connCtx)
	if err != nil {
		zlog.Error("websocket message validation failed", 0, zlog.AddError(err))
		// 如果是心跳包验证失败，记录失败指标
		if isHeartbeat {
			connCtx.Server.recordHeartbeat(false)
		}
		return nil, nil, err
	}

	// 检查是否是新的结构化心跳包
	if jsonBody.Router == "/ws/ping" {
		// 是心跳包，直接更新当前连接的心跳时间
		atomic.StoreInt64(&connCtx.DevConn.Last, utils.UnixSecond())

		// 记录心跳成功指标（心跳包通过验证后基本不会失败）
		connCtx.Server.recordHeartbeat(true)

		if zlog.IsDebug() {
			zlog.Debug("heartbeat_received_and_updated", 0,
				zlog.String("subject", connCtx.Subject.GetSub(nil)),
				zlog.String("device", connCtx.Subject.GetDev(nil)),
				zlog.String("connection_path", connCtx.Path))
		}
		return nil, nil, nil
	}

	// 解密业务数据
	bizData, err := mh.decryptWebSocketData(connCtx)
	if err != nil {
		zlog.Error("websocket data decryption failed", 0, zlog.AddError(err))
		return nil, nil, err
	}

	// 根据消息中的路由选择处理器
	handle := mh.handle // 默认处理器
	if jsonBody.Router != "" {
		if routeInfo, exists := connCtx.Server.routes[jsonBody.Router]; exists {
			handle = routeInfo.Handle
			if zlog.IsDebug() {
				zlog.Debug("using route-specific handler", 0, zlog.String("router", jsonBody.Router))
			}
		} else {
			zlog.Warn("no handler found for router, using default", 0, zlog.String("router", jsonBody.Router))
		}
	}

	result, err := handle(connCtx.ctx, connCtx, bizData)
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
	// 使用从header获取的路由标识进行签名验证，支持通过header指定路由
	signStr := utils.AddStr(body.Router, d, body.Nonce, body.Time, body.Plan)
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
		iv := utils.Str2Bytes(utils.AddStr(body.Time, body.Nonce, body.Plan, body.Router))
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
	zlog.Info("heartbeat_service_started", 0, zlog.String("interval", hs.interval.String()), zlog.String("timeout", hs.timeout.String()))

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

// NewWsServer 创建WebSocket服务器
// connUniquenessMode: 连接唯一性模式
//   - SubjectUnique: 一个用户只能有一个连接（适用于单设备应用）
//   - SubjectDeviceUnique: 一个用户可以在多个设备上连接（适用于多设备场景）
func NewWsServer(connUniquenessMode ConnectionUniquenessMode) *WsServer {
	globalCtx, globalCancel := context.WithCancel(context.Background())

	s := &WsServer{
		routes:             make(map[string]*RouteInfo),
		globalCtx:          globalCtx,
		globalCancel:       globalCancel,
		connUniquenessMode: connUniquenessMode,
		idleTimeout:        3600 * time.Second, // 默认1小时空闲超时
		errorHandler:       &ErrorHandler{},
		configValidator:    &ConfigValidator{},
		metrics:            &WebSocketMetrics{startTime: time.Now()},
		upgrader: &fasthttpWs.FastHTTPUpgrader{
			ReadBufferSize:  1024 * 4,
			WriteBufferSize: 1024 * 4,
			CheckOrigin: func(ctx *fasthttp.RequestCtx) bool {
				// 生产环境应配置具体的Origin白名单
				return true
			},
		},
	}

	// 自动添加默认的连接路由处理器
	err := s.AddRouter(DefaultWsRoute, func(ctx context.Context, connCtx *ConnectionContext, body []byte) (interface{}, error) {
		// 默认处理器：简单回显接收到的数据
		return body, nil
	}, &RouterConfig{})
	if err != nil {
		// 这不应该发生，但如果发生，记录错误
		zlog.Error("failed_to_add_default_ws_router", 0, zlog.AddError(err))
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

	s.connManager = NewConnectionManager(maxConn, s.connUniquenessMode)
	s.limiter = rate.NewLimiter(rate.Limit(limit), bucket)

	pingDuration := time.Duration(ping) * time.Second
	s.heartbeatSvc = NewHeartbeatService(pingDuration, s.idleTimeout, s.connManager)
	return nil
}

// SetIdleTimeout 设置连接空闲超时时间
func (s *WsServer) SetIdleTimeout(timeout time.Duration) {
	s.idleTimeout = timeout
}

func (s *WsServer) StartWebsocket(addr string) error {
	if err := s.configValidator.validateServerConfig(addr, s.server, s.heartbeatSvc); err != nil {
		return err
	}

	zlog.Info("server_starting", 0, zlog.String("address", addr))

	// 启动心跳服务
	s.heartbeatSvc.Start()

	// 启动指标记录定时器（每分钟记录一次）
	go s.startMetricsLogger()

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

// GetMetrics 获取监控指标快照
func (s *WsServer) GetMetrics() *WebSocketMetrics {
	// 创建指标快照，避免并发访问问题
	metrics := &WebSocketMetrics{
		connectionsTotal:  s.metrics.connectionsTotal,
		connectionsActive: atomic.LoadInt64(&s.metrics.connectionsActive),
		connectionsPeak:   atomic.LoadInt64(&s.metrics.connectionsPeak),
		messagesTotal:     atomic.LoadInt64(&s.metrics.messagesTotal),
		messagesSuccess:   atomic.LoadInt64(&s.metrics.messagesSuccess),
		messagesError:     atomic.LoadInt64(&s.metrics.messagesError),
		heartbeatsTotal:   atomic.LoadInt64(&s.metrics.heartbeatsTotal),
		heartbeatsSuccess: atomic.LoadInt64(&s.metrics.heartbeatsSuccess),
		heartbeatsFailed:  atomic.LoadInt64(&s.metrics.heartbeatsFailed),
		uptimeSeconds:     int64(time.Since(s.metrics.startTime).Seconds()),
		startTime:         s.metrics.startTime,
	}
	return metrics
}

// recordConnectionAdded 记录连接增加指标
func (s *WsServer) recordConnectionAdded() {
	atomic.AddInt64(&s.metrics.connectionsTotal, 1)
	active := atomic.AddInt64(&s.metrics.connectionsActive, 1)

	// 更新峰值
	for {
		peak := atomic.LoadInt64(&s.metrics.connectionsPeak)
		if active <= peak || atomic.CompareAndSwapInt64(&s.metrics.connectionsPeak, peak, active) {
			break
		}
	}
}

// recordConnectionRemoved 记录连接移除指标
func (s *WsServer) recordConnectionRemoved() {
	atomic.AddInt64(&s.metrics.connectionsActive, -1)
}

// recordMessageProcessed 记录消息处理指标
func (s *WsServer) recordMessageProcessed(success bool) {
	atomic.AddInt64(&s.metrics.messagesTotal, 1)
	if success {
		atomic.AddInt64(&s.metrics.messagesSuccess, 1)
	} else {
		atomic.AddInt64(&s.metrics.messagesError, 1)
	}
}

// recordHeartbeat 记录心跳指标
func (s *WsServer) recordHeartbeat(success bool) {
	atomic.AddInt64(&s.metrics.heartbeatsTotal, 1)
	if success {
		atomic.AddInt64(&s.metrics.heartbeatsSuccess, 1)
	} else {
		atomic.AddInt64(&s.metrics.heartbeatsFailed, 1)
	}
}

// startMetricsLogger 启动指标记录定时器
func (s *WsServer) startMetricsLogger() {
	ticker := time.NewTicker(60 * time.Second) // 每60秒记录一次指标
	defer ticker.Stop()

	for {
		select {
		case <-s.globalCtx.Done():
			return
		case <-ticker.C:
			s.LogMetrics()
		}
	}
}

// LogMetrics 记录当前监控指标到日志
func (s *WsServer) LogMetrics() {
	metrics := s.GetMetrics()

	// 计算一些派生指标
	var messageSuccessRate, heartbeatSuccessRate float64
	if metrics.messagesTotal > 0 {
		messageSuccessRate = float64(metrics.messagesSuccess) / float64(metrics.messagesTotal) * 100
	}
	if metrics.heartbeatsTotal > 0 {
		heartbeatSuccessRate = float64(metrics.heartbeatsSuccess) / float64(metrics.heartbeatsTotal) * 100
	}

	zlog.Info("websocket_server_metrics", 0,
		zlog.Int64("connections_total", metrics.connectionsTotal),
		zlog.Int64("connections_active", metrics.connectionsActive),
		zlog.Int64("connections_peak", metrics.connectionsPeak),
		zlog.Int64("messages_total", metrics.messagesTotal),
		zlog.Int64("messages_success", metrics.messagesSuccess),
		zlog.Int64("messages_error", metrics.messagesError),
		zlog.Float64("message_success_rate", messageSuccessRate),
		zlog.Int64("heartbeats_total", metrics.heartbeatsTotal),
		zlog.Int64("heartbeats_success", metrics.heartbeatsSuccess),
		zlog.Int64("heartbeats_failed", metrics.heartbeatsFailed),
		zlog.Float64("heartbeat_success_rate", heartbeatSuccessRate),
		zlog.Int64("uptime_seconds", metrics.uptimeSeconds))
}

// StopWebsocket 停止WebSocket服务器
func (s *WsServer) StopWebsocket() error {
	if zlog.IsDebug() {
		zlog.Debug("stopping websocket server...", 0)
	}

	// 停止心跳服务
	if s.heartbeatSvc != nil {
		s.heartbeatSvc.Stop()
	}

	// 取消全局上下文
	if s.globalCancel != nil {
		s.globalCancel()
	}

	// 停止HTTP服务器
	if s.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.server.ShutdownWithContext(ctx); err != nil {
			zlog.Error("error shutting down server", 0, zlog.AddError(err))
			return err
		}
	}

	// 连接管理器的清理会在上下文取消时自动处理

	zlog.Info("websocket server stopped", 0)
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

	// 获取默认处理器（直接从默认路由获取，避免遍历）
	routeInfo, exists := s.routes[DefaultWsRoute]
	if !exists || routeInfo.Handle == nil {
		zlog.Error("NO_DEFAULT_ROUTE_CONFIGURED", 0, zlog.String("expected_route", DefaultWsRoute))
		ctx.SetStatusCode(fasthttp.StatusInternalServerError)
		return
	}
	defaultHandle := routeInfo.Handle

	zlog.Info("WEBSOCKET_UPGRADE_ATTEMPT", 0,
		zlog.String("path", path))

	if s.limiter != nil && !s.limiter.Allow() {
		ctx.SetStatusCode(fasthttp.StatusTooManyRequests)
		ctx.SetBodyString("too many connections")
		return
	}

	if err := s.upgrader.Upgrade(ctx, func(ws *fasthttpWs.Conn) {
		// 使用默认配置，因为路由现在在消息级别确定
		defaultConfig := &RouterConfig{}
		connCtx, err := s.initializeConnection(ctx, ws, path, defaultConfig)
		if err != nil {
			zlog.Error("failed to initialize connection", 0, zlog.AddError(err))
			ws.Close() // 关闭WebSocket连接
			return
		}
		defer s.cleanupConnection(connCtx)

		// 使用统一的处理器，实际路由在消息处理时根据Router字段确定
		s.handleConnectionLoop(connCtx, defaultHandle)
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

	if zlog.IsDebug() {
		zlog.Debug("WEBSOCKET_CONNECTION_INIT", 0,
			zlog.String("connection_path", path))
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
		Sub:    subID,
		Dev:    devID,
		Last:   utils.UnixSecond(),
		Conn:   ws,
		ctx:    connCtx,
		cancel: cancel, // 保存取消函数，用于终止相关goroutine
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
	// 记录连接建立指标
	s.recordConnectionAdded()
	// 确保在函数退出时（无论何种原因）记录连接移除指标
	defer s.recordConnectionRemoved()

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
				// 记录消息处理失败指标
				s.recordMessageProcessed(false)
				s.errorHandler.handleConnectionError(connCtx, err, "process_message")
				continue
			}

			// 记录消息处理成功指标
			s.recordMessageProcessed(true)

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
						utils.Str2Bytes(utils.AddStr(jsonResp.Time, jsonResp.Nonce, jsonResp.Plan, connCtx.JsonBody.Router)))
					if err != nil {
						zlog.Error("response_data_encrypt_failed", 0, zlog.AddError(err))
						continue
					}
					jsonResp.Data = encryptedData
				} else {
					jsonResp.Data = utils.Base64Encode(utils.Bytes2Str(respData))
				}

				// 生成响应签名
				signStr := utils.AddStr(connCtx.JsonBody.Router, jsonResp.Data, jsonResp.Nonce, jsonResp.Time, jsonResp.Plan)
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
	deviceKey := utils.AddStr(connCtx.DevConn.Sub, "_", connCtx.DevConn.Dev)
	s.connManager.Remove(connCtx.DevConn.Sub, deviceKey)
	zlog.Info("client_disconnected", 0, zlog.String("subject", deviceKey))
}

func (s *WsServer) gracefulShutdown() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	zlog.Info("initiating graceful shutdown...", 0)

	// 统一使用带上下文的 ShutdownWithContext，设置 5 秒超时
	if s.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.server.ShutdownWithContext(ctx); err != nil {
			zlog.Error("error shutting down server", 0, zlog.AddError(err))
		}
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

	// 先取消上下文，终止相关goroutine
	if conn.cancel != nil {
		conn.cancel()
	}

	// 再关闭WebSocket连接
	if conn.Conn != nil {
		closeMsg := fasthttpWs.FormatCloseMessage(fasthttpWs.CloseNormalClosure, reason)
		// 使用带超时的写控制
		conn.Conn.WriteControl(fasthttpWs.CloseMessage, closeMsg, time.Now().Add(1*time.Second))
		conn.Conn.Close()
	}
}
