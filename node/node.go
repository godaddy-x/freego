package node

import (
	"bytes"
	"github.com/buaazp/fasthttprouter"
	"github.com/godaddy-x/freego/cache"
	"github.com/godaddy-x/freego/ex"
	"github.com/godaddy-x/freego/node/common"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/utils/crypto"
	"github.com/godaddy-x/freego/utils/jwt"
	"github.com/godaddy-x/freego/zlog"
	"github.com/valyala/fasthttp"
	"net/http"
	"strings"
	"unsafe"
)

const (
	UTF8 = "UTF-8"

	ANDROID = "android"
	IOS     = "ios"
	WEB     = "web"

	TEXT_PLAIN       = "text/plain; charset=utf-8"
	APPLICATION_JSON = "application/json; charset=utf-8"

	GET     = "GET"
	POST    = "POST"
	PUT     = "PUT"
	PATCH   = "PATCH"
	DELETE  = "DELETE"
	HEAD    = "HEAD"
	OPTIONS = "OPTIONS"

	MAX_VALUE_LEN = 200000 // 最大参数值长度

	Authorization = "Authorization"
	RandomCode    = "RandomCode"
)

var (
	jwtConfig = jwt.JwtConfig{}
)

type HookNode struct {
	Context *Context
	Filters []*FilterObject
}

type RouterConfig struct {
	Guest       bool // 游客模式,原始请求 false.否 true.是
	UseRSA      bool // 非登录状态使用RSA模式请求 false.否 true.是
	UseHAX      bool // 非登录状态,判定公钥哈希验签 false.否 true.是
	AesRequest  bool // 请求是否必须AES加密 false.否 true.是
	AesResponse bool // 响应是否必须AES加密 false.否 true.是
}

type HttpLog struct {
	Method   string // 请求方法
	LogNo    string // 日志唯一标记
	CreateAt int64  // 日志创建时间
	UpdateAt int64  // 日志完成时间
	CostMill int64  // 业务耗时,毫秒
}

type JsonBody struct {
	Data  interface{} `json:"d"`
	Time  int64       `json:"t"`
	Nonce string      `json:"n"`
	Plan  int64       `json:"p"` // 0.默认(登录状态) 1.AES(登录状态) 2.RSA/ECC模式(匿名状态) 3.独立验签模式(匿名状态)
	Sign  string      `json:"s"`
}

type JsonResp struct {
	Code    int         `json:"c"`
	Message string      `json:"m"`
	Data    interface{} `json:"d"`
	Time    int64       `json:"t"`
	Nonce   string      `json:"n"`
	Plan    int64       `json:"p"`
	Sign    string      `json:"s"`
}

type Permission struct {
	Ready     bool
	MathchAll int64
	NeedLogin int64
	NeedRole  []int64
}

type Context struct {
	router        *fasthttprouter.Router
	CacheAware    func(ds ...string) (cache.Cache, error)
	AcceptTimeout int64 // 超时主动断开客户端连接,秒
	//Token         string
	Method        string
	Path          string
	RequestCtx    *fasthttp.RequestCtx
	Subject       *jwt.Subject
	JsonBody      *JsonBody
	Response      *Response
	filterChain   *filterChain
	RouterConfig  *RouterConfig
	RSA           crypto.Cipher
	EnableECC     bool
	PermConfig    func(uid, url string, isRole ...bool) ([]int64, Permission, error)
	Storage       map[string]interface{}
	postCompleted bool
	postHandle    PostHandle
}

type Response struct {
	Encoding      string
	ContentType   string
	ContentEntity interface{}
	// response result
	StatusCode        int
	ContentEntityByte bytes.Buffer
}

func (self *JsonBody) ParseData(dst interface{}) error {
	raw, b := self.Data.([]byte)
	if !b {
		return utils.Error("jsonBody data not string")
	}
	return utils.JsonUnmarshal(raw, dst)
}

