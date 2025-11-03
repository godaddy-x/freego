package node

import (
	"bytes"
	"net/http"
	"strings"
	"unsafe"

	"github.com/buaazp/fasthttprouter"
	"github.com/godaddy-x/freego/cache"
	"github.com/godaddy-x/freego/ex"
	"github.com/godaddy-x/freego/node/common"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/utils/crypto"
	"github.com/godaddy-x/freego/utils/jwt"
	"github.com/godaddy-x/freego/zlog"
	"github.com/valyala/fasthttp"
)

const (
	UTF8 = "UTF-8"

	ANDROID = "android"
	IOS     = "ios"
	WEB     = "web"

	TEXT_PLAIN       = "text/plain; charset=utf-8"
	APPLICATION_JSON = "application/json; charset=utf-8"
	NO_BODY          = "no_body"

	GET     = "GET"
	POST    = "POST"
	PUT     = "PUT"
	PATCH   = "PATCH"
	DELETE  = "DELETE"
	HEAD    = "HEAD"
	OPTIONS = "OPTIONS"

	Authorization = "Authorization"
	RandomCode    = "RandomCode"
)

var (
	MAX_BODY_LEN  = 200000 // 最大参数值长度
	MAX_TOKEN_LEN = 2048   // 最大Token值长度
	MAX_CODE_LEN  = 1024   // 最大Code值长度
)

// HookNode 结构体 - 32字节 (2个字段，8字节对齐，无填充)
// 排列优化：指针字段在前，slice字段在后
type HookNode struct {
	Context *Context        // 8字节 - 指针字段
	filters []*FilterObject // 24字节 (8+8+8) - slice字段
}

// Configs 结构体 - 88字节 (4个字段，8字节对齐，无填充)
// 排列优化：大字段优先，string放在最后利用16字节对齐
type Configs struct {
	jwtConfig     jwt.JwtConfig                // 56字节 - 大结构体
	routerConfigs map[string]*RouterConfig     // 8字节 - map字段
	langConfigs   map[string]map[string]string // 8字节 - map字段
	defaultLang   string                       // 16字节 (8+8) - string字段
}

// RouterConfig 结构体 - 4字节 (4个bool字段，1字节对齐)
// 排列优化：bool字段自然排列，无填充问题
type RouterConfig struct {
	Guest       bool // 游客模式,原始请求 false.否 true.是 - 1字节
	UseRSA      bool // 非登录状态使用RSA模式请求 false.否 true.是 - 1字节
	AesRequest  bool // 请求是否必须AES加密 false.否 true.是 - 1字节
	AesResponse bool // 响应是否必须AES加密 false.否 true.是 - 1字节
}

// HttpLog 结构体 - 64字节 (5个字段，8字节对齐，无填充)
// 排列优化：string字段在前，int64字段连续排列
type HttpLog struct {
	Method   string // 请求方法 - 16字节 (8+8)
	LogNo    string // 日志唯一标记 - 16字节 (8+8)
	CreateAt int64  // 日志创建时间 - 8字节
	UpdateAt int64  // 日志完成时间 - 8字节
	CostMill int64  // 业务耗时,毫秒 - 8字节
}

// Permission 结构体 - 50字节 (4个字段，8字节对齐，2字节填充)
// 排列优化：bool字段在前，slice字段连续排列
type Permission struct {
	MatchAll  bool // true.满足所有权限角色才放行 - 1字节
	NeedLogin bool // true.需要登录状态 - 1字节
	//Ready     bool    // true.已有权限配置 - 注释字段
	HasRole  []int64 // 拥有角色ID列表 - 24字节 (8+8+8)
	NeedRole []int64 // 所需角色ID列表 - 24字节 (8+8+8)
}

// System 结构体 - 41字节 (4个字段，8字节对齐，7字节填充)
// 排列优化：string字段在前，int64字段和bool字段在后
type System struct {
	Name          string // 系统名 - 16字节 (8+8)
	Version       string // 系统版本 - 16字节 (8+8)
	AcceptTimeout int64  // 超时主动断开客户端连接,秒 - 8字节
	enableECC     bool   // 1字节
}

