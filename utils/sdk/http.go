package sdk

import (
	cryptocdh "crypto/ecdh"
	"fmt"
	"time"

	DIC "github.com/godaddy-x/freego/common"
	"github.com/godaddy-x/freego/utils/crypto"

	ecc "github.com/godaddy-x/eccrypto"
	"github.com/godaddy-x/freego/ex"
	"github.com/godaddy-x/freego/node"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/zlog"
	"github.com/valyala/fasthttp"
)

// AuthToken 认证令牌结构体
// 包含JWT token、动态secret和过期时间
//
//easyjson:json
type AuthToken struct {
	Token   string `json:"token"`   // JWT认证令牌
	Secret  string `json:"secret"`  // 动态生成的AES密钥(Base64编码)
	Expired int64  `json:"expired"` // 令牌过期时间戳(Unix秒)
}

// HttpSDK FreeGo HTTP客户端SDK
// 支持 X25519 密钥协商 + AES-GCM、强制双向 Ed25519 外层签名、JWT 等
//
// 安全特性:
// - AES-256-GCM 认证加密
// - HMAC-SHA256 完整性验证
// - 强制双向 Ed25519 签名 (必须配置)
// - 动态密钥协商 (X25519 + HKDF)
// - 防重放攻击 (时间戳+Nonce)
//
// 使用模式:
// - PostByECC: 匿名访问，使用 X25519 协商会话密钥 (须配置 Ed25519 双向签名)
// - PostByAuth: 登录后访问，使用JWT令牌 (须配置 Ed25519 双向签名)
type HttpSDK struct {
	Domain        string                      // API域名 (如: https://api.example.com)
	AuthDomain    string                      // 认证域名 (可选，用于/key和/login接口)
	KeyPath       string                      // 公钥获取路径 (默认: /key)
	LoginPath     string                      // 登录路径 (默认: /login)
	language      string                      // 语言设置 (HTTP头)
	timeout       int64                       // 请求超时时间(秒)
	ClientNo      int64                       // 客户端编号
	authObject    func() (interface{}, error) // 登录认证对象 (用户名+密码等)
	authToken     AuthToken                   // JWT认证令牌
	ed25519Object map[int64]crypto.Cipher     // Ed25519 双向外层签名（本端私钥 + 对端公钥）
	x25519Peer    crypto.Cipher               // 可选：复用的 X25519 临时密钥（实现 Cipher，一般留空由 PostByECC 每次生成）
}

// NewHttpSDK 创建新的HttpSDK实例并设置默认值
// 提供便捷的构造函数，避免手动初始化所有字段
//
// 默认值:
// - KeyPath: "/key"
// - LoginPath: "/login"
// - timeout: 120秒
// - language: "zh-CN"
//
// 返回值:
//   - *HttpSDK: 初始化的HttpSDK实例
//
// 使用示例:
//
//	sdk := NewHttpSDK()
//	sdk.Domain = "https://api.example.com"
//	sdk.ClientNo = 12345
func NewHttpSDK() *HttpSDK {
	return &HttpSDK{
		KeyPath:   "/key",
		LoginPath: "/login",
		timeout:   120,
		language:  "",
	}
}

// AuthObject 设置登录认证对象
// 用于存储用户名、密码等登录凭据
// 自动登录时会使用此对象调用登录接口
//
// 参数:
//   - object: 认证对象，包含用户名密码等信息
//
// 注意: 请使用指针对象以避免数据拷贝
func (s *HttpSDK) AuthObject(object func() (interface{}, error)) {
	s.authObject = object
}

// AuthToken 设置JWT认证令牌
// 设置登录成功后获得的令牌，用于后续API调用的身份认证
//
// 参数:
//   - object: AuthToken结构体，包含token、secret、expired字段
//
// 安全特性:
// - Token: JWT令牌，用于身份验证
// - Secret: 动态生成的AES密钥，用于数据加密
// - Expired: 令牌过期时间
//
// 注意: 此方法会覆盖之前设置的令牌
func (s *HttpSDK) AuthToken(object AuthToken) {
	s.authToken = object
}

