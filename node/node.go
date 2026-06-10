package node

import (
	"bytes"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/sha256"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"unsafe"

	rate "github.com/godaddy-x/freego/cache/limiter"

	"github.com/buaazp/fasthttprouter"
	ecc "github.com/godaddy-x/eccrypto"
	"github.com/godaddy-x/freego/cache"
	DIC "github.com/godaddy-x/freego/common"
	"github.com/godaddy-x/freego/ex"
	"github.com/godaddy-x/freego/node/common"
	"github.com/godaddy-x/freego/utils"
	fgocrypto "github.com/godaddy-x/freego/utils/crypto"
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
	SharedKey     = "SharedKey"
	Cipher        = "Cipher"

	// HKDF info：历史常量名含 ecdh，勿改字符串以免与已部署客户端派生密钥不一致
	SharedInfo = "freego-ecdh-aes-gcm"
)

var (
	MAX_BODY_LEN  = 200000 // 最大参数值长度
	MAX_TOKEN_LEN = 2048   // JWT 等常规 Authorization 上限；Plan2 见 maxAuthorizationHeaderLen
	MAX_CODE_LEN  = 1024   // 最大Code值长度
)

// maxAuthorizationHeaderLen Plan2 为 base64(PublicKey JSON)，须容纳 ML-KEM + ML-DSA 外层签名。
func maxAuthorizationHeaderLen() int {
	n := MAX_TOKEN_LEN
	if plan2 := fgocrypto.MaxPlan2AuthorizationB64Len(); plan2 > n {
		n = plan2
	}
	return n
}

// HookNode 结构体 - 32字节 (2个字段，8字节对齐，无填充)
// 排列优化：指针字段在前，slice字段在后
//
// filters：须在进程初始化阶段完成 AddFilter；StartServer 内会经 createFilterChain 整体替换为排序后的链。
// 约定：监听并处理请求期间不得再 AddFilter（无并发 append 与 DoFilter 遍历同一切片的安全保证）。
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
	UsePlan2    bool // Plan2 匿名路由（ML-KEM+ML-DSA） false.否 true.是 - 1字节
	KeyRoute    bool // WebSocket plan2 key 路由（仅允许匿名 plan0）
	LoginRoute  bool // WebSocket plan2 login 路由（仅允许 plan2）
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

// Permission 结构体 - 48字节 (4个字段，8字节对齐，0字节填充)
// 排列优化：bool字段在前，slice字段连续排列，利用8字节对齐
type Permission struct {
	MatchAll  bool    // true.满足所有权限角色才放行 - 1字节
	NeedLogin bool    // true.需要登录状态 - 1字节
	HasRole   []int64 // 拥有角色ID列表 - 24字节 (8+8+8)
	NeedRole  []int64 // 所需角色ID列表 - 24字节 (8+8+8)
}

// System 结构体 - 40字节 (4个字段，8字节对齐，0字节填充)
// 排列优化：string字段在前，bool字段居中，int64字段在后，利用8字节对齐
type System struct {
	Name          string // 系统名 - 16字节 (8+8)
	Version       string // 系统版本 - 16字节 (8+8)
	enableECC     bool   // 1字节
	AcceptTimeout int64  // 超时主动断开客户端连接,秒 - 8字节
}