// Context 结构体 - 232字节 (19个字段，8字节对齐，0字节填充)
// 排列优化：按字段大小和类型分组，指针和8字节类型在前，string字段居中，bool字段在后
type Context struct {
	// 8字节指针和函数字段组 (共17个字段，136字节)
	configs       *Configs                                               // 8字节 - 指针
	router        *fasthttprouter.Router                                 // 8字节 - 指针
	System        *System                                                // 8字节 - 指针
	RequestCtx    *fasthttp.RequestCtx                                   // 8字节 - 指针
	Subject       *jwt.Subject                                           // 8字节 - 指针
	JsonBody      *JsonBody                                              // 8字节 - 指针
	Response      *Response                                              // 8字节 - 指针
	filterChain   *filterChain                                           // 8字节 - 指针
	RouterConfig  *RouterConfig                                          // 8字节 - 指针
	RSA           []crypto.Cipher                                        // 8字节 - 0.主用公钥 1.备用公钥
	CacheAware    func(ds ...string) (cache.Cache, error)                // 8字节 - 函数指针
	roleRealm     func(ctx *Context, onlyRole bool) (*Permission, error) // 8字节 - 函数指针
	postHandle    PostHandle                                             // 8字节 - 函数指针
	errorHandle   ErrorHandle                                            // 8字节 - 函数指针
	Storage       map[string]interface{}                                 // 8字节 - map
	postCompleted bool                                                   // 1字节 - bool (会产生填充)

	// 16字节string字段组 (2个字段，32字节)
	Method string // 16字节 (8+8)
	Path   string // 16字节 (8+8)
}

// Response 结构体 - 80字节 (5个字段，8字节对齐，0字节填充)
// 排列优化：string和interface{}字段在前，int字段和复杂类型字段在后
type Response struct {
	Encoding          string       // 16字节 (8+8) - string字段
	ContentType       string       // 16字节 (8+8) - string字段
	ContentEntity     interface{}  // 16字节 (8+8) - interface{}字段
	StatusCode        int          // 8字节 - int字段
	ContentEntityByte bytes.Buffer // 24字节 - bytes.Buffer (包含内部字段)
}

func SetLengthCheck(bodyLen, tokenLen, codeLen int) {
	if bodyLen > 0 {
		MAX_BODY_LEN = bodyLen // 最大参数值长度
	}
	if tokenLen > 0 {
		MAX_TOKEN_LEN = tokenLen // 最大Token值长度
	}
	if codeLen > 0 {
		MAX_CODE_LEN = codeLen // 最大Code值长度
	}
}

// SetLocalSecret 增加本地secret定义，最少24个字符长度
func SetLocalSecret(key string) {
	utils.SetLocalSecretKey(key)
}

func (self *JsonBody) ParseData(dst interface{}) error {
	return utils.JsonUnmarshal(utils.Str2Bytes(self.Data), dst)
}

func (self *JsonBody) RawData() []byte {
	return utils.Str2Bytes(self.Data)
}

func (self *Context) GetTokenSecret() string {
	return jwt.GetTokenSecret(utils.Bytes2Str(self.GetRawTokenBytes()), self.configs.jwtConfig.TokenKey)
}

func (self *Context) GetHmac256Sign(d, n string, t, p int64, key string) string {
	if len(key) > 0 {
		return utils.HMAC_SHA256(utils.AddStr(self.Path, d, n, t, p), key, true)
	}
	return utils.HMAC_SHA256(utils.AddStr(self.Path, d, n, t, p), self.GetTokenSecret(), true)
}

func (self *Context) AddStorage(k string, v interface{}) {
	if self.Storage == nil {
		self.Storage = map[string]interface{}{}
	}
	if len(k) == 0 || v == nil {
		return
	}
	self.Storage[k] = v
}

func (self *Context) GetStorage(k string) interface{} {
	if self.Storage == nil {
		return nil
	}
	if len(k) == 0 {
		return nil
	}
	v, b := self.Storage[k]
	if !b || v == nil {
		return nil
	}
	return v
}

func (self *Context) DelStorage(k string) {
	if self.Storage == nil {
		return
	}
	delete(self.Storage, k)
}

func (self *Context) Authenticated() bool {
	if self.Subject == nil || !self.Subject.CheckReady() {
		return false
	}
	return true
}

