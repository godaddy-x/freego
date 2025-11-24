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
	"github.com/valyala/fasthttp"
)

type SocketSDK struct {
	Domain      string      // API域名 (如:api.example.com)
	language    string      // 语言设置 (HTTP头)
	timeout     int64       // 请求超时时间(秒)
	authObject  interface{} // 登录认证对象 (用户名+密码等)
	authToken   AuthToken   // JWT认证令牌
	SSL         bool
	ecdsaObject []crypto.Cipher // ECDSA签名验证对象列表

	// WebSocket连接相关
	conn        *websocket.Conn    // WebSocket连接
	connMutex   sync.Mutex         // 连接互斥锁
	isConnected bool               // 连接状态
	ctx         context.Context    // 上下文
	cancel      context.CancelFunc // 取消函数

	// 重连相关配置
	reconnectEnabled     bool               // 是否启用自动重连
	maxReconnectAttempts int                // 最大重连次数 (-1表示无限重连)
	reconnectInterval    time.Duration      // 重连间隔
	maxReconnectInterval time.Duration      // 最大重连间隔
	reconnectAttempts    int                // 当前重连次数
	lastReconnectTime    time.Time          // 上次重连时间
	reconnectMutex       sync.Mutex         // 重连互斥锁
	connectedPath        string             // 已连接的WebSocket路径 (用于重连)
	stopReconnect        context.CancelFunc // 停止重连的函数

	// Token过期回调
	onTokenExpired     func() // Token过期时回调，用户可以重新认证
	tokenExpiredCalled bool   // 是否已经调用过token过期回调，避免重复调用

	// 新增：同步响应映射表 (msgId -> chan interface{})
	responseMap sync.Map // 存储等待响应的通道
}

func (s *SocketSDK) AuthObject(object interface{}) {
	s.authObject = object
}

func (s *SocketSDK) AuthToken(object AuthToken) {
	s.authToken = object
	s.tokenExpiredCalled = false // 重置token过期回调标志
}

func (s *SocketSDK) SetTimeout(timeout int64) {
	s.timeout = timeout
}

func (s *SocketSDK) SetLanguage(language string) {
	s.language = language
}

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

