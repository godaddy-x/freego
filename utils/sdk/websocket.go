package sdk

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/godaddy-x/freego/utils/crypto"

	DIC "github.com/godaddy-x/freego/common"
	"github.com/godaddy-x/freego/ex"
	"github.com/godaddy-x/freego/node"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/zlog"
	"github.com/lxzan/gws"
)

// WebSocket客户端常量
const (
	DefaultWsRoute = "/ws" // 默认WebSocket路由路径
)

// abortDanglingGwsClient 关闭已启动 ReadLoop、但尚未写入 SocketSDK.s.conn 的 gws 客户端（例如握手失败），
// 避免 ReadLoop 与底层 TCP 长期悬挂影响后续重连。
func abortDanglingGwsClient(c *gws.Conn, code uint16, reason string) {
	if c == nil {
		return
	}
	_ = c.WriteClose(code, []byte(reason))
	if nc := c.NetConn(); nc != nil {
		_ = nc.Close()
	}
}

func (s *SocketSDK) gwsHandshakeTimeout() time.Duration {
	sec := s.timeout
	if sec <= 0 {
		sec = 30
	}
	if sec > 300 {
		sec = 300
	}
	return time.Duration(sec) * time.Second
}

// NewSocketSDK 创建新的WebSocket SDK实例并设置默认值
//
// domain: API域名，如"api.example.com"
//
// 默认值:
// - timeout: 120秒
// - maxReconnectAttempts: -1（无限重连，可由 SetReconnectConfig 改为有限次数）
// - reconnectInterval: 1秒
// - maxReconnectInterval: 8秒
// - HealthPing: 15秒（建议 10–15，内部不超过 15）
//
// 返回值:
//   - *SocketSDK: 初始化的WebSocket SDK实例
//
// 使用示例:
//
//	sdk := NewSocketSDK("api.example.com")
//	sdk.AuthToken(AuthToken{...})
//	sdk.SetClientNo(12345)
func NewSocketSDK(domain string) *SocketSDK {
	rootCtx, rootCancel := context.WithCancel(context.Background())

	return &SocketSDK{
		domain:               normalizeWSHost(domain),
		timeout:              120,
		maxReconnectAttempts: -1,
		reconnectInterval:    time.Second,
		maxReconnectInterval: 8 * time.Second,
		healthPing:           15,
		connectedPath:        DefaultWsRoute,
		rootCtx:              rootCtx,
		rootCancel:           rootCancel,
	}
}

// MessageHandler 消息处理器接口
type MessageHandler interface {
	HandleMessage(message *node.JsonResp) error
}

// Subscription 订阅信息
type Subscription struct {
	ID      string
	Router  string
	Handler MessageHandler
	active  bool
}

// ClientEventHandler gws 客户端事件处理器
type ClientEventHandler struct {
	sdk *SocketSDK
}

func (h *ClientEventHandler) OnOpen(socket *gws.Conn) {
	if zlog.IsDebug() {
		zlog.Debug("gws client connected", 0)
	}
}

func (h *ClientEventHandler) OnClose(socket *gws.Conn, err error) {
	if zlog.IsDebug() {
		zlog.Debug("gws client closed", 0, zlog.AddError(err))
	}
	// 触发断开连接逻辑：必须带上关闭的 socket，避免旧连接晚到的 OnClose 误伤已替换的新连接。
	if h.sdk != nil {
		h.sdk.disconnectWebSocketInternal(true, socket)
	}
}

func (h *ClientEventHandler) OnPing(socket *gws.Conn, payload []byte) {
	// 服务端 ping，客户端自动回复 pong（gws 内部处理）
}

func (h *ClientEventHandler) OnPong(socket *gws.Conn, payload []byte) {
	// 收到 pong（如果需要处理）
}

func (h *ClientEventHandler) OnMessage(socket *gws.Conn, message *gws.Message) {
	defer message.Close()
	if h.sdk != nil {
		h.sdk.websocketMessageListenerHandle(message.Bytes())
	}
}

type SocketSDK struct {
	domain        string                    // API域名 (如:api.example.com)，经 SetDomain / NewSocketSDK 规范化
	language      string                    // 语言设置 (HTTP头)
	timeout       int64                     // 请求超时时间(秒)
	authObject    interface{}               // 登录认证对象 (用户名+密码等)
	authToken     atomic.Pointer[AuthToken] // JWT认证令牌（原子快照）
	rawAuthHeader string                    // 自定义Authorization头（用于plan2等非JWT建连）
	broadcastKey  string                    // 广播数据签名密钥
	ssl           bool                      // 是否启用https
	clientNo      int64                     // 客户端ID
	mldsaObject map[int64]crypto.Cipher // Plan2：ML-DSA-87 外层签
	healthPing    int                       // 心跳间隔/秒，建议 10–15，内部最大 15

	// WebSocket连接相关（gws 使用 *gws.Conn）
	conn        *gws.Conn // gws WebSocket 连接
	connMutex   sync.Mutex
	isConnected bool // 连接状态
	connecting  bool // 是否正在建立连接中（防止并发连接）
	// 当前连接绑定的 token secret 快照，避免 token 刷新与重连窗口期出现签名混用。
	connectedTokenSecret atomic.Pointer[string]

	// 上下文管理（关键修复）
	rootCtx    context.Context    // SDK全局上下文（用于Close）
	rootCancel context.CancelFunc // 取消整个SDK
	connCtx    context.Context    // 当前连接上下文（每次重连新建）
	connCancel context.CancelFunc // 取消当前连接

	// goroutine 跟踪
	wg sync.WaitGroup // 跟踪心跳和监听 goroutine

	// 重连相关配置
	reconnectEnabled     bool          // 是否启用自动重连
	maxReconnectAttempts int           // 最大重连次数 (-1表示无限重连)
	reconnectInterval    time.Duration // 重连间隔
	maxReconnectInterval time.Duration // 最大重连间隔
	reconnectAttempts    int           // 当前重连次数
	lastReconnectTime    time.Time     // 上次重连时间
	reconnectMutex       sync.Mutex    // 重连互斥锁
	reconnecting         bool          // 是否正在重连中（防止并发重连）
	reconnectPending     int32         // atomic：为 1 表示曾有 startReconnectProcess 因「已在重连」被跳过，本轮结束后须补一次调度
	reconnectCredWaitRounds uint64     // atomic：无 JWT/raw 的空转轮数，仅用于日志（与 reconnectAttempts 解耦；后者在无凭证分支会回退故不会单调涨）
	connectedPath        string        // 已连接的WebSocket路径 (用于重连)

	// Token过期回调
	onTokenExpired      func() // Token过期时回调，用户可以重新认证
	tokenCallbackActive int32  // 回调执行中标记（0=空闲,1=执行中）
	tokenCallbackLastAt int64  // 回调最近触发时间戳（unix秒），用于节流
	tokenMonitorOnce    sync.Once

	// 新增：同步响应映射表 (nonce -> chan JsonResp)
	responseMap sync.Map // 存储等待响应的通道

	// 新增：服务端主动推送消息的回调
	onPushMessage func(router string, data []byte)

	// 消息订阅相关
	subscriptions sync.Map // 路由 -> 订阅信息 (线程安全)

	// 采样计数：响应匹配 miss 的高频告警限频
	responseNonceMiss uint64
}

// AuthObject 设置WebSocket客户端的登录认证对象
// 用于存储用户名、密码等登录凭据，自动登录时会使用此对象调用登录接口
// object: 认证对象，包含用户名密码等信息，请使用指针对象避免数据拷贝
func (s *SocketSDK) AuthObject(object interface{}) {
	s.authObject = object
}

// AuthToken 设置WebSocket客户端的JWT认证令牌
// 设置登录成功后获得的令牌，用于后续WebSocket消息的身份认证
// object: AuthToken结构体，包含token、secret、expired字段
func (s *SocketSDK) AuthToken(object AuthToken) {
	token := object
	s.authToken.Store(&token)
	// 典型场景：先起客户端、回调里 Plan2 登录成功后写入 JWT；此时主重连协程可能仍在退避 sleep 中。
	// 主动补一次调度，避免「凭证已就绪却仍等满当前退避窗口」甚至与 coalesce 竞态叠加导致迟迟不 dial。
	if s.valid() && s.isReconnectEnabled() && !s.IsWebSocketConnected() {
		go s.startReconnectProcess()
	}
}

func (s *SocketSDK) GetAuth() AuthToken {
	if token := s.authToken.Load(); token != nil {
		return *token
	}
	return AuthToken{}
}

// SetRawAuthorization 设置连接时直接使用的 Authorization 头（如 plan2 场景）。
func (s *SocketSDK) SetRawAuthorization(auth string) {
	s.rawAuthHeader = auth
}

// normalizeWSHost 规范化域名/host:port：去空白并转小写，避免大小写混用导致 URI 不一致。
func normalizeWSHost(host string) string {
	return strings.ToLower(strings.TrimSpace(host))
}

// SetDomain 设置 WebSocket 连接的域名或 host:port（如 api.example.com 或 api.example.com:443）。
func (s *SocketSDK) SetDomain(domain string) {
	s.domain = normalizeWSHost(domain)
}

// GetDomain 返回当前配置的域名/host（只读）。
func (s *SocketSDK) GetDomain() string {
	return s.domain
}

// GetClientNo 返回当前客户端编号（只读）。
func (s *SocketSDK) GetClientNo() int64 {
	return s.clientNo
}

// GetSSL 返回是否使用 wss（只读）。
func (s *SocketSDK) GetSSL() bool {
	return s.ssl
}

func (s *SocketSDK) getTokenSecretForSigning() string {
	if p := s.connectedTokenSecret.Load(); p != nil && len(*p) > 0 {
		return *p
	}
	return s.GetAuth().Secret
}