func (self *Context) Parser(dst interface{}) error {
	if self.JsonBody == nil || len(self.JsonBody.Data) == 0 {
		return nil
	}
	if err := self.JsonBody.ParseData(dst); err != nil {
		msg := "JSON parameter parsing failed"
		// 安全修复：避免记录敏感的JsonBody数据，只记录基本信息
		zlog.Error(msg, 0, zlog.String("path", self.Path), zlog.String("device", self.ClientDevice()), zlog.Int64("time", self.JsonBody.Time), zlog.Int64("plan", self.JsonBody.Plan), zlog.AddError(err))
		return ex.Throw{Msg: msg, Err: err}
	}
	// TODO 备注: 已有会话状态时,指针填充context值,不能随意修改指针偏移值
	identify := &common.Identify{}
	if self.Authenticated() {
		identify.ID = self.Subject.Payload.Sub
	}
	context := common.Context{
		Identify:   identify,
		Path:       self.Path,
		System:     &common.System{Name: self.System.Name, Version: self.System.Version},
		CacheAware: self.CacheAware,
		RSA:        self.RSA,
	}
	src := utils.GetPtr(dst, 0)
	req := common.GetBasePtrReq(src)
	base := common.BaseReq{
		Context: context,
		Offset:  req.Offset,
		Limit:   req.Limit,
		PrevID:  req.PrevID,
		LastID:  req.LastID,
		CountQ:  req.CountQ,
		Cmd:     req.Cmd,
	}
	*((*common.BaseReq)(unsafe.Pointer(src))) = base
	return nil
}

func (self *Context) ClientDevice() string {
	agent := utils.Bytes2Str(self.RequestCtx.Request.Header.Peek("User-Agent"))
	if utils.HasStr(agent, "Android") || utils.HasStr(agent, "Adr") {
		return ANDROID
	} else if utils.HasStr(agent, "iPad") || utils.HasStr(agent, "iPhone") || utils.HasStr(agent, "Mac") {
		return IOS
	} else {
		return WEB
	}
}

func (self *Context) ClientLanguage() string {
	return utils.Bytes2Str(self.RequestCtx.Request.Header.Peek("Language"))
}

func (self *Context) readParams() error {
	if self.Method != POST {
		return nil
	}
	body := self.RequestCtx.PostBody()
	// 原始请求模式
	if self.RouterConfig.Guest {
		if len(body) == 0 {
			return nil
		}
		// 复制副本防止底层引用
		self.JsonBody.Data = string(body)
		return nil
	}
	// 安全请求模式
	auth := self.GetRawTokenBytes()
	if len(auth) > MAX_TOKEN_LEN {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "authorization parameters length is too long"}
	}
	if body == nil || len(body) == 0 {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "body parameters is nil"}
	}
	if len(body) > (MAX_BODY_LEN) {
		return ex.Throw{Code: http.StatusLengthRequired, Msg: "body parameters length is too long"}
	}

	if err := utils.JsonUnmarshal(body, self.JsonBody); err != nil {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "body parameters parse error"}
	}

	if err := self.validJsonBody(); err != nil { // TODO important
		return err
	}
	return nil
}

func (self *Context) validReplayAttack(sign string) error {
	if self.CacheAware == nil {
		return nil
	}
	c, err := self.CacheAware()
	if err != nil {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "cache instance invalid"}
	}
	b, err := c.Exists(sign)
	if err != nil || b {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "replay attack invalid"}
	}
	if err := c.Put(sign, 1, 600); err != nil {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "cache replay attack value error"}
	}
	return nil
}

func (self *Context) GetRawTokenBytes() []byte {
	return self.RequestCtx.Request.Header.Peek(Authorization)
}