func (s *SocketSDK) PostByAuth(path string, requestObj, responseObj interface{}, encrypted bool) error {
	if len(path) == 0 || requestObj == nil || responseObj == nil {
		return ex.Throw{Msg: "params invalid"}
	}
	if !s.valid() {
		return ex.Throw{Msg: "token empty or token expired"}
	}
	if len(s.authToken.Token) == 0 || len(s.authToken.Secret) == 0 {
		return ex.Throw{Msg: "token or secret can't be empty"}
	}
	jsonBody := &node.JsonBody{
		Time:  utils.UnixSecond(),
		Nonce: utils.RandNonce(),
		Plan:  0,
	}
	var jsonData []byte
	var err error
	if v, b := requestObj.(*AuthToken); b {
		jsonData, err = utils.JsonMarshal(v)
		if err != nil {
			return ex.Throw{Msg: "request data JsonMarshal invalid"}
		}
		jsonBody.Data = utils.Bytes2Str(jsonData)
	} else {
		jsonData, err = utils.JsonMarshal(requestObj)
		if err != nil {
			return ex.Throw{Msg: "request data JsonMarshal invalid"}
		}
		jsonBody.Data = utils.Bytes2Str(jsonData)
	}
	// 清理JSON序列化数据
	defer DIC.ClearData(jsonData)
	// 解码token secret用于加密和签名
	tokenSecret := utils.Base64Decode(s.authToken.Secret)
	defer DIC.ClearData(tokenSecret) // 清除临时解码的token secret

	if encrypted {
		jsonBody.Plan = 1
		d, err := utils.AesGCMEncryptBase(utils.Str2Bytes(jsonBody.Data), tokenSecret[:32], utils.Str2Bytes(utils.AddStr(jsonBody.Time, jsonBody.Nonce, jsonBody.Plan, path)))
		if err != nil {
			return ex.Throw{Msg: "request data AES encrypt failed"}
		}
		jsonBody.Data = d
		if zlog.IsDebug() {
			zlog.Debug(fmt.Sprintf("request data encrypted: %s", jsonBody.Data), 0)
		}
		// 注意：不能在这里清理d，因为jsonBody.Data仍然引用着它
	} else {
		jsonBody.Data = utils.Base64Encode(jsonBody.Data)
		if zlog.IsDebug() {
			zlog.Debug(fmt.Sprintf("request data base64: %s", jsonBody.Data), 0)
		}
	}
	jsonBody.Sign = utils.Base64Encode(utils.HMAC_SHA256_BASE(utils.Str2Bytes(utils.AddStr(path, jsonBody.Data, jsonBody.Nonce, jsonBody.Time, jsonBody.Plan)), tokenSecret))

	// 添加ECDSA签名
	if err := s.addECDSASign(jsonBody); err != nil {
		return err
	}

	bytesData, err := utils.JsonMarshal(jsonBody)
	if err != nil {
		return ex.Throw{Msg: "jsonBody data JsonMarshal invalid"}
	}
	// 清理序列化后的请求数据
	defer DIC.ClearData(bytesData)
	if zlog.IsDebug() {
		zlog.Debug("request data: ", 0)
		zlog.Debug(utils.Bytes2Str(bytesData), 0)
	}
	request := fasthttp.AcquireRequest()
	request.Header.SetContentType("application/json;charset=UTF-8")
	request.Header.Set("Authorization", s.authToken.Token)
	request.Header.Set("Language", s.language)
	request.Header.SetMethod("POST")
	request.SetRequestURI(s.getURI(path))
	request.SetBody(bytesData)
	defer fasthttp.ReleaseRequest(request)
	response := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(response)
	timeout := 120 * time.Second
	if s.timeout > 0 {
		timeout = time.Duration(s.timeout) * time.Second
	}
	if err := fasthttp.DoTimeout(request, response, timeout); err != nil {
		return ex.Throw{Msg: "post request failed: " + err.Error()}
	}
	respBytes := response.Body()
	// 清理原始响应数据
	defer DIC.ClearData(respBytes)
	if zlog.IsDebug() {
		zlog.Debug("response data: ", 0)
		zlog.Debug(utils.Bytes2Str(respBytes), 0)
	}
	respData := &node.JsonResp{}
	if err := utils.JsonUnmarshal(respBytes, respData); err != nil {
		return ex.Throw{Msg: "response data parse failed: " + err.Error()}
	}
	if respData.Code != 200 {
		if respData.Code > 0 {
			return ex.Throw{Code: respData.Code, Msg: respData.Message}
		}
		return ex.Throw{Msg: respData.Message}
	}

	// 服务器可以自主选择返回的Plan（0或1），只要在有效范围内即可
	if !utils.CheckInt64(respData.Plan, 0, 1) {
		return ex.Throw{Msg: "response plan invalid, must be 0 or 1, got: " + utils.AnyToStr(respData.Plan)}
	}

	// 验证响应时也需要解码token secret
	respTokenSecret := utils.Base64Decode(s.authToken.Secret)
	defer DIC.ClearData(respTokenSecret) // 清除响应验证时解码的token secret

	validSign := utils.HMAC_SHA256_BASE(utils.Str2Bytes(utils.AddStr(path, respData.Data, respData.Nonce, respData.Time, respData.Plan)), respTokenSecret)
	// 清理签名验证数据
	defer DIC.ClearData(validSign)
	if utils.Base64Encode(validSign) != respData.Sign {
		return ex.Throw{Msg: "post response sign verify invalid"}
	}
	if zlog.IsDebug() {
		zlog.Debug(fmt.Sprintf("response sign verify: %t", utils.Base64Encode(validSign) == respData.Sign), 0)
	}

	// 验证ECDSA签名
	if err := s.verifyECDSASign(validSign, respData); err != nil {
		return err
	}
	var dec []byte
	if respData.Plan == 0 {
		dec = utils.Base64Decode(respData.Data)
		if zlog.IsDebug() {
			zlog.Debug(fmt.Sprintf("response data base64: %s", string(dec)), 0)
		}
	} else if respData.Plan == 1 {
		dec, err = utils.AesGCMDecryptBase(respData.Data, respTokenSecret[:32], utils.Str2Bytes(utils.AddStr(respData.Time, respData.Nonce, respData.Plan, path)))
		if err != nil {
			return ex.Throw{Msg: "post response data AES decrypt failed"}
		}
		if zlog.IsDebug() {
			zlog.Debug(fmt.Sprintf("response data decrypted: %s", utils.Bytes2Str(dec)), 0)
		}
	}

	// 清理解密后的响应数据
	defer DIC.ClearData(dec)

	// 验证解密后的数据是否为空
	if len(dec) == 0 {
		return ex.Throw{Msg: "response data is empty"}
	}
	if v, b := responseObj.(*AuthToken); b {
		if err := utils.JsonUnmarshal(dec, v); err != nil {
			return ex.Throw{Msg: "response data JsonUnmarshal invalid"}
		}
	} else {
		if err := utils.JsonUnmarshal(dec, responseObj); err != nil {
			return ex.Throw{Msg: "response data JsonUnmarshal invalid"}
		}
	}
	return nil
}