// decodeJWTSecretBase64String 将 JWT Secret 的 Base64 字符串解码并校验长度（AES-256 密钥材料至少 32 字节）。
// 成功返回的字节须由调用方 DIC.ClearData；失败时不会泄漏未清理的长切片。
func decodeJWTSecretBase64String(b64 string) ([]byte, error) {
	s := strings.TrimSpace(b64)
	if len(s) == 0 {
		return nil, ex.Throw{Msg: "jwt token secret is empty, set AuthToken or wait for reconnect"}
	}
	raw := utils.Base64Decode(s)
	if len(raw) < 32 {
		if len(raw) > 0 {
			DIC.ClearData(raw)
		}
		return nil, ex.Throw{Msg: "jwt token secret invalid: base64 decoded length must be at least 32 bytes"}
	}
	return raw, nil
}

// decodeTokenSecretForSend 当前用于签名的 Secret 字符串（连接快照或 AuthToken）解码并校验。
// 返回的字节须由调用方 DIC.ClearData。
func (s *SocketSDK) decodeTokenSecretForSend() ([]byte, error) {
	return decodeJWTSecretBase64String(s.getTokenSecretForSigning())
}

// SetTimeout 设置WebSocket请求的超时时间
// timeout: 超时时间(秒)，控制WebSocket消息发送和等待响应的最大时间
func (s *SocketSDK) SetTimeout(timeout int64) {
	s.timeout = timeout
}

// SetBroadcastKey 广播数据密钥
func (s *SocketSDK) SetBroadcastKey(key string) {
	s.broadcastKey = key
}

// SetHealthPing 设置心跳间隔（秒）。建议 10–15 秒，与服务端读超时策略匹配；内部限制最大 15 秒，超过则按 15 秒生效。
func (s *SocketSDK) SetHealthPing(t int) {
	const maxHealthPing = 15
	if t <= 0 {
		t = 15
	}
	if t > maxHealthPing {
		t = maxHealthPing
	}
	s.healthPing = t
}

func (s *SocketSDK) SetClientNo(clientNo int64) {
	s.clientNo = clientNo
}

func (s *SocketSDK) SetSSL(ssl bool) {
	s.ssl = ssl
}

// SetLanguage 设置WebSocket请求的语言标识
// language: 语言代码，如"zh-CN"、"en-US"，用于服务端国际化支持
func (s *SocketSDK) SetLanguage(language string) {
	lang := strings.TrimSpace(language)
	if lang == "" {
		s.language = ""
		return
	}
	s.language = strings.ToLower(lang)
}

// SetPushMessageCallback 设置服务端主动推送消息的回调函数
func (s *SocketSDK) SetPushMessageCallback(callback func(router string, data []byte)) {
	s.onPushMessage = callback
}

func sanitizeWSPath(path string) (string, error) {
	p := strings.TrimSpace(path)
	if len(p) == 0 {
		return DefaultWsRoute, nil
	}
	if p != DefaultWsRoute {
		return "", ex.Throw{Msg: "websocket path invalid, only /ws is allowed"}
	}
	return p, nil
}

// SetWebSocketPath 设置连接路径。默认固定 /ws，若要覆盖需显式调用本方法。
func (s *SocketSDK) SetWebSocketPath(path string) error {
	p, err := sanitizeWSPath(path)
	if err != nil {
		return err
	}
	s.reconnectMutex.Lock()
	s.connectedPath = p
	s.reconnectMutex.Unlock()
	return nil
}

// getURI 构建完整的WebSocket连接URI
// path: WebSocket路径，如"/ws"
// 返回: 完整的WebSocket URI，支持ws和wss协议
func (s *SocketSDK) getURI(path string) string {
	var p string
	if s.ssl {
		u := url.URL{Scheme: "wss", Host: s.domain, Path: path}
		p = u.String()
	} else {
		u := url.URL{Scheme: "ws", Host: s.domain, Path: path}
		p = u.String()
	}
	return p
}

// verifyPushMessageSignature 验证推送消息的签名
func (s *SocketSDK) verifyPushMessageSignature(res *node.JsonResp) error {
	if res.Router == "" {
		return utils.Error("push message router is empty")
	}
	if res.Data == "" {
		return utils.Error("push message data is empty")
	}
	if res.Sign == "" {
		return utils.Error("push message signature is empty")
	}
	if res.Nonce == "" {
		return utils.Error("push message nonce is empty")
	}

	// 使用服务器推送专用签名密钥进行验证
	// 注意：这与服务器端的签名逻辑保持一致
	expectedSign := node.SignBodyMessage(res.Router, res.Data, res.Nonce, res.Time, res.Plan, 0, utils.Str2Bytes(s.broadcastKey))

	if !utils.CompareBase64Sign(expectedSign, res.Sign) {
		return utils.Error("push message signature verification failed")
	}

	return nil
}

// decryptPushMessageData 解密推送消息的数据
func (s *SocketSDK) decryptPushMessageData(res *node.JsonResp) ([]byte, error) {
	if res.Data == "" {
		return nil, utils.Error("push message data is empty")
	}
	if res.Nonce == "" {
		return nil, utils.Error("push message nonce is empty")
	}

	// 推送消息使用明文传输（Plan=0）
	if res.Plan == 0 {
		// 直接Base64解码
		rawData := utils.Base64Decode(res.Data)
		if len(rawData) == 0 {
			return nil, utils.Error("push message base64 decode failed")
		}
		return rawData, nil
	}

	// 如果将来需要支持加密的推送消息，可以在这里添加解密逻辑
	return nil, utils.Error("unsupported push message plan: ", res.Plan)
}

// valid 验证当前认证令牌是否有效
// 检查令牌是否存在、secret 非空且 Base64 解码后长度满足 AES 使用下限，以及未过期。
// 返回: true表示令牌有效，false表示需要重新认证
func (s *SocketSDK) valid() bool {
	auth := s.GetAuth()
	if len(auth.Token) == 0 {
		return false
	}
	if utils.UnixSecond() > auth.Expired {
		return false
	}
	raw, err := decodeJWTSecretBase64String(auth.Secret)
	if err != nil {
		return false
	}
	DIC.ClearData(raw)
	return true
}

func (s *SocketSDK) addOuterSign(jsonBody *node.JsonBody, plan2KeyBootstrap bool) error {
	if !node.JsonBodyRequiresOuterSignature(jsonBody.Plan, plan2KeyBootstrap) {
		return nil
	}
	if s.mldsaObject == nil {
		return ex.Throw{Msg: "ML-DSA object not configured"}
	}
	cipher, exists := s.mldsaObject[s.clientNo]
	if !exists || cipher == nil {
		return ex.Throw{Msg: "ML-DSA object not found for client"}
	}
	outerSign, err := cipher.Sign(node.DigestBodyMessage(jsonBody.Router, jsonBody.Data, jsonBody.Nonce, jsonBody.Time, jsonBody.Plan, jsonBody.User))
	if err != nil {
		return ex.Throw{Msg: "ML-DSA sign failed: " + err.Error()}
	}
	jsonBody.Valid = utils.Base64Encode(outerSign)
	DIC.ClearData(outerSign)
	return nil
}

func (s *SocketSDK) verifyOuterSign(path string, usr int64, respData *node.JsonResp) error {
	if !node.PlanRequiresOuterSignature(respData.Plan) {
		return nil
	}
	if s.mldsaObject == nil {
		return ex.Throw{Msg: "ML-DSA object not configured"}
	}
	cipher, exists := s.mldsaObject[s.clientNo]
	if !exists || cipher == nil {
		return ex.Throw{Msg: "ML-DSA object not found for client"}
	}
	if !crypto.CheckOuterSignatureB64Valid(respData.Valid) {
		return ex.Throw{Msg: "response outer signature length invalid"}
	}
	outerSignData := utils.Base64Decode(respData.Valid)
	defer DIC.ClearData(outerSignData)
	if err := cipher.Verify(node.DigestBodyMessage(path, respData.Data, respData.Nonce, respData.Time, respData.Plan, usr), outerSignData); err != nil {
		return ex.Throw{Msg: "response ML-DSA sign verify invalid"}
	}
	return nil
}

// SetMLDSA87Object 配置 Plan2 WebSocket 客户端身份（与服务端 WsServer.AddPQCipher 镜像）。
func (s *SocketSDK) SetMLDSA87Object(usr int64, prkB64, peerPubB64 string) error {
	if s.mldsaObject == nil {
		s.mldsaObject = make(map[int64]crypto.Cipher)
	}
	cipher, err := crypto.CreateMLDSA87WithBase64(prkB64, peerPubB64)
	if err != nil {
		return err
	}
	s.mldsaObject[usr] = cipher
	return nil
}

// ConnectWebSocket 建立WebSocket连接并启动相关服务（默认 /ws，可通过 SetWebSocketPath 覆盖）。
// 返回: 连接成功返回nil，否则返回连接失败的错误信息
func (s *SocketSDK) ConnectWebSocket() error {
	s.reconnectMutex.Lock()
	p := s.getConnectedPathLocked()
	s.connectedPath = p // 保持重连路径与当前配置一致
	s.reconnectMutex.Unlock()

	p, err := sanitizeWSPath(p)
	if err != nil {
		return err
	}
	// raw authorization 模式不依赖 token 生命周期，避免触发无意义回调
	if len(s.rawAuthHeader) == 0 {
		s.startTokenMonitor()
		// 主线程同步尝试一次 token 获取，避免首次连接前 token 仍为空
		if !s.valid() {
			s.triggerTokenExpiredCallbackSync()
		}
	}

	err = s.connectWebSocketInternal(p, true)
	if err == nil {
		return nil
	}
	// 启用自动重连时，首次连接失败转为后台重连，不阻塞业务启动流程。
	if s.isReconnectEnabled() {
		go s.startReconnectProcess()
		zlog.Info("initial websocket connect failed, starting async reconnect", 0, zlog.String("errorMsg", ex.Catch(err).Msg))
		if zlog.IsDebug() {
			zlog.Debug(fmt.Sprintf("initial websocket connect failed, fallback to async reconnect: %v", err), 0)
		}
		return nil
	}
	return err
}

func (s *SocketSDK) startTokenMonitor() {
	s.tokenMonitorOnce.Do(func() {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			ticker := time.NewTicker(1 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-s.rootCtx.Done():
					return
				case <-ticker.C:
					// 仅在“未连接”状态触发过期回调；已连接会话按连接态继续工作，等断线重连时再刷新 token。
					if len(s.rawAuthHeader) == 0 && !s.valid() && !s.IsWebSocketConnected() {
						s.triggerTokenExpiredCallback()
					}
				}
			}
		}()
	})
}