func (self *JsonBody) RawData() []byte {
	raw, b := self.Data.([]byte)
	if !b {
		return nil
	}
	return raw
}

func (self *Context) GetTokenSecret() string {
	return jwt.GetTokenSecret(utils.Bytes2Str(self.Subject.GetRawBytes()), jwtConfig.TokenKey)
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
	if self.JsonBody == nil || self.JsonBody.Data == nil {
		return nil
	}
	if err := self.JsonBody.ParseData(dst); err != nil {
		msg := "JSON parameter parsing failed"
		zlog.Error(msg, 0, zlog.String("path", self.Path), zlog.String("device", self.ClientDevice()), zlog.Any("data", self.JsonBody), zlog.AddError(err))
		return ex.Throw{Msg: msg}
	}
	// TODO 备注: 已有会话状态时,指针填充context值,不能随意修改指针偏移值
	identify := &common.Identify{}
	if self.Authenticated() {
		identify.ID = self.Subject.GetSub()
	}
	context := common.Context{
		Identify: identify,
	}
	src := utils.GetPtr(dst, 0)
	req := common.GetBaseReq(src)
	base := common.BaseReq{Context: context, Offset: req.Offset, Limit: req.Limit, PrevID: req.PrevID, LastID: req.LastID, CountQ: req.CountQ}
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
		if body == nil || len(body) == 0 {
			return nil
		}
		self.JsonBody.Data = body
		return nil
	}
	// 安全请求模式
	self.Subject.ResetTokenBytes(self.RequestCtx.Request.Header.Peek(Authorization))
	if body == nil || len(body) == 0 {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "body parameters is nil"}
	}
	if len(body) > (MAX_VALUE_LEN) {
		return ex.Throw{Code: http.StatusLengthRequired, Msg: "body parameters length is too long"}
	}
	self.JsonBody.Data = utils.GetJsonString(body, "d")
	self.JsonBody.Time = utils.GetJsonInt64(body, "t")
	self.JsonBody.Nonce = utils.GetJsonString(body, "n")
	self.JsonBody.Plan = utils.GetJsonInt64(body, "p")
	self.JsonBody.Sign = utils.GetJsonString(body, "s")
	//if err := utils.JsonUnmarshal(body, self.JsonBody); err != nil {
	//	panic(err)
	//}
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

func (self *Context) validJsonBody() error {
	if self.JsonBody == nil {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request json body is nil"}
	}
	body := self.JsonBody
	d, b := body.Data.(string)
	if !b || len(d) == 0 {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request data is nil"}
	}
	if !utils.CheckInt64(body.Plan, 0, 1, 2, 3) {
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
	if utils.CheckInt64(body.Plan, 0, 1) && len(self.Subject.GetRawBytes()) == 0 {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request header token is nil"}
	}
	if utils.CheckInt64(body.Plan, 2, 3) { // reset token
		if body.Plan == 2 {
			if !self.RouterConfig.UseRSA {
				return ex.Throw{Code: http.StatusBadRequest, Msg: "request parameters must use RSA encryption"}
			}
		}
		if body.Plan == 3 {
			if !self.RouterConfig.UseHAX {
				return ex.Throw{Code: http.StatusBadRequest, Msg: "request parameters must use HAX signature"}
			}
		}
	}
	var key string
	var anonymous bool // true.匿名状态
	if self.RouterConfig.UseRSA || self.RouterConfig.UseHAX {
		_, key = self.RSA.GetPublicKey()
		anonymous = true
	}
	if self.GetHmac256Sign(d, body.Nonce, body.Time, body.Plan, key) != body.Sign {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request signature invalid"}
	}
	if err := self.validReplayAttack(body.Sign); err != nil {
		return err
	}
	var rawData []byte
	var err error
	if body.Plan == 0 && !anonymous { // 登录状态 P0 Base64
		rawData = utils.Base64Decode(d)
		if rawData == nil || len(rawData) == 0 {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "parameter Base64 parsing failed"}
		}
	} else if body.Plan == 1 && !anonymous { // 登录状态 P1 AES
		rawData, err = utils.AesDecrypt(d, self.GetTokenSecret(), utils.AddStr(body.Nonce, body.Time))
		if err != nil {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "AES failed to parse data", Err: err}
		}
	} else if body.Plan == 2 && self.RouterConfig.UseRSA && anonymous { // 非登录状态 P2 RSA+AES
		randomCode := utils.Bytes2Str(self.RequestCtx.Request.Header.Peek(RandomCode))
		if len(randomCode) == 0 {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "client random code invalid"}
		}
		code, err := self.RSA.Decrypt(randomCode)
		if err != nil {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "server private-key decrypt failed", Err: err}
		}
		if len(code) == 0 {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "server private-key decrypt data is nil", Err: err}
		}
		rawData, err = utils.AesDecrypt(d, code, code)
		if err != nil {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "AES failed to parse data", Err: err}
		}
		self.AddStorage(RandomCode, code)
	} else if body.Plan == 3 && self.RouterConfig.UseHAX && anonymous { // 非登录状态 P3 Base64
		rawData = utils.Base64Decode(d)
		if rawData == nil || len(rawData) == 0 {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "parameter Base64 parsing failed"}
		}
	} else {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request parameters plan invalid"}
	}
	self.JsonBody.Data = rawData
	return nil
}