// Context 结构体 - 184字节 (19个字段，8字节对齐，0字节填充)
// 排列优化：按字段大小和类型分组，16字节string字段在前，8字节指针/函数/map字段居中，bool字段在后
type Context struct {
	// 16字节string字段组 (2个字段，32字节)
	Method string // 16字节 (8+8) - string字段
	Path   string // 16字节 (8+8) - string字段

	// 8字节指针字段组 (9个字段，72字节)
	configs      *Configs               // 8字节 - 指针
	router       *fasthttprouter.Router // 8字节 - 指针
	System       *System                // 8字节 - 指针
	RequestCtx   *fasthttp.RequestCtx   // 8字节 - 指针
	Subject      *jwt.Subject           // 8字节 - 指针
	JsonBody     *JsonBody              // 8字节 - 指针
	Response     *Response              // 8字节 - 指针
	filterChain  *filterChain           // 8字节 - 指针
	RouterConfig *RouterConfig          // 8字节 - 指针

	// 8字节函数指针字段组 (5个字段，40字节)
	RedisCacheAware func(ds ...string) (cache.Cache, error) // 8字节 - 函数指针
	LocalCacheAware func(ds ...string) (cache.Cache, error) // 8字节 - 函数指针
	roleRealm       func(ctx *Context) (*Permission, error) // 8字节 - 函数指针
	postHandle      PostHandle                              // 8字节 - 函数指针
	errorHandle     ErrorHandle                             // 8字节 - 函数指针

	// 8字节其他字段组 (3个字段)
	PQCipher map[int64]crypto.Cipher // Plan2：ML-DSA-87 外层签
	Storage  map[string]interface{}

	// bool字段 (1字节，会产生填充)
	postCompleted bool // 1字节 - bool
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

func HKDFKey(shared []byte, nonce string) ([]byte, error) {
	return hkdf.Key(sha256.New, shared, utils.Base64Decode(nonce), SharedInfo, 32)
}

// GetUserIDString 获取用户ID string类型
func (self *Context) GetUserIDString() string {
	if self.Subject == nil || self.Subject.Payload == nil || len(self.Subject.Payload.Sub) == 0 {
		return ""
	}
	return self.Subject.Payload.Sub
}

// GetUserIDInt64 获取用户ID int64类型, 零值认为是无用户信息
func (self *Context) GetUserIDInt64() int64 {
	if self.Subject == nil || self.Subject.Payload == nil || len(self.Subject.Payload.Sub) == 0 {
		return 0
	}
	r, err := utils.StrToInt64(self.Subject.Payload.Sub)
	if err != nil {
		zlog.Error("GetUserIDInt64 error", 0, zlog.String("sub", self.Subject.Payload.Sub), zlog.AddError(err))
		return 0
	}
	return r
}

func appendBodyMessageTo(dst []byte, path, data, nonce string, t, plan, usr int64) []byte {
	const int64MaxLen = 20
	sep := DIC.SEP
	est := len(path) + len(data) + len(nonce) + len(sep)*5 + int64MaxLen*3
	if cap(dst)-len(dst) < est {
		nd := make([]byte, len(dst), len(dst)+est)
		copy(nd, dst)
		dst = nd
	}
	dst = append(dst, path...)
	dst = append(dst, sep...)
	dst = append(dst, data...)
	dst = append(dst, sep...)
	dst = append(dst, nonce...)
	dst = append(dst, sep...)
	dst = strconv.AppendInt(dst, t, 10)
	dst = append(dst, sep...)
	dst = strconv.AppendInt(dst, plan, 10)
	dst = append(dst, sep...)
	dst = strconv.AppendInt(dst, usr, 10)
	return dst
}

func AppendBodyMessage(path, data, nonce string, time, plan, usr int64) []byte {
	return appendBodyMessageTo(nil, path, data, nonce, time, plan, usr)
}

// DigestBodyMessage returns SHA-256 digest of canonical body message.
func DigestBodyMessage(path, data, nonce string, time, plan, usr int64) []byte {
	return utils.SHA256_BASE(AppendBodyMessage(path, data, nonce, time, plan, usr))
}

// SignAndDigestBodyMessage computes HMAC and SHA-256 digest from one canonical message assembly.
func SignAndDigestBodyMessage(path, data, nonce string, time, plan, usr int64, key []byte) ([]byte, []byte) {
	msg := AppendBodyMessage(path, data, nonce, time, plan, usr)
	return utils.HMAC_SHA256_BASE(msg, key), utils.SHA256_BASE(msg)
}

func SignBodyMessage(path, data, nonce string, time, plan, usr int64, key []byte) []byte {
	h := hmac.New(sha256.New, key)
	sep := DIC.SEP
	_, _ = io.WriteString(h, path)
	_, _ = io.WriteString(h, sep)
	_, _ = io.WriteString(h, data)
	_, _ = io.WriteString(h, sep)
	_, _ = io.WriteString(h, nonce)
	_, _ = io.WriteString(h, sep)
	var ibuf [20]byte
	b := strconv.AppendInt(ibuf[:0], time, 10)
	_, _ = h.Write(b)
	_, _ = io.WriteString(h, sep)
	b = strconv.AppendInt(ibuf[:0], plan, 10)
	_, _ = h.Write(b)
	_, _ = io.WriteString(h, sep)
	b = strconv.AppendInt(ibuf[:0], usr, 10)
	_, _ = h.Write(b)
	return h.Sum(nil)
}

func (self *HttpNode) SetLengthCheck(bodyLen, tokenLen, codeLen int) {
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

// SetDynamicSecretKey 增加本地secret定义，最少24个字符长度
func (self *HttpNode) SetDynamicSecretKey(key string) {
	utils.SetDynamicSecretKey(key)
}

func (self *JsonBody) ParseData(dst interface{}) error {
	return utils.JsonUnmarshal(utils.Str2Bytes(self.Data), dst)
}

func (self *JsonBody) RawData() []byte {
	return utils.Str2Bytes(self.Data)
}

func (self *Context) GetTokenSecret() []byte {
	return self.Subject.GetTokenSecret(utils.Bytes2Str(self.GetRawTokenBytes()), self.configs.jwtConfig.TokenKey)
}

func (self *Context) GetHmac256Sign(d, n string, t, p, u int64, key []byte) []byte {
	return SignBodyMessage(self.Path, d, n, t, p, u, key)
}

// CheckOuterSign 校验外层数字签名（经 cipher.Verify；ML-DSA-87）。
func (self *Context) CheckOuterSign(cipher crypto.Cipher, msg, sign []byte) (crypto.Cipher, error) {
	if cipher == nil {
		return nil, ex.Throw{Code: http.StatusBadRequest, Msg: "request cipher invalid"}
	}
	if err := cipher.Verify(msg, sign); err != nil {
		return nil, ex.Throw{Code: http.StatusBadRequest, Msg: "request signature invalid"}
	}
	return cipher, nil
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
		Identify:        identify,
		Path:            self.Path,
		System:          &common.System{Name: self.System.Name, Version: self.System.Version},
		RedisCacheAware: self.RedisCacheAware,
		LocalCacheAware: self.LocalCacheAware,
		Cipher:          self.PQCipher,
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
	if len(auth) > maxAuthorizationHeaderLen() {
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
	// 重放攻击检测应优先使用全局缓存（Redis），确保跨实例有效性
	c, err := self.GetCacheObject()
	if err != nil {
		return err
	}

	hex := utils.FNV1a64(sign)
	b, err := c.Exists(hex)
	if err != nil {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "cache operation failed", Err: err}
	}
	if b {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "replay attack detected"}
	}

	if err := c.Put(hex, 1, 300); err != nil {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "cache replay attack record failed", Err: err}
	}
	return nil
}