func (s *SocketSDK) isReconnectEnabled() bool {
	s.reconnectMutex.Lock()
	defer s.reconnectMutex.Unlock()
	return s.reconnectEnabled
}

// connectWebSocketInternal WebSocket连接的内部实现方法
// path: WebSocket连接路径
// isInitial: 是否为初始连接，用于重连逻辑判断
// 返回: 连接成功返回nil，否则返回连接失败的错误信息
func (s *SocketSDK) connectWebSocketInternal(path string, isInitial bool) error {
	// 第一阶段：检查状态并设置连接中标志（短暂持有锁）
	s.connMutex.Lock()
	if s.connecting {
		s.connMutex.Unlock()
		return ex.Throw{Msg: "connection already in progress"}
	}
	if s.isConnected && s.conn != nil && !isInitial {
		s.connMutex.Unlock()
		return nil
	}
	if !s.valid() && len(s.rawAuthHeader) == 0 {
		s.connMutex.Unlock()
		return ex.Throw{Msg: "token empty or token expired, and raw authorization is empty"}
	}

	// 取消旧连接上下文；新 connCtx/connCancel 仅在拨号+握手成功后与 s.conn 一并写入，
	// 避免「s.conn 仍指向旧连接时提前切换 ctx」导致旧 OnClose / disconnect 误 cancel 新拨号使用的 context。
	if s.connCancel != nil {
		s.connCancel()
	}

	// 设置连接中标志，防止并发连接
	s.connecting = true
	s.connMutex.Unlock()

	// 确保连接中标志最终重置
	defer func() {
		s.connMutex.Lock()
		s.connecting = false
		s.connMutex.Unlock()
	}()

	// 第二阶段：执行连接和握手（不持有锁，避免阻塞其他操作）
	wsURL := s.getURI(path)

	// 设置认证头（gws Dialer）
	header := http.Header{}
	authSnapshot := s.GetAuth()
	authHeader := authSnapshot.Token
	if len(authHeader) == 0 {
		authHeader = s.rawAuthHeader
	}
	header.Set("Authorization", authHeader)
	header.Set("Language", s.language)

	if zlog.IsDebug() {
		if isInitial {
			zlog.Debug(fmt.Sprintf("connecting to WebSocket: %s", wsURL), 0)
		} else {
			zlog.Debug(fmt.Sprintf("reconnecting to WebSocket (attempt %d): %s", s.reconnectAttempts+1, wsURL), 0)
		}
	}

	// 建立WebSocket连接（gws）
	handler := &ClientEventHandler{sdk: s}
	socket, _, err := gws.NewClient(handler, &gws.ClientOption{
		Addr:                wsURL,
		RequestHeader:       header,
		ReadMaxPayloadSize:  1024 * 1024 * 10,
		WriteMaxPayloadSize: 1024 * 1024 * 10,
		HandshakeTimeout:    s.gwsHandshakeTimeout(),
		// 显式串行 OnMessage：websocketMessageListenerHandle 内每帧独立 GetJsonResp，无共享 conn 级 JsonBody；
		// 若将来改为 true，须保证本函数及 responseMap/订阅回调全路径可重入（与 gws Server 侧 Parallel 策略对齐后再开）。
		ParallelEnabled: false,
		Recovery:        gws.Recovery,
	})
	if err != nil {
		if zlog.IsDebug() {
			zlog.Debug(fmt.Sprintf("WebSocket connection failed: %v", err), 0)
		}
		return ex.Throw{Msg: "WebSocket connection failed: " + err.Error()}
	}

	// 启动 ReadLoop（gws 客户端需要手动启动）
	go socket.ReadLoop()

	// JWT模式下发送认证握手；raw authorization 模式跳过
	if s.valid() {
		if err := s.sendWebSocketAuthHandshake(socket, path); err != nil {
			abortDanglingGwsClient(socket, 1000, "handshake failed")
			if zlog.IsDebug() {
				zlog.Debug(fmt.Sprintf("WebSocket handshake failed: %v", err), 0)
			}
			return err
		}
	}

	// 第三阶段：更新连接状态（再次获取锁）；在此绑定 connCtx，与 s.conn 生命周期一致。
	s.connMutex.Lock()
	s.connCtx, s.connCancel = context.WithCancel(s.rootCtx)
	s.conn = socket
	s.isConnected = true
	if len(authSnapshot.Token) > 0 {
		secret := authSnapshot.Secret
		s.connectedTokenSecret.Store(&secret)
	} else {
		s.connectedTokenSecret.Store(nil)
	}
	s.connMutex.Unlock()

	// 重置重连计数
	if !isInitial {
		s.reconnectMutex.Lock()
		s.reconnectAttempts = 0
		s.lastReconnectTime = time.Time{}
		s.reconnectMutex.Unlock()

		if zlog.IsDebug() {
			zlog.Debug("WebSocket reconnection successful", 0)
		}

		// 重连成功后，自动重新订阅所有主题
		s.resubscribeAfterReconnect()
	} else {
		if zlog.IsDebug() {
			zlog.Debug("WebSocket connection established successfully", 0)
		}
	}

	// --- 启动心跳（gws 已自动处理消息监听） ---
	s.wg.Add(1)
	go s.websocketHeartbeat()

	return nil
}

// SendWebSocketRawBody 直接发送已构造好的 JsonBody（用于 plan2/key-login 等自定义流程）。
func (s *SocketSDK) SendWebSocketRawBody(body *node.JsonBody, waitResponse bool, timeout int64) (*node.JsonResp, error) {
	if body == nil {
		return nil, ex.Throw{Msg: "json body is nil"}
	}
	if waitResponse && !utils.CheckLen(body.Nonce, 8, 32) {
		return nil, ex.Throw{Msg: "json body nonce invalid"}
	}
	bytesData, err := utils.JsonMarshal(body)
	if err != nil {
		return nil, ex.Throw{Msg: "jsonBody marshal failed"}
	}
	defer DIC.ClearData(bytesData)

	var respChan chan *node.JsonResp
	if waitResponse {
		respChan = make(chan *node.JsonResp, 1)
		s.responseMap.Store(body.Nonce, respChan)
		defer s.responseMap.LoadAndDelete(body.Nonce)
	}

	s.connMutex.Lock()
	if !s.isConnected || s.conn == nil {
		s.connMutex.Unlock()
		return nil, ex.Throw{Msg: "WebSocket not connected, call ConnectWebSocket first"}
	}
	conn := s.conn
	if err := conn.WriteMessage(gws.OpcodeText, bytesData); err != nil {
		s.connMutex.Unlock()
		return nil, ex.Throw{Msg: "WebSocket message send failed: " + err.Error()}
	}
	s.connMutex.Unlock()

	if !waitResponse {
		return nil, nil
	}

	waitTimeout := 10 * time.Second
	if timeout > 0 {
		waitTimeout = time.Duration(timeout) * time.Second
	}
	select {
	case resp := <-respChan:
		return resp, nil
	case <-time.After(waitTimeout):
		return nil, ex.Throw{
			Code: ex.WS_WAIT,
			Msg:  fmt.Sprintf("wait raw response timeout (router=%s, nonce=%s, timeout=%ds)", body.Router, body.Nonce, int(waitTimeout/time.Second)),
		}
	case <-s.connCtx.Done():
		return nil, ex.Throw{
			Code: ex.WS_WAIT,
			Msg:  fmt.Sprintf("connection closed while waiting raw response (router=%s, nonce=%s, timeout=%ds)", body.Router, body.Nonce, int(waitTimeout/time.Second)),
		}
	}
}

// ConnectWebSocketWithRawAuth 使用自定义 Authorization 头建立连接（适合 plan2 首连）。
func (s *SocketSDK) ConnectWebSocketWithRawAuth(path, authHeader string) error {
	if len(authHeader) == 0 {
		return ex.Throw{Msg: "authorization header is empty"}
	}
	p, err := sanitizeWSPath(path)
	if err != nil {
		return err
	}
	s.SetRawAuthorization(authHeader)
	if err := s.SetWebSocketPath(p); err != nil {
		return err
	}
	return s.ConnectWebSocket()
}

func (s *SocketSDK) buildPlan2BootstrapAuthorization() (string, error) {
	if s.mldsaObject == nil {
		return "", ex.Throw{Msg: "ML-DSA object not configured"}
	}
	cipher, exists := s.mldsaObject[s.clientNo]
	if !exists || cipher == nil {
		return "", ex.Throw{Msg: "ML-DSA object not found for client"}
	}
	pub, err := node.CreatePublicKey(
		utils.Base64Encode(utils.GetRandomSecure(32)),
		utils.Base64Encode(utils.GetRandomSecure(32)),
		s.clientNo,
		cipher,
	)
	if err != nil {
		return "", err
	}
	authBytes, err := utils.JsonMarshal(pub)
	if err != nil {
		return "", ex.Throw{Msg: "plan2 bootstrap authorization marshal failed"}
	}
	defer DIC.ClearData(authBytes)
	return utils.Base64Encode(authBytes), nil
}