func (s *SocketSDK) addECDSASign(jsonBody *node.JsonBody) error {
	if len(s.ecdsaObject) > 0 && s.ecdsaObject[0] != nil {
		ecdsaSign, err := s.ecdsaObject[0].Sign(utils.Base64Decode(jsonBody.Sign))
		if err != nil {
			return ex.Throw{Msg: "ECDSA sign failed: " + err.Error()}
		}
		jsonBody.Valid = utils.Base64Encode(ecdsaSign)
		// 清理ECDSA签名数据（在设置完jsonBody.Valid之后）
		DIC.ClearData(ecdsaSign)
		if zlog.IsDebug() {
			zlog.Debug(fmt.Sprintf("ECDSA sign added for HMAC signature: %s", jsonBody.Valid), 0)
		}
	}
	return nil
}

func (s *SocketSDK) verifyECDSASign(validSign []byte, respData *node.JsonResp) error {
	if len(s.ecdsaObject) > 0 && len(respData.Valid) > 0 {
		// 预先解码ECDSA签名数据，避免在循环中重复解码
		ecdsaSignData := utils.Base64Decode(respData.Valid)
		// 清理ECDSA签名解码数据
		defer DIC.ClearData(ecdsaSignData)

		ecdsaValid := false
		for _, ecdsaObj := range s.ecdsaObject {
			if ecdsaObj == nil {
				continue
			}
			if err := ecdsaObj.Verify(validSign, ecdsaSignData); err == nil {
				ecdsaValid = true
				if zlog.IsDebug() {
					zlog.Debug("response ECDSA sign verify: success", 0)
				}
				break
			}
		}
		if !ecdsaValid {
			return ex.Throw{Msg: "post response ECDSA sign verify invalid"}
		}
	}
	return nil
}

func (s *SocketSDK) SetECDSAObject(prkB64, pubB64 string) error {
	cipher, err := crypto.CreateS256ECDSAWithBase64(prkB64, pubB64)
	if err != nil {
		return err
	}
	s.ecdsaObject = append(s.ecdsaObject, cipher)
	return nil
}

func (s *SocketSDK) ConnectWebSocket(path string) error {
	s.reconnectMutex.Lock()
	s.connectedPath = path // 存储连接路径用于重连
	s.reconnectMutex.Unlock()

	return s.connectWebSocketInternal(path, true)
}