// SetTimeout 设置HTTP请求超时时间
// 控制单个API请求的最大等待时间
//
// 参数:
//   - timeout: 超时时间(秒)，0表示使用默认120秒
//
// 默认值: 120秒
// 建议值: 根据网络状况设置，如30-300秒
func (s *HttpSDK) SetTimeout(timeout int64) {
	s.timeout = timeout
}

func (s *HttpSDK) SetClientNo(usr int64) {
	s.ClientNo = usr
}

// SetLanguage 设置HTTP请求语言头
// 用于服务端国际化支持
//
// 参数:
//   - language: 语言代码，如"zh-CN"、"en-US"等
//
// HTTP头: 设置为"Language: {language}"
func (s *HttpSDK) SetLanguage(language string) {
	s.language = language
}

// SetEd25519Object 配置当前 HTTP 客户端身份：本端 Ed25519 私钥 + 对端（服务端）Ed25519 公钥。
// 与服务端 HttpNode.AddCipher 独立；镜像关系见 crypto.CreateEd25519WithBase64 注释。
func (s *HttpSDK) SetEd25519Object(usr int64, prkB64, peerPubB64 string) error {
	if s.ed25519Object == nil {
		s.ed25519Object = make(map[int64]crypto.Cipher)
	}
	cipher, err := crypto.CreateEd25519WithBase64(prkB64, peerPubB64)
	if err != nil {
		return err
	}
	s.ed25519Object[usr] = cipher
	return nil
}

func (s *HttpSDK) addEd25519Sign(jsonBody *node.JsonBody) error {
	if s.ed25519Object == nil {
		return ex.Throw{Msg: "Ed25519 object not configured, bidirectional Ed25519 signature is required"}
	}
	cipher, exists := s.ed25519Object[s.ClientNo]
	if !exists || cipher == nil {
		return ex.Throw{Msg: "Ed25519 object not found for client, bidirectional Ed25519 signature is required"}
	}
	outerSign, err := cipher.Sign(utils.Base64Decode(jsonBody.Sign))
	if err != nil {
		return ex.Throw{Msg: "Ed25519 sign failed: " + err.Error()}
	}
	jsonBody.Valid = utils.Base64Encode(outerSign)
	DIC.ClearData(outerSign)
	if zlog.IsDebug() {
		zlog.Debug(fmt.Sprintf("Ed25519 sign added for HMAC signature: %s", jsonBody.Valid), 0)
	}
	return nil
}

func (s *HttpSDK) verifyEd25519Sign(validSign []byte, respData *node.JsonResp) error {
	if s.ed25519Object == nil {
		return ex.Throw{Msg: "Ed25519 object not configured, bidirectional Ed25519 signature is required"}
	}
	cipher, exists := s.ed25519Object[s.ClientNo]
	if !exists || cipher == nil {
		return ex.Throw{Msg: "Ed25519 object not found for client, bidirectional Ed25519 signature is required"}
	}
	outerSignData := utils.Base64Decode(respData.Valid)
	defer DIC.ClearData(outerSignData)
	if err := cipher.Verify(validSign, outerSignData); err != nil {
		return ex.Throw{Msg: "post response Ed25519 sign verify invalid"}
	}
	return nil
}

// SetX25519Object 设置预生成的 X25519 临时密钥对象，用于复用密钥对（少见场景）。
func (s *HttpSDK) SetX25519Object(object *crypto.X25519Object) error {
	s.x25519Peer = object
	return nil
}

// debugOut 调试输出函数
// 仅在Debug模式开启时使用zlog输出调试信息
//
// 参数:
//   - a: 可变参数，任意类型的值
//
// 输出格式: 使用zlog.Debug输出到日志系统
// 性能影响: 在非调试模式下开销为零
// getURI 构建完整的请求URI
// 根据路径类型选择不同的域名
//
// 域名选择逻辑:
// - 如果是KeyPath(/key)或LoginPath(/login)，使用AuthDomain
// - 其他API路径使用主Domain
//
// 参数:
//   - path: API路径，如"/user/info"
//
// 返回值:
//   - string: 完整的URI，如"https://api.example.com/user/info"
//
// 用途: 支持将认证接口和业务接口部署在不同域名下
func (s *HttpSDK) getURI(path string) string {
	if s.KeyPath == path || s.LoginPath == path {
		if len(s.AuthDomain) > 0 {
			return s.AuthDomain + path
		}
	}
	return s.Domain + path
}