// GetWebSocketPlan2Auth 通过 /key 路由完成 key 协商，返回用于重连的 raw Authorization 与共享密钥。
func (s *SocketSDK) GetWebSocketPlan2Auth(keyRouter string, timeout int64) (string, []byte, error) {
	if len(strings.TrimSpace(keyRouter)) == 0 {
		return "", nil, ex.Throw{Msg: "key router is empty"}
	}
	if s.mldsaObject == nil {
		return "", nil, ex.Throw{Msg: "ML-DSA object not configured"}
	}
	cipher, exists := s.mldsaObject[s.clientNo]
	if !exists || cipher == nil {
		return "", nil, ex.Throw{Msg: "ML-DSA object not found for client"}
	}

	reqPublic, err := node.CreatePublicKey(
		utils.Base64Encode(utils.GetRandomSecure(32)),
		utils.Base64Encode(utils.GetRandomSecure(32)),
		s.clientNo,
		cipher,
	)
	if err != nil {
		return "", nil, err
	}
	reqData, err := utils.JsonMarshal(reqPublic)
	if err != nil {
		return "", nil, ex.Throw{Msg: "plan2 key request marshal failed"}
	}
	defer DIC.ClearData(reqData)

	jsonBody := node.GetJsonBody()
	defer node.PutJsonBody(jsonBody)
	jsonBody.Time = utils.UnixSecond()
	jsonBody.Nonce = utils.GetUUID(true)
	jsonBody.Plan = 0
	jsonBody.Router = keyRouter
	jsonBody.User = s.clientNo
	jsonBody.Data = utils.Base64Encode(reqData)

	sign, _ := node.SignAndDigestBodyMessage(jsonBody.Router, jsonBody.Data, jsonBody.Nonce, jsonBody.Time, jsonBody.Plan, jsonBody.User, []byte{})
	jsonBody.Sign = utils.Base64Encode(sign)
	if err := s.addOuterSign(jsonBody, true); err != nil {
		return "", nil, err
	}

	resp, err := s.SendWebSocketRawBody(jsonBody, true, timeout)
	if err != nil {
		return "", nil, err
	}
	if resp == nil {
		return "", nil, ex.Throw{Msg: "plan2 key response is nil"}
	}
	if resp.Code != 200 {
		return "", nil, ex.Throw{Code: resp.Code, Msg: resp.Message}
	}
	if resp.Time <= 0 || utils.MathAbs(utils.UnixSecond()-resp.Time) > 300 {
		return "", nil, ex.Throw{Msg: "plan2 key response time invalid"}
	}
	validSign, _ := node.SignAndDigestBodyMessage(keyRouter, resp.Data, resp.Nonce, resp.Time, resp.Plan, s.clientNo, []byte{})
	defer DIC.ClearData(validSign)
	if !utils.CompareBase64Sign(validSign, resp.Sign) {
		return "", nil, ex.Throw{Msg: "plan2 key response signature verification failed"}
	}
	if err := s.verifyOuterSign(keyRouter, s.clientNo, resp); err != nil {
		return "", nil, err
	}
	respData := utils.Base64Decode(resp.Data)
	defer DIC.ClearData(respData)
	if len(respData) == 0 {
		return "", nil, ex.Throw{Msg: "plan2 key response data is empty"}
	}
	serverPub := &node.PublicKey{}
	if err := utils.JsonUnmarshal(respData, serverPub); err != nil {
		return "", nil, ex.Throw{Msg: "plan2 key response parse failed"}
	}
	if err := node.CheckPublicKey(nil, serverPub, cipher); err != nil {
		return "", nil, err
	}

	sharedB64, kemCtB64, err := crypto.EncapsulateToPeer(serverPub.Key)
	if err != nil {
		return "", nil, ex.Throw{Msg: "ML-KEM encapsulate failed: " + err.Error()}
	}
	sharedRaw := utils.Base64Decode(sharedB64)
	if len(sharedRaw) == 0 {
		return "", nil, ex.Throw{Msg: "ML-KEM shared secret invalid"}
	}
	defer DIC.ClearData(sharedRaw)
	sharedKey, err := node.HKDFKey(sharedRaw, serverPub.Noc)
	if err != nil {
		return "", nil, ex.Throw{Msg: "HKDF shared key failed"}
	}

	authPub, err := node.CreatePublicKey(serverPub.Key, kemCtB64, s.clientNo, cipher)
	if err != nil {
		DIC.ClearData(sharedKey)
		return "", nil, err
	}
	authBytes, err := utils.JsonMarshal(authPub)
	if err != nil {
		DIC.ClearData(sharedKey)
		return "", nil, ex.Throw{Msg: "plan2 authorization marshal failed"}
	}
	defer DIC.ClearData(authBytes)
	return utils.Base64Encode(authBytes), sharedKey, nil
}

// LoginByWebSocketPlan2Auto 自动完成 WS plan2 的 key + login 全流程，并将登录响应写入 responseObj。
// 连接路径默认使用 SDK 当前路径配置（默认 /ws，可通过 SetWebSocketPath 覆盖）。
func (s *SocketSDK) LoginByWebSocketPlan2Auto(keyRouter, loginRouter string, requestObj, responseObj interface{}, timeout int64) error {
	if requestObj == nil || responseObj == nil {
		return ex.Throw{Msg: "params invalid"}
	}
	s.reconnectMutex.Lock()
	wsPath := s.getConnectedPathLocked()
	s.reconnectMutex.Unlock()
	bootstrapAuth, err := s.buildPlan2BootstrapAuthorization()
	if err != nil {
		return err
	}
	if err := s.ConnectWebSocketWithRawAuth(wsPath, bootstrapAuth); err != nil {
		return err
	}

	authHeader, sharedKey, err := s.GetWebSocketPlan2Auth(keyRouter, timeout)
	if err != nil {
		s.DisconnectWebSocket()
		return err
	}
	defer DIC.ClearData(sharedKey)

	s.DisconnectWebSocket()
	if err := s.ConnectWebSocketWithRawAuth(wsPath, authHeader); err != nil {
		return err
	}
	if err := s.SendWebSocketPlan2Message(loginRouter, requestObj, responseObj, sharedKey, timeout); err != nil {
		return err
	}
	// 自动流程成功后切回 JWT 模式，避免后续复用同一 SDK 时误走 raw-auth 分支。
	s.rawAuthHeader = ""
	return nil
}

// SendWebSocketPlan2Message 发送 plan2 消息（AES-GCM + HMAC + ML-DSA），用于 key/login 等流程。
func (s *SocketSDK) SendWebSocketPlan2Message(router string, requestObj, responseObj interface{}, sharedKey []byte, timeout int64) error {
	if len(router) == 0 || requestObj == nil || responseObj == nil {
		return ex.Throw{Msg: "params invalid"}
	}
	if len(sharedKey) < 32 {
		return ex.Throw{Msg: "shared key invalid"}
	}
	if s.mldsaObject == nil {
		return ex.Throw{Msg: "ML-DSA object not configured"}
	}

	jsonData, err := utils.JsonMarshal(requestObj)
	if err != nil {
		return ex.Throw{Msg: "request data marshal failed"}
	}
	defer DIC.ClearData(jsonData)

	jsonBody := node.GetJsonBody()
	defer node.PutJsonBody(jsonBody)
	jsonBody.Time = utils.UnixSecond()
	jsonBody.Nonce = utils.GetUUID(true)
	jsonBody.Plan = 2
	jsonBody.Router = router
	jsonBody.User = s.clientNo

	enc, err := utils.AesGCMEncryptBase(jsonData, sharedKey[:32], node.AppendBodyMessage(jsonBody.Router, "", jsonBody.Nonce, jsonBody.Time, jsonBody.Plan, jsonBody.User))
	if err != nil {
		return ex.Throw{Msg: "plan2 data encrypt failed"}
	}
	jsonBody.Data = enc
	signData, _ := node.SignAndDigestBodyMessage(jsonBody.Router, jsonBody.Data, jsonBody.Nonce, jsonBody.Time, jsonBody.Plan, jsonBody.User, sharedKey)
	jsonBody.Sign = utils.Base64Encode(signData)
	DIC.ClearData(signData)

	if err := s.addOuterSign(jsonBody, false); err != nil {
		return err
	}

	resp, err := s.SendWebSocketRawBody(jsonBody, true, timeout)
	if err != nil {
		return err
	}
	if resp == nil {
		return ex.Throw{Msg: "plan2 response is nil"}
	}
	if resp.Code != 200 {
		return ex.Throw{Code: resp.Code, Msg: resp.Message}
	}
	if resp.Time <= 0 || utils.MathAbs(utils.UnixSecond()-resp.Time) > 300 {
		return ex.Throw{Msg: "response time invalid"}
	}

	validSign, _ := node.SignAndDigestBodyMessage(router, resp.Data, resp.Nonce, resp.Time, resp.Plan, s.clientNo, sharedKey)
	defer DIC.ClearData(validSign)
	if !utils.CompareBase64Sign(validSign, resp.Sign) {
		return ex.Throw{Msg: "plan2 response signature verification failed"}
	}
	if err := s.verifyOuterSign(router, s.clientNo, resp); err != nil {
		return err
	}

	var dec []byte
	switch resp.Plan {
	case 2, 1:
		dec, err = utils.AesGCMDecryptBase(resp.Data, sharedKey[:32], node.AppendBodyMessage(router, "", resp.Nonce, resp.Time, resp.Plan, s.clientNo))
		if err != nil {
			return ex.Throw{Msg: "plan2 response data decrypt failed"}
		}
	case 0:
		dec = utils.Base64Decode(resp.Data)
	default:
		return ex.Throw{Msg: "response plan invalid"}
	}
	defer DIC.ClearData(dec)
	if len(dec) == 0 {
		return ex.Throw{Msg: "response data is empty"}
	}
	if err := utils.JsonUnmarshalFast(dec, responseObj); err != nil {
		return ex.Throw{Msg: "response data unmarshal failed"}
	}
	return nil
}

// LoginByWebSocketPlan2 通过 WebSocket plan2 登录，并自动写入 AuthToken。
func (s *SocketSDK) LoginByWebSocketPlan2(loginRouter string, requestObj interface{}, sharedKey []byte, timeout int64) error {
	if len(loginRouter) == 0 {
		return ex.Throw{Msg: "login router is empty"}
	}
	resp := AuthToken{}
	if err := s.SendWebSocketPlan2Message(loginRouter, requestObj, &resp, sharedKey, timeout); err != nil {
		return err
	}
	s.AuthToken(resp)
	if len(resp.Token) > 0 {
		s.rawAuthHeader = ""
	}
	return nil
}