func (s *SocketSDK) connectWebSocketInternal(path string, isInitial bool) error {
	s.connMutex.Lock()
	defer s.connMutex.Unlock()

	// 检查是否已有连接
	if s.isConnected && s.conn != nil && !isInitial {
		return nil
	}

	// 验证认证信息
	if !s.valid() {
		// 触发Token过期回调
		s.triggerTokenExpiredCallback()
		return ex.Throw{Msg: "token empty or token expired"}
	}
	if len(s.authToken.Token) == 0 || len(s.authToken.Secret) == 0 {
		return ex.Throw{Msg: "token or secret can't be empty"}
	}

	// --- 修复：取消旧ctx，创建新ctx（无论是否初始连接）---
	if s.cancel != nil {
		s.cancel() // 取消旧的ctx，停止之前的心跳和监听
	}
	s.ctx, s.cancel = context.WithCancel(context.Background())

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

func (s *SocketSDK) prepareWebSocketMessage(path string, data interface{}, plan int64) (*node.JsonBody, []byte, error) {
	jsonBody := &node.JsonBody{
		Time:  utils.UnixSecond(),
		Nonce: utils.RandNonce(),
		Plan:  plan,
	}

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
	if plan == 1 {
		encryptedData, err := utils.AesGCMEncryptBase(jsonData, tokenSecret[:32],
			utils.Str2Bytes(utils.AddStr(jsonBody.Time, jsonBody.Nonce, jsonBody.Plan, path)))
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
	signData := utils.HMAC_SHA256_BASE(
		utils.Str2Bytes(utils.AddStr(path, jsonBody.Data, jsonBody.Nonce, jsonBody.Time, jsonBody.Plan)),
		tokenSecret)
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

func (s *SocketSDK) sendWebSocketAuthHandshake(conn *websocket.Conn, path string) error {
	// 使用通用方法准备握手数据
	_, bytesData, err := s.prepareWebSocketMessage(path, "auth_handshake", 1)
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
	var response node.JsonResp
	if err := utils.JsonUnmarshal(responseBytes, &response); err != nil {
		return ex.Throw{Msg: "handshake response parse failed: " + err.Error()}
	}

	// 检查响应
	if response.Code != 200 {
		return ex.Throw{Msg: fmt.Sprintf("handshake failed: %s", response.Message)}
	}

	if zlog.IsDebug() {
		zlog.Debug("WebSocket authentication handshake completed", 0)
	}

	return nil
}

func (s *SocketSDK) SendWebSocketMessage(path string, requestObj, responseObj interface{}, waitResponse, encryptRequest bool, timeout int64) error {
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
	plan := int64(0)
	if encryptRequest {
		plan = 1
	}
	jsonBody, bytesData, err := s.prepareWebSocketMessage(path, requestObj, plan)
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
			s.responseMap.Delete(jsonBody.Nonce)
			close(respChan)
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
		zlog.Debug(fmt.Sprintf("WebSocket message sent to path: %s, msgID: %s", path, jsonBody.Nonce), 0)
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
		if err := s.verifyWebSocketResponseFromJsonResp(path, responseObj, resp); err != nil {
			return err
		}
		return nil
	case <-time.After(waitTimeout):
		return ex.Throw{Msg: fmt.Sprintf("wait response timeout (msgID: %s)", jsonBody.Nonce)}
	case <-s.ctx.Done():
		return ex.Throw{Msg: fmt.Sprintf("connection closed while waiting response (msgID: %s)", jsonBody.Nonce)}
	}
}

func (s *SocketSDK) verifyWebSocketResponse(path string, response map[string]interface{}) (interface{}, error) {
	// 检查响应代码
	if code, ok := response["code"].(float64); !ok || int(code) != 200 {
		message := "unknown error"
		if msg, ok := response["message"].(string); ok {
			message = msg
		}
		return nil, ex.Throw{Msg: fmt.Sprintf("response error: %s", message)}
	}

	respData, ok := response["data"].(string)
	if !ok {
		return nil, ex.Throw{Msg: "invalid response data"}
	}

	// 验证签名
	tokenSecret := utils.Base64Decode(s.authToken.Secret)
	defer DIC.ClearData(tokenSecret)

	validSign := utils.HMAC_SHA256_BASE(
		utils.Str2Bytes(utils.AddStr(path, respData, response["nonce"], response["time"], response["plan"])),
		tokenSecret)
	expectedSign := utils.Base64Encode(validSign)
	defer DIC.ClearData(validSign)

	if responseSign, ok := response["sign"].(string); !ok || responseSign != expectedSign {
		return nil, ex.Throw{Msg: "response signature verification failed"}
	}

	// 验证ECDSA签名 (如果有)
	if validStr, ok := response["valid"].(string); ok && len(s.ecdsaObject) > 0 {
		ecdsaSignData := utils.Base64Decode(validStr)
		defer DIC.ClearData(ecdsaSignData)

		ecdsaValid := false
		for _, ecdsaObj := range s.ecdsaObject {
			if ecdsaObj == nil {
				continue
			}
			if err := ecdsaObj.Verify(validSign, ecdsaSignData); err == nil {
				ecdsaValid = true
				break
			}
		}
		if !ecdsaValid {
			return nil, ex.Throw{Msg: "response ECDSA signature verification failed"}
		}
	}

	// 解密响应数据
	decryptedData, err := utils.AesGCMDecryptBase(respData, tokenSecret[:32],
		utils.Str2Bytes(utils.AddStr(response["time"], response["nonce"], response["plan"], path)))
	if err != nil {
		return nil, ex.Throw{Msg: "response data decrypt failed"}
	}
	defer DIC.ClearData(decryptedData)

	// 反序列化为目标类型
	var result interface{}
	if err := utils.JsonUnmarshal(decryptedData, &result); err != nil {
		return nil, ex.Throw{Msg: "response data unmarshal failed"}
	}

	return result, nil
}

func (s *SocketSDK) verifyWebSocketResponseFromJsonResp(path string, result interface{}, jsonResp *node.JsonResp) error {
	if jsonResp.Code != 200 {
		return ex.Throw{Msg: fmt.Sprintf("response error: %s", jsonResp.Message)}
	}

	tokenSecret := utils.Base64Decode(s.authToken.Secret)
	defer DIC.ClearData(tokenSecret)

	validSign := utils.HMAC_SHA256_BASE(
		utils.Str2Bytes(utils.AddStr(path, jsonResp.Data, jsonResp.Nonce, jsonResp.Time, jsonResp.Plan)),
		tokenSecret)
	defer DIC.ClearData(validSign)

	if jsonResp.Sign != utils.Base64Encode(validSign) {
		return ex.Throw{Msg: "response signature verification failed"}
	}

	if jsonResp.Valid != "" && len(s.ecdsaObject) > 0 {
		ecdsaSignData := utils.Base64Decode(jsonResp.Valid)
		defer DIC.ClearData(ecdsaSignData)

		ecdsaValid := false
		for _, ecdsaObj := range s.ecdsaObject {
			if ecdsaObj == nil {
				continue
			}
			if err := ecdsaObj.Verify(validSign, ecdsaSignData); err == nil {
				ecdsaValid = true
				break
			}
		}
		if !ecdsaValid {
			return ex.Throw{Msg: "response ECDSA signature verification failed"}
		}
	}

	var decryptedData []byte
	var err error
	if jsonResp.Plan == 1 {
		decryptedData, err = utils.AesGCMDecryptBase(jsonResp.Data, tokenSecret[:32],
			utils.Str2Bytes(utils.AddStr(jsonResp.Time, jsonResp.Nonce, jsonResp.Plan, path)))
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

func (s *SocketSDK) websocketHeartbeat() {
	ticker := time.NewTicker(30 * time.Second) // 每30秒发送一次心跳
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.connMutex.Lock()
			if s.isConnected && s.conn != nil {
				heartbeatMsg := map[string]interface{}{
					"type":      "heartbeat",
					"time":      utils.UnixSecond(),
					"timestamp": time.Now().UnixNano(),
				}
				s.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
				if err := s.conn.WriteJSON(heartbeatMsg); err != nil {
					if zlog.IsDebug() {
						zlog.Debug("heartbeat send failed, connection may be lost", 0)
					}
					s.connMutex.Unlock()
					s.disconnectWebSocket() // 触发重连逻辑
					return
				}
				if zlog.IsDebug() {
					zlog.Debug("heartbeat sent", 0)
				}
			}
			s.connMutex.Unlock()
		}
	}
}

func (s *SocketSDK) websocketMessageListener() {
	for {
		select {
		case <-s.ctx.Done():
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

			res := &node.JsonResp{}
			if err := utils.JsonUnmarshal(body, res); err != nil {
				zlog.Error(fmt.Sprintf("WebSocket read data parse error: %v", err), 0, zlog.String("body", string(body)))
			}

			if len(res.Nonce) == 0 {
				continue
			}

			// 从映射中获取响应通道
			respChanVal, exists := s.responseMap.Load(res.Nonce)
			if exists {
				respChan, ok := respChanVal.(chan *node.JsonResp)
				if ok {
					respChan <- res
				}
				// 移除映射（避免重复处理）
				s.responseMap.Delete(res.Nonce)
			}

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

func (s *SocketSDK) DisconnectWebSocket() {
	s.connMutex.Lock()
	defer s.connMutex.Unlock()
	s.disconnectWebSocket()
}

func (s *SocketSDK) disconnectWebSocket() {
	s.disconnectWebSocketInternal(true)
}

func (s *SocketSDK) disconnectWebSocketInternal(triggerReconnect bool) {
	if s.cancel != nil {
		s.cancel()
	}
	// 清理响应映射（避免goroutine泄漏）
	s.responseMap.Range(func(key, value interface{}) bool {
		s.responseMap.Delete(key)
		if ch, ok := value.(chan *node.JsonResp); ok {
			close(ch)
		}
		return true
	})
	s.connMutex.Lock()
	if s.conn != nil {
		s.conn.Close()
		s.conn = nil
	}
	wasConnected := s.isConnected
	s.isConnected = false
	s.connMutex.Unlock()

	if zlog.IsDebug() {
		zlog.Debug("WebSocket connection closed", 0)
	}

	// 如果之前是连接状态且启用了重连，则触发重连
	if triggerReconnect && wasConnected && s.reconnectEnabled {
		go s.startReconnectProcess()
	}
}

// --- 修复：限制attempts最大为30，防止指数退避溢出 ---
func (s *SocketSDK) calculateReconnectInterval() time.Duration {
	s.reconnectMutex.Lock()
	defer s.reconnectMutex.Unlock()

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

func (s *SocketSDK) startReconnectProcess() {
	s.reconnectMutex.Lock()

	// 检查是否已启用重连
	if !s.reconnectEnabled {
		s.reconnectMutex.Unlock()
		return
	}

	// 检查重连次数限制
	if s.maxReconnectAttempts != -1 && s.reconnectAttempts >= s.maxReconnectAttempts {
		if zlog.IsDebug() {
			zlog.Debug(fmt.Sprintf("Max reconnect attempts (%d) reached, stopping reconnect", s.maxReconnectAttempts), 0)
		}
		s.reconnectMutex.Unlock()
		return
	}

	s.reconnectAttempts++
	interval := s.calculateReconnectInterval()
	s.lastReconnectTime = time.Now()

	if zlog.IsDebug() {
		zlog.Debug(fmt.Sprintf("Scheduling reconnect attempt %d in %v", s.reconnectAttempts, interval), 0)
	}

	s.reconnectMutex.Unlock()

	// 创建重连上下文 (可以被取消)
	reconnectCtx, cancel := context.WithCancel(context.Background())
	s.reconnectMutex.Lock()
	if s.stopReconnect != nil {
		s.stopReconnect()
	}
	s.stopReconnect = cancel
	s.reconnectMutex.Unlock()

	// 等待重连间隔
	select {
	case <-time.After(interval):
		// 继续重连
	case <-reconnectCtx.Done():
		// 重连被取消
		return
	}

	// 检查token是否仍然有效
	if !s.valid() {
		if zlog.IsDebug() {
			zlog.Debug("Token expired during reconnect, stopping reconnect process", 0)
		}

		// 触发Token过期回调
		s.triggerTokenExpiredCallback()
		return
	}

	// 尝试重连 (使用存储的连接路径)
	s.reconnectMutex.Lock()
	path := s.connectedPath
	if path == "" {
		path = "/ws" // 默认路径
	}
	s.reconnectMutex.Unlock()

	if err := s.connectWebSocketInternal(path, false); err != nil {
		if zlog.IsDebug() {
			zlog.Debug(fmt.Sprintf("Reconnect attempt %d failed: %v", s.reconnectAttempts, err), 0)
		}
		// 如果重连失败，继续下一次重连
		go s.startReconnectProcess()
	} else {
		if zlog.IsDebug() {
			zlog.Debug(fmt.Sprintf("Reconnect attempt %d successful", s.reconnectAttempts), 0)
		}
		// 重连成功，重置计数
		s.reconnectMutex.Lock()
		s.reconnectAttempts = 0
		s.reconnectMutex.Unlock()
	}
}

func (s *SocketSDK) ForceReconnect() error {
	s.reconnectMutex.Lock()
	s.reconnectAttempts = 0
	s.lastReconnectTime = time.Time{}
	path := s.connectedPath
	if path == "" {
		path = "/ws" // 默认路径
	}
	s.reconnectMutex.Unlock()

	return s.connectWebSocketInternal(path, false)
}

func (s *SocketSDK) IsWebSocketConnected() bool {
	s.connMutex.Lock()
	defer s.connMutex.Unlock()
	return s.isConnected && s.conn != nil
}

func (s *SocketSDK) SetTokenExpiredCallback(callback func()) {
	s.onTokenExpired = callback
}

func (s *SocketSDK) triggerTokenExpiredCallback() {
	if s.onTokenExpired != nil && !s.tokenExpiredCalled {
		s.tokenExpiredCalled = true // 标记已调用
		if zlog.IsDebug() {
			zlog.Debug("Calling token expired callback", 0)
		}
		go s.onTokenExpired() // 在独立的goroutine中执行，避免阻塞
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
	if s.stopReconnect != nil {
		s.stopReconnect()
	}
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