func (self *Context) GetRawTokenBytes() []byte {
	return self.RequestCtx.Request.Header.Peek(Authorization)
}

func (self *Context) GetCacheObject() (cache.Cache, error) {
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

// 内网IP段：需过滤的私有IP范围
var privateIPBlocks = []*net.IPNet{
	{IP: net.ParseIP("10.0.0.0"), Mask: net.CIDRMask(8, 32)},
	{IP: net.ParseIP("172.16.0.0"), Mask: net.CIDRMask(12, 32)},
	{IP: net.ParseIP("192.168.0.0"), Mask: net.CIDRMask(16, 32)},
	{IP: net.ParseIP("127.0.0.0"), Mask: net.CIDRMask(8, 32)},
	{IP: net.ParseIP("0.0.0.0"), Mask: net.CIDRMask(8, 32)},
}

// isPrivateIP 判断IP是否为内网IP
func isPrivateIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}
	for _, block := range privateIPBlocks {
		if block.Contains(ip) {
			return true
		}
	}
	return false
}

// resolveRemoteIP 从代理头与直连地址解析真实客户端 IP（过滤内网 IP）。
// 优先级：CF-Connecting-IP（Cloudflare 回源）> X-Forwarded-For > X-Real-Ip > fallback。
func resolveRemoteIP(cfConnectingIP, xffHeader, realIPHeader, fallback string) string {
	cfConnectingIP = strings.TrimSpace(cfConnectingIP)
	if cfConnectingIP != "" {
		ip := net.ParseIP(cfConnectingIP)
		if ip != nil && !isPrivateIP(ip) {
			return ip.String()
		}
	}

	xffHeader = strings.TrimSpace(xffHeader)
	if xffHeader != "" {
		ips := strings.Split(xffHeader, ",")
		for i := len(ips) - 1; i >= 0; i-- {
			ipStr := strings.TrimSpace(ips[i])
			if ipStr == "" {
				continue
			}
			ip := net.ParseIP(ipStr)
			if ip != nil && !isPrivateIP(ip) {
				return ip.String()
			}
		}
	}

	realIPHeader = strings.TrimSpace(realIPHeader)
	if realIPHeader != "" {
		ip := net.ParseIP(realIPHeader)
		if ip != nil && !isPrivateIP(ip) {
			return ip.String()
		}
	}

	fallback = strings.TrimSpace(fallback)
	if fallback != "" {
		ip := net.ParseIP(fallback)
		if ip != nil && !isPrivateIP(ip) {
			return fallback
		}
	}
	return ""
}

