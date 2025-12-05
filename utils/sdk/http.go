package sdk

import (
	ecdh2 "crypto/ecdh"
	"crypto/sha512"
	"fmt"
	"time"

	DIC "github.com/godaddy-x/freego/common"
	"github.com/godaddy-x/freego/utils/crypto"

	"golang.org/x/crypto/pbkdf2"

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
// 支持ECC+AES-GCM加密传输、强制双向ECDSA签名验证、JWT认证等
//
// 安全特性:
// - AES-256-GCM 认证加密
// - HMAC-SHA256 完整性验证
// - 强制双向ECDSA签名验证 (必须配置)
// - 动态密钥协商 (ECDH)
// - 防重放攻击 (时间戳+Nonce)
//
// 使用模式:
// - PostByECC: 匿名访问，使用ECC密钥协商 (必须配置ECDSA)
// - PostByAuth: 登录后访问，使用JWT令牌 (必须配置ECDSA)
type HttpSDK struct {
	Domain      string                  // API域名 (如: https://api.example.com)
	AuthDomain  string                  // 认证域名 (可选，用于/key和/login接口)
	KeyPath     string                  // 公钥获取路径 (默认: /key)
	LoginPath   string                  // 登录路径 (默认: /login)
	language    string                  // 语言设置 (HTTP头)
	timeout     int64                   // 请求超时时间(秒)
	ClientNo    int64                   // 客户端编号
	authObject  interface{}             // 登录认证对象 (用户名+密码等)
	authToken   AuthToken               // JWT认证令牌
	ecdsaObject map[int64]crypto.Cipher // ECDSA签名验证对象列表
	ecdhObject  crypto.Cipher           // ECDH密钥协商对象
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
func (s *HttpSDK) AuthObject(object interface{}) {
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

// SetECDSAObject 设置ECDSA签名验证对象
// 用于强制双向ECDSA签名验证，提供金融级身份认证
//
// 安全特性:
// - 强制双向ECDSA签名验证 (必须配置)
// - 支持多个备用ECDSA密钥对
// - 客户端签名，服务端验证
// - 服务端签名，客户端验证
// - 防止中间人攻击和身份伪造
//
// 参数:
//   - prkB64: ECDSA私钥(Base64编码)，用于客户端签名
//   - pubB64: ECDSA公钥(Base64编码)，用于服务端验证
//
// 返回值:
//   - error: 创建失败时的错误信息
//
// 注意:
// - 必须配置ECDSA对象，否则所有请求都会失败
// - 可以多次调用添加多个备用密钥对
// - 验证时会尝试所有配置的密钥对
// - 私钥仅用于签名，公钥用于验证
func (s *HttpSDK) SetECDSAObject(usr int64, prkB64, pubB64 string) error {
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

// addECDSASign 为请求JSON体添加ECDSA签名
// 对HMAC-SHA256签名进行二次ECDSA签名，实现双重签名验证
//
// 签名流程:
// 1. 首先生成HMAC-SHA256签名 (jsonBody.Sign)
// 2. 使用ECDSA私钥对HMAC签名进行签名
// 3. 将ECDSA签名结果存储到jsonBody.Valid字段
//
// 安全优势:
// - HMAC保证数据完整性
// - ECDSA保证身份真实性和不可否认性
// - 双重验证，安全性大幅提升
//
// 参数:
//   - jsonBody: 请求JSON体结构体指针
//
// 返回值:
//   - error: 签名失败时的错误信息
//
// 注意: 必须配置双向ECDSA签名，否则会抛出异常
func (s *HttpSDK) addECDSASign(jsonBody *node.JsonBody) error {
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

// verifyECDSASign 验证响应数据的ECDSA签名
// 对服务端返回的ECDSA签名进行验证，确保响应数据的真实性和完整性
//
// 验证流程:
// 1. 检查是否配置了ECDSA对象（必须配置）
// 2. 检查响应数据是否包含ECDSA签名 (respData.Valid)
// 3. 使用配置的ECDSA对象进行验证
//
// 安全特性:
// - 双向ECDSA签名验证
// - 验证服务端的身份真实性
// - 防止响应数据被篡改
// - 提供不可否认性保证
//
// 参数:
//   - validSign: 已验证的HMAC签名字节数组
//   - respData: 响应数据结构体指针
//
// 返回值:
//   - error: 验证失败时的错误信息
//
// 注意:
// - 必须配置双向ECDSA签名，否则会抛出异常
// - 如果响应不包含ECDSA签名，会验证失败
func (s *HttpSDK) verifyECDSASign(validSign []byte, respData *node.JsonResp) error {
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

// SetECDHObject 设置预生成的ECDH密钥协商对象
// 用于复用ECDH密钥对，避免重复生成
//
// 参数:
//   - object: ECDH密钥协商对象指针
//
// 返回值:
//   - error: 设置失败时的错误信息
//
// 高级用法: 在连接池或长连接场景下可以复用ECDH对象
func (s *HttpSDK) SetECDHObject(object *crypto.EcdhObject) error {
	s.ecdhObject = object
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

// GetPublicKey 获取服务端ECC公钥并建立安全通信信道
// 这是ECC模式的核心初始化函数，实现密钥协商和强制身份验证
//
// 执行流程:
// 1. 生成客户端临时ECDH密钥对 (一次性使用)
// 2. 构造公钥交换请求 (包含客户端公钥)
// 3. 请求服务端/key接口获取服务端公钥
// 4. 验证服务端ECDSA签名 (强制要求)
// 5. 返回协商结果用于后续加密通信
//
// 安全特性:
// - ECDH密钥协商: 前向保密性 (PFS)
// - 临时密钥: 每次请求使用新的密钥对
// - 强制ECDSA验证: 验证服务端身份真实性
//
// 返回值:
//   - *crypto.EcdhObject: 客户端ECDH对象 (包含私钥)
//   - *node.PublicKey: 服务端公钥信息
//   - crypto.Cipher: ECDSA验证对象 (必须配置)
//   - error: 执行失败时的错误信息
//
// 注意:
// - 必须预先配置ECDSA对象，否则会抛出异常
// - ECDH对象包含敏感的私钥信息，使用后应立即清除
// - 此方法会在每次PostByECC调用时执行
func (s *HttpSDK) GetPublicKey() (*crypto.EcdhObject, *node.PublicKey, crypto.Cipher, error) {
	// 生成临时客户端公私钥
	ecdh := &crypto.EcdhObject{}
	if err := ecdh.CreateECDH(); err != nil {
		return nil, nil, nil, ex.Throw{Msg: "create ecdh object error: " + err.Error()}
	}
	// 获取ECDSA cipher，必须配置双向ECDSA签名
	if s.ecdsaObject == nil {
		return nil, nil, nil, ex.Throw{Msg: "ECDSA object not configured, bidirectional ECDSA signature is required"}
	}
	cipher, exists := s.ecdsaObject[s.ClientNo]
	if !exists || cipher == nil {
		return nil, nil, nil, ex.Throw{Msg: "ECDSA object not found for client, bidirectional ECDSA signature is required"}
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
	return ecdh, responseObject, cipher, nil
}

// PostByECC 通过ECC+AES-GCM+强制ECDSA模式发送POST请求
// 实现最高安全级别的API调用，支持匿名访问
//
// 安全协议栈 (从下到上):
// 1. TLS 1.2+ (网络层加密)
// 2. ECDH + AES-256-GCM (密钥协商 + 对称加密)
// 3. HMAC-SHA256 (数据完整性)
// 4. 强制双向ECDSA签名验证 (必须配置)
//
// 执行流程:
// 1. 获取服务端ECC公钥 (GetPublicKey)
// 2. 生成客户端ECDH密钥对
// 3. 计算共享密钥 (ECDH协商)
// 4. PBKDF2密钥派生
// 5. AES-GCM加密请求数据
// 6. 生成HMAC-SHA256签名
// 7. 添加ECDSA签名 (强制要求)
// 8. 发送HTTP请求
// 9. 验证响应HMAC签名
// 10. 验证响应ECDSA签名 (强制要求)
// 11. AES-GCM解密响应数据
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
// - 完美前向保密: ECDH协商的密钥不依赖长期密钥
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
	ecdh, public, cipher, err := s.GetPublicKey()
	if err != nil {
		return err
	}
	// 标记ECDH对象需要在函数结束时清理
	defer func() {
		if ecdh != nil {
			// 清理ECDH对象的敏感数据
			if prk, getErr := ecdh.GetPrivateKey(); getErr == "" && prk != nil {
				if ecdhPrk, ok := prk.(*ecdh2.PrivateKey); ok {
					DIC.ClearData(ecdhPrk.Bytes())
				}
			}
		}
	}()

	pub, err := ecc.LoadECDHPublicKeyFromBase64(public.Key)
	if err != nil {
		return ex.Throw{Msg: "load ECC public key failed"}
	}
	prk, _ := ecdh.GetPrivateKey()
	prkBytes := prk.(*ecdh2.PrivateKey).Bytes()
	sharedKey, err := ecc.GenSharedKeyECDH(prk.(*ecdh2.PrivateKey), pub)
	if err != nil {
		return ex.Throw{Msg: "ECC shared key failed"}
	}
	// 使用标准PBKDF2密钥派生（HMAC-SHA512，1024次迭代） 输出32字节密钥（SHA-512）
	sharedKey = pbkdf2.Key(sharedKey, utils.Base64Decode(public.Noc), 1024, 32, sha512.New)
	defer DIC.ClearData(prkBytes, sharedKey) // 同时清除ECDH私钥和派生密钥

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
		zlog.Debug(fmt.Sprintf("client key: %s", ecdh.PublicKeyBase64), 0)
		zlog.Debug(fmt.Sprintf("shared key: %s", utils.Base64Encode(sharedKey)), 0)
	}
	// 使用 AES-GCM 加密，Nonce 作为 AAD
	d, err := utils.AesGCMEncryptBase(utils.Str2Bytes(jsonBody.Data), sharedKey, node.AppendBodyMessage(path, "", jsonBody.Nonce, jsonBody.Time, jsonBody.Plan, jsonBody.User))
	if err != nil {
		return ex.Throw{Msg: "request data AES encrypt failed"}
	}
	jsonBody.Data = d
	jsonBody.Sign = utils.Base64Encode(node.SignBodyMessage(path, jsonBody.Data, jsonBody.Nonce, jsonBody.Time, jsonBody.Plan, jsonBody.User, sharedKey))
	// 添加ECDSA签名
	if err := s.addECDSASign(jsonBody); err != nil {
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

	key, err := node.CreatePublicKey(public.Key, utils.Base64Encode(prk.(*ecdh2.PrivateKey).PublicKey().Bytes()), s.ClientNo, cipher)
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

	// 验证ECDSA签名
	if err := s.verifyECDSASign(validSign, respData); err != nil {
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
	if utils.UnixSecond() > s.authToken.Expired-3600 {
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
	if s.authObject == nil { // 没授权对象则忽略
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
	responseObj := AuthToken{}
	if err := s.PostByECC(s.LoginPath, s.authObject, &responseObj); err != nil {
		return err
	}
	s.AuthToken(responseObj)
	return nil
}

// PostByAuth 通过JWT认证+强制ECDSA模式发送POST请求
// 适用于登录后的业务API调用，使用令牌进行身份认证
//
// 安全协议栈:
// 1. TLS 1.2+ (网络层加密)
// 2. JWT令牌认证 (身份验证)
// 3. AES-GCM或Base64 (数据加密，可选)
// 4. HMAC-SHA256 (数据完整性)
// 5. 强制双向ECDSA签名验证 (必须配置)
//
// 执行流程:
// 1. 检查认证状态，自动登录 (如果需要)
// 2. 获取认证令牌 (Token + Secret)
// 3. 根据encrypted参数选择加密方式
// 4. 生成HMAC-SHA256签名
// 5. 添加ECDSA签名 (如果配置)
// 6. 发送HTTP请求 (Authorization头)
// 7. 验证响应HMAC签名
// 8. 验证响应ECDSA签名 (如果配置)
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
// - 双向签名: 可配置ECDSA提供金融级安全
//
// 使用场景:
// - 用户登录后的业务API调用
// - 需要身份认证的接口
// - 中等安全要求的场景
//
// 性能特点:
// - 比ECC模式更快 (复用令牌，无需密钥协商)
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
	if len(s.authToken.Token) == 0 || len(s.authToken.Secret) == 0 {
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
	tokenSecret := utils.Base64Decode(s.authToken.Secret)
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

	// 验证响应时也需要解码token secret
	respTokenSecret := utils.Base64Decode(s.authToken.Secret)
	defer DIC.ClearData(respTokenSecret) // 清除响应验证时解码的token secret

	validSign := node.SignBodyMessage(path, respData.Data, respData.Nonce, respData.Time, respData.Plan, jsonBody.User, respTokenSecret)
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
		dec, err = utils.AesGCMDecryptBase(respData.Data, respTokenSecret[:32], node.AppendBodyMessage(path, "", respData.Nonce, respData.Time, respData.Plan, jsonBody.User))
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
// - 不支持ECDSA签名
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