// prepareHeartbeatMessage 准备心跳消息数据
func (s *SocketSDK) prepareHeartbeatMessage(router string, data interface{}) ([]byte, string, error) {
	heartbeatUUID := utils.GetUUID(true)

	// 序列化心跳数据
	jsonData, err := utils.JsonMarshal(data)
	if err != nil {
		return nil, "", ex.Throw{Msg: "heartbeat data marshal failed"}
	}

	jsonBody := node.GetJsonBody()
	defer node.PutJsonBody(jsonBody)

	// 设置心跳消息的基本字段
	jsonBody.Time = utils.UnixSecond()
	jsonBody.Nonce = heartbeatUUID
	jsonBody.Router = router
	jsonBody.Plan = 0 // 心跳使用明文
	jsonBody.User = s.clientNo

	// 根据plan设置数据
	jsonBody.Data = utils.Base64Encode(utils.Bytes2Str(jsonData))
	DIC.ClearData(jsonData) // 立即清理序列化后的数据

	// 生成签名（心跳也需要签名验证）
	tokenSecret, err := s.decodeTokenSecretForSend()
	if err != nil {
		return nil, "", err
	}

	signData, _ := node.SignAndDigestBodyMessage(jsonBody.Router, jsonBody.Data, jsonBody.Nonce, jsonBody.Time, jsonBody.Plan, jsonBody.User, tokenSecret)
	jsonBody.Sign = utils.Base64Encode(signData)
	DIC.ClearData(signData)    // 立即清理签名数据
	DIC.ClearData(tokenSecret) // 立即清理token secret

	bytesData, err := utils.JsonMarshal(jsonBody)
	if err != nil {
		return nil, "", ex.Throw{Msg: "heartbeat jsonBody marshal failed"}
	}

	if zlog.IsDebug() {
		zlog.Debug("prepared heartbeat data", 0,
			zlog.String("data", utils.Bytes2Str(bytesData)),
			zlog.String("uuid", heartbeatUUID))
	}

	return bytesData, heartbeatUUID, nil
}

// prepareWebSocketMessage 准备WebSocket消息数据，包括加密和签名
// jsonBody: 消息体结构体
// data: 原始请求数据
// 返回: 处理后的消息体、序列化后的字节数据和可能的错误
func (s *SocketSDK) prepareWebSocketMessage(jsonBody *node.JsonBody, data interface{}) (*node.JsonBody, []byte, error) {
	// 序列化数据
	jsonData, err := utils.JsonMarshal(data)
	if err != nil {
		return nil, nil, ex.Throw{Msg: "data marshal failed"}
	}
	defer DIC.ClearData(jsonData)

	// 解码 token secret 用于加密和签名（发送前强校验，避免空/过短导致切片越界或无效包）
	tokenSecret, err := s.decodeTokenSecretForSend()
	if err != nil {
		return nil, nil, err
	}
	defer DIC.ClearData(tokenSecret)

	// 根据plan决定是否加密
	if jsonBody.Plan == 1 {
		encryptedData, err := utils.AesGCMEncryptBase(jsonData, tokenSecret[:32], node.AppendBodyMessage(jsonBody.Router, "", jsonBody.Nonce, jsonBody.Time, jsonBody.Plan, jsonBody.User))
		if err != nil {
			return nil, nil, ex.Throw{Msg: "data encrypt failed"}
		}
		jsonBody.Data = encryptedData
		if zlog.IsDebug() {
			zlog.Debug(fmt.Sprintf("data encrypted: %s", jsonBody.Data), 0)
		}
	} else {
		jsonBody.Data = utils.Base64Encode(utils.Bytes2Str(jsonData))
		if zlog.IsDebug() {
			zlog.Debug(fmt.Sprintf("data base64: %s", jsonBody.Data), 0)
		}
	}

	// 生成签名
	signData, _ := node.SignAndDigestBodyMessage(jsonBody.Router, jsonBody.Data, jsonBody.Nonce, jsonBody.Time, jsonBody.Plan, jsonBody.User, tokenSecret)
	jsonBody.Sign = utils.Base64Encode(signData)
	defer DIC.ClearData(signData)

	// 序列化最终的JsonBody
	bytesData, err := utils.JsonMarshal(jsonBody)
	if err != nil {
		return nil, nil, ex.Throw{Msg: "jsonBody marshal failed"}
	}

	if zlog.IsDebug() {
		zlog.Debug("prepared message data: ", 0)
		zlog.Debug(utils.Bytes2Str(bytesData), 0)
	}

	return jsonBody, bytesData, nil
}

// sendWebSocketAuthHandshake 发送WebSocket认证握手消息
// conn: WebSocket 连接（gws 使用 *gws.Conn）
// path: 连接路径
// 返回: 握手成功返回nil，否则返回握手失败的错误信息
func (s *SocketSDK) sendWebSocketAuthHandshake(conn *gws.Conn, path string) error {
	// 使用通用方法准备握手数据
	jsonBody := node.GetJsonBody()
	defer node.PutJsonBody(jsonBody)
	jsonBody.Time = utils.UnixSecond()
	jsonBody.Nonce = utils.GetUUID(true)
	jsonBody.Plan = 1
	jsonBody.Router = path
	jsonBody.User = s.clientNo
	_, bytesData, err := s.prepareWebSocketMessage(jsonBody, "auth_handshake")
	if err != nil {
		return err
	}
	defer DIC.ClearData(bytesData)

	// 直接发送 JsonBody 格式的握手消息（gws）
	if err := conn.WriteMessage(gws.OpcodeText, bytesData); err != nil {
		return ex.Throw{Msg: "handshake message send failed: " + err.Error()}
	}

	// 同步等待服务端握手响应（认证必须同步确认）
	// 注意：gws 使用事件驱动，这里需要特殊处理同步等待
	// 我们使用一个临时的 channel 来等待响应
	respChan := make(chan *node.JsonResp, 1)
	nonce := jsonBody.Nonce
	s.responseMap.Store("auth_"+nonce, respChan)
	defer s.responseMap.Delete("auth_" + nonce)

	// 等待响应（5秒超时）
	select {
	case response := <-respChan:
		// 检查响应状态
		if response.Code != 200 {
			return ex.Throw{Msg: fmt.Sprintf("handshake failed: %s", response.Message)}
		}

		// 验证响应签名（与HTTP流程保持一致的安全性）
		tokenSecret, err := s.decodeTokenSecretForSend()
		if err != nil {
			return err
		}
		defer DIC.ClearData(tokenSecret)

		// 构建签名字符串（使用握手路径）
		validSign, _ := node.SignAndDigestBodyMessage(path, response.Data, response.Nonce, response.Time, response.Plan, jsonBody.User, tokenSecret)
		defer DIC.ClearData(validSign)

		// 验证HMAC签名
		if !utils.CompareBase64Sign(validSign, response.Sign) {
			return ex.Throw{Msg: "handshake response signature verification failed"}
		}

		// 验证握手响应时间戳，防止重放攻击
		if response.Time <= 0 {
			return ex.Throw{Msg: "handshake response time must be > 0"}
		}
		if utils.MathAbs(utils.UnixSecond()-response.Time) > 300 { // 5分钟时间窗口
			return ex.Throw{Msg: "handshake response time invalid"}
		}

		// 验证响应数据（握手成功通常返回简单的确认信息）
		var decryptedData []byte
		if response.Plan == 1 {
			// AES解密
			decryptedData, err = utils.AesGCMDecryptBase(response.Data, tokenSecret[:32], node.AppendBodyMessage(path, "", response.Nonce, response.Time, response.Plan, jsonBody.User))
			if err != nil {
				return ex.Throw{Msg: "handshake response data decrypt failed"}
			}
		} else {
			// Base64解码
			decryptedData = utils.Base64Decode(response.Data)
		}
		defer DIC.ClearData(decryptedData)

		// 验证解密后的数据不为空
		if len(decryptedData) == 0 {
			return ex.Throw{Msg: "handshake response data is empty"}
		}

		if zlog.IsDebug() {
			zlog.Debug("WebSocket authentication handshake completed with signature verification", 0)
		}

		return nil
	case <-time.After(5 * time.Second):
		return ex.Throw{Msg: "handshake response read failed (auth sync required): timeout"}
	}
}

// SendWebSocketMessage 发送WebSocket业务消息并可选等待响应
// router: 业务路由标识符，用于服务端路由分发
// requestObj: 请求数据对象
// responseObj: 响应数据对象，用于接收服务端返回数据（当waitResponse=true时）
// waitResponse: 是否等待服务端响应
// encryptRequest: 是否对请求数据进行加密
// timeout: 等待响应的超时时间(秒)
// 返回: 发送成功返回nil，否则返回发送失败的错误信息
func (s *SocketSDK) SendWebSocketMessage(router string, requestObj, responseObj interface{}, waitResponse, encryptRequest bool, timeout int64) error {
	// 使用通用方法准备消息数据
	jsonBody := node.GetJsonBody()
	defer node.PutJsonBody(jsonBody)
	jsonBody.Time = utils.UnixSecond()
	jsonBody.Nonce = utils.GetUUID(true)
	jsonBody.Plan = 0
	if encryptRequest {
		jsonBody.Plan = 1
	}
	jsonBody.Router = router
	jsonBody.User = s.clientNo
	// 使用指定的路由路径进行签名和路由分发
	jsonBody, bytesData, err := s.prepareWebSocketMessage(jsonBody, requestObj)
	if err != nil {
		return err
	}
	defer DIC.ClearData(bytesData)

	// --- 修复：添加msgID用于同步响应匹配 ---
	var respChan chan *node.JsonResp
	if waitResponse {
		respChan = make(chan *node.JsonResp, 1) // 缓冲1，避免阻塞
		s.responseMap.Store(jsonBody.Nonce, respChan)
		// 超时后清理映射（不关闭通道，由接收方负责）
		defer s.responseMap.LoadAndDelete(jsonBody.Nonce)
	}

	s.connMutex.Lock()
	if !s.isConnected || s.conn == nil {
		s.connMutex.Unlock()
		if !s.valid() {
			return ex.Throw{Msg: "token empty or token expired"}
		}
		return ex.Throw{Msg: "WebSocket not connected, call ConnectWebSocket first"}
	}
	conn := s.conn
	if err := conn.WriteMessage(gws.OpcodeText, bytesData); err != nil {
		s.connMutex.Unlock()
		return ex.Throw{Msg: "WebSocket message send failed: " + err.Error()}
	}
	s.connMutex.Unlock()

	if zlog.IsDebug() {
		zlog.Debug(fmt.Sprintf("WebSocket message sent to path: %s, msgID: %s", router, jsonBody.Nonce), 0)
	}

	// 如果不需要等待响应，直接返回
	if !waitResponse {
		return nil
	}

	// 等待响应（带超时）
	waitTimeout := 10 * time.Second
	if timeout > 0 {
		waitTimeout = time.Duration(timeout) * time.Second
	}

	select {
	case resp := <-respChan: // 获得同步响应的数据，检查响应签名和进行解密，解析成目标对象
		// 验证和解密响应数据
		if err := s.verifyWebSocketResponseFromJsonResp(router, responseObj, resp); err != nil {
			return err
		}
		return nil
	case <-time.After(waitTimeout):
		return ex.Throw{
			Code: ex.WS_WAIT,
			Msg:  fmt.Sprintf("wait response timeout (router=%s, nonce=%s, timeout=%ds)", router, jsonBody.Nonce, int(waitTimeout/time.Second)),
		}
	case <-s.connCtx.Done(): // 监听当前连接上下文
		return ex.Throw{
			Code: ex.WS_WAIT,
			Msg:  fmt.Sprintf("connection closed while waiting response (router=%s, nonce=%s, timeout=%ds)", router, jsonBody.Nonce, int(waitTimeout/time.Second)),
		}
	}
}