// RemoteIPFromRequest 从标准 http.Request 安全获取真实客户端 IP（与 Context.RemoteIP 规则一致）。
func RemoteIPFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	fallback := r.RemoteAddr
	if host, _, err := net.SplitHostPort(fallback); err == nil {
		fallback = host
	}
	return resolveRemoteIP(
		r.Header.Get("CF-Connecting-IP"),
		r.Header.Get("X-Forwarded-For"),
		r.Header.Get("X-Real-Ip"),
		fallback,
	)
}

// RemoteIP 安全获取真实客户端IP（防伪造、过滤内网IP）
func (self *Context) RemoteIP() string {
	if self == nil || self.RequestCtx == nil {
		return ""
	}
	return resolveRemoteIP(
		utils.Bytes2Str(self.RequestCtx.Request.Header.Peek("CF-Connecting-IP")),
		utils.Bytes2Str(self.RequestCtx.Request.Header.Peek("X-Forwarded-For")),
		utils.Bytes2Str(self.RequestCtx.Request.Header.Peek("X-Real-Ip")),
		self.RequestCtx.RemoteIP().String(),
	)
}

func (self *Context) reset(ctx *Context, handle PostHandle, request *fasthttp.RequestCtx, fs []*FilterObject) {
	// 全局函数配置（只在首次设置）
	if self.RedisCacheAware == nil {
		self.RedisCacheAware = ctx.RedisCacheAware
	}
	if self.LocalCacheAware == nil {
		self.LocalCacheAware = ctx.LocalCacheAware
	}
	if len(self.PQCipher) == 0 && len(ctx.PQCipher) > 0 {
		self.PQCipher = make(map[int64]crypto.Cipher, len(ctx.PQCipher))
		for k, v := range ctx.PQCipher {
			self.PQCipher[k] = v
		}
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

	// 过滤器链：每条请求绑定当前 HttpNode.filters（与 StartServer 中 createFilterChain 结果一致，无每请求分配）
	if self.filterChain == nil {
		self.filterChain = &filterChain{}
	}
	self.filterChain.filters = fs
	self.filterChain.pos = 0

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

func CreatePublicKey(key, tag string, usr int64, cipher crypto.Cipher) (*PublicKey, error) {
	requestObject := &PublicKey{}
	requestObject.Key = key
	requestObject.Tag = tag
	requestObject.Noc = utils.Base64Encode(utils.GetRandomSecure(32))
	requestObject.Exp = utils.UnixSecond()
	requestObject.Usr = usr
	sig, err := cipher.Sign(utils.Str2Bytes(utils.AddStr(requestObject.Key, DIC.SEP, requestObject.Tag, DIC.SEP, requestObject.Noc, DIC.SEP, requestObject.Exp, DIC.SEP, requestObject.Usr)))
	if err != nil {
		return nil, ex.Throw{Msg: "outer sign message error: " + err.Error()}
	}
	requestObject.Sig = utils.Base64Encode(sig)
	return requestObject, nil
}

func CheckPublicKey(c cache.Cache, requestObject *PublicKey, cipher crypto.Cipher) error {
	if cipher == nil {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request cipher invalid"}
	}
	if len(requestObject.Key) < 32 {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request key invalid"}
	}
	if len(requestObject.Tag) < 32 {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request tag invalid"}
	}
	if !fgocrypto.CheckOuterSignatureB64Valid(requestObject.Sig) {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request sig invalid"}
	}
	if len(requestObject.Noc) < 32 {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request noc invalid"}
	}
	if requestObject.Usr < 0 {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request usr invalid"}
	}
	if utils.MathAbs(utils.UnixSecond()-requestObject.Exp) > jwt.FIVE_MINUTES { // 判断绝对时间差超过5分钟
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request exp invalid"}
	}
	key := utils.FNV1a64(requestObject.Noc)
	if c != nil {
		if ok, err := c.Exists(key); err != nil {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "noc check failed", Err: err}
		} else if ok {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "request noc duplicated"}
		}
	}
	// 外层签名验证（Plan2 PublicKey 握手：ML-DSA-87）
	if err := cipher.Verify(utils.Str2Bytes(utils.AddStr(requestObject.Key, DIC.SEP, requestObject.Tag, DIC.SEP, requestObject.Noc, DIC.SEP, requestObject.Exp, DIC.SEP, requestObject.Usr)), utils.Base64Decode(requestObject.Sig)); err != nil {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request verify sig invalid"}
	}
	if c != nil {
		if err := c.Put(key, 1, 300); err != nil {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "request noc cache error", Err: err}
		}
	}
	return nil
}

