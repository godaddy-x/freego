package node

import (
	"fmt"
	"github.com/godaddy-x/freego/cache"
	"github.com/godaddy-x/freego/component/jwt"
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

	TEXT_HTML        = "text/html"
	TEXT_PLAIN       = "text/plain"
	APPLICATION_JSON = "application/json"

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
	Context      *Context
	SessionAware SessionAware
	CacheAware   func(ds ...string) (cache.ICache, error)
	OverrideFunc *OverrideFunc
	Customize    bool
}

type NodePtr struct {
	Node    interface{}
	Input   *http.Request
	Output  http.ResponseWriter
	Pattern string
	Handle  func(ctx *Context) error
}

type ProtocolNode interface {
	// 初始化上下文
	InitContext(ptr *NodePtr) error
	// 初始化连接
	Connect(ctx *Context, s Session) error
	// 关闭连接
	Release(ctx *Context) error
	// 校验会话
	ValidSession() error
	// 校验重放攻击
	ValidReplayAttack() error
	// 刷新会话
	TouchSession() error
	// 校验权限
	ValidPermission() error
	// 获取请求头数据
	GetHeader() error
	// 获取请求参数
	GetParams() error
	// 设置响应头格式
	SetContentType(contentType string)
	// 核心代理方法
	Proxy(ptr *NodePtr)
	// 核心绑定路由方法, customize=true自定义不执行默认流程
	Router(pattern string, handle func(ctx *Context) error, customize ... bool)
	// html响应模式
	Html(ctx *Context, view string, data interface{}) error
	// json响应模式
	Json(ctx *Context, data interface{}) error
	// 前置检测方法(业务方法前执行)
	PreHandle(handle func(ctx *Context) error) error
	// 业务执行方法->自定义处理执行方法(业务方法执行后,视图渲染前执行)
	PostHandle(handle func(resp *Response, err error) error, err error) error
	// 最终响应执行方法(视图渲染后执行,可操作资源释放,保存日志等)
	AfterCompletion(handle func(ctx *Context, resp *Response, err error) error, err error) error
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
	Data  interface{} `json:"d"`
	Nonce string      `json:"n"`
	Time  int64       `json:"t"`
	Sign  string      `json:"g"`
}

type RespDto struct {
	Code    int         `json:"c"`
	Data    interface{} `json:"d"`
	Message string      `json:"m"`
	Nonce   string      `json:"n"`
	Time    int64       `json:"t"`
	Sign    string      `json:"g"`
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
	ContentEncoding string
	ContentType     string
	RespEntity      interface{}
	TemplDir        string
	RespView        string
}

type OverrideFunc struct {
	GetHeaderFunc       func(ctx *Context) error
	GetParamsFunc       func(ctx *Context) error
	PreHandleFunc       func(ctx *Context) error
	PostHandleFunc      func(resp *Response, err error) error
	AfterCompletionFunc func(ctx *Context, resp *Response, err error) error
	RenderErrorFunc     func(err error) error
	LoginFunc           func(ctx *Context) error
	LogoutFunc          func(ctx *Context) error
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
	if !ValidSign(req, d, self.SecretKey()) {
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
func ValidSign(req *ReqDto, data string, key *jwt.SecretKey) bool {
	api_secret_key := key.ApiSecretKey
	secret_key_alg := key.SecretKeyAlg
	token := req.Token
	nonce := req.Nonce
	time := req.Time
	if secret_key_alg == jwt.MD5 {
		key := util.GetApiAccessKeyByMD5(token, api_secret_key)
		sign_str := util.AddStr(token, data, key, nonce, time)
		return req.Sign == util.MD5(sign_str)
	} else if secret_key_alg == jwt.SHA256 {
		key := util.GetApiAccessKeyBySHA256(token, api_secret_key)
		sign_str := util.AddStr(token, data, key, nonce, time)
		return req.Sign == util.SHA256(sign_str)
	}
	return false
}

// 校验签名有效性
func BuidSign(token string, resp *RespDto, key *jwt.SecretKey) error {
	api_secret_key := key.ApiSecretKey
	secret_key_alg := key.SecretKeyAlg
	code := resp.Code
	message := resp.Message
	nonce := resp.Nonce
	time := resp.Time
	data := ""
	if resp.Data != nil {
		if d, err := util.JsonMarshal(resp.Data); err != nil {
			return ex.Throw{Code: http.StatusInternalServerError, Msg: "响应数据构建异常"}
		} else if data = util.Base64URLEncode(d); len(data) == 0 {
			return ex.Throw{Code: http.StatusInternalServerError, Msg: "响应数据构建为空"}
		}
		resp.Data = data
	}
	if secret_key_alg == jwt.MD5 {
		key := util.GetApiAccessKeyByMD5(token, api_secret_key)
		sign_str := util.AddStr(token, code, data, key, message, nonce, time)
		resp.Sign = util.MD5(sign_str)
	} else if secret_key_alg == jwt.SHA256 {
		key := util.GetApiAccessKeyBySHA256(token, api_secret_key)
		sign_str := util.AddStr(token, code, data, key, message, nonce, time)
		resp.Sign = util.SHA256(sign_str)
	}
	return nil
}

func (ctx *Context) SendText(data string) {
	ctx.Output.Header().Set("Content-Type", TEXT_PLAIN)
	fmt.Println(ctx.Output.Write(util.Str2Bytes(data)))
}

func (ctx *Context) SendJson(data interface{}) {
	ctx.Output.Header().Set("Content-Type", APPLICATION_JSON)
	if b, err := util.JsonMarshal(data); err != nil {
		ctx.Output.Write(util.Str2Bytes(""))
	} else {
		fmt.Println(ctx.Output.Write(b))
	}
}