// verifyWebSocketResponseFromJsonResp 验证WebSocket响应数据的完整性和真实性
// path: 请求路径
// response: 响应数据映射
// 返回: 验证后的响应数据和可能的错误信息
func (s *SocketSDK) verifyWebSocketResponseFromJsonResp(path string, result interface{}, jsonResp *node.JsonResp) error {

	if jsonResp.Code != 200 {
		return ex.Throw{
			Code: jsonResp.Code,
			Msg:  fmt.Sprintf("response error (code=%d, router=%s, nonce=%s, message=%s)", jsonResp.Code, path, jsonResp.Nonce, jsonResp.Message),
		}
	}

	tokenSecret, err := s.decodeTokenSecretForSend()
	if err != nil {
		return err
	}
	defer DIC.ClearData(tokenSecret)

	validSign, _ := node.SignAndDigestBodyMessage(path, jsonResp.Data, jsonResp.Nonce, jsonResp.Time, jsonResp.Plan, s.clientNo, tokenSecret)
	defer DIC.ClearData(validSign)

	if !utils.CompareBase64Sign(validSign, jsonResp.Sign) {
		return ex.Throw{Msg: "response signature verification failed"}
	}

	if node.PlanRequiresOuterSignature(jsonResp.Plan) {
		if err := s.verifyOuterSign(path, s.clientNo, jsonResp); err != nil {
			return err
		}
	}
	// 验证服务端响应时间戳，防止重放攻击
	if jsonResp.Time <= 0 {
		return ex.Throw{Msg: "response time must be > 0"}
	}
	if utils.MathAbs(utils.UnixSecond()-jsonResp.Time) > 300 { // 5分钟时间窗口
		return ex.Throw{Msg: "response time invalid"}
	}

	var decryptedData []byte
	if jsonResp.Plan == 1 {
		var decErr error
		decryptedData, decErr = utils.AesGCMDecryptBase(jsonResp.Data, tokenSecret[:32], node.AppendBodyMessage(path, "", jsonResp.Nonce, jsonResp.Time, jsonResp.Plan, s.clientNo))
		if decErr != nil {
			return ex.Throw{Msg: "response data decrypt failed"}
		}
	} else {
		decryptedData = utils.Base64Decode(jsonResp.Data)
	}
	defer DIC.ClearData(decryptedData)

	if err := utils.JsonUnmarshalFast(decryptedData, result); err != nil {
		return ex.Throw{Msg: "response data unmarshal failed"}
	}

	return nil
}

// websocketHeartbeat 心跳协程：周期发送 /ws/ping，不等待 pong（与服务端“不回 pong”策略一致）。
// 设计要点：fire-and-forget，不占响应等待表、不阻塞；连接存活以 WriteMessage 失败为准，失败则 disconnectWebSocket。
// 心跳间隔建议 10–15 秒，内部上限 15 秒（与 SetHealthPing 一致）。
func (s *SocketSDK) websocketHeartbeat() {
	defer s.wg.Done()

	const maxHealthPing = 15

	// 动态计算心跳间隔的辅助函数
	getInterval := func() time.Duration {
		healthPing := s.healthPing
		if healthPing <= 0 {
			healthPing = 15
		}
		if healthPing > maxHealthPing {
			healthPing = maxHealthPing
		}
		return time.Duration(healthPing) * time.Second
	}

	// 使用计时器实现动态间隔
	timer := time.NewTimer(getInterval())
	defer timer.Stop()

	for {
		select {
		case <-s.connCtx.Done():
			return
		case <-timer.C:
			// 匿名/raw-auth 且尚未拿到 JWT 时不发送 /ws/ping（服务端普通路由会要求 token）。
			// 一旦 token 就绪，即使 rawAuthHeader 尚未清理，也允许发送业务心跳。
			if len(s.rawAuthHeader) > 0 && !s.valid() {
				timer.Reset(getInterval())
				continue
			}
			bytesData, _, err := s.prepareHeartbeatMessage("/ws/ping", "ping")
			if err != nil {
				if zlog.IsDebug() {
					zlog.Debug("heartbeat prepare failed", 0)
				}
				// 重置计时器，继续下一次心跳
				timer.Reset(getInterval())
				continue
			}

			// 短暂获取锁，获取连接引用和状态
			s.connMutex.Lock()
			if !s.isConnected || s.conn == nil {
				s.connMutex.Unlock()
				return
			}
			conn := s.conn
			ctx := s.connCtx
			s.connMutex.Unlock()

			// 检查连接上下文是否已取消（避免向已关闭连接写入）
			select {
			case <-ctx.Done():
				return
			default:
			}

			// 执行写操作（不持有锁，避免阻塞业务消息发送）
			if err := conn.WriteMessage(gws.OpcodeText, bytesData); err != nil {
				if zlog.IsDebug() {
					zlog.Debug("heartbeat send failed, connection may be lost", 0)
				}
				s.disconnectWebSocket()
				return
			}

			if zlog.IsDebug() {
				zlog.Debug("heartbeat sent", 0)
			}

			// 重置计时器，使用最新的 HealthPing 值
			timer.Reset(getInterval())
		}
	}
}

// websocketMessageListenerHandle 处理接收到的 WebSocket 文本帧。
// 每帧从池取出独立 *node.JsonResp，解析后 defer Put；与「连接上挂共享 JsonBody」类问题无关。
// 推送分支通过 messageCopy 值拷贝再异步 HandleMessage，避免 Put 池后仍读同一指针。
// body: 本帧 JSON 字节（须在调用方 OnMessage 返回前有效；与 message.Close 生命周期一致）
func (s *SocketSDK) websocketMessageListenerHandle(body []byte) {
	// 使用对象池获取JsonResp对象，提高内存利用率
	res := node.GetJsonResp()
	defer node.PutJsonResp(res)

	if err := utils.JsonUnmarshalFast(body, res); err != nil {
		zlog.Error(fmt.Sprintf("WebSocket read data parse error: %v", err), 0, zlog.String("body", string(body)))
		return
	}

	messageCopy := *res // 拷贝值

	// 先检查是否是握手响应（auth_ 前缀）
	authRespChanVal, loaded := s.responseMap.LoadAndDelete("auth_" + res.Nonce)
	if loaded {
		if respChan, ok := authRespChanVal.(chan *node.JsonResp); ok {
			select {
			case respChan <- &messageCopy:
				return // 握手响应已处理，直接返回
			default:
			}
		}
	}

	// 先按 nonce 匹配同步响应（无论 code 是 200 还是业务错误码），避免误判成 unknown 后导致等待超时
	if respChanVal, loaded := s.responseMap.LoadAndDelete(res.Nonce); loaded {
		respChan, ok := respChanVal.(chan *node.JsonResp)
		if ok {
			select {
			case respChan <- &messageCopy:
				if zlog.IsDebug() {
					zlog.Debug("response sent to waiting channel successfully", 0, zlog.String("nonce", res.Nonce), zlog.Int("code", res.Code))
				}
			default:
				// 通道已满或已超时（发送方已放弃等待），直接丢弃
				zlog.Warn("response channel full or abandoned, dropping response", 0, zlog.String("nonce", res.Nonce), zlog.Int("code", res.Code))
			}
		}
		return
	}

	// 通过code字段区分消息类型：300=推送消息，其它未匹配 nonce 的消息按未知处理
	if res.Code == 300 {
		// 推送消息：这是服务端主动推送的消息
		if zlog.IsDebug() {
			zlog.Debug("received server push message", 0, zlog.String("router", res.Router), zlog.String("nonce", res.Nonce), zlog.String("data", res.Data))
		}

		// 验证推送消息签名
		if err := s.verifyPushMessageSignature(res); err != nil {
			zlog.Error("push message signature verification failed", 0, zlog.AddError(err))
			return
		}

		// 解密推送消息数据
		pushData, err := s.decryptPushMessageData(res)
		if err != nil {
			zlog.Error("push message data decryption failed", 0, zlog.AddError(err))
			return
		}

		// 查找对应的订阅处理器并分发消息
		if sub, exists := s.subscriptions.Load(res.Router); exists {
			if subscription, ok := sub.(*Subscription); ok && subscription.active && subscription.Handler != nil {
				// 异步调用处理器，避免阻塞消息监听
				go func(handler MessageHandler, msg *node.JsonResp) {
					if err := handler.HandleMessage(msg); err != nil {
						zlog.Error("message handler error", 0, zlog.String("router", res.Router), zlog.AddError(err))
					}
				}(subscription.Handler, &messageCopy)
			}
		}

		// 调用全局推送回调（如果设置了）
		if s.onPushMessage != nil {
			go s.onPushMessage(res.Router, pushData)
		}
	} else {
		// 高频压测下该分支可能大量出现（超时后响应晚到/无订阅消息），按 1/1024 采样，避免日志风暴反向拖慢性能
		miss := atomic.AddUint64(&s.responseNonceMiss, 1)
		if miss&1023 == 1 {
			zlog.Warn("received unknown message type (sampled)", 0, zlog.Int("code", res.Code), zlog.String("nonce", res.Nonce), zlog.Int64("miss_total", int64(miss)))
		}
	}
}