var (
	limiter = rate.NewRateLimiter(rate.Option{
		// /key 入口限流：保留突发吸收能力，同时避免无限放大攻击流量
		Limit:       5000.0, // 每秒5000个令牌
		Bucket:      15000,  // 桶容量15000
		Expire:      60000,  // 60秒过期
		Distributed: false,
	})
)

func (self *Context) CreatePublicKey() (*PublicKey, error) {
	// 检查请求的对象是否有效
	if self.JsonBody == nil || len(self.JsonBody.Data) == 0 {
		return nil, ex.Throw{Code: http.StatusBadRequest, Msg: "request data is nil"}
	}

	if len(self.JsonBody.Data) > fgocrypto.MaxPublicKeyJSONLen() {
		return nil, ex.Throw{Code: http.StatusBadRequest, Msg: "request data too long"}
	}

	checkObject := &PublicKey{}
	if err := utils.JsonUnmarshal(utils.Str2Bytes(self.JsonBody.Data), checkObject); err != nil {
		return nil, ex.Throw{Code: http.StatusBadRequest, Msg: "request data parse error", Err: err}
	}

	if checkObject.Usr < 0 {
		return nil, ex.Throw{Code: http.StatusBadRequest, Msg: "request usr invalid"}
	}

	cipher, exists := self.PQCipher[checkObject.Usr]
	if !exists {
		zlog.Error("CreatePublicKey usr error", 0, zlog.String("ip", self.RemoteIP()), zlog.Int64("usr", checkObject.Usr))
		return nil, ex.Throw{Code: http.StatusBadRequest, Msg: "plan2 cipher not found for user"}
	}

	// 增加限流器控制USR访问量
	if !limiter.Allow(utils.AddStr("Limiter:CreatePublicKey:", checkObject.Usr)) {
		zlog.Error("CreatePublicKey usr frequent error", 0, zlog.String("ip", self.RemoteIP()), zlog.Int64("usr", checkObject.Usr))
		return nil, ex.Throw{Code: http.StatusBadRequest, Msg: "request too frequent"}
	}

	c, err := self.GetCacheObject()
	if err != nil {
		return nil, ex.Throw{Code: http.StatusBadRequest, Msg: "cache not found for user", Err: err}
	}
	if err = CheckPublicKey(c, checkObject, cipher); err != nil {
		return nil, err
	}

	dk, err := ecc.CreateMLKEM1024()
	if err != nil {
		return nil, ex.Throw{Code: http.StatusBadRequest, Msg: "ML-KEM key generation failed", Err: err}
	}
	serverEkB64 := utils.Base64Encode(ecc.GetMLKEM1024EncapsulationKeyBytes(dk.EncapsulationKey()))
	dkB64 := ecc.MLKEM1024DecapsulationKeyToBase64(dk)

	requestObject, err := CreatePublicKey(serverEkB64, utils.Base64Encode(utils.GetRandomSecure(32)), checkObject.Usr, cipher)
	if err != nil {
		return nil, err
	}

	if err := c.Put(utils.FNV1a64(utils.AddStr(requestObject.Key, ":", requestObject.Usr)), &PrivateKey{Key: dkB64, Noc: requestObject.Noc}, 180); err != nil {
		return nil, ex.Throw{Code: http.StatusBadRequest, Msg: "prk cache setting error", Err: err}
	}

	return requestObject, nil
}