func (self *Context) validJsonBody() error {
	if self.JsonBody == nil {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request json body is nil"}
	}
	body := self.JsonBody
	d := body.Data
	if len(d) == 0 {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request data is nil"}
	}
	if !utils.CheckInt64(body.Plan, 0, 1, 2) {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request plan invalid"}
	}
	if !utils.CheckLen(body.Nonce, 8, 32) {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request nonce invalid"}
	}
	if body.Time <= 0 {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request time must be > 0"}
	}
	if utils.MathAbs(utils.UnixSecond()-body.Time) > jwt.FIVE_MINUTES { // 判断绝对时间差超过5分钟
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request time invalid"}
	}
	if self.RouterConfig.AesRequest && body.Plan != 1 {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request parameters must use AES encryption"}
	}
	if !utils.CheckStrLen(body.Sign, 32, 64) {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request signature length invalid"}
	}

	if utils.CheckInt64(body.Plan, 0, 1) {
		if len(self.GetRawTokenBytes()) == 0 {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "request header token is nil"}
		}
	}

	var key string
	var anonymous bool // true.匿名状态

	// Plan 2是匿名状态（使用ECC加密，需要特殊处理）
	if body.Plan == 2 {
		anonymous = true
		if !self.RouterConfig.UseRSA {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "request parameters must use RSA encryption"}
		}

		// Plan 2需要解密RandomCode获取clientSecretKey进行签名验证
		codeBs := self.RequestCtx.Request.Header.Peek(RandomCode)
		if len(codeBs) > MAX_CODE_LEN {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "client random code invalid"}
		}
		randomCode := utils.Bytes2Str(codeBs)
		if len(randomCode) == 0 {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "client random code invalid"}
		}
		if len(self.RSA) == 0 {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "no RSA private keys configured for decryption"}
		}

		var code []byte
		var lastErr error
		for _, v := range self.RSA { // 尝试遍历存在的私钥进行解密
			decrypted, err := v.Decrypt(randomCode)
			if err != nil {
				lastErr = err
				// 记录解密失败，但继续尝试下一个私钥
				_, pub := v.GetPublicKey()
				zlog.Warn("server private-key decrypt error", 0, zlog.String("publicKey", pub))
				continue
			}
			if len(decrypted) > 0 { // 解密成功
				code = decrypted
				break
			}
			// 解密成功但结果为空，记录错误但继续尝试
			lastErr = ex.Throw{Code: http.StatusBadRequest, Msg: "server private-key decrypt returned empty data"}
		}

		if len(code) == 0 { // 所有私钥都无法解密成功
			if lastErr != nil {
				return ex.Throw{Code: http.StatusBadRequest, Msg: "all RSA private keys failed to decrypt client random code", Err: lastErr}
			}
			return ex.Throw{Code: http.StatusBadRequest, Msg: "all RSA private keys failed to decrypt client random code"}
		}
		key = utils.Base64Encode(code) // 使用clientSecretKey验证签名
	}

	// 签名验证：Plan 0/1使用token secret（key为空时会自动获取），Plan 2使用clientSecretKey
	if self.GetHmac256Sign(d, body.Nonce, body.Time, body.Plan, key) != body.Sign {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request signature invalid"}
	}
	if err := self.validReplayAttack(body.Sign); err != nil {
		return err
	}
	var rawData []byte
	var err error
	if body.Plan == 0 && !anonymous { // 登录状态 P0 Base64
		rawData = utils.Base64DecodeWithPool(d)
		if len(rawData) == 0 {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "parameter Base64 parsing failed"}
		}
	} else if body.Plan == 1 && !anonymous { // 登录状态 P1 AES
		rawData, err = utils.AesGCMDecryptWithAAD(d, self.GetTokenSecret(), utils.AddStr(self.JsonBody.Time, self.JsonBody.Nonce, self.JsonBody.Plan, self.Path))
		if err != nil {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "AES failed to parse data", Err: err}
		}
	} else if body.Plan == 2 && self.RouterConfig.UseRSA && anonymous { // 非登录状态 P2 ECC+AES
		rawData, err = utils.AesGCMDecryptWithAAD(d, key, utils.AddStr(self.JsonBody.Time, self.JsonBody.Nonce, self.JsonBody.Plan, self.Path))
		if err != nil {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "AES failed to parse data", Err: err}
		}
		self.AddStorage(RandomCode, key)
	} else {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request parameters plan invalid"}
	}
	self.JsonBody.Data = utils.Bytes2Str(rawData)
	return nil
}

func (self *Context) GetHeader(key string) string {
	return utils.Bytes2Str(self.RequestCtx.Request.Header.Peek(key))
}

func (self *Context) GetPostBody() string {
	return utils.Bytes2Str(self.RequestCtx.Request.Body())
}

func (self *Context) GetJwtConfig() jwt.JwtConfig {
	return self.configs.jwtConfig
}

func (self *Context) Handle() error {
	if self.postCompleted {
		return nil
	}
	self.postCompleted = true
	return self.postHandle(self)
}

