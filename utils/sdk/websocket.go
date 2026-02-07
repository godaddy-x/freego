package sdk

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"net/url"
	"sync"
	"time"

	"github.com/godaddy-x/freego/utils/crypto"

	"github.com/fasthttp/websocket"
	DIC "github.com/godaddy-x/freego/common"
	"github.com/godaddy-x/freego/ex"
	"github.com/godaddy-x/freego/node"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/zlog"
)

// WebSocket客户端常量
const (
	DefaultWsRoute = "/ws" // 默认WebSocket路由路径
)

// NewSocketSDK 创建新的WebSocket SDK实例并设置默认值
//
// domain: API域名，如"api.example.com"
//
// 默认值:
// - timeout: 120秒
// - maxReconnectAttempts: 10
// - reconnectInterval: 1秒
// - maxReconnectInterval: 30秒
// - HealthPing: 30秒
//
// 返回值:
//   - *SocketSDK: 初始化的WebSocket SDK实例
//
// 使用示例:
//
//	sdk := NewSocketSDK("api.example.com")
//	sdk.AuthToken(AuthToken{...})
//	sdk.ClientNo = 12345
func NewSocketSDK(domain string) *SocketSDK {
	rootCtx, rootCancel := context.WithCancel(context.Background())
	return &SocketSDK{
		Domain:               domain,
		timeout:              120,
		maxReconnectAttempts: 10,
		reconnectInterval:    time.Second,
		maxReconnectInterval: 30 * time.Second,
		HealthPing:           30,
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

type SocketSDK struct {
	Domain      string                  // API域名 (如:api.example.com)
	language    string                  // 语言设置 (HTTP头)
	timeout     int64                   // 请求超时时间(秒)
	authObject  interface{}             // 登录认证对象 (用户名+密码等)
	authToken   AuthToken               // JWT认证令牌
	SSL         bool                    // 是否启用https
	ClientNo    int64                   // 客户端ID
	ecdsaObject map[int64]crypto.Cipher // ECDSA签名验证对象列表
	HealthPing  int                     // 健康PING间隔时间/秒

	// WebSocket连接相关
	conn        *websocket.Conn // WebSocket连接
	connMutex   sync.Mutex      // 连接互斥锁
	isConnected bool            // 连接状态

	// 上下文管理（关键修复）
	rootCtx    context.Context    // SDK全局上下文（用于Close）
	rootCancel context.CancelFunc // 取消整个SDK
	connCtx    context.Context    // 当前连接上下文（每次重连新建）
	connCancel context.CancelFunc // 取消当前连接

	// 重连相关配置
	reconnectEnabled     bool          // 是否启用自动重连
	maxReconnectAttempts int           // 最大重连次数 (-1表示无限重连)
	reconnectInterval    time.Duration // 重连间隔
	maxReconnectInterval time.Duration // 最大重连间隔
	reconnectAttempts    int           // 当前重连次数
	lastReconnectTime    time.Time     // 上次重连时间
	reconnectMutex       sync.Mutex    // 重连互斥锁
	connectedPath        string        // 已连接的WebSocket路径 (用于重连)

	// Token过期回调
	onTokenExpired   func()    // Token过期时回调，用户可以重新认证
	tokenExpiredOnce sync.Once // 防止重复调用

	// 新增：同步响应映射表 (nonce -> chan JsonResp)
	responseMap sync.Map // 存储等待响应的通道

	// 新增：服务端主动推送消息的回调
	onPushMessage func(router string, data []byte)

	// 消息订阅相关
	subscriptions sync.Map // 路由 -> 订阅信息 (线程安全)
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
	s.authToken = object
	s.tokenExpiredOnce = sync.Once{} // 重置token过期回调标志
}

// SetTimeout 设置WebSocket请求的超时时间
// timeout: 超时时间(秒)，控制WebSocket消息发送和等待响应的最大时间
func (s *SocketSDK) SetTimeout(timeout int64) {
	s.timeout = timeout
}

func (s *SocketSDK) SetHealthPing(t int) {
	if t <= 0 {
		t = 30
	}
	s.HealthPing = t
}

func (s *SocketSDK) SetClientNo(clientNo int64) {
	s.ClientNo = clientNo
}

// SetLanguage 设置WebSocket请求的语言标识
// language: 语言代码，如"zh-CN"、"en-US"，用于服务端国际化支持
func (s *SocketSDK) SetLanguage(language string) {
	s.language = language
}

// SetPushMessageCallback 设置服务端主动推送消息的回调函数
func (s *SocketSDK) SetPushMessageCallback(callback func(router string, data []byte)) {
	s.onPushMessage = callback
}

// getURI 构建完整的WebSocket连接URI
// path: WebSocket路径，如"/ws"
// 返回: 完整的WebSocket URI，支持ws和wss协议
func (s *SocketSDK) getURI(path string) string {
	var p string
	if s.SSL {
		u := url.URL{Scheme: "wss", Host: s.Domain, Path: path}
		p = u.String()
	} else {
		u := url.URL{Scheme: "ws", Host: s.Domain, Path: path}
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
	pushSignKey := []byte("server_push_secret_key")
	expectedSign := node.SignBodyMessage(res.Router, res.Data, res.Nonce, res.Time, res.Plan, 0, pushSignKey)

	if utils.Base64Encode(expectedSign) != res.Sign {
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
// 检查令牌是否存在、secret是否存在，以及是否即将过期(提前1小时预警)
// 返回: true表示令牌有效，false表示需要重新认证
func (s *SocketSDK) valid() bool {
	if len(s.authToken.Token) == 0 {
		return false
	}
	if len(s.authToken.Secret) == 0 {
		return false
	}
	if utils.UnixSecond() > s.authToken.Expired-3600 {
		return false
	}
	return true
}

// addECDSASign 为WebSocket消息添加ECDSA数字签名
// jsonBody: 消息体结构体，包含待签名的HMAC签名
// 返回: 签名成功返回nil，否则返回错误信息
// 注意: 必须配置双向ECDSA签名，否则会抛出异常
func (s *SocketSDK) addECDSASign(jsonBody *node.JsonBody) error {
	if s.ecdsaObject == nil {
		return ex.Throw{Msg: "ECDSA object not configured, bidirectional ECDSA signature is required"}
	}
	cipher, exists := s.ecdsaObject[s.ClientNo]
	if !exists || cipher == nil {
		return ex.Throw{Msg: "ECDSA object not found for client, bidirectional ECDSA signature is required"}
	}
	ecdsaSign, err := cipher.Sign(utils.Base64Decode(jsonBody.Sign))
	if err != nil {
		return ex.Throw{Msg: "ECDSA sign failed: " + err.Error()}
	}
	jsonBody.Valid = utils.Base64Encode(ecdsaSign)
	// 清理ECDSA签名数据（在设置完jsonBody.Valid之后）
	DIC.ClearData(ecdsaSign)
	if zlog.IsDebug() {
		zlog.Debug(fmt.Sprintf("ECDSA sign added for HMAC signature: %s", jsonBody.Valid), 0)
	}
	return nil
}

// verifyECDSASign 验证WebSocket响应数据的ECDSA签名
// validSign: 预期的签名数据
// respData: 响应数据结构体
// 返回: 验证成功返回nil，否则返回验证失败的错误信息
// 注意: 必须配置双向ECDSA签名，否则会抛出异常
func (s *SocketSDK) verifyECDSASign(validSign []byte, respData *node.JsonResp) error {
	if s.ecdsaObject == nil {
		return ex.Throw{Msg: "ECDSA object not configured, bidirectional ECDSA signature is required"}
	}
	cipher, exists := s.ecdsaObject[s.ClientNo]
	if !exists || cipher == nil {
		return ex.Throw{Msg: "ECDSA object not found for client, bidirectional ECDSA signature is required"}
	}
	// 预先解码ECDSA签名数据，避免在循环中重复解码
	ecdsaSignData := utils.Base64Decode(respData.Valid)
	// 清理ECDSA签名解码数据
	defer DIC.ClearData(ecdsaSignData)

	if err := cipher.Verify(validSign, ecdsaSignData); err != nil {
		return ex.Throw{Msg: "post response ECDSA sign verify invalid"}
	}
	return nil
}

// SetECDSAObject 配置WebSocket客户端的ECDSA密钥对用于数字签名验证
// usr: 客户端ID，服务端提供
// prkB64: ECDSA私钥的Base64编码字符串
// pubB64: ECDSA公钥的Base64编码字符串
// 返回: 配置成功返回nil，否则返回密钥解析错误
func (s *SocketSDK) SetECDSAObject(usr int64, prkB64, pubB64 string) error {
	if s.ecdsaObject == nil {
		s.ecdsaObject = make(map[int64]crypto.Cipher)
	}
	cipher, err := crypto.CreateS256ECDSAWithBase64(prkB64, pubB64)
	if err != nil {
		return err
	}
	s.ecdsaObject[usr] = cipher
	return nil
}

// ConnectWebSocket 建立WebSocket连接并启动相关服务
// path: WebSocket连接路径，如"/ws"
// 返回: 连接成功返回nil，否则返回连接失败的错误信息
func (s *SocketSDK) ConnectWebSocket(path string) error {
	s.reconnectMutex.Lock()
	s.connectedPath = path // 存储连接路径用于重连
	s.reconnectMutex.Unlock()

	return s.connectWebSocketInternal(path, true)
}

// connectWebSocketInternal WebSocket连接的内部实现方法
// path: WebSocket连接路径
// isInitial: 是否为初始连接，用于重连逻辑判断
// 返回: 连接成功返回nil，否则返回连接失败的错误信息
func (s *SocketSDK) connectWebSocketInternal(path string, isInitial bool) error {
	s.connMutex.Lock()
	defer s.connMutex.Unlock()

	if s.isConnected && s.conn != nil && !isInitial {
		return nil
	}

	if !s.valid() {
		s.triggerTokenExpiredCallback()
		return ex.Throw{Msg: "token empty or token expired"}
	}

	// 取消旧的连接上下文（安全）
	if s.connCancel != nil {
		s.connCancel()
	}

	// 创建新的连接上下文（绑定到 rootCtx）
	s.connCtx, s.connCancel = context.WithCancel(s.rootCtx)

	// 构建WebSocket URL
	wsURL := s.getURI(path)

	// 创建WebSocket拨号器
	dialer := websocket.DefaultDialer
	dialer.HandshakeTimeout = 30 * time.Second
	if s.timeout > 0 {
		dialer.HandshakeTimeout = time.Duration(s.timeout) * time.Second
	}

	// 设置认证头
	header := make(map[string][]string)
	header["Authorization"] = []string{s.authToken.Token}
	header["Language"] = []string{s.language}

	if zlog.IsDebug() {
		if isInitial {
			zlog.Debug(fmt.Sprintf("connecting to WebSocket: %s", wsURL), 0)
		} else {
			zlog.Debug(fmt.Sprintf("reconnecting to WebSocket (attempt %d): %s", s.reconnectAttempts+1, wsURL), 0)
		}
	}

	// 建立WebSocket连接
	conn, _, err := dialer.Dial(wsURL, header)
	if err != nil {
		if zlog.IsDebug() {
			zlog.Debug(fmt.Sprintf("WebSocket connection failed: %v", err), 0)
		}
		return ex.Throw{Msg: "WebSocket connection failed: " + err.Error()}
	}

	// 发送认证握手消息
	if err := s.sendWebSocketAuthHandshake(conn, path); err != nil {
		conn.Close()
		if zlog.IsDebug() {
			zlog.Debug(fmt.Sprintf("WebSocket handshake failed: %v", err), 0)
		}
		return err
	}

	// 设置连接状态
	s.conn = conn
	s.isConnected = true

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

	// --- 修复：无论是否初始连接，都启动心跳和监听 ---
	go s.websocketHeartbeat()
	go s.websocketMessageListener()

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
	defer DIC.ClearData(jsonData)

	jsonBody := node.GetJsonBody()
	defer node.PutJsonBody(jsonBody)

	// 设置心跳消息的基本字段
	jsonBody.Time = utils.UnixSecond()
	jsonBody.Nonce = heartbeatUUID
	jsonBody.Router = router
	jsonBody.Plan = 0 // 心跳使用明文
	jsonBody.User = s.ClientNo

	// 根据plan设置数据
	jsonBody.Data = utils.Base64Encode(utils.Bytes2Str(jsonData))

	// 生成签名（心跳也需要签名验证）
	tokenSecret := utils.Base64Decode(s.authToken.Secret)
	defer DIC.ClearData(tokenSecret)

	signData := node.SignBodyMessage(jsonBody.Router, jsonBody.Data, jsonBody.Nonce, jsonBody.Time, jsonBody.Plan, jsonBody.User, tokenSecret)
	jsonBody.Sign = utils.Base64Encode(signData)
	defer DIC.ClearData(signData)

	// 添加ECDSA签名（如果配置了）
	if s.ecdsaObject != nil {
		if err := s.addECDSASign(jsonBody); err != nil {
			return nil, "", err
		}
	}

	bytesData, err := utils.JsonMarshal(jsonBody)
	if err != nil {
		return nil, "", ex.Throw{Msg: "heartbeat jsonBody marshal failed"}
	}

	zlog.Info("prepared heartbeat data", 0,
		zlog.String("data", utils.Bytes2Str(bytesData)),
		zlog.String("uuid", heartbeatUUID))

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

	// 解码token secret用于加密和签名
	tokenSecret := utils.Base64Decode(s.authToken.Secret)
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
	signData := node.SignBodyMessage(jsonBody.Router, jsonBody.Data, jsonBody.Nonce, jsonBody.Time, jsonBody.Plan, jsonBody.User, tokenSecret)
	jsonBody.Sign = utils.Base64Encode(signData)
	defer DIC.ClearData(signData)

	// 添加ECDSA签名
	if err := s.addECDSASign(jsonBody); err != nil {
		return nil, nil, err
	}

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
// conn: WebSocket连接对象
// path: 连接路径
// 返回: 握手成功返回nil，否则返回握手失败的错误信息
func (s *SocketSDK) sendWebSocketAuthHandshake(conn *websocket.Conn, path string) error {
	// 使用通用方法准备握手数据
	jsonBody := node.GetJsonBody()
	defer node.PutJsonBody(jsonBody)
	jsonBody.Time = utils.UnixSecond()
	jsonBody.Nonce = utils.GetUUID(true)
	jsonBody.Plan = 1
	jsonBody.Router = path
	jsonBody.User = s.ClientNo
	_, bytesData, err := s.prepareWebSocketMessage(jsonBody, "auth_handshake")
	if err != nil {
		return err
	}
	defer DIC.ClearData(bytesData)

	// 直接发送JsonBody格式的握手消息
	conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if err := conn.WriteMessage(websocket.TextMessage, bytesData); err != nil {
		return ex.Throw{Msg: "handshake message send failed: " + err.Error()}
	}

	// 同步等待服务端握手响应（认证必须同步确认）
	conn.SetReadDeadline(time.Now().Add(5 * time.Second)) // 缩短超时时间
	_, responseBytes, err := conn.ReadMessage()
	if err != nil {
		return ex.Throw{Msg: "handshake response read failed (auth sync required): " + err.Error()}
	}

	// 解析响应
	response := node.GetJsonResp()
	defer node.PutJsonResp(response)
	if err := utils.JsonUnmarshal(responseBytes, response); err != nil {
		return ex.Throw{Msg: "handshake response parse failed: " + err.Error()}
	}

	// 检查响应状态
	if response.Code != 200 {
		return ex.Throw{Msg: fmt.Sprintf("handshake failed: %s", response.Message)}
	}

	// 验证握手响应时间戳，防止重放攻击
	if response.Time <= 0 {
		return ex.Throw{Msg: "handshake response time must be > 0"}
	}
	if utils.MathAbs(utils.UnixSecond()-response.Time) > 300 { // 5分钟时间窗口
		return ex.Throw{Msg: "handshake response time invalid"}
	}

	// 验证响应签名（与HTTP流程保持一致的安全性）
	tokenSecret := utils.Base64Decode(s.authToken.Secret)
	defer DIC.ClearData(tokenSecret)

	// 构建签名字符串（使用握手路径）
	validSign := node.SignBodyMessage(path, response.Data, response.Nonce, response.Time, response.Plan, jsonBody.User, tokenSecret)
	expectedSign := utils.Base64Encode(validSign)
	defer DIC.ClearData(validSign)

	// 验证HMAC签名
	if response.Sign != expectedSign {
		return ex.Throw{Msg: "handshake response signature verification failed"}
	}

	// 验证ECDSA签名（强制要求，必须配置ECDSA对象）
	if err := s.verifyECDSASign(validSign, response); err != nil {
		return err
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
	s.connMutex.Lock()
	if !s.isConnected || s.conn == nil {
		s.connMutex.Unlock()
		return ex.Throw{Msg: "WebSocket not connected, call ConnectWebSocket first"}
	}
	conn := s.conn
	s.connMutex.Unlock()

	// 验证认证信息
	if !s.valid() {
		// 触发Token过期回调
		s.triggerTokenExpiredCallback()
		return ex.Throw{Msg: "token empty or token expired"}
	}

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
	jsonBody.User = s.ClientNo
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
		// 超时后清理映射和通道
		defer func() {
			// 原子删除，如果条目存在则关闭channel
			if _, loaded := s.responseMap.LoadAndDelete(jsonBody.Nonce); loaded {
				close(respChan)
			}
		}()
	}

	// 发送消息
	if timeout > 0 {
		conn.SetWriteDeadline(time.Now().Add(time.Duration(timeout) * time.Second))
	} else {
		conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	}
	if err := conn.WriteMessage(websocket.TextMessage, bytesData); err != nil {
		return ex.Throw{Msg: "WebSocket message send failed: " + err.Error()}
	}

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
		return ex.Throw{Msg: fmt.Sprintf("wait response timeout (msgID: %s)", jsonBody.Nonce)}
	case <-s.connCtx.Done(): // 监听当前连接上下文
		return ex.Throw{Msg: fmt.Sprintf("connection closed while waiting response (msgID: %s)", jsonBody.Nonce)}
	}
}

// verifyWebSocketResponseFromJsonResp 验证WebSocket响应数据的完整性和真实性
// path: 请求路径
// response: 响应数据映射
// 返回: 验证后的响应数据和可能的错误信息
func (s *SocketSDK) verifyWebSocketResponseFromJsonResp(path string, result interface{}, jsonResp *node.JsonResp) error {

	if jsonResp.Code != 200 {
		return ex.Throw{Msg: fmt.Sprintf("response error: %s", jsonResp.Message)}
	}

	// 验证服务端响应时间戳，防止重放攻击
	if jsonResp.Time <= 0 {
		return ex.Throw{Msg: "response time must be > 0"}
	}
	if utils.MathAbs(utils.UnixSecond()-jsonResp.Time) > 300 { // 5分钟时间窗口
		return ex.Throw{Msg: "response time invalid"}
	}

	tokenSecret := utils.Base64Decode(s.authToken.Secret)
	defer DIC.ClearData(tokenSecret)

	validSign := node.SignBodyMessage(path, jsonResp.Data, jsonResp.Nonce, jsonResp.Time, jsonResp.Plan, s.ClientNo, tokenSecret)
	defer DIC.ClearData(validSign)

	if jsonResp.Sign != utils.Base64Encode(validSign) {
		return ex.Throw{Msg: "response signature verification failed"}
	}

	ecdsaSignData := utils.Base64Decode(jsonResp.Valid)
	defer DIC.ClearData(ecdsaSignData)

	if s.ecdsaObject == nil {
		return ex.Throw{Msg: "ECDSA object not configured, bidirectional ECDSA signature is required"}
	}
	cipher, exists := s.ecdsaObject[s.ClientNo]
	if !exists || cipher == nil {
		return ex.Throw{Msg: "ECDSA object not found for client, bidirectional ECDSA signature is required"}
	}

	if err := cipher.Verify(validSign, ecdsaSignData); err != nil {
		return ex.Throw{Msg: "response ECDSA signature verification failed"}
	}

	var decryptedData []byte
	var err error
	if jsonResp.Plan == 1 {
		decryptedData, err = utils.AesGCMDecryptBase(jsonResp.Data, tokenSecret[:32], node.AppendBodyMessage(path, "", jsonResp.Nonce, jsonResp.Time, jsonResp.Plan, s.ClientNo))
		if err != nil {
			return ex.Throw{Msg: "response data decrypt failed"}
		}
	} else {
		decryptedData = utils.Base64Decode(jsonResp.Data)
	}
	defer DIC.ClearData(decryptedData)

	if err := utils.JsonUnmarshal(decryptedData, result); err != nil {
		return ex.Throw{Msg: "response data unmarshal failed"}
	}

	return nil
}

// websocketHeartbeat WebSocket心跳机制，保持连接活跃状态
func (s *SocketSDK) websocketHeartbeat() {
	healthPing := s.HealthPing
	if healthPing <= 0 {
		healthPing = 30
	}
	ticker := time.NewTicker(time.Duration(healthPing) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.connCtx.Done(): // 监听当前连接上下文
			return
		case <-ticker.C:
			s.connMutex.Lock()
			if !s.isConnected || s.conn == nil {
				s.connMutex.Unlock()
				return
			}
			conn := s.conn
			s.connMutex.Unlock()

			bytesData, heartbeatUUID, err := s.prepareHeartbeatMessage("/ws/ping", "ping")
			if err != nil {
				if zlog.IsDebug() {
					zlog.Debug("heartbeat prepare failed", 0)
				}
				continue
			}

			heartbeatChan := make(chan *node.JsonResp, 1)
			s.responseMap.Store(heartbeatUUID, heartbeatChan)

			conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if err := conn.WriteMessage(websocket.TextMessage, bytesData); err != nil {
				s.responseMap.Delete(heartbeatUUID)
				close(heartbeatChan)
				if zlog.IsDebug() {
					zlog.Debug("heartbeat send failed, connection may be lost", 0)
				}
				s.disconnectWebSocket()
				return
			}

			zlog.Info("heartbeat sent, waiting for pong", 0, zlog.String("uuid", heartbeatUUID))

			select {
			case resp := <-heartbeatChan:
				if resp.Code == 200 {
					zlog.Info("heartbeat pong received", 0, zlog.String("uuid", heartbeatUUID))
				} else {
					zlog.Warn("heartbeat pong error", 0, zlog.String("uuid", heartbeatUUID), zlog.Int("code", resp.Code))
					s.disconnectWebSocket()
					return
				}
			case <-time.After(5 * time.Second):
				s.responseMap.Delete(heartbeatUUID)
				close(heartbeatChan)
				zlog.Error("heartbeat pong timeout", 0, zlog.String("uuid", heartbeatUUID))
				s.disconnectWebSocket()
				return
			}
		}
	}
}

// websocketMessageListenerHandle 处理接收到的WebSocket消息
// body: 接收到的消息字节数据
func (s *SocketSDK) websocketMessageListenerHandle(body []byte) {
	// 使用对象池获取JsonResp对象，提高内存利用率
	res := node.GetJsonResp()
	defer node.PutJsonResp(res)

	if err := utils.JsonUnmarshal(body, res); err != nil {
		zlog.Error(fmt.Sprintf("WebSocket read data parse error: %v", err), 0, zlog.String("body", string(body)))
		return
	}

	messageCopy := *res // 拷贝值

	// 通过code字段区分消息类型：200=响应消息，300=推送消息
	if res.Code == 200 {
		// 响应消息：这是对某个请求的响应
		zlog.Info("received response message", 0, zlog.String("nonce", res.Nonce), zlog.String("data", res.Data))
		respChanVal, loaded := s.responseMap.LoadAndDelete(res.Nonce)
		if loaded {
			respChan, ok := respChanVal.(chan *node.JsonResp)
			if ok {
				select {
				case respChan <- &messageCopy:
					zlog.Info("response sent to waiting channel successfully", 0, zlog.String("nonce", res.Nonce))
				case <-time.After(100 * time.Millisecond):
					zlog.Warn("response channel timeout, may be closed", 0, zlog.String("nonce", res.Nonce))
				}
			}
		} else {
			zlog.Warn("no waiting channel found for nonce", 0, zlog.String("nonce", res.Nonce))
		}
	} else if res.Code == 300 {
		// 推送消息：这是服务端主动推送的消息
		zlog.Info("received server push message", 0, zlog.String("router", res.Router), zlog.String("nonce", res.Nonce), zlog.String("data", res.Data))

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

		// 调用推送回调
		if s.onPushMessage != nil {
			go s.onPushMessage(res.Router, pushData)
		} else {
			zlog.Debug("push message received but no callback set", 0)
		}
	} else {
		// 未知消息类型
		zlog.Warn("received unknown message type", 0, zlog.Int("code", res.Code), zlog.String("nonce", res.Nonce))
	}
}

// websocketMessageListener WebSocket消息监听器，持续接收和处理服务端推送的消息
func (s *SocketSDK) websocketMessageListener() {
	for {
		select {
		case <-s.connCtx.Done(): // 监听当前连接上下文
			return
		default:
			s.connMutex.Lock()
			if !s.isConnected || s.conn == nil {
				s.connMutex.Unlock()
				return
			}
			conn := s.conn
			s.connMutex.Unlock()

			conn.SetReadDeadline(time.Now().Add(60 * time.Second)) // 60秒超时
			_, body, err := conn.ReadMessage()
			if err != nil {
				// 检测是否是意外的连接关闭错误
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) ||
					websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseNoStatusReceived) {
					if zlog.IsDebug() {
						zlog.Debug(fmt.Sprintf("WebSocket connection lost: %v", err), 0)
					}
				} else {
					if zlog.IsDebug() {
						zlog.Debug(fmt.Sprintf("WebSocket read error: %v", err), 0)
					}
				}
				s.disconnectWebSocket() // 触发重连逻辑
				return
			}

			s.websocketMessageListenerHandle(body)

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
	s.disconnectWebSocketInternal(true) // 连接丢失时触发重连
}

func (s *SocketSDK) disconnectWebSocketNoReconnect() {
	s.disconnectWebSocketInternal(false) // 主动断开时不触发重连
}

func (s *SocketSDK) disconnectWebSocketInternal(triggerReconnect bool) {
	// 只取消当前连接上下文，不影响重连能力
	if s.connCancel != nil {
		s.connCancel()
	}

	// 清理 responseMap
	s.responseMap.Range(func(key, value interface{}) bool {
		s.responseMap.Delete(key)
		return true
	})

	s.connMutex.Lock()
	wasConnected := s.isConnected
	if s.conn != nil {
		s.conn.Close()
		s.conn = nil
	}
	s.isConnected = false
	s.connMutex.Unlock()

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
	zlog.Info("startReconnectProcess called", 0)
	s.reconnectMutex.Lock()

	if !s.reconnectEnabled {
		zlog.Info("reconnect not enabled, skipping", 0)
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

	s.reconnectAttempts++
	interval := s.calculateReconnectIntervalLocked()
	s.lastReconnectTime = time.Now()
	s.reconnectMutex.Unlock()

	// 等待时监听 rootCtx（只有用户主动 Close 才退出）
	select {
	case <-time.After(interval):
	case <-s.rootCtx.Done():
		return
	}

	if !s.valid() {
		if zlog.IsDebug() {
			zlog.Debug("Token expired during reconnect, stopping reconnect process", 0)
		}
		s.triggerTokenExpiredCallback()
		return
	}

	path := s.getConnectedPath()
	if err := s.connectWebSocketInternal(path, false); err != nil {
		zlog.Error(fmt.Sprintf("Reconnect attempt %d failed: %v", s.reconnectAttempts, err), 0)
		go s.startReconnectProcess()
	} else {
		zlog.Info(fmt.Sprintf("Reconnect attempt %d successful", s.reconnectAttempts), 0)
		s.reconnectMutex.Lock()
		s.reconnectAttempts = 0
		s.reconnectMutex.Unlock()
	}
}

// getConnectedPath 获取已连接的路径（调用者必须已持有 s.reconnectMutex）
func (s *SocketSDK) getConnectedPath() string {
	s.reconnectMutex.Lock()
	defer s.reconnectMutex.Unlock()
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
	s.reconnectMutex.Unlock()

	path := s.getConnectedPath()
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
		s.tokenExpiredOnce.Do(func() {
			if zlog.IsDebug() {
				zlog.Debug("Calling token expired callback", 0)
			}
			go s.onTokenExpired()
		})
	}
}

func (s *SocketSDK) SetReconnectConfig(enabled bool, maxAttempts int, initialInterval, maxInterval time.Duration) {
	s.reconnectMutex.Lock()
	defer s.reconnectMutex.Unlock()

	s.reconnectEnabled = enabled
	s.maxReconnectAttempts = maxAttempts
	if s.maxReconnectAttempts <= 0 && s.maxReconnectAttempts != -1 {
		s.maxReconnectAttempts = 10 // 默认10次
	}

	s.reconnectInterval = initialInterval
	if s.reconnectInterval <= 0 {
		s.reconnectInterval = time.Second // 默认1秒
	}

	s.maxReconnectInterval = maxInterval
	if s.maxReconnectInterval <= s.reconnectInterval {
		s.maxReconnectInterval = 30 * time.Second // 默认30秒
	}

	s.reconnectAttempts = 0
	s.lastReconnectTime = time.Time{}

	if zlog.IsDebug() {
		zlog.Debug(fmt.Sprintf("WebSocket reconnect config set: enabled=%t, maxAttempts=%d, interval=%v, maxInterval=%v",
			enabled, maxAttempts, initialInterval, maxInterval), 0)
	}
}

func (s *SocketSDK) EnableReconnect() {
	s.SetReconnectConfig(true, 10, time.Second, 30*time.Second)
}

// DisableReconnect 禁用自动重连
func (s *SocketSDK) DisableReconnect() {
	s.reconnectMutex.Lock()
	defer s.reconnectMutex.Unlock()
	s.reconnectEnabled = false
}

func (s *SocketSDK) GetReconnectStatus() (enabled bool, attempts int, maxAttempts int, nextReconnectTime time.Time) {
	s.reconnectMutex.Lock()
	defer s.reconnectMutex.Unlock()

	enabled = s.reconnectEnabled
	attempts = s.reconnectAttempts
	maxAttempts = s.maxReconnectAttempts

	if !s.lastReconnectTime.IsZero() {
		interval := s.calculateReconnectInterval()
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

// Close 主动关闭整个 SDK（停止所有重连和连接）
func (s *SocketSDK) Close() {
	s.DisconnectWebSocket()
	s.rootCancel()
}

// 示例：如何使用消息订阅功能
//
// // 1. 连接WebSocket
// wsSdk := NewSocketSDK()
// err := wsSdk.ConnectWebSocket("/ws")
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
