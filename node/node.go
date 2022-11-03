package node

import (
	"github.com/buaazp/fasthttprouter"
	"github.com/godaddy-x/freego/cache"
	"github.com/godaddy-x/freego/ex"
	"github.com/godaddy-x/freego/node/common"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/utils/gorsa"
	"github.com/godaddy-x/freego/utils/jwt"
	"github.com/godaddy-x/freego/zlog"
	"github.com/valyala/fasthttp"
	"net/http"
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

	MAX_VALUE_LEN = 4000 // 最大参数值长度

	Authorization = "Authorization"
	RandomCode    = "RandomCode"
)

var (
	jwtConfig = jwt.JwtConfig{}
)

type HookNode struct {
	Context *Context
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
	Plan  int64       `json:"p"` // 0.默认 1.AES 2.RSA模式 3.独立验签模式
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
	Token         string
	Device        string
	CreateAt      int64
	Method        string
	Path          string
	RequestCtx    *fasthttp.RequestCtx
	Subject       *jwt.Payload
	JsonBody      *JsonBody
	Response      *Response
	filterChain   *filterChain
	RouterConfig  *RouterConfig
	RSA           gorsa.RSA
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
	ContentEntityByte []byte
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
	return jwt.GetTokenSecret(self.Token, jwtConfig.TokenKey)
}

func (self *Context) GetHmac256Sign(d, n string, t, p int64, key ...string) string {
	var secret string
	if len(key) > 0 && len(key[0]) > 0 {
		secret = key[0]
	} else {
		secret = self.GetTokenSecret()
	}
	return utils.HMAC_SHA256(utils.AddStr(self.Path, d, n, t, p), secret, true)
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
	if self.Subject == nil || len(self.Subject.Sub) == 0 {
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
		zlog.Error(msg, 0, zlog.String("path", self.Path), zlog.String("device", self.Device), zlog.Any("data", self.JsonBody), zlog.AddError(err))
		return ex.Throw{Msg: msg}
	}
	// TODO 备注: 已有会话状态时,指针填充context值,不能随意修改指针偏移值
	identify := &common.Identify{}
	if self.Authenticated() {
		identify.ID = self.Subject.Sub
	}
	context := common.Context{
		Identify: identify,
	}
	src := utils.GetPtr(dst, 0)
	req := common.GetBaseReq(src)
	base := common.BaseReq{Context: context, Offset: req.Offset, Limit: req.Limit, PrevID: req.PrevID, LastID: req.LastID}
	*((*common.BaseReq)(unsafe.Pointer(src))) = base
	return nil
}

func (self *Context) readParams() error {
	agent := utils.Bytes2Str(self.RequestCtx.Request.Header.Peek("User-Agent"))
	if utils.HasStr(agent, "Android") || utils.HasStr(agent, "Adr") {
		self.Device = ANDROID
	} else if utils.HasStr(agent, "iPad") || utils.HasStr(agent, "iPhone") || utils.HasStr(agent, "Mac") {
		self.Device = IOS
	} else {
		self.Device = WEB
	}
	if self.Method != POST {
		return nil
	}
	body := self.RequestCtx.PostBody()
	// 原始请求模式
	if self.RouterConfig.Guest {
		if body != nil && len(body) > 0 {
			self.JsonBody = &JsonBody{Data: body}
		}
		return nil
	}
	// 安全请求模式
	self.Token = utils.Bytes2Str(self.RequestCtx.Request.Header.Peek(Authorization))
	if body == nil || len(body) == 0 {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "body parameters is nil"}
	}
	if len(body) > (MAX_VALUE_LEN * 5) {
		return ex.Throw{Code: http.StatusLengthRequired, Msg: "body parameters length is too long"}
	}
	req := &JsonBody{
		Data:  utils.GetJsonString(body, "d"),
		Time:  int64(utils.GetJsonInt(body, "t")),
		Nonce: utils.GetJsonString(body, "n"),
		Plan:  int64(utils.GetJsonInt(body, "p")),
		Sign:  utils.GetJsonString(body, "s"),
	}
	if err := self.validJsonBody(req); err != nil { // TODO important
		return err
	}
	return nil
}

func (self *Context) validJsonBody(body *JsonBody) error {
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
	if utils.CheckInt64(body.Plan, 0, 1) && len(self.Token) == 0 {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request header token is nil"}
	}
	if utils.CheckInt64(body.Plan, 2, 3) { // reset token
		if self.RouterConfig.UseRSA && !self.RouterConfig.UseHAX && body.Plan != 2 {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "request parameters must use RSA encryption"}
		}
		if self.RouterConfig.UseHAX && !self.RouterConfig.UseRSA && body.Plan != 3 {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "request parameters must use HAX signature"}
		}
		self.Token = ""
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
	body.Data = rawData
	self.JsonBody = body
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

func (self *Context) reset(ctx *Context, handle PostHandle, request *fasthttp.RequestCtx) {
	self.CacheAware = ctx.CacheAware
	self.RSA = ctx.RSA
	self.PermConfig = ctx.PermConfig
	self.postHandle = handle
	self.RequestCtx = request
	self.Method = utils.Bytes2Str(self.RequestCtx.Method())
	self.Path = utils.Bytes2Str(self.RequestCtx.Path())
	self.RouterConfig = routerConfigs[self.Path]
	self.postCompleted = false
	self.filterChain.pos = 0
	self.Response.Encoding = UTF8
	self.Response.ContentType = APPLICATION_JSON
	self.Response.ContentEntity = nil
	self.Response.StatusCode = 0
	self.Response.ContentEntityByte = nil
}
