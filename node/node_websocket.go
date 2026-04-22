package node

import (
	"context"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	DIC "github.com/godaddy-x/freego/common"
	"github.com/godaddy-x/freego/zlog"

	"github.com/godaddy-x/freego/cache"
	rate "github.com/godaddy-x/freego/cache/limiter"
	"github.com/godaddy-x/freego/ex"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/utils/crypto"
	"github.com/godaddy-x/freego/utils/jwt"
	"github.com/lxzan/gws"
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

// wsFrameHexPreview 记录原始 WS 帧前缀（十六进制），用于区分「键名/JSON 错」与「d 未填」等，避免整包打日志造成泄露。
func wsFrameHexPreview(b []byte, maxBytes int) string {
	if len(b) == 0 {
		return "len=0"
	}
	if maxBytes <= 0 {
		maxBytes = 64
	}
	n := len(b)
	if n > maxBytes {
		n = maxBytes
	}
	return fmt.Sprintf("total_len=%d prefix_hex(%d)=%s", len(b), n, hex.EncodeToString(b[:n]))
}

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
// 对象池约束：*JsonResp 由 replyData Put 回池；*JsonBody 在 WebSocket 路径为每条消息的栈上/池对象（Process 的 jb），不得异步持有。
// WS 业务 Handle 须在返回前结束对 bizData/connCtx 的使用；若启动 goroutine，不得捕获池化 JsonBody 指针。
type Handle func(ctx context.Context, connCtx *ConnectionContext, body []byte) (interface{}, error)

// ConnectionContext 每个 WebSocket 连接的上下文（与 HTTP 的 node.Context 不同，不含 JsonBody）。
// 单帧协议体使用 Process(..., jb) 的池化 *JsonBody，避免与 ParallelEnabled 下的并发收包共享指针。
type ConnectionContext struct {
	Subject      *jwt.Subject
	WsConn       *gws.Conn // WebSocket 连接（gws）
	DevConn      *DevConn
	Server       *WsServer
	RouterConfig *RouterConfig // 路由配置
	Path         string        // WebSocket连接的路径
	RawToken     []byte        // 原始JWT token字节，用于签名验证
	ctx          context.Context
	cancel       context.CancelFunc
}

// requestMeta 是单条请求在回复阶段需要的不可变快照（来自本条消息的 jsonBody）。
type requestMeta struct {
	Router string
	Nonce  string
	Plan   int64
	User   int64
}

func (cc *ConnectionContext) wsTraceConnID() string {
	if cc == nil || cc.WsConn == nil {
		return "nil-conn"
	}
	return fmt.Sprintf("%s|%p", cc.WsConn.RemoteAddr().String(), cc.WsConn)
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
		if zlog.IsDebug() {
			zlog.Debug("get_user_id_parse_failed", 0, zlog.String("sub", cc.Subject.Payload.Sub), zlog.AddError(err))
		}
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
	if cc.Subject != nil && cc.Subject.Payload != nil {
		return cc.Subject.Payload.Dev
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
// - 所有"收集 conn 再关闭/发送"的路径均在 RLock 内只收集指针，在锁外执行 I/O，避免持锁时间过长。
// - reverseIndex：反向索引 *gws.Conn -> reverseIndexEntry，用于 O(1) 复杂度的 RemoveByConn（适合连接数 > 10000 的场景）。
type ConnectionManager struct {
	mu           sync.RWMutex
	conns        map[string]map[string]*DevConn // subject -> deviceKey -> connection
	max          int                            // 最大并发连接数（与 limiter 的连接速率限制不同）
	totalConn    int32                          // 原子计数器：当前总连接数（限流 + 预分配容量）
	mode         ConnectionUniquenessMode       // 连接唯一性模式
	broadcastKey string                         // 广播密钥

	// 反向索引：*gws.Conn -> reverseIndexEntry（使用 sync.Map 适合读多写少、key 稳定的场景）
	reverseIndex sync.Map
}

// reverseIndexEntry 反向索引条目
type reverseIndexEntry struct {
	subject   string
	deviceKey string
}

const maxPooledTargetsCap = 4096

var devConnTargetsPool = sync.Pool{
	New: func() interface{} {
		return make([]*DevConn, 0, 64)
	},
}

func acquireDevConnTargets(hint int) []*DevConn {
	s := devConnTargetsPool.Get().([]*DevConn)
	if cap(s) < hint {
		return make([]*DevConn, 0, hint)
	}
	return s[:0]
}

func releaseDevConnTargets(s []*DevConn) {
	if s == nil {
		return
	}
	if cap(s) > maxPooledTargetsCap {
		return
	}
	for i := range s {
		s[i] = nil
	}
	devConnTargetsPool.Put(s[:0])
}

// MessageHandler 消息处理器：统一处理消息校验、解码、路由
type MessageHandler struct {
	cipher map[int64]crypto.Cipher
	handle Handle
}

// HeartbeatService 心跳服务：gws 已内置 ping/pong，此服务仅用于定期清理过期连接
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
// - Last：使用原子读写（UpdateLast 写、LastSeen 读），使 CleanupExpired 遍历时无需加锁。
// - 多 goroutine 并发 Send 依赖 gws.Conn.WriteMessage 的线程安全（库内 channel + 单写）；不再额外加 sendMu，避免与 ParallelEnabled 下多回包场景无谓争用。
// - closed：与 CleanupExpired / 踢线等路径上的 WriteClose 配合，Send 内先读后写；与 WriteClose 之间无应用层全局锁（历来如此）。
// - closeOnce：保证 Close() 只执行一次，避免重复关闭导致 panic。
type DevConn struct {
	Sub       string
	Dev       string
	Last      int64     // 最后活跃时间戳，原子读写，供 CleanupExpired 无锁判断
	Conn      *gws.Conn // WebSocket 连接（gws）
	ctx       context.Context
	cancel    context.CancelFunc
	closed    int32     // 0=未关闭，1=已关闭，原子读写
	closeOnce sync.Once // 确保 Close() 只执行一次
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

// LastSeen 返回最近一次活跃时间。原子读、无锁，供 CleanupExpired 在 RLock 内批量调用。
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
	server       *http.Server          // 标准 HTTP 服务器
	routes       map[string]*RouteInfo // 路由映射：path -> 路由信息 (启动后只读)
	connManager  *ConnectionManager
	heartbeatSvc *HeartbeatService
	upgrader     *gws.Upgrader // gws 升级器

	broadcastKey string

	// 配置项
	ping            int           // 心跳间隔（秒）
	maxConn         int           // 最大并发连接数（与 limiter 的连接速率限制不同）
	maxBodyLen      int           // 单条消息体最大长度（字节），默认 DefaultWsMaxBodyLen
	parallelEnabled bool          // 是否并行处理同连接消息（映射 gws.ServerOption.ParallelEnabled）
	limiter         *rate.Limiter // 连接建立速率限制（每秒允许的新连接数，与 maxConn 并发连接数限制不同）
	idleTimeout     time.Duration // 连接空闲超时时间
	globalCtx       context.Context
	globalCancel    context.CancelFunc

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
	Cipher          map[int64]crypto.Cipher                 // 8字节 - RSA / Ed25519 等 Cipher 列表（双向外层签名等）
	RedisCacheAware func(ds ...string) (cache.Cache, error) // 8字节 - 函数指针
	LocalCacheAware func(ds ...string) (cache.Cache, error) // 8字节 - 函数指针

	// 用于确保 Stop 只执行一次
	shutdownOnce sync.Once
	// 确保信号监听只注册一次，避免重复调用 StartWebsocket 时重复 Notify
	signalOnce sync.Once

	// 存储 socket 到 connCtx 的映射
	connContextMap sync.Map // key: *gws.Conn, value: *ConnectionContext
}

// ErrorHandler WebSocket错误处理器（统一错误处理）
type ErrorHandler struct {
}

func (eh *ErrorHandler) handleConnectionError(connCtx *ConnectionContext, err error, operation string, fallbackNonce string) {
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
	zlog.Error(operation+"_failed", 0, zlog.AddError(err), zlog.String("operation", operation), zlog.String("user_id", userID), zlog.String("remote_addr", remoteAddr), zlog.String("device_id", deviceID), zlog.String("connection_path", connectionPath))

	// 尝试发送错误响应
	if conn := connCtx.WsConn; conn != nil {
		resp := GetJsonResp()
		defer PutJsonResp(resp)
		resp.Code = ex.WS_SEND
		resp.Message = "websocket error: " + operation
		resp.Time = utils.UnixSecond()

		if len(fallbackNonce) > 0 {
			resp.Nonce = fallbackNonce
		} else {
			resp.Nonce = utils.GetUUID(true)
		}

		result, marshalErr := utils.JsonMarshal(resp)
		if marshalErr == nil {
			if connCtx.DevConn != nil {
				if err := connCtx.DevConn.Send(result); err != nil {
					if zlog.IsDebug() {
						zlog.Debug("failed to send error response to closed connection", 0, zlog.AddError(err))
					}
				}
			} else if connCtx.WsConn != nil {
				if err := connCtx.WsConn.WriteMessage(gws.OpcodeText, result); err != nil {
					if zlog.IsDebug() {
						zlog.Debug("failed to send error response to closed connection", 0, zlog.AddError(err))
					}
				}
			}
		}

	}
}

// ConfigValidator 配置验证器（统一配置检查）
type ConfigValidator struct{}

func (cv *ConfigValidator) validateServerConfig(addr string, server interface{}, heartbeatSvc *HeartbeatService) error {
	if addr == "" {
		return utils.Error("server address cannot be empty")
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
		if strings.TrimSpace(conn.Dev) == "" {
			return utils.Error("device id is required in SubjectDeviceUnique mode")
		}
		uniqueKey = utils.AddStr(conn.Sub, "_", conn.Dev)
	}

	if cm.conns[conn.Sub] == nil {
		cm.conns[conn.Sub] = make(map[string]*DevConn)
	}

	// 替换旧连接：先移除引用并减计数，再在 goroutine 中 closeConn，避免锁内 I/O
	if oldConn, exists := cm.conns[conn.Sub][uniqueKey]; exists {
		delete(cm.conns[conn.Sub], uniqueKey)
		atomic.AddInt32(&cm.totalConn, -1)
		// 删除旧连接的反向索引
		if oldConn != nil && oldConn.Conn != nil {
			cm.reverseIndex.Delete(oldConn.Conn)
		}
		go func() {
			// 关闭旧连接
			if oldConn != nil && oldConn.Conn != nil {
				_ = oldConn.Conn.WriteClose(1000, []byte("replaced by new connection"))
			}
		}()
	}

	cm.conns[conn.Sub][uniqueKey] = conn
	atomic.AddInt32(&cm.totalConn, 1)

	// 存储反向索引（O(1) 查找）
	if conn.Conn != nil {
		cm.reverseIndex.Store(conn.Conn, &reverseIndexEntry{
			subject:   conn.Sub,
			deviceKey: uniqueKey,
		})
	}

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
// 设计要点：
// - 使用 reverseIndex 反向索引实现 O(1) 查找，避免遍历所有连接（适合连接数 > 10000 的场景）。
// - 必须用指针精确匹配，避免“新连接已替换旧连接”时误把新连接从 map 删掉。
// - 关闭由 closeConnFromLoop 等调用方负责。
// 返回是否成功移除（若连接已被替换则未找到，返回 false）。
func (cm *ConnectionManager) RemoveByConn(conn *DevConn) bool {
	if conn == nil || conn.Conn == nil {
		return false
	}

	// 从反向索引获取位置信息（O(1)）
	val, ok := cm.reverseIndex.Load(conn.Conn)
	if !ok {
		return false // 连接不在索引中，可能已被移除
	}
	entry, ok := val.(*reverseIndexEntry)
	if !ok || entry == nil {
		// 索引数据异常，清理后返回
		cm.reverseIndex.Delete(conn.Conn)
		return false
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()

	// 验证连接是否仍然存在且匹配
	subjectConns, exists := cm.conns[entry.subject]
	if !exists {
		// 索引过期，清理
		cm.reverseIndex.Delete(conn.Conn)
		return false
	}

	if c, exists := subjectConns[entry.deviceKey]; exists && c == conn {
		delete(subjectConns, entry.deviceKey)
		atomic.AddInt32(&cm.totalConn, -1)
		if len(subjectConns) == 0 {
			delete(cm.conns, entry.subject)
		}
		// 清理反向索引
		cm.reverseIndex.Delete(conn.Conn)
		return true
	}

	// 连接已被替换，清理过期索引
	cm.reverseIndex.Delete(conn.Conn)
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
	n := atomic.LoadInt32(&cm.totalConn)
	if n < 0 {
		n = 0
	}
	targets := acquireDevConnTargets(int(n))
	defer releaseDevConnTargets(targets)
	cm.mu.RLock()
	for _, subjectConns := range cm.conns {
		for _, conn := range subjectConns {
			targets = append(targets, conn)
		}
	}
	cm.mu.RUnlock()

	for _, conn := range targets {
		_ = conn.Send(data)
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
	defer PutJsonResp(jsonResp)

	jsonResp.Code = 300 // 推送消息使用特殊code值300
	jsonResp.Message = "push"
	jsonResp.Router = router
	jsonResp.Time = utils.UnixSecond()

	// 序列化数据
	dataBytes, err := utils.JsonMarshal(data)
	if err != nil {
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
	return cm.sendToSubjectByJsonResp(subject, jsonResp)
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
	targets := acquireDevConnTargets(int(n))
	defer releaseDevConnTargets(targets)
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

// CleanupExpired 清理空闲超过 timeoutSeconds 的连接。
//
// 设计要点：
// - 在 RLock 内仅收集过期 conn 指针（LastSeen 为原子读无锁），RUnlock 后再 closeConn，避免持锁做 I/O。
// - toClose 按 totalConn 预分配容量，减少 append 扩容与 GC。
// - 超时判断依赖 DevConn.Last（每次收包/心跳 UpdateLast），由 HeartbeatService 按 idleTimeout 周期性调用。
// - 先发送关闭帧，OnClose 回调会负责从 map 中移除（避免重复移除）。
func (cm *ConnectionManager) CleanupExpired(timeoutSeconds int64) int {
	cleaned := 0
	currentTime := utils.UnixSecond()
	n := atomic.LoadInt32(&cm.totalConn)
	if n < 0 {
		n = 0
	}
	cm.mu.RLock()
	toClose := acquireDevConnTargets(int(n))
	defer releaseDevConnTargets(toClose)
	for _, subjectConns := range cm.conns {
		for _, conn := range subjectConns {
			if currentTime-conn.LastSeen() > timeoutSeconds {
				toClose = append(toClose, conn)
			}
		}
	}
	cm.mu.RUnlock()

	for _, conn := range toClose {
		if conn == nil {
			continue
		}
		// 标记为已关闭，防止重复处理
		if atomic.CompareAndSwapInt32(&conn.closed, 0, 1) {
			// 发送关闭帧（OnClose 回调会负责从 map 中移除）
			if conn.Conn != nil {
				_ = conn.Conn.WriteClose(1000, []byte("expired"))
			}
			cleaned++
		}
	}
	return cleaned
}

// CleanupAll 关闭所有连接。先 RLock 内收集全部 conn 指针并预分配 slice，锁外再统一 closeConn。
// 设计要点：先发送关闭帧，OnClose 回调会负责从 map 中移除（避免重复移除）。
func (cm *ConnectionManager) CleanupAll() {
	n := atomic.LoadInt32(&cm.totalConn)
	if n < 0 {
		n = 0
	}
	cm.mu.RLock()
	toClose := acquireDevConnTargets(int(n))
	defer releaseDevConnTargets(toClose)
	for _, subjectConns := range cm.conns {
		for _, conn := range subjectConns {
			toClose = append(toClose, conn)
		}
	}
	cm.mu.RUnlock()

	for _, conn := range toClose {
		if conn == nil {
			continue
		}
		// 标记为已关闭，防止重复处理
		if atomic.CompareAndSwapInt32(&conn.closed, 0, 1) {
			// 发送关闭帧（OnClose 回调会负责从 map 中移除）
			if conn.Conn != nil {
				_ = conn.Conn.WriteClose(1000, []byte("cleanup all"))
			}
		}
	}
}

// Count 获取当前连接数
func (cm *ConnectionManager) Count() int {
	return int(atomic.LoadInt32(&cm.totalConn))
}

// validateMessageSize 验证消息大小，防止恶意消息攻击，使用 WsServer 配置的 maxBodyLen
func (mh *MessageHandler) validateMessageSize(connCtx *ConnectionContext, body []byte) error {
	maxLen := DefaultWsMaxBodyLen
	if connCtx != nil && connCtx.Server != nil {
		maxLen = connCtx.Server.maxBodyLen
	}
	if len(body) > maxLen {
		return ex.Throw{Code: http.StatusRequestEntityTooLarge, Msg: "message too large"}
	}
	return nil
}

// Process 处理单条 WS 文本帧。jb 为本条消息独占的 JsonBody（由调用方从池取出并在调用结束后 Put），不得与 connCtx 共享指针以免并发覆盖。
func (mh *MessageHandler) Process(connCtx *ConnectionContext, body []byte, jb *JsonBody) (crypto.Cipher, interface{}, error) {
	// 验证消息大小，防止恶意消息攻击
	if err := mh.validateMessageSize(connCtx, body); err != nil {
		return nil, nil, err
	}
	if jb == nil {
		return nil, nil, ex.Throw{Code: http.StatusBadRequest, Msg: "websocket json body is nil"}
	}

	// 解析WebSocket消息体
	if err := utils.JsonUnmarshal(body, jb); err != nil {
		zlog.Error("websocket message json unmarshal failed", 0, zlog.ByteString("raw_body", body), zlog.AddError(err))
		return nil, nil, ex.Throw{Code: http.StatusBadRequest, Msg: "invalid JSON format"}
	}

	// 检查是否是心跳包（用于失败时的指标记录）
	isHeartbeat := jb.Router == "/ws/ping"

	// 验证消息体（按照HTTP协议标准）
	cipher, err := mh.validWebSocketBody(connCtx, body, jb)
	if err != nil {
		if zlog.IsDebug() {
			zlog.Error("websocket message validation failed", 0,
				zlog.String("frame_preview", wsFrameHexPreview(body, 160)),
				zlog.Int("frame_len", len(body)),
				zlog.AddError(err))
		} else {
			zlog.Error("websocket message validation failed", 0, zlog.Int("frame_len", len(body)), zlog.AddError(err))
		}
		// 如果是心跳包验证失败，记录失败指标
		if isHeartbeat {
			connCtx.Server.recordHeartbeat(false)
		}
		return nil, nil, err
	}

	// 心跳包 /ws/ping：只更新 Last 与指标，不返回 PONG。
	// 设计要点：服务端不回 pong，降低服务端写压力；连接活性靠客户端定时 ping + 服务端读超时与 CleanupExpired 判定。
	if jb.Router == "/ws/ping" {
		connCtx.DevConn.UpdateLast()
		connCtx.Server.recordHeartbeat(true)

		if zlog.IsDebug() {
			deviceID := ""
			if connCtx.Subject != nil && connCtx.Subject.Payload != nil {
				deviceID = connCtx.Subject.Payload.Dev
			}
			zlog.Debug("heartbeat_received_and_updated", 0, zlog.String("subject", connCtx.Subject.GetSub(nil)), zlog.String("device", deviceID), zlog.String("connection_path", connCtx.Path), zlog.String("nonce", jb.Nonce))
		}

		return cipher, nil, nil
	}

	// 解密业务数据
	bizData, err := mh.decryptWebSocketData(connCtx, jb)
	if err != nil {
		if zlog.IsDebug() {
			zlog.Error("websocket data decryption failed", 0,
				zlog.String("frame_preview", wsFrameHexPreview(body, 160)),
				zlog.String("router", jb.Router),
				zlog.Int64("plan", jb.Plan),
				zlog.AddError(err))
		} else {
			zlog.Error("websocket data decryption failed", 0,
				zlog.String("router", jb.Router),
				zlog.Int64("plan", jb.Plan),
				zlog.AddError(err))
		}
		return nil, nil, err
	}

	// 根据消息中的路由选择处理器
	handle := mh.handle // 默认处理器
	if jb.Router != "" {
		if routeInfo, exists := connCtx.Server.routes[jb.Router]; exists {
			handle = routeInfo.Handle
			if zlog.IsDebug() {
				zlog.Debug("using route-specific handler", 0, zlog.String("router", jb.Router))
			}
		} else {
			zlog.Warn("no handler found for router, using default", 0, zlog.String("router", jb.Router))
		}
	}

	result, err := handle(connCtx.ctx, connCtx, bizData)
	return cipher, result, err
}

// CheckOuterSign 按用户 ID 取 Cipher 并校验外层签名（cipher.Verify；典型为 Ed25519Object）。
func (self *MessageHandler) CheckOuterSign(usr int64, msg, sign []byte) (crypto.Cipher, error) {
	cipher, exists := self.cipher[usr]
	if !exists || cipher == nil {
		return nil, ex.Throw{Code: http.StatusBadRequest, Msg: "cipher not found for user, bidirectional Ed25519 signature is required"}
	}
	if err := cipher.Verify(msg, sign); err != nil {
		return nil, ex.Throw{Code: http.StatusBadRequest, Msg: "request signature invalid"}
	}
	return cipher, nil
}

// validWebSocketBody 验证 WebSocket 消息体（签名、时间窗、可选 token 有效期）。
func (mh *MessageHandler) validWebSocketBody(connCtx *ConnectionContext, rawFrame []byte, body *JsonBody) (crypto.Cipher, error) {
	if body == nil {
		return nil, ex.Throw{Code: http.StatusBadRequest, Msg: "websocket json body is nil"}
	}
	d := body.Data
	if len(d) == 0 {
		return nil, ex.Throw{Code: http.StatusBadRequest, Msg: "websocket data is nil"}
	}

	// 只支持 Plan 0（明文）和 1（AES），不再支持 Plan 2
	if !utils.CheckInt64(body.Plan, 0, 1) {
		return nil, ex.Throw{Code: http.StatusBadRequest, Msg: "websocket plan invalid"}
	}

	if !utils.CheckLen(body.Nonce, 8, 32) {
		return nil, ex.Throw{Code: http.StatusBadRequest, Msg: "websocket nonce invalid"}
	}
	if body.Time <= 0 {
		return nil, ex.Throw{Code: http.StatusBadRequest, Msg: "websocket time must be > 0"}
	}
	if utils.MathAbs(utils.UnixSecond()-body.Time) > jwt.FIVE_MINUTES {
		return nil, ex.Throw{Code: http.StatusBadRequest, Msg: "websocket time invalid"}
	}

	// 检查是否需要AES加密
	if connCtx.RouterConfig != nil && connCtx.RouterConfig.AesRequest && body.Plan != 1 {
		return nil, ex.Throw{Code: http.StatusBadRequest, Msg: "websocket parameters must use encryption"}
	}

	if !utils.CheckStrLen(body.Sign, 32, 64) {
		return nil, ex.Throw{Code: http.StatusBadRequest, Msg: "websocket signature length invalid"}
	}

	// 对于Plan 0/1，必须要有token
	if len(connCtx.GetRawTokenBytes()) == 0 {
		return nil, ex.Throw{Code: http.StatusBadRequest, Msg: "websocket header token is nil"}
	}

	// 可选：每条消息校验 token 有效期（与 jwt.Verify 一致：提前 15 秒视为过期）
	if connCtx.Server.validateTokenPerMessage {
		if connCtx.Subject == nil || connCtx.Subject.Payload == nil {
			return nil, ex.Throw{Code: http.StatusUnauthorized, Msg: "token not ready"}
		}
		if connCtx.Subject.Payload.Exp <= utils.UnixSecond()-15 {
			return nil, ex.Throw{Code: http.StatusUnauthorized, Msg: "token expired or invalid"}
		}
	}

	var sharedKey []byte

	// Plan 0/1使用token secret，用毕清理
	sharedKey = connCtx.GetTokenSecret()
	if len(sharedKey) == 0 {
		return nil, ex.Throw{Code: http.StatusBadRequest, Msg: "websocket secret is nil"}
	}
	defer DIC.ClearData(sharedKey)

	// 构建签名字符串
	// 使用从header获取的路由标识进行签名验证，支持通过header指定路由
	sign := SignBodyMessage(body.Router, d, body.Nonce, body.Time, body.Plan, body.User, sharedKey)
	if utils.Base64Encode(sign) != body.Sign {
		return nil, ex.Throw{Code: http.StatusUnauthorized, Msg: "websocket signature verify invalid"}
	}

	cipher, err := mh.CheckOuterSign(body.User, sign, utils.Base64Decode(body.Valid))
	if err != nil {
		return nil, err
	}

	return cipher, nil
}

// decryptWebSocketData 解密WebSocket数据（参考HTTP协议）
func (mh *MessageHandler) decryptWebSocketData(connCtx *ConnectionContext, body *JsonBody) ([]byte, error) {
	if body == nil {
		return nil, ex.Throw{Code: http.StatusBadRequest, Msg: "websocket json body is nil"}
	}
	d := body.Data

	switch body.Plan {
	case 0: // 明文
		rawData := utils.Base64Decode(d)
		if len(rawData) == 0 {
			return nil, ex.Throw{Code: http.StatusBadRequest, Msg: "websocket base64 parsing failed"}
		}
		return rawData, nil
	case 1: // AES-GCM加密
		secret := connCtx.GetTokenSecret()
		if len(secret) < 32 {
			return nil, ex.Throw{Code: http.StatusBadRequest, Msg: "websocket aes secret length invalid"}
		}
		defer DIC.ClearData(secret)
		rawData, err := utils.AesGCMDecryptBase(d, secret[:32], AppendBodyMessage(body.Router, "", body.Nonce, body.Time, body.Plan, body.User))
		if err != nil {
			return nil, ex.Throw{Code: http.StatusBadRequest, Msg: "websocket aes decrypt failed", Err: err}
		}
		return rawData, nil
	default:
		return nil, ex.Throw{Code: http.StatusBadRequest, Msg: "websocket unsupported plan"}
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
		return
	}

	hs.running = true
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

// run 周期性执行：按 idleTimeout 清理空闲连接
func (hs *HeartbeatService) run() {
	ticker := time.NewTicker(hs.interval)
	defer ticker.Stop()

	for {
		select {
		case <-hs.stopCh:
			return
		case <-ticker.C:
			cleaned := hs.manager.CleanupExpired(int64(hs.timeout.Seconds()))
			if cleaned > 0 && zlog.IsDebug() {
				zlog.Debug("cleanup_expired_connections", 0, zlog.Int("cleaned", cleaned), zlog.Int("remaining", hs.manager.Count()))
			}
		}
	}
}

// -------------------------- DevConn 实现 --------------------------

// Send 向连接写入一条文本消息。
// 并发安全由 gws.Conn.WriteMessage 保证；closed 仅作快速失败，与 WriteClose 竞态时以 gws 返回错误为准。
func (dc *DevConn) Send(data []byte) error {
	if dc == nil {
		return utils.Error("connection is closed")
	}
	if atomic.LoadInt32(&dc.closed) == 1 || dc.Conn == nil {
		return utils.Error("connection is closed")
	}
	err := dc.Conn.WriteMessage(gws.OpcodeText, data)
	if err != nil {
		zlog.Warn("WS_TRACE_SEND_FAILED", 0,
			zlog.String("subject", dc.Sub),
			zlog.String("device", dc.Dev),
			zlog.Int32("closed", atomic.LoadInt32(&dc.closed)),
			zlog.AddError(err))
	}
	return err
}

// WsEventHandler gws 事件处理器：处理 WebSocket 连接的生命周期事件
type WsEventHandler struct {
	server *WsServer
}

// OnOpen 连接建立时的回调
func (h *WsEventHandler) OnOpen(socket *gws.Conn) {
	// 从 map 中获取 ConnectionContext
	if val, ok := h.server.connContextMap.Load(socket); ok {
		if connCtx, ok := val.(*ConnectionContext); ok {
			connCtx.WsConn = socket
			h.server.recordConnectionAdded()

			deviceID := ""
			if connCtx.Subject != nil && connCtx.Subject.Payload != nil {
				deviceID = connCtx.Subject.Payload.Dev
			}
			zlog.Info("CLIENT_CONNECTED", 0, zlog.String("client_address", socket.RemoteAddr().String()), zlog.String("user_id", connCtx.Subject.GetSub(nil)), zlog.String("device_id", deviceID))
		}
	}
}

// OnClose 连接关闭时的回调
func (h *WsEventHandler) OnClose(socket *gws.Conn, err error) {
	if val, ok := h.server.connContextMap.Load(socket); ok {
		if connCtx, ok := val.(*ConnectionContext); ok {
			h.server.recordConnectionRemoved()
			subject := ""
			device := ""
			if connCtx.DevConn != nil {
				subject = connCtx.DevConn.Sub
				device = connCtx.DevConn.Dev
			}
			zlog.Info("client_disconnected", 0, zlog.String("subject", subject), zlog.String("device", device))

			// 从连接管理器中移除
			if h.server.connManager != nil && connCtx.DevConn != nil {
				h.server.connManager.RemoveByConn(connCtx.DevConn)
			}

			// 从 map 中删除
			h.server.connContextMap.Delete(socket)

			// 取消上下文
			if connCtx.cancel != nil {
				connCtx.cancel()
			}
		}
	}
}

// OnPing 收到 Ping 帧时的回调（gws 会自动回复 Pong）
func (h *WsEventHandler) OnPing(socket *gws.Conn, payload []byte) {
	if val, ok := h.server.connContextMap.Load(socket); ok {
		if connCtx, ok := val.(*ConnectionContext); ok {
			connCtx.DevConn.UpdateLast()
			h.server.recordHeartbeat(true)
		}
	}
}

// OnPong 收到 Pong 帧时的回调（gws 接口要求）
func (h *WsEventHandler) OnPong(socket *gws.Conn, payload []byte) {}

// OnMessage 收到消息时的回调
func (h *WsEventHandler) OnMessage(socket *gws.Conn, message *gws.Message) {
	defer message.Close() // 确保消息被关闭，释放资源

	if val, ok := h.server.connContextMap.Load(socket); ok {
		if connCtx, ok := val.(*ConnectionContext); ok {
			// 更新最后活跃时间
			connCtx.DevConn.UpdateLast()

			// 处理消息
			h.server.processMessage(connCtx, message.Bytes())
		}
	}
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
		parallelEnabled:    true,
		idleTimeout:        3600 * time.Second, // 默枧1小时空闲超时
		errorHandler:       &ErrorHandler{},
		configValidator:    &ConfigValidator{},
		metrics:            &WebSocketMetrics{startTime: time.Now()},
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

	// 初始化 gws Upgrader
	s.initUpgrader()

	s.server = &http.Server{
		Addr:         "",
		Handler:      http.HandlerFunc(s.serveHTTP),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return s
}

func (s *WsServer) initUpgrader() {
	s.upgrader = gws.NewUpgrader(&WsEventHandler{server: s}, &gws.ServerOption{
		ParallelEnabled:    s.parallelEnabled,
		Recovery:           gws.Recovery,                          // 异常恢复
		ReadMaxPayloadSize: s.maxBodyLen,                          // 最大消息体长度
		PermessageDeflate:  gws.PermessageDeflate{Enabled: false}, // 可选：启用压缩
	})
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
	if utils.CheckRangeInt(ping, 15, 30) {
		s.ping = ping
	} else {
		s.ping = 30
	}

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

// SetMaxBodyLen 设置单条消息体最大长度（字节），需在 StartWebsocket 前调用
func (s *WsServer) SetMaxBodyLen(n int) {
	if n <= 0 {
		return
	}
	s.maxBodyLen = n
	s.initUpgrader()
}

// SetParallelEnabled 设置是否并行处理同一连接上的消息（需在 StartWebsocket 前调用）。
// true 可提升吞吐；false 可保证单连接消息按处理顺序串行。
func (s *WsServer) SetParallelEnabled(enabled bool) {
	s.parallelEnabled = enabled
	s.initUpgrader()
}

// SetBroadcastKey 广播数据密钥
func (s *WsServer) SetBroadcastKey(key string) {
	s.broadcastKey = key
}

func (s *WsServer) StartWebsocket(addr string) error {
	if err := s.configValidator.validateServerConfig(addr, nil, s.heartbeatSvc); err != nil {
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
	s.server.Addr = addr
	if err := s.server.ListenAndServe(); err != nil {
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

	zlog.Info("websocket_server_metrics", 0, zlog.Int64("connections_total", metrics.connectionsTotal), zlog.Int64("connections_active", metrics.connectionsActive), zlog.Int64("connections_peak", metrics.connectionsPeak), zlog.Int64("messages_total", metrics.messagesTotal), zlog.Int64("messages_success", metrics.messagesSuccess), zlog.Int64("messages_error", metrics.messagesError), zlog.Float64("message_success_rate", messageSuccessRate), zlog.Int64("heartbeats_total", metrics.heartbeatsTotal), zlog.Int64("heartbeats_success", metrics.heartbeatsSuccess), zlog.Int64("heartbeats_failed", metrics.heartbeatsFailed), zlog.Float64("heartbeat_success_rate", heartbeatSuccessRate), zlog.Int64("uptime_seconds", metrics.uptimeSeconds))
}

// StopWebsocket 停止WebSocket服务器
func (s *WsServer) StopWebsocket() (err error) {
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
			err = s.server.Shutdown(ctx)
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

	// 通知所有连接优雅关闭（发送 Close 帧）
	if s.connManager != nil {
		s.connManager.CleanupAll()
	}

	// 等待客户端响应关闭帧（最多 1 秒）
	time.Sleep(1 * time.Second)

	// 执行标准关闭流程（关闭 HTTP 服务器）
	return s.StopWebsocket()
}

func (s *WsServer) serveHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	if zlog.IsDebug() {
		zlog.Debug("HTTP_REQUEST_RECEIVED", 0, zlog.String("method", r.Method), zlog.String("path", path), zlog.String("client_ip", r.RemoteAddr))
	}

	// 检查路由是否存在
	if _, exists := s.routes[path]; !exists {
		http.Error(w, "route not found", http.StatusNotFound)
		return
	}

	// 连接速率限制（limiter 控制每秒新建连接数，与 maxConn 并发连接数限制不同）
	if s.limiter != nil && !s.limiter.Allow() {
		http.Error(w, "too many connections", http.StatusTooManyRequests)
		return
	}

	// JWT 校验
	subject, rawToken, err := s.validateTokenFromRequest(r, path)
	if err != nil {
		if exErr, ok := err.(ex.Throw); ok {
			http.Error(w, exErr.Msg, exErr.Code)
		} else {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
		}
		return
	}

	// 使用 gws 升级连接
	socket, err := s.upgrader.Upgrade(w, r)
	if err != nil {
		if zlog.IsDebug() {
			zlog.Debug("websocket_upgrade_failed", 0, zlog.AddError(err))
		}
		return
	}

	// 创建连接上下文
	connCtx := s.createConnectionContext(subject, socket, path, &RouterConfig{}, rawToken)

	// 存储到 map
	s.connContextMap.Store(socket, connCtx)

	// 添加到连接管理器（maxConn 在此处限制并发连接总数）
	if err := s.connManager.Add(connCtx.DevConn); err != nil {
		connCtx.cancel()
		socket.WriteClose(1000, []byte("connection limit exceeded"))
		zlog.Error("conn_manager_add_failed", 0, zlog.AddError(err))
		return
	}

	// 启动 ReadLoop
	go socket.ReadLoop()
}

// validateTokenFromRequest 在升级前校验 JWT
func (s *WsServer) validateTokenFromRequest(r *http.Request, path string) (*jwt.Subject, []byte, error) {
	authHeader := []byte(r.Header.Get("Authorization"))
	if len(authHeader) == 0 {
		return nil, nil, ex.Throw{Code: http.StatusUnauthorized, Msg: "missing authorization header"}
	}

	subject := &jwt.Subject{Payload: &jwt.Payload{}}
	if s.LocalCacheAware != nil {
		c, err := s.LocalCacheAware()
		if err != nil {
			return nil, nil, ex.Throw{Code: http.StatusServiceUnavailable, Msg: "missing cache object", Err: err}
		}
		subject.SetCache(c)
	}

	if err := subject.Verify(authHeader, s.jwtConfig.TokenKey); err != nil {
		return nil, nil, ex.Throw{Code: http.StatusUnauthorized, Msg: "invalid or expired token", Err: err}
	}

	if !subject.CheckReady() {
		return nil, nil, ex.Throw{Code: http.StatusUnauthorized, Msg: "token not ready"}
	}

	rawToken := make([]byte, len(authHeader))
	copy(rawToken, authHeader)
	return subject, rawToken, nil
}

func (s *WsServer) createConnectionContext(subject *jwt.Subject, socket *gws.Conn, path string, routerConfig *RouterConfig, rawToken []byte) *ConnectionContext {
	connCtx, cancel := context.WithCancel(s.globalCtx)

	devID := ""
	if subject != nil && subject.Payload != nil {
		devID = subject.Payload.Dev
	}
	if len(strings.TrimSpace(devID)) == 0 {
		devID = subject.GetDev(rawToken)
	}
	subID := subject.GetSub(nil)

	// Last 使用原子写入，与 UpdateLast/LastSeen 一致，便于 CleanupExpired 无锁读取
	now := utils.UnixSecond()
	devConn := &DevConn{
		Sub:    subID,
		Dev:    devID,
		Conn:   socket,
		ctx:    connCtx,
		cancel: cancel,
	}
	atomic.StoreInt64(&devConn.Last, now)

	return &ConnectionContext{
		Subject:      subject,
		WsConn:       socket,
		DevConn:      devConn,
		Server:       s,
		RouterConfig: routerConfig, // 使用传入的路由配置
		Path:         path,         // 设置WebSocket连接路径
		RawToken:     rawToken,     // 设置原始token字节
		ctx:          connCtx,
		cancel:       cancel,
	}
}

// processMessage 单条消息处理：从池中取 JsonBody 作为本条帧独占的 jsonBody，经 Process→replyData，defer Put 回池。
// 设计要点：ConnectionContext 不挂载 JsonBody；元信息快照一律来自本条 jsonBody。
func (s *WsServer) processMessage(connCtx *ConnectionContext, message []byte) {
	startAt := time.Now()
	defer func() {
		if r := recover(); r != nil {
			zlog.Error("process_message_panic", 0, zlog.Any("panic", r))
			s.recordMessageProcessed(false)
		}
	}()

	jsonBody := GetJsonBody()
	defer PutJsonBody(jsonBody)

	if zlog.IsDebug() {
		zlog.Debug("processing message", 0, zlog.String("message", string(message)), zlog.String("user_id", connCtx.GetUserIDString()), zlog.String("device_id", connCtx.getDeviceID()), zlog.String("connection_path", connCtx.Path))
	}

	// 创建消息处理器
	routeInfo, exists := s.routes[DefaultWsRoute]
	if !exists || routeInfo.Handle == nil {
		zlog.Error("NO_DEFAULT_ROUTE_CONFIGURED", 0, zlog.String("expected_route", DefaultWsRoute))
		return
	}
	messageHandler := GetMessageHandler(s.Cipher, routeInfo.Handle)
	defer PutMessageHandler(messageHandler)

	cipher, reply, err := messageHandler.Process(connCtx, message, jsonBody)
	if err != nil {
		// 记录消息处理失败指标
		s.recordMessageProcessed(false)
		if connCtx != nil && jsonBody != nil {
			if zlog.IsDebug() {
				zlog.Warn("WS_TRACE_PROCESS_FAILED", 0,
					zlog.String("conn_id", connCtx.wsTraceConnID()),
					zlog.String("subject", connCtx.GetUserIDString()),
					zlog.String("device", connCtx.getDeviceID()),
					zlog.String("router", jsonBody.Router),
					zlog.String("nonce", jsonBody.Nonce),
					zlog.Int64("plan", jsonBody.Plan),
					zlog.Int("frame_len", len(message)),
					zlog.String("frame_preview_hex", wsFrameHexPreview(message, 192)),
					zlog.Duration("elapsed", time.Since(startAt)),
					zlog.AddError(err))
			} else {
				zlog.Warn("WS_TRACE_PROCESS_FAILED", 0,
					zlog.String("conn_id", connCtx.wsTraceConnID()),
					zlog.String("subject", connCtx.GetUserIDString()),
					zlog.String("device", connCtx.getDeviceID()),
					zlog.String("router", jsonBody.Router),
					zlog.String("nonce", jsonBody.Nonce),
					zlog.Int64("plan", jsonBody.Plan),
					zlog.Int("frame_len", len(message)),
					zlog.Duration("elapsed", time.Since(startAt)),
					zlog.AddError(err))
			}
		}
		fallbackNonce := ""
		if jsonBody != nil {
			fallbackNonce = jsonBody.Nonce
		}
		s.errorHandler.handleConnectionError(connCtx, err, "process_message", fallbackNonce)
		return
	}

	// 记录消息处理成功指标
	s.recordMessageProcessed(true)

	// 快照当前请求元信息（来自本条消息的 jsonBody）
	meta := requestMeta{
		Router: jsonBody.Router,
		Nonce:  jsonBody.Nonce,
		Plan:   jsonBody.Plan,
		User:   jsonBody.User,
	}
	replyData(connCtx, meta, cipher, reply)
}

// replyData 将业务返回值写回客户端。
// 设计要点：若 Handle 返回 *JsonResp（如某些中间件已构造），直接序列化发送并 Put 回池；否则构造 Code=200 的 JsonResp，按 Plan 加密与签名后发送，所有 JsonResp 均由本函数或调用方 Put 回池。
func replyData(connCtx *ConnectionContext, req requestMeta, cipher crypto.Cipher, reply interface{}) {
	if reply == nil {
		return
	}

	if jsonResp, ok := reply.(*JsonResp); ok {
		bytesData, err := utils.JsonMarshal(jsonResp)
		PutJsonResp(jsonResp)
		if err != nil {
			zlog.Error("failed_to_marshal_jsonresp", 0, zlog.AddError(err), zlog.String("user_id", connCtx.GetUserIDString()), zlog.String("device_id", connCtx.getDeviceID()), zlog.String("connection_path", connCtx.Path))
			return
		}

		if err := connCtx.DevConn.Send(bytesData); err != nil {
			zlog.Error("failed_to_send_response", 0, zlog.AddError(err), zlog.String("user_id", connCtx.GetUserIDString()), zlog.String("device_id", connCtx.getDeviceID()), zlog.String("connection_path", connCtx.Path))
		}
		return
	}

	// 原有的逻辑：构造新的JsonResp并序列化reply
	jsonResp := GetJsonResp()
	defer PutJsonResp(jsonResp)
	jsonResp.Code = 200
	jsonResp.Message = "success"
	jsonResp.Data = ""
	jsonResp.Nonce = req.Nonce // 使用请求的nonce快照
	jsonResp.Time = utils.UnixSecond()
	jsonResp.Plan = req.Plan // 使用请求的plan快照

	// 序列化响应数据
	respData, err := utils.JsonMarshal(reply)
	if err != nil {
		zlog.Error("failed_to_marshal_reply_data", 0, zlog.AddError(err), zlog.String("user_id", connCtx.GetUserIDString()), zlog.String("device_id", connCtx.getDeviceID()), zlog.String("connection_path", connCtx.Path))
		return
	}

	// 派生密钥：用于响应加密(Plan==1)与签名，用毕统一清理以降低驻留风险
	secret := connCtx.GetTokenSecret()
	if len(secret) == 0 || len(secret) < 32 {
		zlog.Error("response_secret_missing", 0, zlog.String("user_id", connCtx.GetUserIDString()), zlog.String("device_id", connCtx.getDeviceID()), zlog.String("connection_path", connCtx.Path))
		return
	}
	defer DIC.ClearData(secret)

	if req.Plan == 1 {
		encryptedData, err := utils.AesGCMEncryptBase(respData, secret[:32], AppendBodyMessage(req.Router, "", jsonResp.Nonce, jsonResp.Time, jsonResp.Plan, req.User))
		if err != nil {
			zlog.Error("response_data_encrypt_failed", 0, zlog.AddError(err), zlog.String("user_id", connCtx.GetUserIDString()), zlog.String("device_id", connCtx.getDeviceID()), zlog.String("connection_path", connCtx.Path))
			return
		}
		jsonResp.Data = encryptedData
	} else {
		jsonResp.Data = utils.Base64Encode(utils.Bytes2Str(respData))
	}

	// 生成响应签名（使用同一 secret，避免二次派生）
	sign := SignBodyMessage(req.Router, jsonResp.Data, jsonResp.Nonce, jsonResp.Time, jsonResp.Plan, req.User, secret)
	jsonResp.Sign = utils.Base64Encode(sign)

	var validBytes []byte
	if cipher != nil {
		var err error
		validBytes, err = cipher.Sign(sign)
		if err != nil {
			zlog.Error("failed_to_outer_sign_response", 0, zlog.AddError(err), zlog.String("user_id", connCtx.GetUserIDString()), zlog.String("device_id", connCtx.getDeviceID()), zlog.String("connection_path", connCtx.Path))
			return
		}
	}
	jsonResp.Valid = utils.Base64Encode(validBytes)

	// 发送JsonResp格式的响应
	replyBytes, err := utils.JsonMarshal(jsonResp)
	if err != nil {
		zlog.Error("failed_to_marshal_jsonresp", 0, zlog.AddError(err), zlog.String("user_id", connCtx.GetUserIDString()), zlog.String("device_id", connCtx.getDeviceID()), zlog.String("connection_path", connCtx.Path))
		return
	}

	if err := connCtx.DevConn.Send(replyBytes); err != nil {
		zlog.Error("failed_to_send_reply", 0, zlog.AddError(err), zlog.String("user_id", connCtx.GetUserIDString()), zlog.String("device_id", connCtx.getDeviceID()), zlog.String("connection_path", connCtx.Path))
		return // 发送失败通常意味着连接已断开
	}

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

// AddCipher 注册本 WebSocket 服务端对 usr 的验签 Cipher；双向 Ed25519 时为 CreateEd25519WithBase64（服务端私钥，该客户端公钥），与 SocketSDK.SetEd25519Object 镜像。
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