func (s *SocketSDK) handlePushMessage(message map[string]interface{}) {
	// 这里可以添加自定义的推送消息处理逻辑
	// 例如：触发事件、更新UI状态等
	if zlog.IsDebug() {
		zlog.Debug(fmt.Sprintf("push message received: %v", message), 0)
	}
}

// DisconnectWebSocket 断开WebSocket连接，停止所有相关服务
func (s *SocketSDK) DisconnectWebSocket() {
	s.disconnectWebSocketNoReconnect() // 主动断开，不触发重连（内部会处理锁）
}

func (s *SocketSDK) disconnectWebSocket() {
	s.disconnectWebSocketInternal(true, nil) // 连接丢失时触发重连
}

func (s *SocketSDK) disconnectWebSocketNoReconnect() {
	s.disconnectWebSocketInternal(false, nil) // 主动断开时不触发重连
}

// disconnectWebSocketInternal 断开当前 WebSocket。
// eventConn: 若为 nil（用户主动断开/心跳失败等），始终处理当前 s.conn；若为 gws OnClose 的 socket：
//   - 若 s.conn != nil 且与 eventConn 不是同一指针：说明已是新物理连接上的 OnClose 迟到，必须忽略（否则会误关新连、误 cancel 新 ctx）。
//   - 若 s.conn == nil：迟到关闭，不再 WriteClose/cancel；若仍处于 isConnected（异常态）则补一次重连，否则视为与首轮断开重复、直接返回。
func (s *SocketSDK) disconnectWebSocketInternal(triggerReconnect bool, eventConn *gws.Conn) {
	s.connMutex.Lock()
	if eventConn != nil && s.conn != nil && s.conn != eventConn {
		s.connMutex.Unlock()
		return
	}
	if eventConn != nil && s.conn == nil {
		was := s.isConnected
		s.connMutex.Unlock()
		if triggerReconnect && s.reconnectEnabled && was {
			go s.startReconnectProcess()
		}
		return
	}
	wasConnected := s.isConnected
	conn := s.conn
	cancelFn := s.connCancel
	s.conn = nil
	s.isConnected = false
	s.connectedTokenSecret.Store(nil)
	s.connCancel = nil
	s.connMutex.Unlock()

	if cancelFn != nil {
		cancelFn()
	}

	// 不关闭通道，仅从 map 删除。等待方会因 connCtx.Done() 或超时自然退出，避免向已关闭通道发送导致 panic
	s.responseMap.Range(func(key, value interface{}) bool {
		s.responseMap.Delete(key)
		return true
	})

	if conn != nil {
		_ = conn.WriteClose(1000, []byte("client disconnect"))
	}

	if zlog.IsDebug() {
		zlog.Debug("WebSocket connection closed", 0)
	}

	if triggerReconnect && wasConnected && s.reconnectEnabled {
		go s.startReconnectProcess()
	}
}

// calculateReconnectIntervalLocked 计算重连间隔（调用者必须已持有 s.reconnectMutex）
func (s *SocketSDK) calculateReconnectIntervalLocked() time.Duration {
	// 限制最大尝试次数，避免位运算溢出（1<<30已接近int64上限）
	attempts := s.reconnectAttempts
	if attempts > 30 {
		attempts = 30
	}

	// 指数退避: 1s, 2s, 4s, 8s, 16s, 30s, 30s...
	interval := s.reconnectInterval * time.Duration(1<<uint(attempts))

	// 限制最大间隔
	if interval > s.maxReconnectInterval {
		interval = s.maxReconnectInterval
	}

	// 添加随机抖动 (0-1秒)，避免同时重连
	randomJitter, _ := rand.Int(rand.Reader, big.NewInt(1000))
	jitter := time.Duration(randomJitter.Int64()) * time.Millisecond
	interval += jitter

	return interval
}

func (s *SocketSDK) calculateReconnectInterval() time.Duration {
	s.reconnectMutex.Lock()
	defer s.reconnectMutex.Unlock()
	return s.calculateReconnectIntervalLocked()
}

func (s *SocketSDK) startReconnectProcess() {
	//zlog.Info("startReconnectProcess called", 0)
	s.reconnectMutex.Lock()

	if !s.reconnectEnabled {
		zlog.Info("reconnect not enabled, skipping", 0)
		s.reconnectMutex.Unlock()
		return
	}

	// 检查是否已有重连在进行中
	if s.reconnecting {
		// 必须记下「被跳过」，否则并发 OnClose/多路 go startReconnect 会永久丢一次重连（表现为断线后假死且无日志）。
		atomic.StoreInt32(&s.reconnectPending, 1)
		if zlog.IsDebug() {
			zlog.Debug("reconnect already in progress, coalesced (pending follow-up)", 0)
		}
		s.reconnectMutex.Unlock()
		return
	}

	if s.maxReconnectAttempts != -1 && s.reconnectAttempts >= s.maxReconnectAttempts {
		if zlog.IsDebug() {
			zlog.Debug(fmt.Sprintf("Max reconnect attempts (%d) reached, stopping reconnect", s.maxReconnectAttempts), 0)
		}
		s.reconnectMutex.Unlock()
		return
	}

	// 标记重连进行中
	s.reconnecting = true
	s.reconnectAttempts++
	currentAttempt := s.reconnectAttempts
	interval := s.calculateReconnectIntervalLocked()
	s.lastReconnectTime = time.Now()
	path := s.getConnectedPathLocked()
	s.reconnectMutex.Unlock()

	// 确保重连标志最终重置；若有被合并的「待补重连」，在本轮退出后再起一轮（且须在 reconnecting 已清之后）。
	defer func() {
		s.reconnectMutex.Lock()
		s.reconnecting = false
		s.reconnectMutex.Unlock()

		if s.rootCtx.Err() != nil {
			atomic.StoreInt32(&s.reconnectPending, 0)
			return
		}
		if atomic.SwapInt32(&s.reconnectPending, 0) != 1 {
			return
		}
		if !s.isReconnectEnabled() {
			return
		}
		if s.IsWebSocketConnected() {
			return
		}
		go s.startReconnectProcess()
	}()

	// 等待时监听 rootCtx（只有用户主动 Close 才退出）
	select {
	case <-time.After(interval):
	case <-s.rootCtx.Done():
		return
	}

	if !s.valid() && len(s.rawAuthHeader) == 0 {
		// 未走到 dial，故不会出现「Reconnect attempt N failed」；仅 Debug 时容易误以为重连没跑。
		// 无凭证分支会对 reconnectAttempts 回退，currentAttempt 会长期为 1；用 credentialWaitRound 反映真实空转次数。
		waitRound := atomic.AddUint64(&s.reconnectCredWaitRounds, 1)
		zlog.Info("reconnect waiting for credentials (no JWT and no raw authorization)", 0, zlog.Uint64("credentialWaitRound", waitRound))
		if zlog.IsDebug() {
			zlog.Debug("reconnect skipped: token invalid and raw authorization empty", 0)
		}
		// 本轮在入口已对 reconnectAttempts++，但未消耗 dial；回退以免长时间空转让 attempt 无限堆积、退避语义失真。
		s.reconnectMutex.Lock()
		if s.reconnectAttempts > 0 {
			s.reconnectAttempts--
		}
		s.reconnectMutex.Unlock()
		// 无可用鉴权信息时延后重试，避免热循环；保留下一次由外部刷新token或设置rawAuth后恢复
		go s.startReconnectProcess()
		return
	}

	var connectErr error
	func() {
		defer func() {
			if r := recover(); r != nil {
				zlog.Error(fmt.Sprintf("Reconnect attempt %d panic", currentAttempt), 0, zlog.Any("panic", r))
				connectErr = ex.Throw{Msg: fmt.Sprintf("websocket reconnect panic: %v", r)}
			}
		}()
		connectErr = s.connectWebSocketInternal(path, false)
	}()

	if connectErr != nil {
		zlog.Error(fmt.Sprintf("Reconnect attempt %d failed", currentAttempt), 0, zlog.String("errorMsg", ex.Catch(connectErr).Msg))
		go s.startReconnectProcess()
	} else {
		zlog.Info(fmt.Sprintf("Reconnect attempt %d successful", currentAttempt), 0)
		s.reconnectMutex.Lock()
		s.reconnectAttempts = 0
		s.reconnectMutex.Unlock()
	}
}

// getConnectedPathLocked 获取已连接的路径（调用者必须已持有 s.reconnectMutex）
func (s *SocketSDK) getConnectedPathLocked() string {
	if s.connectedPath == "" {
		return DefaultWsRoute
	}
	return s.connectedPath
}

// ForceReconnect 强制重新连接WebSocket，适用于连接异常恢复
// 返回: 重连成功返回nil，否则返回重连失败的错误信息
func (s *SocketSDK) ForceReconnect() error {
	s.reconnectMutex.Lock()
	s.reconnectAttempts = 0
	s.lastReconnectTime = time.Time{}
	s.reconnecting = false // 重置重连标志，允许强制重连
	path := s.getConnectedPathLocked()
	s.reconnectMutex.Unlock()

	// 须先同步拆掉当前连接：connectWebSocketInternal(isInitial=false) 在 isConnected 仍为 true 时会直接 return nil，根本不会拨号。
	s.disconnectWebSocketNoReconnect()

	return s.connectWebSocketInternal(path, false)
}

// IsWebSocketConnected 检查WebSocket连接是否处于活跃状态
// 返回: true表示连接正常，false表示连接已断开
func (s *SocketSDK) IsWebSocketConnected() bool {
	s.connMutex.Lock()
	defer s.connMutex.Unlock()
	return s.isConnected && s.conn != nil
}