// GetPublicKey 获取服务端 X25519 临时公钥并建立 Plan2 安全信道
// 核心流程：请求 /key、校验 Ed25519、生成本端 X25519 临时密钥对。
//
// 执行流程:
// 1. 生成本端临时 X25519 密钥对（每次 PostByECC 一轮协商）
// 2. 构造公钥交换请求
// 3. 请求服务端 KeyPath 获取服务端临时公钥
// 4. 校验服务端 Ed25519 等（由 node.CheckPublicKey 等完成）
// 5. 返回本端 X25519 对象与服务端 PublicKey 封装
//
// 安全特性:
// - X25519 密钥协商 + HKDF：前向保密（PFS）
// - 临时密钥：每次请求新密钥对
// - 强制 Ed25519：验证对端身份
//
// 返回值:
//   - *crypto.X25519Object: 客户端临时 X25519 密钥（含私钥）
//   - *node.PublicKey: 服务端公钥与随机数等
//   - crypto.Cipher: Ed25519 校验用 Cipher
//   - error: 错误信息
//
// 注意:
// - 须预先配置 ed25519Object
// - X25519 私钥敏感，用完应清零（PostByECC 内 defer 已处理）
func (s *HttpSDK) GetPublicKey() (*crypto.X25519Object, *node.PublicKey, crypto.Cipher, error) {
	if s.ed25519Object == nil {
		return nil, nil, nil, ex.Throw{Msg: "Ed25519 object not configured, bidirectional Ed25519 signature is required"}
	}
	cipher, exists := s.ed25519Object[s.ClientNo]
	if !exists || cipher == nil {
		return nil, nil, nil, ex.Throw{Msg: "Ed25519 object not found for client, bidirectional Ed25519 signature is required"}
	}
	public, err := node.CreatePublicKey(utils.Base64Encode(utils.GetRandomSecure(32)), utils.Base64Encode(utils.GetRandomSecure(32)), s.ClientNo, cipher)
	if err != nil {
		return nil, nil, nil, err
	}
	publicBody, err := utils.JsonMarshal(public)
	if err != nil {
		return nil, nil, nil, ex.Throw{Msg: "request object json marshal error: " + err.Error(), Err: err}
	}
	// 清理临时公钥数据
	defer DIC.ClearData(publicBody)
	// 发送请求
	request := fasthttp.AcquireRequest()
	request.Header.SetMethod("POST")
	request.SetRequestURI(s.getURI(s.KeyPath))
	request.SetBody(publicBody)
	defer fasthttp.ReleaseRequest(request)
	response := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(response)

	timeout := 120 * time.Second
	if s.timeout > 0 {
		timeout = time.Duration(s.timeout) * time.Second
	}
	if err := fasthttp.DoTimeout(request, response, timeout); err != nil {
		return nil, nil, nil, ex.Throw{Msg: "post request failed: " + err.Error()}
	}
	respBytes := response.Body()
	if len(respBytes) == 0 {
		return nil, nil, nil, ex.Throw{Msg: "response public key invalid"}
	}
	if !utils.JsonValid(respBytes) {
		return nil, nil, nil, ex.Throw{Msg: "request public error: " + utils.Bytes2Str(respBytes)}
	}
	if zlog.IsDebug() {
		zlog.Debug(fmt.Sprintf("response message: %s", utils.Bytes2Str(respBytes)), 0)
	}
	responseObject := &node.PublicKey{}
	if err := utils.JsonUnmarshal(respBytes, responseObject); err != nil {
		return nil, nil, nil, ex.Throw{Msg: "request public key parse error: " + err.Error()}
	}
	if err := node.CheckPublicKey(nil, responseObject, cipher); err != nil {
		return nil, nil, nil, err
	}
	// 生成本端临时 X25519 密钥对
	xKey := &crypto.X25519Object{}
	if err := xKey.CreateX25519(); err != nil {
		return nil, nil, nil, ex.Throw{Msg: "create x25519 key exchange object error: " + err.Error()}
	}
	return xKey, responseObject, cipher, nil
}

