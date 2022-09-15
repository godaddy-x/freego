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
	Guest bool // 游客模式 false.否 true.是
	Login bool // 是否登录请求 false.否 true.是
	//Original    bool // 是否原始方式 false.否 true.是
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
	Plan  int64       `json:"p"` // 0.默认 1.AES 2.RSA登录
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
	ServerTLS     *gorsa.RsaObj
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

func (self *Context) Parser(v interface{}) error {
	if self.JsonBody == nil || self.JsonBody.Data == nil {
		return nil
	}
	if err := utils.JsonToAny(self.JsonBody.Data, v); err != nil {
		msg := "JSON parameter parsing failed"
		zlog.Error(msg, 0, zlog.String("path", self.Path), zlog.String("device", self.Device), zlog.Any("data", self.JsonBody))
		return ex.Throw{Msg: msg}
	}
	// TODO 备注: 已有会话状态时,指针填充context值,不能随意修改指针偏移值
	userId := int64(0)
	if self.Authenticated() {
		v, err := utils.StrToInt64(self.Subject.Sub)
		if err != nil {
			return ex.Throw{Msg: "userId invalid"}
		}
		userId = v
	}
	context := common.Context{
		UserId: userId,
	}
	src := utils.GetPtr(v, 0)
	req := common.GetBaseReq(src)
	dst := common.BaseReq{Context: context, Offset: req.Offset, Limit: req.Limit}
	*((*common.BaseReq)(unsafe.Pointer(src))) = dst
	self.JsonBody.Data = v
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
	self.Token = utils.Bytes2Str(self.RequestCtx.Request.Header.Peek(Authorization))
	body := self.RequestCtx.PostBody()
	if len(body) == 0 {
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

func (self *Context) validJsonBody(req *JsonBody) error {
	d, b := req.Data.(string)
	if !b || len(d) == 0 {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request data is nil"}
	}
	if !utils.CheckInt64(req.Plan, 0, 1, 2) {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request plan invalid"}
	}
	if !utils.CheckLen(req.Nonce, 8, 32) {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request nonce invalid"}
	}
	if req.Time <= 0 {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request time must be > 0"}
	}
	if utils.MathAbs(utils.UnixSecond()-req.Time) > jwt.FIVE_MINUTES { // 判断绝对时间差超过5分钟
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request time invalid"}
	}
	if self.RouterConfig.AesRequest && req.Plan != 1 {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request parameters must use AES encryption"}
	}
	if self.RouterConfig.Login && req.Plan != 2 {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request parameters must use RSA encryption"}
	}
	if !utils.CheckStrLen(req.Sign, 32, 64) {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request signature length invalid"}
	}
	var key string
	if self.RouterConfig.Login {
		key = self.ServerTLS.PubkeyBase64
	}
	if self.GetHmac256Sign(d, req.Nonce, req.Time, req.Plan, key) != req.Sign {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request signature invalid"}
	}
	data := make(map[string]interface{}, 0)
	if req.Plan == 1 && !self.RouterConfig.Login { // AES
		dec, err := utils.AesDecrypt(d, self.GetTokenSecret(), utils.AddStr(req.Nonce, req.Time))
		if err != nil {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "AES failed to parse data", Err: err}
		}
		if err := utils.JsonUnmarshal(utils.Str2Bytes(dec), &data); err != nil {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "parameter JSON parsing failed"}
		}
	} else if req.Plan == 2 && self.RouterConfig.Login { // RSA
		randomCode := utils.Bytes2Str(self.RequestCtx.Request.Header.Peek(RandomCode))
		if len(randomCode) == 0 {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "client random code invalid"}
		}
		code, err := self.ServerTLS.Decrypt(randomCode)
		if err != nil {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "server private-key decrypt failed", Err: err}
		}
		if len(code) == 0 {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "server private-key decrypt data is nil", Err: err}
		}
		dec, err := utils.AesDecrypt(d, code, code)
		if err != nil {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "AES failed to parse data", Err: err}
		}
		if err := utils.JsonUnmarshal(utils.Str2Bytes(dec), &data); err != nil {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "parameter JSON parsing failed"}
		}
		self.AddStorage(RandomCode, code)
	} else if req.Plan == 0 && !self.RouterConfig.Login && !self.RouterConfig.AesRequest {
		if err := utils.ParseJsonBase64(d, &data); err != nil {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "parameter JSON parsing failed"}
		}
	} else {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request parameters plan invalid"}
	}
	req.Data = data
	self.JsonBody = req
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
	self.ServerTLS = ctx.ServerTLS
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
