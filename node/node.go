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

	ANDROID = "Android"
	IPHONE  = "iPhone"
	IPAD    = "iPad"
	WEB     = "Web"

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

	Token = "a"
	Data  = "d"
	Key   = "k"
	Nonce = "n"
	Time  = "t"
	Sign  = "g"
)

type HookNode struct {
	CreateAt     int64
	Context      *Context
	SessionAware SessionAware
	CacheAware   func(ds ...string) (cache.ICache, error)
	OverrideFunc *OverrideFunc
	Limiter      *rate.RateLimiter
	Handler      *http.ServeMux
	Option       *Option
	OptionMap    map[string]*Option
}

type NodePtr struct {
	Node    interface{}
	Input   *http.Request
	Output  http.ResponseWriter
	Pattern string
	Handle  func(ctx *Context) error
}

type Option struct {
	Customize bool
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
	Router(pattern string, handle func(ctx *Context) error, option *Option)
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
	// 保存用户会话密钥
	LoginBySubject(sub, key string, exp int64) error
	// 删除用户会话密钥
	LogoutBySubject(subs ...string) error
	// 渲染输出
	RenderTo() error
	// 异常错误响应方法
	RenderError(err error) error
	// 启动服务
	StartServer()
}

type ReqDto struct {
	Token string      `json:"a"`
	Nonce string      `json:"n"`
	Time  int64       `json:"t"`
	Data  interface{} `json:"d"`
	Sign  string      `json:"g"`
}

type RespDto struct {
	Code    int         `json:"c"`
	Data    interface{} `json:"d"`
	Message string      `json:"m"`
	Time    int64       `json:"t"`
}

type Permission struct {
	MathchAll int64
	NeedLogin int64
	NeedRole  []string
}

type Context struct {
	Host          string
	Port          int64
	Style         string
	Device        string
	Method        string
	Headers       map[string]string
	Params        *ReqDto
	Session       Session
	Response      *Response
	Version       string
	Input         *http.Request
	Output        http.ResponseWriter
	SecretKey     func() *jwt.SecretKey
	UserId        int64
	Storage       map[string]interface{}
	Roles         []string
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

func (self *Context) Authorized() bool {
	session := self.Session
	if session != nil && !session.Invalid() {
		return true
	}
	return false
}

// 按指定规则进行数据解码,校验参数安全
func (self *Context) SecurityCheck(req *ReqDto) error {
	d, _ := req.Data.(string)
	if len(d) == 0 {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "业务参数无效"}
	}
	if len(req.Sign) == 0 || len(req.Sign) < 32 {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "签名参数无效"}
	}
	if len(req.Nonce) == 0 {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "随机参数无效"}
	}
	if req.Time == 0 {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "时间参数无效"}
	} else if req.Time+jwt.FIVE_MINUTES < util.Time() { // 判断时间是否超过5分钟
		return ex.Throw{Code: http.StatusBadRequest, Msg: "时间参数已过期"}
	}
	if !validSign(req, d, self.SecretKey()) {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "API签名校验失败"}
	}
	data := make(map[string]interface{})
	if ret := util.Base64URLDecode(d); ret == nil {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "业务参数解码失败"}
	} else if err := util.JsonUnmarshal(ret, &data); err != nil {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "业务参数解析失败"}
	} else {
		req.Data = data
		self.Params = req
	}
	return nil
}

// 校验签名有效性
func validSign(req *ReqDto, dataStr string, key *jwt.SecretKey) bool {
	token := req.Token
	nonce := req.Nonce
	time := req.Time
	api_secret_key := key.ApiSecretKey
	secret_key_alg := key.SecretKeyAlg
	if secret_key_alg == jwt.MD5 {
		sign_str := util.AddStr(token, dataStr, util.GetApiAccessKeyByMD5(token, api_secret_key), nonce, time)
		return req.Sign == util.MD5(sign_str)
	} else if secret_key_alg == jwt.SHA256 {
		sign_str := util.AddStr(token, dataStr, util.GetApiAccessKeyBySHA256(token, api_secret_key), nonce, time)
		return req.Sign == util.SHA256(sign_str)
	}
	return false
}