// PostByECC 通过 X25519 + HKDF + AES-GCM + 强制 Ed25519 发送 POST（Plan2，匿名场景）
//
// 安全协议栈 (从下到上):
// 1. TLS 1.2+（传输层，由部署保证）
// 2. X25519 共享秘密 + HKDF → 会话密钥，再 AES-256-GCM
// 3. HMAC-SHA256 完整性
// 4. 双向 Ed25519
//
// 执行流程:
// 1. GetPublicKey：服务端临时 X25519 公钥 + 本端临时密钥对
// 2. GenSharedKeyX25519 得到共享字节
// 3. HKDF 派生对称密钥
// 4. AES-GCM 加密请求体
// 5. HMAC-SHA256
// 6. Ed25519 签名
// 7. 发送请求
// 8. 校验响应 HMAC / Ed25519
// 9. AES-GCM 解密响应
//
// 参数:
//   - path: API路径，如"/user/info"
//   - requestObj: 请求数据对象 (会自动JSON序列化)
//   - responseObj: 响应数据对象指针 (会自动JSON反序列化)
//
// 返回值:
//   - error: 请求失败时的错误信息
//
// 安全特性:
// - 前向保密性 (PFS): 每次请求使用新密钥
// - 完美前向保密: X25519 临时密钥不依赖长期对称密钥
// - 双向认证: 客户端和服务端相互验证身份
// - 防重放攻击: 时间戳 + Nonce机制
//
// 使用场景:
// - 金融交易API
// - 敏感数据传输
// - 匿名访问但需要高安全性的接口
//
// 注意:
// - 每次调用都会执行完整的密钥协商流程
// - 适合低频但高安全要求的API调用
func (s *HttpSDK) PostByECC(path string, requestObj, responseObj interface{}) error {
	if len(path) == 0 || requestObj == nil || responseObj == nil {
		return ex.Throw{Msg: "params invalid"}
	}
	xKey, public, cipher, err := s.GetPublicKey()
	if err != nil {
		return err
	}
	defer func() {
		if xKey != nil {
			if prk, getErr := xKey.GetPrivateKey(); getErr == "" && prk != nil {
				if xPrk, ok := prk.(*cryptocdh.PrivateKey); ok {
					DIC.ClearData(xPrk.Bytes())
				}
			}
		}
	}()

	pub, err := ecc.LoadX25519PublicKeyFromBase64(public.Key)
	if err != nil {
		return ex.Throw{Msg: "load X25519 public key failed"}
	}
	prk, _ := xKey.GetPrivateKey()
	prkBytes := prk.(*cryptocdh.PrivateKey).Bytes()
	x25519Shared, err := ecc.GenSharedKeyX25519(prk.(*cryptocdh.PrivateKey), pub)
	if err != nil {
		return ex.Throw{Msg: "X25519 shared secret failed"}
	}
	defer DIC.ClearData(x25519Shared)

	sharedKey, err := node.HKDFKey(x25519Shared, public.Noc)
	defer DIC.ClearData(prkBytes, sharedKey) // 清除 X25519 私钥原始字节与 HKDF 输出
	if err != nil {
		return ex.Throw{Msg: "HKDF shared key failed"}
	}

	jsonBody := node.GetJsonBody()
	defer node.PutJsonBody(jsonBody)
	jsonBody.Time = utils.UnixSecond()
	jsonBody.Nonce = utils.RandNonce()
	jsonBody.Plan = 2
	jsonBody.User = s.ClientNo

	var jsonData []byte
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
	if zlog.IsDebug() {
		zlog.Debug(fmt.Sprintf("server key: %s", public.Key), 0)
		zlog.Debug(fmt.Sprintf("client key: %s", xKey.PublicKeyBase64), 0)
		zlog.Debug(fmt.Sprintf("shared key: %s", utils.Base64Encode(sharedKey)), 0)
	}
	// 使用 AES-GCM 加密，Nonce 作为 AAD
	d, err := utils.AesGCMEncryptBase(utils.Str2Bytes(jsonBody.Data), sharedKey, node.AppendBodyMessage(path, "", jsonBody.Nonce, jsonBody.Time, jsonBody.Plan, jsonBody.User))
	if err != nil {
		return ex.Throw{Msg: "request data AES encrypt failed"}
	}
	jsonBody.Data = d
	jsonBody.Sign = utils.Base64Encode(node.SignBodyMessage(path, jsonBody.Data, jsonBody.Nonce, jsonBody.Time, jsonBody.Plan, jsonBody.User, sharedKey))
	// 添加Ed25519签名
	if err := s.addEd25519Sign(jsonBody); err != nil {
		return err
	}

	bytesData, err := utils.JsonMarshal(jsonBody)
	// 注意：不能清理d，因为jsonBody.Data仍然引用着它
	// d的数据会在jsonBody的生命周期结束后被清理
	if err != nil {
		return ex.Throw{Msg: "jsonBody data JsonMarshal invalid"}
	}
	// 清理序列化后的请求数据
	defer DIC.ClearData(bytesData)
	if zlog.IsDebug() {
		zlog.Debug("request data: ", 0)
		zlog.Debug(utils.Bytes2Str(bytesData), 0)
	}

	key, err := node.CreatePublicKey(public.Key, utils.Base64Encode(prk.(*cryptocdh.PrivateKey).PublicKey().Bytes()), s.ClientNo, cipher)
	if err != nil {
		return err
	}
	auth, err := utils.JsonMarshal(key)
	if err != nil {
		return ex.Throw{Msg: "public key json parse invalid"}
	}
	// 清理授权数据
	defer DIC.ClearData(auth)

	request := fasthttp.AcquireRequest()
	request.Header.SetContentType("application/json;charset=UTF-8")
	request.Header.Set("Authorization", utils.Base64Encode(auth))
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
	if !utils.JsonValid(respBytes) {
		return ex.Throw{Msg: "response data not json: " + utils.Bytes2Str(respBytes)}
	}
	respData := node.GetJsonResp()
	defer node.PutJsonResp(respData)
	if err := utils.JsonUnmarshal(respBytes, respData); err != nil {
		return ex.Throw{Msg: "response data parse failed: " + err.Error()}
	}
	if respData.Code != 200 {
		if !utils.JsonValid(respBytes) && len(respData.Message) == 0 {
			return ex.Throw{Msg: utils.Bytes2Str(respBytes)}
		}
		if respData.Code > 0 {
			return ex.Throw{Code: respData.Code, Msg: respData.Message}
		}
		return ex.Throw{Msg: respData.Message}
	}

	// 验证服务端响应时间戳，防止重放攻击
	if respData.Time <= 0 {
		return ex.Throw{Msg: "response time must be > 0"}
	}
	if utils.MathAbs(utils.UnixSecond()-respData.Time) > 300 { // 5分钟时间窗口
		return ex.Throw{Msg: "response time invalid"}
	}

	validSign := node.SignBodyMessage(path, respData.Data, respData.Nonce, respData.Time, respData.Plan, jsonBody.User, sharedKey)
	// 清理签名验证数据
	defer DIC.ClearData(validSign)
	if utils.Base64Encode(validSign) != respData.Sign {
		return ex.Throw{Msg: "post response sign verify invalid"}
	}
	if zlog.IsDebug() {
		zlog.Debug(fmt.Sprintf("response sign verify: %t", utils.Base64Encode(validSign) == respData.Sign), 0)
	}

	// 验证Ed25519签名
	if err := s.verifyEd25519Sign(validSign, respData); err != nil {
		return err
	}
	dec, err := utils.AesGCMDecryptBase(respData.Data, sharedKey, node.AppendBodyMessage(path, "", respData.Nonce, respData.Time, respData.Plan, jsonBody.User))
	if err != nil {
		return ex.Throw{Msg: "post response data AES decrypt failed"}
	}
	// 清理解密后的响应数据
	defer DIC.ClearData(dec)
	if zlog.IsDebug() {
		zlog.Debug(fmt.Sprintf("response data decrypted: %s", utils.Bytes2Str(dec)), 0)
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

// valid 验证当前认证令牌是否有效
// 检查令牌是否存在、Secret是否存在以及是否即将过期
//
// 过期判断逻辑:
// - 当前时间 > 过期时间 - 3600秒 (提前1小时判断过期)
// - 避免在请求过程中令牌突然过期
//
// 返回值:
//   - bool: true表示令牌有效，false表示无效
//
// 安全考虑:
// - 即使令牌在技术上仍有效，也会提前预警
// - 防止在请求执行过程中令牌过期导致认证失败
func (s *HttpSDK) valid() bool {
	if len(s.authToken.Token) == 0 {
		return false
	}
	if len(s.authToken.Secret) == 0 {
		return false
	}
	if utils.UnixSecond() > s.authToken.Expired {
		return false
	}
	return true
}

// checkAuth 检查认证状态并自动登录
// 如果当前没有有效的认证令牌，会自动调用登录流程
//
// 自动登录流程:
// 1. 检查当前令牌是否有效
// 2. 如果无效，使用authObject调用PostByECC登录
// 3. 保存新的认证令牌到authToken字段
//
// 返回值:
//   - error: 认证失败时的错误信息
//
// 设计优势:
// - 透明的认证管理，上层调用无需关心认证状态
// - 自动处理令牌过期和续期
// - 支持各种登录方式 (用户名密码、证书等)
//
// 注意:
// - 需要预先设置AuthObject和相关配置
// - 登录接口路径通过LoginPath指定
func (s *HttpSDK) checkAuth() error {
	if s.valid() {
		return nil
	}
	if len(s.Domain) == 0 {
		return ex.Throw{Msg: "domain is nil"}
	}
	if len(s.KeyPath) == 0 {
		return ex.Throw{Msg: "keyPath is nil"}
	}
	if len(s.LoginPath) == 0 {
		return ex.Throw{Msg: "loginPath is nil"}
	}
	if s.authObject == nil {
		return ex.Throw{Msg: "authObject is nil"}
	}
	requestObject, err := s.authObject()
	if err != nil {
		return ex.Throw{Msg: "auth object error: " + err.Error()}
	}
	responseObj := AuthToken{}
	if err := s.PostByECC(s.LoginPath, requestObject, &responseObj); err != nil {
		return err
	}
	s.AuthToken(responseObj)
	return nil
}

func (s *HttpSDK) ResetAuth() error {
	if len(s.Domain) == 0 {
		return ex.Throw{Msg: "domain is nil"}
	}
	if len(s.KeyPath) == 0 {
		return ex.Throw{Msg: "keyPath is nil"}
	}
	if len(s.LoginPath) == 0 {
		return ex.Throw{Msg: "loginPath is nil"}
	}
	if s.authObject == nil {
		return ex.Throw{Msg: "authObject is nil"}
	}
	requestObject, err := s.authObject()
	if err != nil {
		return ex.Throw{Msg: "auth object error: " + err.Error()}
	}
	responseObj := AuthToken{}
	if err := s.PostByECC(s.LoginPath, requestObject, &responseObj); err != nil {
		return err
	}
	s.AuthToken(responseObj)
	return nil
}

func (s *HttpSDK) GetAuth() AuthToken {
	return AuthToken{Token: s.authToken.Token, Secret: s.authToken.Secret, Expired: s.authToken.Expired}
}

// PostByAuth 通过JWT认证+强制Ed25519模式发送POST请求
// 适用于登录后的业务API调用，使用令牌进行身份认证
//
// 安全协议栈:
// 1. TLS 1.2+ (网络层加密)
// 2. JWT令牌认证 (身份验证)
// 3. AES-GCM或Base64 (数据加密，可选)
// 4. HMAC-SHA256 (数据完整性)
// 5. 强制双向Ed25519签名验证 (必须配置)
//
// 执行流程:
// 1. 检查认证状态，自动登录 (如果需要)
// 2. 获取认证令牌 (Token + Secret)
// 3. 根据encrypted参数选择加密方式
// 4. 生成HMAC-SHA256签名
// 5. 添加Ed25519签名 (如果配置)
// 6. 发送HTTP请求 (Authorization头)
// 7. 验证响应HMAC签名
// 8. 验证响应Ed25519签名 (如果配置)
// 9. 解密响应数据
//
// 参数:
//   - path: API路径，如"/user/info"
//   - requestObj: 请求数据对象
//   - responseObj: 响应数据对象指针
//   - encrypted: 是否使用AES-GCM加密 (true=Plan1, false=Plan0)
//
// 返回值:
//   - error: 请求失败时的错误信息
//
// 安全特性:
// - JWT令牌认证: 身份验证和授权
// - 动态Secret: 每次登录生成新的AES密钥
// - 可选加密: 支持明文(Base64)和加密传输
// - 双向签名: 可配置Ed25519提供金融级安全
//
// 使用场景:
// - 用户登录后的业务API调用
// - 需要身份认证的接口
// - 中等安全要求的场景
//
// 性能特点:
// - 比 PostByECC（Plan2 / X25519 协商）更快：复用令牌，无每请求协商
// - 支持高并发 (令牌复用)
// - 适合高频API调用
//
// 注意:
// - 需要预先登录或设置有效的authToken
// - 会自动处理令牌过期和续期
func (s *HttpSDK) PostByAuth(path string, requestObj, responseObj interface{}, encrypted bool) error {
	if len(path) == 0 || requestObj == nil || responseObj == nil {
		return ex.Throw{Msg: "params invalid"}
	}
	if err := s.checkAuth(); err != nil {
		return err
	}
	token := s.authToken.Token
	secret := s.authToken.Secret
	if len(token) == 0 || len(secret) == 0 {
		return ex.Throw{Msg: "token or secret can't be empty"}
	}
	jsonBody := node.GetJsonBody()
	defer node.PutJsonBody(jsonBody)
	jsonBody.Time = utils.UnixSecond()
	jsonBody.Nonce = utils.RandNonce()
	jsonBody.Plan = 0
	jsonBody.User = s.ClientNo
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
	tokenSecret := utils.Base64Decode(secret)
	defer DIC.ClearData(tokenSecret) // 清除临时解码的token secret

	if encrypted {
		jsonBody.Plan = 1
		d, err := utils.AesGCMEncryptBase(utils.Str2Bytes(jsonBody.Data), tokenSecret[:32], node.AppendBodyMessage(path, "", jsonBody.Nonce, jsonBody.Time, jsonBody.Plan, jsonBody.User))
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
	jsonBody.Sign = utils.Base64Encode(node.SignBodyMessage(path, jsonBody.Data, jsonBody.Nonce, jsonBody.Time, jsonBody.Plan, jsonBody.User, tokenSecret))
	// 添加Ed25519签名
	if err := s.addEd25519Sign(jsonBody); err != nil {
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
	request.Header.Set("Authorization", token)
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
	if !utils.JsonValid(respBytes) {
		return ex.Throw{Msg: "response data not json: " + utils.Bytes2Str(respBytes)}
	}
	respData := node.GetJsonResp()
	defer node.PutJsonResp(respData)
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

	// 验证服务端响应时间戳，防止重放攻击
	if respData.Time <= 0 {
		return ex.Throw{Msg: "response time must be > 0"}
	}
	if utils.MathAbs(utils.UnixSecond()-respData.Time) > 300 { // 5分钟时间窗口
		return ex.Throw{Msg: "response time invalid"}
	}

	validSign := node.SignBodyMessage(path, respData.Data, respData.Nonce, respData.Time, respData.Plan, jsonBody.User, tokenSecret)
	// 清理签名验证数据
	defer DIC.ClearData(validSign)
	if utils.Base64Encode(validSign) != respData.Sign {
		return ex.Throw{Msg: "post response sign verify invalid"}
	}
	if zlog.IsDebug() {
		zlog.Debug(fmt.Sprintf("response sign verify: %t", utils.Base64Encode(validSign) == respData.Sign), 0)
	}

	// 验证Ed25519签名
	if err := s.verifyEd25519Sign(validSign, respData); err != nil {
		return err
	}
	var dec []byte
	if respData.Plan == 0 {
		dec = utils.Base64Decode(respData.Data)
		if zlog.IsDebug() {
			zlog.Debug(fmt.Sprintf("response data base64: %s", string(dec)), 0)
		}
	} else if respData.Plan == 1 {
		dec, err = utils.AesGCMDecryptBase(respData.Data, tokenSecret[:32], node.AppendBodyMessage(path, "", respData.Nonce, respData.Time, respData.Plan, jsonBody.User))
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

// BuildRequestObject 构建标准的API请求数据
// 用于生成符合FreeGo协议的请求JSON数据
//
// 协议格式:
//
//	{
//	  "d": "数据(Base64或AES加密)",
//	  "t": 时间戳,
//	  "n": Nonce随机数,
//	  "p": Plan模式(0=Base64, 1=AES),
//	  "s": HMAC-SHA256签名
//	}
//
// 构建流程:
// 1. JSON序列化请求对象
// 2. 根据encrypted参数选择编码方式
// 3. 生成时间戳和Nonce
// 4. 计算HMAC-SHA256签名
// 5. 构建完整的请求JSON
//
// 参数:
//   - path: API路径，用于签名计算
//   - requestObj: 请求数据对象，会被JSON序列化
//   - secret: HMAC签名密钥
//   - encrypted: 可选参数，是否使用AES加密 (默认false)
//
// 返回值:
//   - []byte: 构建好的请求JSON字节数组
//   - error: 构建失败时的错误信息
//
// 兼容性:
// - 这是早期版本的构建函数
// - 新代码推荐使用PostByAuth或PostByECC方法
// - 主要用于向后兼容和简单场景
//
// 注意:
// - 不支持Ed25519签名
// - 使用固定的AES-CBC加密 (非GCM模式)
// - HMAC签名算法略有不同
func BuildRequestObject(path string, requestObj interface{}, secret string, encrypted ...bool) ([]byte, error) {
	if len(path) == 0 || requestObj == nil {
		return nil, ex.Throw{Msg: "params invalid"}
	}
	jsonData, err := utils.JsonMarshal(requestObj)
	if err != nil {
		return nil, ex.Throw{Msg: "request data JsonMarshal invalid"}
	}
	// 清理JSON序列化数据
	defer DIC.ClearData(jsonData)

	jsonBody := &node.JsonBody{
		Data:  utils.Bytes2Str(jsonData),
		Time:  utils.UnixSecond(),
		Nonce: utils.RandNonce(),
		Plan:  0,
	}
	if len(encrypted) > 0 && encrypted[0] {
		d, err := utils.AesCBCEncrypt(utils.Str2Bytes(jsonBody.Data), secret)
		if err != nil {
			return nil, ex.Throw{Msg: "request data AES encrypt failed"}
		}
		jsonBody.Data = d
		jsonBody.Plan = 1
	} else {
		jsonBody.Data = utils.Base64Encode(jsonBody.Data)
	}
	jsonBody.Sign = utils.Base64Encode(node.SignBodyMessage(path, jsonBody.Data, jsonBody.Nonce, jsonBody.Time, jsonBody.Plan, jsonBody.User, utils.Str2Bytes(secret)))
	bytesData, err := utils.JsonMarshal(jsonBody)
	if err != nil {
		return nil, ex.Throw{Msg: "jsonBody data JsonMarshal invalid"}
	}
	// 注意：不能清理jsonBody.Data，因为bytesData包含了对这些数据的序列化引用
	// 注意：bytesData是返回值，不能清理
	return bytesData, nil
}
