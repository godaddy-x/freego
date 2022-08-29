package node

import (
	"github.com/godaddy-x/freego/cache"
	"github.com/godaddy-x/freego/ex"
	"github.com/godaddy-x/freego/node/common"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/utils/gorsa"
	"github.com/godaddy-x/freego/utils/jwt"
	"github.com/godaddy-x/freego/zlog"
	"net/http"
	"unsafe"
)

const (
	HTTP2     = "http2"
	WEBSOCKET = "websocket"
	UTF8      = "UTF-8"

	ANDROID = "android"
	IOS     = "ios"
	WEB     = "web"

	TEXT_PLAIN       = "text/plain; charset=utf-8"
	APPLICATION_JSON = "application/json; charset=utf-8"

	GET    = "GET"
	POST   = "POST"
	PUT    = "PUT"
	PATCH  = "PATCH"
	DELETE = "DELETE"

	MAX_HEADER_SIZE     = 25   // 最大响应头数量
	MAX_PARAMETER_SIZE  = 50   // 最大参数数量
	MAX_FIELD_LEN       = 25   // 最大参数名长度
	MAX_QUERYSTRING_LEN = 1000 // 最大GET参数名长度
	MAX_VALUE_LEN       = 4000 // 最大参数值长度

	Authorization = "Authorization"
	USER_AGENT    = "User-Agent"
	CLIENT_PUBKEY = "pubkey"
)

type HookNode struct {
	handler           *http.ServeMux
	Context           *Context
	Render            *Render
	SessionAware      SessionAware
	CacheAware        func(ds ...string) (cache.ICache, error)
	DisconnectTimeout int64 // 超时主动断开客户端连接,秒
}

type Render struct {
	To    func(*Context) error
	Pre   func(*Context) error
	Error func(*Context, error) error
}

type RouterConfig struct {
	Guest       bool // 游客模式 false.否 true.是
	Login       bool // 是否登录请求 false.否 true.是
	Original    bool // 是否原始方式 false.否 true.是
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

type ProtocolNode interface {
	// 绑定路由方法
	Router(pattern string, handle func(ctx *Context) error, routerConfig *RouterConfig)
	// json响应模式
	Json(ctx *Context, data interface{}) error
	// text响应模式
	Text(ctx *Context, data string) error
	// 启动服务
	StartServer()
}

type ReqDto struct {
	Data  interface{} `json:"d"`
	Time  int64       `json:"t"`
	Nonce string      `json:"n"`
	Plan  int64       `json:"p"` // 0.默认 1.AES 2.RSA登录
	Sign  string      `json:"s"`
}

type RespDto struct {
	Code    int         `json:"c"`
	Message string      `json:"m"`
	Data    interface{} `json:"d"`
	Time    int64       `json:"t"`
	Nonce   string      `json:"n"`
	Plan    int64       `json:"p"`
	Sign    string      `json:"s"`
}

type Permission struct {
	ready     bool
	MathchAll int64
	NeedLogin int64
	NeedRole  []int64
}

type Context struct {
	CreateAt     int64
	Host         string
	Port         int64
	Style        string
	Device       string
	Method       string
	Token        string
	Headers      map[string]string
	Params       *ReqDto
	Subject      *jwt.Payload
	Response     *Response
	Version      string
	Input        *http.Request
	Output       http.ResponseWriter
	RouterConfig *RouterConfig
	ServerCert   *gorsa.RsaObj
	ClientCert   *gorsa.RsaObj
	JwtConfig    func() jwt.JwtConfig
	PermConfig   func(url string) (Permission, error)
	Storage      map[string]interface{}
	Roles        []int64
}

type Response struct {
	Encoding      string
	ContentType   string
	ContentEntity interface{}
	// response result
	StatusCode        int
	ContentEntityByte []byte
}

func (self *Context) GetHeader(k string) string {
	if len(k) == 0 || len(self.Headers) == 0 {
		return ""
	}
	if v, b := self.Headers[k]; b {
		return v
	}
	return ""
}

func (self *Context) GetTokenSecret() string {
	return jwt.GetTokenSecret(self.Token, self.JwtConfig().TokenKey)
}

func (self *Context) GetDataSign(d, n string, t, p int64, key ...string) string {
	var secret string
	if len(key) > 0 && len(key[0]) > 0 {
		secret = key[0]
	} else {
		secret = self.GetTokenSecret()
	}
	return utils.HMAC_SHA256(utils.AddStr(self.Method, d, n, t, p), secret, true)
}

func (self *Context) AddStorage(k string, v interface{}) error {
	if len(k) == 0 || v == nil {
		return utils.Error("key/value is nil")
	}
	self.Storage[k] = v
	return nil
}

func (self *Context) GetStorage(k string) interface{} {
	if len(k) == 0 {
		return nil
	}
	v, b := self.Storage[k]
	if !b || v == nil {
		return nil
	}
	return v
}

func (self *Context) Authenticated() bool {
	if self.Subject == nil || len(self.Subject.Sub) == 0 {
		return false
	}
	return true
}

func (self *Context) Parser(v interface{}) error {
	if self.Params == nil || self.Params.Data == nil {
		return nil
	}
	if err := utils.JsonToAny(self.Params.Data, v); err != nil {
		msg := "JSON parameter parsing failed"
		zlog.Error(msg, 0, zlog.String("method", self.Method), zlog.String("host", self.Host), zlog.String("device", self.Device), zlog.Any("data", self.Params))
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
		UserIP: self.Host,
	}
	src := utils.GetPtr(v, 0)
	req := common.GetBaseReq(src)
	dst := common.BaseReq{Context: context, Offset: req.Offset, Limit: req.Limit}
	*((*common.BaseReq)(unsafe.Pointer(src))) = dst
	self.Params.Data = v
	return nil
}