func (self *Context) RemoteIP() string {
	clientIP := string(self.RequestCtx.Request.Header.Peek("X-Forwarded-For"))
	if index := strings.IndexByte(clientIP, ','); index >= 0 {
		clientIP = clientIP[0:index]
	}
	clientIP = strings.TrimSpace(clientIP)
	if len(clientIP) > 0 {
		return clientIP
	}
	clientIP = strings.TrimSpace(utils.Bytes2Str(self.RequestCtx.Request.Header.Peek("X-Real-Ip")))
	if len(clientIP) > 0 {
		return clientIP
	}
	return self.RequestCtx.RemoteIP().String()
}

func (self *Context) reset(ctx *Context, handle PostHandle, request *fasthttp.RequestCtx, fs []*FilterObject) {
	// 全局函数配置（只在首次设置）
	if self.CacheAware == nil {
		self.CacheAware = ctx.CacheAware
	}
	// RSA是全局配置，通常只在系统启动时设置一次
	// 只在RSA未初始化时才从模板拷贝，避免不必要的内存分配
	if len(self.RSA) == 0 && len(ctx.RSA) > 0 {
		// 深拷贝全局RSA配置
		self.RSA = make([]crypto.Cipher, len(ctx.RSA))
		copy(self.RSA, ctx.RSA)
	}
	// 如果RSA已有值（全局配置），保持不变
	if self.roleRealm == nil {
		self.roleRealm = ctx.roleRealm
	}
	if self.errorHandle == nil {
		self.errorHandle = ctx.errorHandle
	}

	// System配置（直接赋值，避免对象池浪费）
	if ctx.System != nil {
		*self.System = *ctx.System // 复制内容而不是重新赋值指针
	}

	// 请求相关状态重置
	self.postHandle = handle
	self.RequestCtx = request
	self.Method = utils.Bytes2Str(self.RequestCtx.Method())
	self.Path = utils.Bytes2Str(self.RequestCtx.Path())
	self.RouterConfig = self.configs.routerConfigs[self.Path]
	self.postCompleted = false

	// 过滤器链重置（优化：减少条件检查）
	self.filterChain.pos = 0
	// 注意：filters已在对象池中预设，无需重置

	// 重置请求处理对象
	self.resetJsonBody()
	self.resetResponse()
	self.resetSubject()
	self.resetTokenStorage()
}

func (self *Context) resetTokenStorage() {
	// 优化：延迟初始化Storage map
	if self.Storage == nil {
		self.Storage = make(map[string]interface{}, 4) // 预分配小容量
		return
	}

	// 优化：只有在确实使用过Storage时才清空
	if len(self.Storage) > 0 {
		// 高效清空：重新分配而不是逐个删除
		self.Storage = make(map[string]interface{}, 4)
	}
}

func (self *Context) resetJsonBody() {
	// 对象池已预创建，无需nil检查
	self.JsonBody.Data = ""
	self.JsonBody.Nonce = ""
	self.JsonBody.Sign = ""
	self.JsonBody.Time = 0
	self.JsonBody.Plan = 0
}

func (self *Context) resetResponse() {
	// 对象池已预创建并初始化，无需nil检查和初始化检查
	self.Response.ContentEntity = nil
	self.Response.StatusCode = 0
	// 优化：只有在确实有内容时才Reset
	if self.Response.ContentEntityByte.Len() > 0 {
		self.Response.ContentEntityByte.Reset()
	}
}

func (self *Context) resetSubject() {
	// 对象池已预创建，无需nil检查

	// 优化：批量重置Payload字段
	payload := self.Subject.Payload
	payload.Sub = ""
	payload.Iss = ""
	payload.Aud = ""
	payload.Iat = 0
	payload.Exp = 0
	payload.Dev = ""
	payload.Jti = ""
	payload.Ext = ""
}

func (self *Context) Json(data interface{}) error {
	self.Response.ContentType = APPLICATION_JSON
	if data == nil {
		self.Response.ContentEntity = emptyMap
	} else {
		self.Response.ContentEntity = data
	}
	return nil
}

func (self *Context) Text(data string) error {
	self.Response.ContentType = TEXT_PLAIN
	self.Response.ContentEntity = data
	return nil
}

func (self *Context) Bytes(data []byte) error {
	self.Response.ContentType = TEXT_PLAIN
	self.Response.ContentEntity = data
	return nil
}

func (self *Context) NoBody() error {
	self.Response.ContentType = NO_BODY
	self.Response.ContentEntity = nil
	return nil
}
