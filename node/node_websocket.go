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

	DIC "github.com/godaddy-x/freego/common"
	"github.com/godaddy-x/freego/zlog"

	"github.com/fasthttp/websocket"
	fasthttpWs "github.com/fasthttp/websocket"
	"github.com/godaddy-x/freego/cache"
	rate "github.com/godaddy-x/freego/cache/limiter"
	"github.com/godaddy-x/freego/ex"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/utils/crypto"
	"github.com/godaddy-x/freego/utils/jwt"
	"github.com/valyala/fasthttp"
)

// WebSocket服务器实现
//
// 错误类型约定：
//   - 协议错误（客户端请求不合法、鉴权失败等）：使用 ex.Throw{Code, Msg[, Err]}，便于上层按 HTTP 状态码处理或回写。
//   - 配置/内部错误（校验配置、连接池、调用方参数等）：使用 utils.Error，仅作日志或返回给调用方，无状态码需求。
//
// WebSocket专用常量
const (
	DefaultWsRoute      = "/ws"       // 默认WebSocket路由路径
	DefaultWsMaxBodyLen = 1024 * 1024 // 默认单条消息体最大 1MB，可通过 WsServer.SetMaxBodyLen 覆盖
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
// Handle 业务处理函数，返回 nil 则不回复。
// 对象池约束：返回的 *JsonResp 等由框架在 replyData 内统一 Put 回池；handler 不得在返回后继续持有或异步使用 connCtx.JsonBody/GetJsonResp 等池对象，应同步处理完毕。
type Handle func(ctx context.Context, connCtx *ConnectionContext, body []byte) (interface{}, error)

// ConnectionContext 每个WebSocket连接的上下文，包含连接相关的所有信息。
//
// 设计要点：JsonBody 由 processMessage 从对象池获取并注入，业务 Handle 只读使用，不得在返回后继续持有。
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

// getDeviceID 获取设备 ID，用于日志上下文
func (cc *ConnectionContext) getDeviceID() string {
	if cc.DevConn != nil {
		return cc.DevConn.Dev
	}
	if cc.Subject != nil {
		return cc.Subject.GetDev(nil)
	}
	return ""
}

// GetTokenSecret 获取WebSocket连接的 token 派生密钥（每次调用重新派生，不缓存）。
// 为保证安全需在用毕后 DIC.ClearData(secret)；为此接受每次 HMAC 派生的性能损耗。
func (cc *ConnectionContext) GetTokenSecret() []byte {
	if cc.Subject == nil || len(cc.GetRawTokenBytes()) == 0 || cc.Server == nil {
		return nil
	}
	return cc.Subject.GetTokenSecret(utils.Bytes2Str(cc.GetRawTokenBytes()), cc.Server.jwtConfig.TokenKey)
}

// ConnectionManager 连接管理器：线程安全的连接管理，支持广播、按 subject 推送、过期清理。
//
// 设计要点：
// - conns：二级 map subject -> deviceKey -> *DevConn，deviceKey 由 mode 决定（SubjectUnique 时为 sub，SubjectDeviceUnique 时为 sub_dev）。
// - totalConn：原子计数，用于限流、Count()、以及 CleanupExpired/sendToSubjectByJsonResp 的 slice 预分配容量，避免在 RLock 内做重逻辑。
// - 所有“收集 conn 再关闭/发送”的路径均在 RLock 内只收集指针，在锁外执行 I/O，避免持锁时间过长。
type ConnectionManager struct {
	mu           sync.RWMutex
	conns        map[string]map[string]*DevConn // subject -> deviceKey -> connection
	max          int                            // 最大连接数
	totalConn    int32                          // 原子计数器：当前总连接数（限流 + 预分配容量）
	mode         ConnectionUniquenessMode       // 连接唯一性模式
	broadcastKey string                         // 广播密钥
}

// MessageHandler 消息处理器：统一处理消息校验、解码、路由
type MessageHandler struct {
	cipher map[int64]crypto.Cipher
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

// DevConn 设备连接实体：存储单连接的核心信息。
//
// 设计要点：
// - Last：使用原子读写（UpdateLast 写、LastSeen 读），使 CleanupExpired 遍历时无需对每条连接加 sendMu，降低高连接数下的锁竞争。
// - sendMu：仅保护 Conn 的写与 closed 的可见性，Send/关闭路径使用；Last 不再依赖 sendMu。
// - closeOnce：保证 ws.Close() 只执行一次，避免重复关闭导致 panic。
type DevConn struct {
	Sub       string
	Dev       string
	Last      int64            // 最后活跃时间戳，原子读写，供 CleanupExpired 无锁判断
	Conn      *fasthttpWs.Conn // WebSocket连接
	sendMu    sync.Mutex       // 发送与关闭路径互斥，避免并发写与空指针
	ctx       context.Context
	cancel    context.CancelFunc
	closed    int32     // 0=未关闭，1=已关闭，原子读写
	closeOnce sync.Once // 确保 ws.Close() 只执行一次
}

// UpdateLast 更新连接最后活跃时间。原子写，无锁，便于消息循环中高频调用且不影响 CleanupExpired 遍历。
func (dc *DevConn) UpdateLast() {
	if dc == nil {
		return
	}
	if atomic.LoadInt32(&dc.closed) == 0 {
		atomic.StoreInt64(&dc.Last, utils.UnixSecond())
	}
}

// LastSeen 返回最近一次活跃时间。原子读、无锁，供 CleanupExpired 在 RLock 内批量调用而不产生 sendMu 竞争。
func (dc *DevConn) LastSeen() int64 {
	if dc == nil {
		return 0
	}
	return atomic.LoadInt64(&dc.Last)
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

	broadcastKey string

	// 配置项
	ping         int           // 心跳间隔（秒）
	maxConn      int           // 最大连接数
	maxBodyLen   int           // 单条消息体最大长度（字节），默认 DefaultWsMaxBodyLen
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

	// validateTokenPerMessage：false=仅建连时校验 token，连接期间过期不踢线；true=每条消息校验 exp，过期即 401，适合强安全/合规。
	validateTokenPerMessage bool

	// ECC和缓存配置（用于Plan 2）
	// 8字节函数指针字段组 (5个字段，40字节)
	Cipher          map[int64]crypto.Cipher                 // 8字节 - RSA/ECDSA加密解密对象列表
	RedisCacheAware func(ds ...string) (cache.Cache, error) // 8字节 - 函数指针
	LocalCacheAware func(ds ...string) (cache.Cache, error) // 8字节 - 函数指针

	// 用于确保 Stop 只执行一次
	shutdownOnce sync.Once
	// 确保信号监听只注册一次，避免重复调用 StartWebsocket 时重复 Notify
	signalOnce sync.Once
}

// ErrorHandler WebSocket错误处理器（统一错误处理）
type ErrorHandler struct {
}

func (eh *ErrorHandler) handleConnectionError(connCtx *ConnectionContext, err error, operation string) {
	// 准备上下文信息
	userID := connCtx.GetUserIDString()
	remoteAddr := ""
	if connCtx.WsConn != nil {
		remoteAddr = connCtx.WsConn.RemoteAddr().String()
	}
	deviceID := ""
	if connCtx.DevConn != nil {
		deviceID = connCtx.DevConn.Dev
	}
	connectionPath := connCtx.Path

	// 添加更多上下文信息到错误日志
	zlog.Error(operation+"_failed", 0,
		zlog.AddError(err),
		zlog.String("operation", operation),
		zlog.String("user_id", userID),
		zlog.String("remote_addr", remoteAddr),
		zlog.String("device_id", deviceID),
		zlog.String("connection_path", connectionPath))

	// 尝试发送错误响应
	if ws := connCtx.WsConn; ws != nil {
		resp := GetJsonResp()
		resp.Code = ex.WS_SEND
		resp.Message = "websocket error: " + operation
		resp.Time = utils.UnixSecond()

		if connCtx.Subject == nil || connCtx.Subject.Payload == nil || len(connCtx.Subject.Payload.Sub) == 0 {
			resp.Nonce = utils.GetUUID(true)
		} else if connCtx.JsonBody != nil {
			resp.Nonce = connCtx.JsonBody.Nonce
		}

		result, marshalErr := utils.JsonMarshal(resp)
		PutJsonResp(resp)
		if marshalErr == nil {
			if connCtx.DevConn != nil {
				if err := connCtx.DevConn.Send(result); err != nil {
					if zlog.IsDebug() {
						zlog.Debug("failed to send error response to closed connection", 0, zlog.AddError(err))
					}
				}
			} else if err := ws.WriteMessage(fasthttpWs.TextMessage, result); err != nil {
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

// -------------------------- ConnectionManager 实现 --------------------------

// NewConnectionManager 创建连接管理器
func NewConnectionManager(maxConn int, mode ConnectionUniquenessMode, broadcastKey string) *ConnectionManager {
	return &ConnectionManager{
		conns:        make(map[string]map[string]*DevConn),
		max:          maxConn,
		mode:         mode,
		broadcastKey: broadcastKey,
	}
}

// Add 添加连接
// Add 将连接加入管理器。根据连接唯一性模式决定策略：
// - SubjectUnique: 替换同 subject 的所有连接，只保留一个。
// - SubjectDeviceUnique: 替换同 subject+device 的连接，允许多设备同时在线。
// 设备键格式：subject_device (如: user123_web, user123_app)
//
// 设计要点：若存在旧连接，先从 map 移除并减 totalConn，再在 goroutine 中 closeConn，避免锁内 I/O 阻塞。
func (cm *ConnectionManager) Add(conn *DevConn) error {
	if cm == nil || cm.conns == nil || conn == nil {
		return utils.Error("invalid connection or manager")
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()

	currentTotal := atomic.LoadInt32(&cm.totalConn)
	if currentTotal >= int32(cm.max) {
		return utils.Error("connection pool full", currentTotal)
	}

	var uniqueKey string
	if cm.mode == SubjectUnique {
		uniqueKey = conn.Sub
	} else {
		uniqueKey = utils.AddStr(conn.Sub, "_", conn.Dev)
	}

	if cm.conns[conn.Sub] == nil {
		cm.conns[conn.Sub] = make(map[string]*DevConn)
	}

	// 替换旧连接：先移除引用并减计数，再在锁外关闭，避免锁内 I/O
	if oldConn, exists := cm.conns[conn.Sub][uniqueKey]; exists {
		delete(cm.conns[conn.Sub], uniqueKey)
		atomic.AddInt32(&cm.totalConn, -1)
		go func() {
			closeConn(oldConn, "replaced by new connection")
		}()
	}

	cm.conns[conn.Sub][uniqueKey] = conn
	atomic.AddInt32(&cm.totalConn, 1)
	return nil
}

// Remove 移除连接（不关闭，仅从管理器移除）
func (cm *ConnectionManager) Remove(subject, deviceKey string) *DevConn {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if subjectConns, exists := cm.conns[subject]; exists {
		if conn, exists := subjectConns[deviceKey]; exists {
			delete(subjectConns, deviceKey)
			atomic.AddInt32(&cm.totalConn, -1)
			if len(subjectConns) == 0 {
				delete(cm.conns, subject)
			}
			return conn
		}
	}
	return nil
}

// RemoveByConn 按连接指针从管理器中移除该连接。
// 设计要点：必须用指针精确匹配，避免“新连接已替换旧连接”时误把新连接从 map 删掉；关闭由 closeConnFromLoop 等调用方负责。
// 返回是否成功移除（若连接已被替换则未找到，返回 false）。
func (cm *ConnectionManager) RemoveByConn(conn *DevConn) bool {
	if conn == nil {
		return false
	}
	cm.mu.Lock()
	defer cm.mu.Unlock()
	for subject, subjectConns := range cm.conns {
		for deviceKey, c := range subjectConns {
			if c == conn {
				delete(subjectConns, deviceKey)
				atomic.AddInt32(&cm.totalConn, -1)
				if len(subjectConns) == 0 {
					delete(cm.conns, subject)
				}
				return true
			}
		}
	}
	return false
}

// GetAllSubjectDevices 获取所有用户连接subject
func (cm *ConnectionManager) GetAllSubjectDevices() map[string][]string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	result := make(map[string][]string, len(cm.conns))
	for k, v := range cm.conns {
		vs := make([]string, 0, len(v))
		for _, dev := range v {
			vs = append(vs, dev.Dev)
		}
		result[k] = vs
	}
	return result
}

// GetSubjectDevices
func (cm *ConnectionManager) GetSubjectDevices(subject string) map[string][]string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	if subject == "" {
		return nil
	}
	pick, b := cm.conns[subject]
	if !b {
		return nil
	}
	result := make(map[string][]string, 1)
	vs := make([]string, 0, len(pick))
	for _, dev := range pick {
		vs = append(vs, dev.Dev)
	}
	result[subject] = vs
	return result
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

// Broadcast 广播消息到所有连接。
// 连接活性由 LastSeen 与 CleanupExpired 维护；Send 内部会检查 closed 并安全返回错误。
func (cm *ConnectionManager) Broadcast(data []byte) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	for _, subjectConns := range cm.conns {
		for _, conn := range subjectConns {
			_ = conn.Send(data)
		}
	}
}

// SendToSubject 发送消息到指定主题的所有连接
func (cm *ConnectionManager) SendToSubject(subject, router string, data interface{}) error {
	if router == "" {
		return utils.Error("router is nil")
	}
	if data == nil {
		return utils.Error("data is nil")
	}

	// 构造JsonResp格式的推送消息
	jsonResp := GetJsonResp()

	jsonResp.Code = 300 // 推送消息使用特殊code值300
	jsonResp.Message = "push"
	jsonResp.Router = router
	jsonResp.Time = utils.UnixSecond()

	// 序列化数据
	dataBytes, err := utils.JsonMarshal(data)
	if err != nil {
		PutJsonResp(jsonResp)
		return utils.Error("data marshal failed: ", err.Error())
	}

	// 推送消息采用明文传输（Plan=0）
	jsonResp.Data = utils.Base64Encode(dataBytes)
	jsonResp.Plan = 0

	// 推送消息也需要Nonce，便于追踪
	pushNonce := utils.GetUUID(true)
	jsonResp.Nonce = pushNonce

	// 推送消息签名：与心跳响应不同，推送消息使用服务器专用密钥
	// 因为推送消息没有用户请求上下文，无法使用connCtx.GetTokenSecret()
	// 客户端通过预共享的服务器密钥进行验签
	serverPushKey := utils.Str2Bytes(cm.broadcastKey) // 建议从配置中获取
	signData := SignBodyMessage(router, jsonResp.Data, pushNonce, jsonResp.Time, jsonResp.Plan, 0, serverPushKey)
	jsonResp.Sign = utils.Base64Encode(signData)

	// 发送构造好的JsonResp
	err = cm.sendToSubjectByJsonResp(subject, jsonResp)
	PutJsonResp(jsonResp)
	return err
}

// sendToSubjectByJsonResp 发送结构化消息到指定主题的所有连接
func (cm *ConnectionManager) sendToSubjectByJsonResp(subject string, res *JsonResp) error {
	if res.Router == "" {
		return utils.Error("res.router is nil")
	}
	if res.Data == "" {
		return utils.Error("res.data is nil")
	}
	data, err := utils.JsonMarshal(res)
	if err != nil {
		return err
	}

	// 按 totalConn 预分配，减少 append 扩容；RLock 内只收集指针，锁外再 Send，避免持锁 I/O
	n := atomic.LoadInt32(&cm.totalConn)
	if n < 0 {
		n = 0
	}
	targets := make([]*DevConn, 0, n)
	cm.mu.RLock()
	if subject != "" {
		if subjectConns, exists := cm.conns[subject]; exists {
			for _, conn := range subjectConns {
				targets = append(targets, conn)
			}
		}
	} else {
		for _, conn := range cm.conns {
			for _, v := range conn {
				targets = append(targets, v)
			}
		}
	}
	cm.mu.RUnlock()

	for _, conn := range targets {
		if err := conn.Send(data); err != nil && zlog.IsDebug() {
			zlog.Debug("push send error", 0, zlog.String("subject", subject), zlog.String("errMsg", err.Error()))
		}
	}
	return nil
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
func (cm *ConnectionManager) RemoveWithCallback(subject, deviceKey string, callback func()) *DevConn {
	return cm.Remove(subject, deviceKey) // callback 由调用方在锁外执行
}

// HealthCheck 健康检查：返回每个 subject 的连接数。
// 活性由 LastSeen 与 CleanupExpired 维护。
func (cm *ConnectionManager) HealthCheck() map[string]int {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	stats := make(map[string]int, len(cm.conns))
	for subject, conns := range cm.conns {
		stats[subject] = len(conns)
	}
	return stats
}

// AddTestConnection 添加测试连接（仅用于单元测试）
func (cm *ConnectionManager) AddTestConnection(subject, deviceKey string, conn *DevConn) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.conns == nil {
		cm.conns = make(map[string]map[string]*DevConn)
	}
	if cm.conns[subject] == nil {
		cm.conns[subject] = make(map[string]*DevConn)
	}
	cm.conns[subject][deviceKey] = conn
}

// CleanupExpired 清理空闲超过 timeoutSeconds 的连接。
//
// 设计要点：
// - 在 RLock 内仅收集过期 conn 指针（LastSeen 为原子读无锁），RUnlock 后再 closeConn，避免持锁做 I/O。
// - toClose 按 totalConn 预分配容量，减少 append 扩容与 GC。
// - 超时判断依赖 DevConn.Last（每次收包/心跳 UpdateLast），由 HeartbeatService 按 idleTimeout 周期性调用。
func (cm *ConnectionManager) CleanupExpired(timeoutSeconds int64) int {
	cleaned := 0
	currentTime := utils.UnixSecond()
	n := atomic.LoadInt32(&cm.totalConn)
	if n < 0 {
		n = 0
	}
	cm.mu.RLock()
	toClose := make([]*DevConn, 0, n)
	for _, subjectConns := range cm.conns {
		for _, conn := range subjectConns {
			if currentTime-conn.LastSeen() > timeoutSeconds {
				toClose = append(toClose, conn)
			}
		}
	}
	cm.mu.RUnlock()

	for _, conn := range toClose {
		closeConn(conn, "expired")
		cleaned++
	}
	return cleaned
}

// CleanupAll 关闭所有连接。先 RLock 内收集全部 conn 指针并预分配 slice，锁外再统一 closeConn，由各连接循环执行 RemoveByConn + ws.Close()。
func (cm *ConnectionManager) CleanupAll() {
	n := atomic.LoadInt32(&cm.totalConn)
	if n < 0 {
		n = 0
	}
	cm.mu.RLock()
	toClose := make([]*DevConn, 0, n)
	for _, subjectConns := range cm.conns {
		for _, conn := range subjectConns {
			toClose = append(toClose, conn)
		}
	}
	cm.mu.RUnlock()

	for _, conn := range toClose {
		closeConn(conn, "cleanup all")
	}
}

// Count 获取当前连接数
func (cm *ConnectionManager) Count() int {
	return int(atomic.LoadInt32(&cm.totalConn))
}

// -------------------------- MessageHandler 实现 --------------------------

func NewMessageHandler(cipher map[int64]crypto.Cipher, handle Handle) *MessageHandler {
	return &MessageHandler{
		cipher: cipher,
		handle: handle,
	}
}

// validateMessageSize 验证消息大小，防止恶意消息攻击，使用 WsServer 配置的 maxBodyLen
func (mh *MessageHandler) validateMessageSize(connCtx *ConnectionContext, body []byte) error {
	maxLen := DefaultWsMaxBodyLen
	if connCtx != nil && connCtx.Server != nil {
		maxLen = connCtx.Server.maxBodyLen
	}
	if len(body) > maxLen {
		return ex.Throw{Code: fasthttp.StatusRequestEntityTooLarge, Msg: "message too large"}
	}
	return nil
}

func (mh *MessageHandler) Process(connCtx *ConnectionContext, body []byte) (crypto.Cipher, interface{}, error) {
	// 验证消息大小，防止恶意消息攻击
	if err := mh.validateMessageSize(connCtx, body); err != nil {
		return nil, nil, err
	}

	// 解析WebSocket消息体
	if err := utils.JsonUnmarshal(body, connCtx.JsonBody); err != nil {
		// 解析失败时释放对象回到池中
		zlog.Error("websocket message json unmarshal failed", 0, zlog.ByteString("raw_body", body), zlog.AddError(err))
		return nil, nil, ex.Throw{Code: fasthttp.StatusBadRequest, Msg: "invalid JSON format"}
	}

	// 检查是否是心跳包（用于失败时的指标记录）
	isHeartbeat := connCtx.JsonBody.Router == "/ws/ping"

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

	// 心跳包 /ws/ping：只更新 Last 与指标，不返回 PONG。
	// 设计要点：服务端不回 pong，降低服务端写压力；连接活性靠客户端定时 ping + 服务端读超时与 CleanupExpired 判定。
	if connCtx.JsonBody.Router == "/ws/ping" {
		connCtx.DevConn.UpdateLast()
		connCtx.Server.recordHeartbeat(true)

		if zlog.IsDebug() {
			zlog.Debug("heartbeat_received_and_updated", 0,
				zlog.String("subject", connCtx.Subject.GetSub(nil)),
				zlog.String("device", connCtx.Subject.GetDev(nil)),
				zlog.String("connection_path", connCtx.Path),
				zlog.String("nonce", connCtx.JsonBody.Nonce))
		}

		return cipher, nil, nil
	}

	// 解密业务数据
	bizData, err := mh.decryptWebSocketData(connCtx)
	if err != nil {
		zlog.Error("websocket data decryption failed", 0, zlog.AddError(err))
		return nil, nil, err
	}

	// 根据消息中的路由选择处理器
	handle := mh.handle // 默认处理器
	if connCtx.JsonBody.Router != "" {
		if routeInfo, exists := connCtx.Server.routes[connCtx.JsonBody.Router]; exists {
			handle = routeInfo.Handle
			if zlog.IsDebug() {
				zlog.Debug("using route-specific handler", 0, zlog.String("router", connCtx.JsonBody.Router))
			}
		} else {
			zlog.Warn("no handler found for router, using default", 0, zlog.String("router", connCtx.JsonBody.Router))
		}
	}

	result, err := handle(connCtx.ctx, connCtx, bizData)
	return cipher, result, err
}

func (self *MessageHandler) CheckECDSASign(usr int64, msg, sign []byte) (crypto.Cipher, error) {
	cipher, exists := self.cipher[usr]
	if !exists || cipher == nil {
		return nil, ex.Throw{Code: http.StatusBadRequest, Msg: "cipher not found for user, bidirectional ECDSA signature is required"}
	}
	if err := cipher.Verify(msg, sign); err != nil {
		return nil, ex.Throw{Code: http.StatusBadRequest, Msg: "request signature valid invalid"}
	}
	return cipher, nil
}

// validWebSocketBody 验证 WebSocket 消息体（签名、时间窗、可选 token 有效期）。
func (mh *MessageHandler) validWebSocketBody(connCtx *ConnectionContext) (crypto.Cipher, error) {
	body := connCtx.JsonBody
	d := body.Data
	if len(d) == 0 {
		return nil, ex.Throw{Code: fasthttp.StatusBadRequest, Msg: "websocket data is nil"}
	}

	// 只支持 Plan 0（明文）和 1（AES），不再支持 Plan 2
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

	// 可选：每条消息校验 token 有效期（与 jwt.Verify 一致：提前 15 秒视为过期）
	if connCtx.Server.validateTokenPerMessage {
		if connCtx.Subject == nil || connCtx.Subject.Payload == nil {
			return nil, ex.Throw{Code: fasthttp.StatusUnauthorized, Msg: "token not ready"}
		}
		if connCtx.Subject.Payload.Exp <= utils.UnixSecond()-15 {
			return nil, ex.Throw{Code: fasthttp.StatusUnauthorized, Msg: "token expired or invalid"}
		}
	}

	var sharedKey []byte

	// Plan 0/1使用token secret，用毕清理
	sharedKey = connCtx.GetTokenSecret()
	if len(sharedKey) == 0 {
		return nil, ex.Throw{Code: fasthttp.StatusBadRequest, Msg: "websocket secret is nil"}
	}
	defer DIC.ClearData(sharedKey)

	// 构建签名字符串
	// 使用从header获取的路由标识进行签名验证，支持通过header指定路由
	sign := SignBodyMessage(body.Router, d, body.Nonce, body.Time, body.Plan, body.User, sharedKey)
	if utils.Base64Encode(sign) != body.Sign {
		return nil, ex.Throw{Code: fasthttp.StatusUnauthorized, Msg: "websocket signature verify invalid"}
	}

	cipher, err := mh.CheckECDSASign(body.User, sign, utils.Base64Decode(body.Valid))
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
		rawData, err := utils.AesGCMDecryptBase(d, secret[:32], AppendBodyMessage(body.Router, "", body.Nonce, body.Time, body.Plan, body.User))
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

// run 周期性执行：按 idleTimeout 清理空闲连接，不在锁内做 I/O。
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

// Send 向连接写入一条文本消息。所有对 Conn 与 closed 的访问均在 sendMu 内，避免与 closeConnFromLoop 竞态导致空指针或重复写。
func (dc *DevConn) Send(data []byte) error {
	if dc == nil {
		return utils.Error("connection is closed")
	}
	dc.sendMu.Lock()
	defer dc.sendMu.Unlock()
	if atomic.LoadInt32(&dc.closed) == 1 || dc.Conn == nil {
		return utils.Error("connection is closed")
	}
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
		maxBodyLen:         DefaultWsMaxBodyLen,
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
		MaxRequestBodySize: s.maxBodyLen,
	}

	return s
}

// AddRouter 注册路由：path -> Handle。应在 StartWebsocket 之前完成所有注册。
// 不支持在服务启动后动态添加：消息循环中会无锁读 s.routes，动态添加会产生并发读写风险。
func (s *WsServer) AddRouter(path string, handle Handle, routerConfig *RouterConfig) error {
	if err := s.configValidator.validateRouterConfig(path, handle); err != nil {
		return err
	}

	if routerConfig == nil {
		routerConfig = &RouterConfig{}
	}

	s.routes[path] = &RouteInfo{
		Handle:       handle,
		RouterConfig: routerConfig,
	}
	return nil
}

// maxConn   = 300          // 允许的最大并发连接数
// limit     = 20           // 每秒允许的平均消息数（令牌桶速率）
// bucket    = 100          // 令牌桶容量（突发消息缓冲）
// ping      = 15           // 心跳间隔（秒）
func (s *WsServer) NewPool(maxConn, limit, bucket, ping int) error {
	if err := s.configValidator.validatePoolConfig(maxConn, limit, bucket, ping); err != nil {
		return err
	}

	s.maxConn = maxConn
	s.ping = ping

	s.connManager = NewConnectionManager(maxConn, s.connUniquenessMode, s.broadcastKey)
	s.limiter = rate.NewLimiter(rate.Limit(limit), bucket)

	pingDuration := time.Duration(ping) * time.Second
	s.heartbeatSvc = NewHeartbeatService(pingDuration, s.idleTimeout, s.connManager)
	return nil
}

// SetIdleTimeout 设置连接空闲超时时间
func (s *WsServer) SetIdleTimeout(timeout time.Duration) {
	s.idleTimeout = timeout
}

// SetValidateTokenPerMessage 设置是否在每条消息时校验 token 有效期（validWebSocketBody 内生效）。
// false（默认）：仅建连时校验，连接期间 token 过期不踢线，性能更好；
// true：每条消息校验 exp，过期即 401，适合强安全/合规场景。
func (s *WsServer) SetValidateTokenPerMessage(validate bool) {
	s.validateTokenPerMessage = validate
}

// SetMaxBodyLen 设置单条消息体最大长度（字节），需在 StartWebsocket 前调用；若已创建 Server 则同步更新 MaxRequestBodySize
func (s *WsServer) SetMaxBodyLen(n int) {
	if n <= 0 {
		return
	}
	s.maxBodyLen = n
	if s.server != nil {
		s.server.MaxRequestBodySize = n
	}
}

// SetBroadcastKey 广播数据密钥
func (s *WsServer) SetBroadcastKey(key string) {
	s.broadcastKey = key
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

	// 监听中断信号（仅注册一次，避免重复调用 StartWebsocket 时重复 Notify）
	s.signalOnce.Do(func() { go s.gracefulShutdown() })

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

// GetMetrics 获取监控指标快照（所有计数器均用 atomic 读取）
func (s *WsServer) GetMetrics() *WebSocketMetrics {
	metrics := &WebSocketMetrics{
		connectionsTotal:  atomic.LoadInt64(&s.metrics.connectionsTotal),
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

// GetConnManager 获取连接管理器（用于测试）
func (s *WsServer) GetConnManager() *ConnectionManager {
	return s.connManager
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
	var err error
	s.shutdownOnce.Do(func() {
		zlog.Info("stopping websocket server...", 0)

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
			err = s.server.ShutdownWithContext(ctx)
			if err != nil {
				zlog.Error("error shutting down server", 0, zlog.AddError(err))
			}
		}

		zlog.Info("websocket server stopped", 0)
	})
	return err
}

// GetConnectionManager 获取连接管理器（用于健康检查等操作）
func (s *WsServer) GetConnectionManager() *ConnectionManager {
	return s.connManager
}

// StopWebsocketWithTimeout 带超时的优雅关闭
func (s *WsServer) StopWebsocketWithTimeout(timeout time.Duration) error {
	if zlog.IsDebug() {
		zlog.Debug("stopping websocket server with timeout...", 0, zlog.Duration("timeout", timeout))
	}

	// 停止心跳服务
	if s.heartbeatSvc != nil {
		s.heartbeatSvc.Stop()
	}

	// 通知所有连接优雅关闭
	done := make(chan struct{})
	go func() {
		defer close(done)
		if s.connManager != nil {
			s.connManager.CleanupAll()
		}
	}()

	// 等待连接关闭完成或超时
	select {
	case <-done:
		zlog.Info("all connections closed gracefully", 0)
	case <-time.After(timeout):
		zlog.Warn("graceful shutdown timeout, forcing close", 0, zlog.Duration("timeout", timeout))
	}

	// 执行标准关闭流程
	return s.StopWebsocket()
}

func (s *WsServer) serveHTTP(ctx *fasthttp.RequestCtx) {
	path := utils.Bytes2Str(ctx.Path())
	method := utils.Bytes2Str(ctx.Method())

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

// initializeConnection 建连时认证：校验 JWT（含过期）、CheckReady，创建 ConnectionContext 与 DevConn 并加入 connManager。
// 设计要点：token 有效期仅在此处校验；若未开启 validateTokenPerMessage，连接建立后 token 过期也不会被踢线。
func (s *WsServer) initializeConnection(httpCtx *fasthttp.RequestCtx, ws *fasthttpWs.Conn, path string, routerConfig *RouterConfig) (*ConnectionContext, error) {
	authHeader := httpCtx.Request.Header.Peek("Authorization")
	if zlog.IsDebug() {
		zlog.Debug("WEBSOCKET_UPGRADE_ATTEMPT", 0,
			zlog.String("path", path),
			zlog.String("client_ip", httpCtx.RemoteIP().String()),
			zlog.Bool("auth_header_present", len(authHeader) > 0))
	}

	if len(authHeader) == 0 {
		zlog.Error("MISSING_AUTH_HEADER", 0, zlog.String("client_ip", httpCtx.RemoteIP().String()))
		return nil, ex.Throw{Code: http.StatusUnauthorized, Msg: "missing authorization header"}
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
			return nil, ex.Throw{Code: http.StatusServiceUnavailable, Msg: "missing cache object", Err: err}
		}
	}

	subject.SetCache(c)

	if len(authHeader) == 0 {
		return nil, ex.Throw{Code: http.StatusUnauthorized, Msg: "empty authorization header"}
	}

	// 验证token (使用配置的JWT密钥)
	if err := subject.Verify(authHeader, s.jwtConfig.TokenKey); err != nil {
		return nil, ex.Throw{Code: http.StatusUnauthorized, Msg: "invalid or expired token", Err: err}
	}

	// 检查token是否有效
	if !subject.CheckReady() {
		return nil, ex.Throw{Code: http.StatusUnauthorized, Msg: "token not ready"}
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

	// Last 使用原子写入，与 UpdateLast/LastSeen 一致，便于 CleanupExpired 无锁读取
	now := utils.UnixSecond()
	devConn := &DevConn{
		Sub:    subID,
		Dev:    devID,
		Conn:   ws,
		ctx:    connCtx,
		cancel: cancel,
	}
	atomic.StoreInt64(&devConn.Last, now)

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

// handleConnectionLoop 单连接的消息循环：读消息 → 校验 → 路由 → 业务 → 回包；退出时从管理器移除并关闭连接。
// 设计要点：读超时按 ping*3 设置，每 N 条消息刷新一次以降低 time.Now 调用；UpdateLast 与 LastSeen 使用原子操作，避免与 CleanupExpired 争用 sendMu。
func (s *WsServer) handleConnectionLoop(connCtx *ConnectionContext, handle Handle) {
	s.recordConnectionAdded()
	defer s.recordConnectionRemoved()

	zlog.Info("CLIENT_CONNECTED", 0,
		zlog.String("client_address", connCtx.WsConn.RemoteAddr().String()),
		zlog.String("user_id", connCtx.Subject.GetSub(nil)),
		zlog.String("device_id", connCtx.Subject.GetDev(nil)))

	if zlog.IsDebug() {
		zlog.Debug("STARTING_MESSAGE_LOOP", 0, zlog.String("client", connCtx.WsConn.RemoteAddr().String()))
	}
	messageHandler := GetMessageHandler(s.Cipher, handle)
	defer PutMessageHandler(messageHandler) // 连接结束时释放MessageHandler

	// 读超时：ping*3 秒内无数据则断开；每 readDeadlineRefreshEvery 条消息才刷新一次 deadline，减少 time.Now() 与 SetReadDeadline 调用。
	readDeadlineDur := time.Duration(s.ping) * 3 * time.Second
	messageCount := 0
	const readDeadlineRefreshEvery = 32

	for {
		if err := connCtx.ctx.Err(); err != nil {
			if zlog.IsDebug() {
				zlog.Debug("connection context cancelled", 0,
					zlog.AddError(err),
					zlog.String("user_id", connCtx.GetUserIDString()),
					zlog.String("device_id", connCtx.getDeviceID()),
					zlog.String("connection_path", connCtx.Path))
			}
			return
		}

		if messageCount%readDeadlineRefreshEvery == 0 {
			connCtx.WsConn.SetReadDeadline(time.Now().Add(readDeadlineDur))
		}

		messageType, message, err := connCtx.WsConn.ReadMessage()
		if err != nil {
			if connCtx.ctx.Err() != nil || atomic.LoadInt32(&connCtx.DevConn.closed) == 1 {
				if zlog.IsDebug() {
					zlog.Debug("connection loop stopped after cancellation", 0,
						zlog.AddError(err),
						zlog.String("user_id", connCtx.GetUserIDString()),
						zlog.String("device_id", connCtx.getDeviceID()),
						zlog.String("connection_path", connCtx.Path))
				}
				return
			}

			zlog.Error("READ_MESSAGE_FAILED", 0,
				zlog.AddError(err),
				zlog.String("client", connCtx.WsConn.RemoteAddr().String()),
				zlog.String("user_id", connCtx.GetUserIDString()),
				zlog.String("device_id", connCtx.getDeviceID()),
				zlog.String("connection_path", connCtx.Path))
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				zlog.Error("read_message_error", 0,
					zlog.AddError(err),
					zlog.String("user_id", connCtx.GetUserIDString()),
					zlog.String("device_id", connCtx.getDeviceID()),
					zlog.String("connection_path", connCtx.Path))
			} else {
				zlog.Info("connection_closed_by_client", 0,
					zlog.AddError(err),
					zlog.String("user_id", connCtx.GetUserIDString()),
					zlog.String("device_id", connCtx.getDeviceID()),
					zlog.String("connection_path", connCtx.Path))
			}
			return
		}

		if zlog.IsDebug() {
			zlog.Debug("MESSAGE_RECEIVED", 0,
				zlog.Int("message_type", messageType),
				zlog.Int("message_length", len(message)),
				zlog.String("raw_message", string(message)),
				zlog.String("client", connCtx.WsConn.RemoteAddr().String()),
				zlog.String("user_id", connCtx.GetUserIDString()),
				zlog.String("device_id", connCtx.getDeviceID()),
				zlog.String("connection_path", connCtx.Path))
		}

		if messageType != fasthttpWs.TextMessage {
			zlog.Warn("unsupported_message_type", 0,
				zlog.Int("type", messageType),
				zlog.String("user_id", connCtx.GetUserIDString()),
				zlog.String("device_id", connCtx.getDeviceID()),
				zlog.String("connection_path", connCtx.Path))
			continue
		}

		connCtx.DevConn.UpdateLast()
		s.processMessage(messageHandler, connCtx, message)
		messageCount++
	}
}

// processMessage 单条消息处理：从池中取 JsonBody 注入 connCtx，校验→路由→业务→replyData，最后 Put 回池。
// 设计要点：JsonBody 仅在本调用栈内有效，业务 Handle 不得在返回后继续持有或异步使用。
func (s *WsServer) processMessage(messageHandler *MessageHandler, connCtx *ConnectionContext, message []byte) {
	jsonBody := GetJsonBody()
	connCtx.JsonBody = jsonBody
	defer PutJsonBody(jsonBody)

	if zlog.IsDebug() {
		zlog.Debug("processing message", 0,
			zlog.String("message", string(message)),
			zlog.String("user_id", connCtx.GetUserIDString()),
			zlog.String("device_id", connCtx.getDeviceID()),
			zlog.String("connection_path", connCtx.Path))
	}

	cipher, reply, err := messageHandler.Process(connCtx, message)
	if err != nil {
		// 记录消息处理失败指标
		s.recordMessageProcessed(false)
		s.errorHandler.handleConnectionError(connCtx, err, "process_message")
		return
	}

	// 记录消息处理成功指标
	s.recordMessageProcessed(true)

	replyData(connCtx, cipher, reply)
}

// replyData 将业务返回值写回客户端。
// 设计要点：若 Handle 返回 *JsonResp（如某些中间件已构造），直接序列化发送并 Put 回池；否则构造 Code=200 的 JsonResp，按 Plan 加密与签名后发送，所有 JsonResp 均由本函数或调用方 Put 回池。
func replyData(connCtx *ConnectionContext, cipher crypto.Cipher, reply interface{}) {
	if reply == nil {
		return
	}

	if jsonResp, ok := reply.(*JsonResp); ok {
		bytesData, err := utils.JsonMarshal(jsonResp)
		PutJsonResp(jsonResp)
		if err != nil {
			zlog.Error("failed_to_marshal_jsonresp", 0,
				zlog.AddError(err),
				zlog.String("user_id", connCtx.GetUserIDString()),
				zlog.String("device_id", connCtx.getDeviceID()),
				zlog.String("connection_path", connCtx.Path))
			return
		}

		if err := connCtx.DevConn.Send(bytesData); err != nil {
			zlog.Error("failed_to_send_response", 0,
				zlog.AddError(err),
				zlog.String("user_id", connCtx.GetUserIDString()),
				zlog.String("device_id", connCtx.getDeviceID()),
				zlog.String("connection_path", connCtx.Path))
		}
		return
	}

	// 原有的逻辑：构造新的JsonResp并序列化reply
	jsonResp := GetJsonResp()
	jsonResp.Code = 200
	jsonResp.Message = "success"
	jsonResp.Data = ""
	jsonResp.Nonce = connCtx.JsonBody.Nonce // 使用请求的nonce
	jsonResp.Time = utils.UnixSecond()
	jsonResp.Plan = connCtx.JsonBody.Plan // 使用请求的plan

	// 序列化响应数据
	respData, err := utils.JsonMarshal(reply)
	if err != nil {
		PutJsonResp(jsonResp)
		zlog.Error("failed_to_marshal_reply_data", 0,
			zlog.AddError(err),
			zlog.String("user_id", connCtx.GetUserIDString()),
			zlog.String("device_id", connCtx.getDeviceID()),
			zlog.String("connection_path", connCtx.Path))
		return
	}

	// 派生密钥：用于响应加密(Plan==1)与签名，用毕统一清理以降低驻留风险
	secret := connCtx.GetTokenSecret()
	if len(secret) == 0 {
		PutJsonResp(jsonResp)
		zlog.Error("response_secret_missing", 0,
			zlog.String("user_id", connCtx.GetUserIDString()),
			zlog.String("device_id", connCtx.getDeviceID()),
			zlog.String("connection_path", connCtx.Path))
		return
	}
	defer DIC.ClearData(secret)

	if connCtx.JsonBody.Plan == 1 {
		encryptedData, err := utils.AesGCMEncryptBase(respData, secret[:32], AppendBodyMessage(connCtx.JsonBody.Router, "", jsonResp.Nonce, jsonResp.Time, jsonResp.Plan, connCtx.JsonBody.User))
		if err != nil {
			PutJsonResp(jsonResp)
			zlog.Error("response_data_encrypt_failed", 0,
				zlog.AddError(err),
				zlog.String("user_id", connCtx.GetUserIDString()),
				zlog.String("device_id", connCtx.getDeviceID()),
				zlog.String("connection_path", connCtx.Path))
			return
		}
		jsonResp.Data = encryptedData
	} else {
		jsonResp.Data = utils.Base64Encode(utils.Bytes2Str(respData))
	}

	// 生成响应签名（使用同一 secret，避免二次派生）
	sign := SignBodyMessage(connCtx.JsonBody.Router, jsonResp.Data, jsonResp.Nonce, jsonResp.Time, jsonResp.Plan, connCtx.JsonBody.User, secret)
	jsonResp.Sign = utils.Base64Encode(sign)

	var validBytes []byte
	if cipher != nil {
		var err error
		validBytes, err = cipher.Sign(sign)
		if err != nil {
			PutJsonResp(jsonResp)
			zlog.Error("failed_to_ecdsa_sign_data", 0,
				zlog.AddError(err),
				zlog.String("user_id", connCtx.GetUserIDString()),
				zlog.String("device_id", connCtx.getDeviceID()),
				zlog.String("connection_path", connCtx.Path))
			return
		}
	}
	jsonResp.Valid = utils.Base64Encode(validBytes)

	// 发送JsonResp格式的响应
	replyBytes, err := utils.JsonMarshal(jsonResp)
	PutJsonResp(jsonResp)
	if err != nil {
		zlog.Error("failed_to_marshal_jsonresp", 0,
			zlog.AddError(err),
			zlog.String("user_id", connCtx.GetUserIDString()),
			zlog.String("device_id", connCtx.getDeviceID()),
			zlog.String("connection_path", connCtx.Path))
		return
	}

	if err := connCtx.DevConn.Send(replyBytes); err != nil {
		zlog.Error("failed_to_send_reply", 0,
			zlog.AddError(err),
			zlog.String("user_id", connCtx.GetUserIDString()),
			zlog.String("device_id", connCtx.getDeviceID()),
			zlog.String("connection_path", connCtx.Path))
		return // 发送失败通常意味着连接已断开
	}

}

func (s *WsServer) cleanupConnection(connCtx *ConnectionContext) {
	// 先发关闭信号，阻止任何新 Send
	closeConn(connCtx.DevConn, "cleanup")

	if s.connManager != nil {
		if s.connManager.RemoveByConn(connCtx.DevConn) {
			deviceKey := connCtx.DevConn.Sub
			if s.connUniquenessMode == SubjectDeviceUnique {
				deviceKey = utils.AddStr(connCtx.DevConn.Sub, "_", connCtx.DevConn.Dev)
			}
			zlog.Info("client_disconnected", 0, zlog.String("subject", deviceKey))
		}
	}

	// 最后由 loop 执行实际 Close
	closeConnFromLoop(connCtx.DevConn)
}

func (s *WsServer) gracefulShutdown() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	zlog.Info("initiating graceful shutdown from signal...", 0)
	s.StopWebsocket() // 使用统一入口
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

// AddCipher 注册 ECDSA/RSA 加解密对象，用于 Plan 1 等场景的请求验签与响应签名。
// 应在 StartWebsocket 之前完成所有注册。不支持运行期动态添加（MessageHandler 无锁读 Cipher，动态添加有并发风险）；若以后需要运行期扩展再考虑加锁或 copy-on-write 等方案。
func (self *WsServer) AddCipher(usr int64, cipher crypto.Cipher) error {
	if self.Cipher == nil {
		self.Cipher = make(map[int64]crypto.Cipher)
	}
	if cipher == nil {
		return utils.Error("cipher is nil")
	}
	self.Cipher[usr] = cipher
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

// closeConn 仅发关闭信号（CAS + cancel），不调用 ws.Close()。
// 所有对 ws 的 Close 必须由消息循环 goroutine 在 closeConnFromLoop 中执行，避免与 ReadMessage 并发导致 panic。
func closeConn(conn *DevConn, reason string) {
	if conn == nil {
		return
	}
	if !atomic.CompareAndSwapInt32(&conn.closed, 0, 1) {
		return
	}
	if conn.cancel != nil {
		conn.cancel()
	}
}

// closeConnFromLoop 由消息循环 goroutine 在退出时调用，执行实际 ws.Close()。
// 禁止在其他 goroutine 中调用。仅当 Conn 已为 nil（已关闭）时跳过；closed==1 不影响执行，因 cleanupConnection 会先 closeConn 再调用本函数，必须在本函数内执行 Close。
func closeConnFromLoop(conn *DevConn) {
	if conn == nil {
		return
	}
	if conn.cancel != nil {
		conn.cancel()
	}
	conn.sendMu.Lock()
	defer conn.sendMu.Unlock()
	if conn.Conn == nil {
		return
	}
	atomic.StoreInt32(&conn.closed, 1)
	ws := conn.Conn
	conn.Conn = nil
	conn.closeOnce.Do(func() {
		defer func() {
			if r := recover(); r != nil {
				zlog.Error("websocket_close_panic_recovered", 0, zlog.Any("panic", r))
			}
		}()
		_ = ws.Close()
	})
}
