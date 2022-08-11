package node

import (
	"github.com/godaddy-x/freego/cache"
	"github.com/godaddy-x/freego/component/gorsa"
	"github.com/godaddy-x/freego/component/jwt"
	"github.com/godaddy-x/freego/component/limiter"
	"github.com/godaddy-x/freego/component/log"
	"github.com/godaddy-x/freego/ex"
	"github.com/godaddy-x/freego/node/common"
	"github.com/godaddy-x/freego/util"
	"net/http"
	"unsafe"
)

const (
	HTTP      = "http"
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
	CreateAt          int64
	Context           *Context
	SessionAware      SessionAware
	CacheAware        func(ds ...string) (cache.ICache, error)
	OverrideFunc      *OverrideFunc
	GatewayRate       *rate.RateOpetion
	Handler           *http.ServeMux
	RouterConfig      *RouterConfig
	DisconnectTimeout int64 // 超时主动断开客户端连接,秒
}

type NodePtr struct {
	Node         interface{}
	RouterConfig *RouterConfig
	Input        *http.Request
	Output       http.ResponseWriter
	Pattern      string
	Handle       func(ctx *Context) error
}

type RouterConfig struct {
	Guest       bool // 游客模式 false.否 true.是
	Login       bool // 是否登录请求 false.否 true.是
	Original    bool // 是否原始方式 false.否 true.是
	AesRequest  bool // 请求是否必须AES加密 false.否 true.是
	AesResponse bool // 响应是否必须AES加密 false.否 true.是
}

type LogHandleRes struct {
	LogNo    string // 日志唯一标记
	CreateAt int64  // 日志创建时间
	UpdateAt int64  // 日志完成时间
	CostMill int64  // 业务耗时,毫秒
}

type ProtocolNode interface {
	// 初始化上下文
	InitContext(ptr *NodePtr) error
	// 校验会话
	ValidSession() error
	// 校验重放攻击
	ValidReplayAttack() error
	// 校验权限
	ValidPermission() error
	// 获取请求头数据
	GetHeader() error
	// 获取请求参数
	GetParams() error
	// 核心代理方法
	Proxy(ptr *NodePtr)
	// 核心绑定路由方法, customize=true自定义不执行默认流程
	Router(pattern string, handle func(ctx *Context) error, routerConfig *RouterConfig)
	// json响应模式
	Json(ctx *Context, data interface{}) error
	// text响应模式
	Text(ctx *Context, data string) error
	// 前置检测方法(业务方法前执行)
	PreHandle() error
	// 日志监听方法(业务方法前执行)
	LogHandle() (LogHandleRes, error)
	// 业务执行方法->自定义处理执行方法(业务方法执行后,视图渲染前执行)
	PostHandle(err error) error
	// 最终响应执行方法(视图渲染后执行,可操作资源释放,保存日志等)
	AfterCompletion(res LogHandleRes, err error) error
	// 渲染输出
	RenderTo() error
	// 异常错误响应方法
	RenderError(err error) error
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
	Host       string
	Port       int64
	Style      string
	Device     string
	Method     string
	Token      string
	Headers    map[string]string
	Params     *ReqDto
	Subject    *jwt.Payload
	Response   *Response
	Version    string
	Input      *http.Request
	Output     http.ResponseWriter
	ServerCert *gorsa.RsaObj
	ClientCert *gorsa.RsaObj
	JwtConfig  func() jwt.JwtConfig
	PermConfig func(url string) (Permission, error)
	Storage    map[string]interface{}
	Roles      []int64
}

type Response struct {
	Encoding      string
	ContentType   string
	ContentEntity interface{}
}

type OverrideFunc struct {
	PreHandleFunc       func(ctx *Context) error
	LogHandleFunc       func(ctx *Context) (LogHandleRes, error)
	PostHandleFunc      func(resp *Response, err error) error
	AfterCompletionFunc func(ctx *Context, res LogHandleRes, err error) error
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
	return util.HMAC_SHA256(util.AddStr(self.Method, d, n, t, p), secret, true)
}

func (self *Context) GetStorageStringValue(k string) string {
	v, b := self.Storage[k]
	if b {
		return v.(string)
	}
	return ""
}

func (self *Context) Authenticated() bool {
	if self.Subject == nil || self.Subject.Sub == 0 {
		return false
	}
	return true
}

func (self *Context) Parser(v interface{}) error {
	if err := util.JsonToAny(self.Params.Data, v); err != nil {
		msg := "JSON parameter parsing failed"
		log.Error(msg, 0, log.String("method", self.Method), log.String("host", self.Host), log.String("device", self.Device), log.Any("data", self.Params))
		return ex.Throw{Msg: msg}
	}
	// TODO 备注: 已有会话状态时,指针填充context值,不能随意修改指针偏移值
	userId := int64(0)
	if self.Authenticated() {
		userId = self.Subject.Sub
	}
	context := common.Context{
		UserId: userId,
		UserIP: self.Host,
	}
	src := util.GetPtr(v, 0)
	req := common.GetBaseReq(src)
	dst := common.BaseReq{Context: context, Offset: req.Offset, Limit: req.Limit}
	*((*common.BaseReq)(unsafe.Pointer(src))) = dst
	self.Params.Data = v
	return nil
}