func (s *SocketSDK) SetTokenExpiredCallback(callback func()) {
	s.onTokenExpired = callback
}

func (s *SocketSDK) triggerTokenExpiredCallback() {
	if s.onTokenExpired != nil {
		// 二次校验：可能外层判定过期后，token 已在其他协程完成刷新。
		if s.valid() || len(s.rawAuthHeader) > 0 {
			return
		}
		// 节流：避免多个触发点在短时间内重复拉起 token 回调导致风暴
		now := utils.UnixSecond()
		last := atomic.LoadInt64(&s.tokenCallbackLastAt)
		if now-last < 5 {
			return
		}
		if !atomic.CompareAndSwapInt32(&s.tokenCallbackActive, 0, 1) {
			return
		}
		// 抢到执行权后再次校验，避免等待期间 token 已恢复仍发起重复回调。
		if s.valid() || len(s.rawAuthHeader) > 0 {
			atomic.StoreInt32(&s.tokenCallbackActive, 0)
			return
		}
		atomic.StoreInt64(&s.tokenCallbackLastAt, now)
		if zlog.IsDebug() {
			zlog.Debug("Calling token expired callback", 0)
		}
		go func() {
			defer atomic.StoreInt32(&s.tokenCallbackActive, 0)
			s.onTokenExpired()
		}()
	}
}

func (s *SocketSDK) triggerTokenExpiredCallbackSync() {
	if s.onTokenExpired == nil {
		return
	}
	if s.valid() || len(s.rawAuthHeader) > 0 {
		return
	}
	now := utils.UnixSecond()
	last := atomic.LoadInt64(&s.tokenCallbackLastAt)
	if now-last < 5 {
		return
	}
	if !atomic.CompareAndSwapInt32(&s.tokenCallbackActive, 0, 1) {
		return
	}
	if s.valid() || len(s.rawAuthHeader) > 0 {
		atomic.StoreInt32(&s.tokenCallbackActive, 0)
		return
	}
	atomic.StoreInt64(&s.tokenCallbackLastAt, now)
	defer atomic.StoreInt32(&s.tokenCallbackActive, 0)
	if zlog.IsDebug() {
		zlog.Debug("Calling token expired callback (sync)", 0)
	}
	s.onTokenExpired()
}

func (s *SocketSDK) SetReconnectConfig(enabled bool, maxAttempts int, initialInterval, maxInterval time.Duration) {
	s.reconnectMutex.Lock()
	defer s.reconnectMutex.Unlock()

	s.reconnectEnabled = enabled
	s.maxReconnectAttempts = maxAttempts
	if s.maxReconnectAttempts <= 0 && s.maxReconnectAttempts != -1 {
		s.maxReconnectAttempts = -1 // 非法值按默认：无限重连
	}

	s.reconnectInterval = initialInterval
	if s.reconnectInterval <= 0 {
		s.reconnectInterval = time.Second // 默认1秒
	}

	s.maxReconnectInterval = maxInterval
	if s.maxReconnectInterval <= s.reconnectInterval {
		s.maxReconnectInterval = 8 * time.Second // 默认8秒
	}

	s.reconnectAttempts = 0
	s.lastReconnectTime = time.Time{}
	s.reconnecting = false // 重置重连标志
	atomic.StoreInt32(&s.reconnectPending, 0)

	if zlog.IsDebug() {
		zlog.Debug(fmt.Sprintf("WebSocket reconnect config set: enabled=%t, maxAttempts=%d, interval=%v, maxInterval=%v",
			enabled, maxAttempts, initialInterval, maxInterval), 0)
	}
}

func (s *SocketSDK) EnableReconnect() {
	s.SetReconnectConfig(true, -1, time.Second, 8*time.Second)
}

// DisableReconnect 禁用自动重连
func (s *SocketSDK) DisableReconnect() {
	s.reconnectMutex.Lock()
	defer s.reconnectMutex.Unlock()
	s.reconnectEnabled = false
	s.reconnecting = false // 重置重连标志
	atomic.StoreInt32(&s.reconnectPending, 0)
}

func (s *SocketSDK) GetReconnectStatus() (enabled bool, attempts int, maxAttempts int, reconnecting bool, nextReconnectTime time.Time) {
	s.reconnectMutex.Lock()
	defer s.reconnectMutex.Unlock()

	enabled = s.reconnectEnabled
	attempts = s.reconnectAttempts
	maxAttempts = s.maxReconnectAttempts
	reconnecting = s.reconnecting

	if !s.lastReconnectTime.IsZero() {
		interval := s.calculateReconnectIntervalLocked()
		nextReconnectTime = s.lastReconnectTime.Add(interval)
	}

	return
}

// SubscribeMessage 订阅指定路由的消息
// SubscribeMessage 订阅指定路由的消息推送
// router: 要订阅的路由标识符
// handler: 消息处理函数，当接收到对应路由的消息时会被调用
// 返回: 订阅ID和可能的错误信息
func (s *SocketSDK) SubscribeMessage(router string, handler MessageHandler) (subscriptionID string, err error) {
	if handler == nil {
		return "", utils.Error("handler cannot be nil")
	}
	if router == "" {
		return "", utils.Error("router cannot be empty")
	}

	subscriptionID = utils.GetUUID(true)

	subscription := &Subscription{
		ID:      subscriptionID,
		Router:  router,
		Handler: handler,
		active:  true,
	}

	// sync.Map 本身就是线程安全的，无需额外锁
	s.subscriptions.Store(router, subscription)

	if zlog.IsDebug() {
		zlog.Debug("message subscription created", 0,
			zlog.String("subscription_id", subscriptionID),
			zlog.String("router", router))
	}

	return subscriptionID, nil
}

// UnsubscribeMessage 取消消息订阅
// UnsubscribeMessage 取消订阅指定路由的消息推送
// router: 要取消订阅的路由标识符
// 返回: 取消订阅成功返回nil，否则返回错误信息
func (s *SocketSDK) UnsubscribeMessage(router string) error {
	// sync.Map 的 Load 和 Delete 操作本身就是线程安全的
	if sub, exists := s.subscriptions.Load(router); exists {
		if subscription, ok := sub.(*Subscription); ok {
			subscription.active = false
		}
		s.subscriptions.Delete(router)

		if zlog.IsDebug() {
			zlog.Debug("message subscription removed", 0, zlog.String("router", router))
		}

		return nil
	}

	return utils.Error("subscription not found for router: " + router)
}

// resubscribeAfterReconnect 重连成功后自动重新订阅所有主题
func (s *SocketSDK) resubscribeAfterReconnect() {
	// 使用 sync.Map 的 Range 方法，它是线程安全的
	var activeSubscriptions []string

	s.subscriptions.Range(func(key, value interface{}) bool {
		router := key.(string)
		subscription := value.(*Subscription)
		if subscription.active {
			activeSubscriptions = append(activeSubscriptions, router)
		}
		return true
	})

	if len(activeSubscriptions) == 0 {
		return
	}

	if zlog.IsDebug() {
		zlog.Debug(fmt.Sprintf("Reconnecting %d subscriptions after reconnect", len(activeSubscriptions)), 0)
	}

	// 遍历并重新订阅每个路由
	// 注意：在当前架构中，订阅是客户端本地行为
	// 这里可以为未来的服务器端订阅管理做准备，或者向服务器发送订阅请求
	for _, router := range activeSubscriptions {
		// 目前只记录日志，将来可以在这里向服务器发送订阅请求
		// 例如：s.SendWebSocketMessage("/ws/subscribe", map[string]interface{}{"topic": router}, nil, false, true, 5)
		if zlog.IsDebug() {
			zlog.Debug(fmt.Sprintf("Resubscribed to %s after reconnect", router), 0)
		}
	}
}

// GetSubscriptions 获取所有活跃的订阅
func (s *SocketSDK) GetSubscriptions() map[string]*Subscription {
	result := make(map[string]*Subscription)

	// sync.Map 的 Range 方法是线程安全的，无需额外锁
	s.subscriptions.Range(func(key, value interface{}) bool {
		if router, ok := key.(string); ok {
			if subscription, ok := value.(*Subscription); ok && subscription.active {
				result[router] = subscription
			}
		}
		return true
	})

	return result
}

// Close 主动关闭整个 SDK（停止所有重连和连接），并等待所有 goroutine 退出
func (s *SocketSDK) Close() {
	// 禁用重连，防止断开后再自动重连
	s.DisableReconnect()

	// 断开当前连接（这会触发 connCancel，通知 goroutine 退出）
	s.DisconnectWebSocket()

	// 等待心跳和监听 goroutine 退出
	s.wg.Wait()

	// 取消整个 SDK 的上下文
	s.rootCancel()
}

// 示例：如何使用消息订阅功能
//
// // 1. 连接WebSocket
// wsSdk := NewSocketSDK()
// err := wsSdk.ConnectWebSocket()
// if err != nil {
//     log.Fatal(err)
// }
//
// // 2. 定义消息处理器
// type ChatMessageHandler struct{}
// func (h *ChatMessageHandler) HandleMessage(message *node.JsonResp) error {
//     fmt.Printf("收到聊天消息: %s\n", message.Data)
//     return nil
// }
//
// // 3. 订阅消息
// subscriptionID, err := wsSdk.SubscribeMessage("/ws/chat", &ChatMessageHandler{})
// if err != nil {
//     log.Fatal(err)
// }
//
// // 4. 发送订阅请求到服务器（这取决于服务器的实现）
// // 例如：发送一个订阅聊天消息的请求
// request := map[string]interface{}{"action": "subscribe", "channel": "general"}
// response, err := wsSdk.SendWebSocketMessage("/ws/chat", request, &node.JsonResp{}, true, true, 5)
//
// // 5. 持续接收推送消息
// // 消息会自动分发到 ChatMessageHandler.HandleMessage
//
// // 6. 取消订阅（可选）
// err = wsSdk.UnsubscribeMessage("/ws/chat")
//
// // 7. 断开连接
// wsSdk.DisconnectWebSocket()
