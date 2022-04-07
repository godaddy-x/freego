package node

import (
	"github.com/godaddy-x/freego/cache"
	"github.com/godaddy-x/freego/component/jwt"
	"github.com/godaddy-x/freego/component/limiter"
	"github.com/godaddy-x/freego/ex"
	"github.com/godaddy-x/freego/util"
	"net/http"
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

	JWT_SUB_ = "jwt_sub_"
	JWT_SIG_ = "jwt_sig_"
)

type HookNode struct {
	CreateAt     int64
	Context      *Context
	SessionAware SessionAware
	CacheAware   func(ds ...string) (cache.ICache, error)
	OverrideFunc *OverrideFunc
	RateOpetion  *rate.RateOpetion
	Handler      *http.ServeMux
	Config       *Config
}

type NodePtr struct {
	Node    interface{}
	Input   *http.Request
	Output  http.ResponseWriter
	Pattern string
	Handle  func(ctx *Context) error
}

type Config struct {
	Original      bool // 是否原始方式 false.否 true.是
	Authorization bool // 游客模式 false.否 true.是
	AesEncrypt    bool // 响应是否AES加密 false.否 true.是
	RsaEncrypt    bool // 响应是否RSA加密 false.否 true.是
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
	Router(pattern string, handle func(ctx *Context) error, config *Config)
	// json响应模式
	Json(ctx *Context, data interface{}) error
	// text响应模式
	Text(ctx *Context, data string) error
	// 前置检测方法(业务方法前执行)
	PreHandle() error
	// 业务执行方法->自定义处理执行方法(业务方法执行后,视图渲染前执行)
	PostHandle(err error) error
	// 最终响应执行方法(视图渲染后执行,可操作资源释放,保存日志等)
	AfterCompletion(err error) error
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
	Plan  int64       `json:"p"`
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
	MathchAll int64
	NeedLogin int64
	NeedRole  []int64
}

type Context struct {
	Host          string
	Port          int64
	Style         string
	Device        string
	Method        string
	Token         string
	Headers       map[string]string
	Params        *ReqDto
	Subject       *jwt.Payload
	Response      *Response
	Version       string
	Input         *http.Request
	Output        http.ResponseWriter
	Storage       map[string]interface{}
	Roles         []int64
	SecretKey     func() *jwt.SecretKey
	PermissionKey func(url string) (*Permission, error)
}

type Response struct {
	Encoding      string
	ContentType   string
	ContentEntity interface{}
}

type OverrideFunc struct {
	PreHandleFunc       func(ctx *Context) error
	PostHandleFunc      func(resp *Response, err error) error
	AfterCompletionFunc func(ctx *Context, resp *Response, err error) error
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
	return jwt.GetTokenSecret(self.Token)
}

func (self *Context) GetDataSign(d, n string, t, p int64) string {
	return util.HMAC_SHA256(util.AddStr(self.Method, d, n, t, p), self.GetTokenSecret())
}

// 按指定规则进行数据解码,校验参数安全
func (self *Context) SecurityCheck(req *ReqDto) error {
	d, b := req.Data.(string)
	if !b || len(d) == 0 {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "业务参数无效"}
	}
	if len(req.Sign) != 32 && len(req.Sign) != 64 {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "签名参数无效"}
	}
	if !util.CheckLen(req.Nonce, 8, 32) {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "随机参数无效"}
	}
	if util.MathAbs(util.Time()-req.Time) > jwt.FIVE_MINUTES { // 判断绝对时间差超过5分钟
		return ex.Throw{Code: http.StatusBadRequest, Msg: "时间参数无效"}
	}
	if !util.CheckInt64(req.Plan, 0, 1, 2) {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "计划参数无效"}
	}
	if req.Plan == 1 { // AES
		dec, err := util.AesDecrypt(d, self.GetTokenSecret(), util.AddStr(req.Nonce, req.Time))
		if err != nil {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "数据逆向解析失败", Err: err}
		}
		d = dec
	} else if req.Plan == 2 { // RSA

	}
	if self.GetDataSign(d, req.Nonce, req.Time, req.Plan) != req.Sign {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "API签名校验失败"}
	}
	data := make(map[string]interface{})
	if err := util.ParseJsonBase64(d, &data); err != nil {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "业务参数解析失败"}
	}
	req.Data = data
	self.Params = req
	return nil
}