func (self *Context) GetJwtConfig() jwt.JwtConfig {
	return jwtConfig
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
	if self.CacheAware == nil {
		self.CacheAware = ctx.CacheAware
	}
	if self.RSA == nil {
		self.RSA = ctx.RSA
	}
	if self.PermConfig == nil {
		self.PermConfig = ctx.PermConfig
	}
	if len(self.filterChain.filters) == 0 {
		self.filterChain.filters = fs
	}
	self.postHandle = handle
	self.RequestCtx = request
	self.Method = utils.Bytes2Str(self.RequestCtx.Method())
	self.Path = utils.Bytes2Str(self.RequestCtx.Path())
	self.RouterConfig = routerConfigs[self.Path]
	self.postCompleted = false
	self.filterChain.pos = 0
	self.resetJsonBody()
	self.resetResponse()
	self.resetSubject()
	self.resetTokenStorage()
}

func (self *Context) resetTokenStorage() {
	if len(self.Storage) == 0 {
		return
	}
	for k, _ := range self.Storage {
		delete(self.Storage, k)
	}
}

func (self *Context) resetJsonBody() {
	if self.JsonBody == nil {
		self.JsonBody = &JsonBody{}
	}
	self.JsonBody.Data = nil
	self.JsonBody.Nonce = ""
	self.JsonBody.Sign = ""
	self.JsonBody.Time = 0
	self.JsonBody.Plan = 0
}

func (self *Context) resetResponse() {
	if self.Response == nil {
		self.Response = &Response{}
	}
	if len(self.Response.Encoding) == 0 {
		self.Response.Encoding = UTF8
	}
	if len(self.Response.ContentType) == 0 {
		self.Response.ContentType = APPLICATION_JSON
	}
	self.Response.ContentEntity = nil
	self.Response.StatusCode = 0
	if self.Response.ContentEntityByte.Len() > 0 {
		self.Response.ContentEntityByte.Reset()
	}
}

func (self *Context) resetSubject() {
	if self.Subject == nil {
		self.Subject = &jwt.Subject{}
		self.Subject.Header = &jwt.Header{}
		self.Subject.Payload = &jwt.Payload{}
	}
	self.Subject.Payload.Sub = ""
	self.Subject.Payload.Iss = ""
	self.Subject.Payload.Aud = ""
	self.Subject.Payload.Iat = 0
	self.Subject.Payload.Exp = 0
	self.Subject.Payload.Dev = ""
	self.Subject.Payload.Jti = ""
	self.Subject.Payload.Ext = ""
	self.Subject.ResetTokenBytes(nil)
	self.Subject.ResetPayloadBytes(nil)
}
