package node

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

)

var (
	Global = GlobalConfig{SessionIdName: "token", SessionTimeout: 1800, SessionSecret: "9r*gploYnA&zMg^U"}
)

type GlobalConfig struct {
	SessionIdName  string // 会话ID名称
	SessionTimeout int    // 会话过期时间/秒
	SessionSecret  string // 会话密钥
}

type ProtocolNode interface {
	// 初始化上下文
	InitContext(ob, output, input interface{}, pattern string) error
	// 初始化连接
	Connect(ctx *Context, s Session) error
	// 关闭连接
	Release(ctx *Context) error
	// 校验会话
	ValidSession() error
	// 刷新会话
	TouchSession() error
	// 获取请求头数据
	GetHeader(input interface{}) error
	// 获取请求参数
	GetParams(input interface{}) error
	// 设置响应头格式
	SetContentType(contentType string)
	// 核心代理方法
	Proxy(output, input interface{}, pattern string, handle func(ctx *Context) error)
	// 核心绑定路由方法
	Router(pattern string, handle func(ctx *Context) error)
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
	// 渲染输出
	RenderTo() error
	// 异常错误响应方法
	RenderError(err error) error
	// 启动服务
	StartServer()
}

type HookNode struct {
	Context      *Context
	SessionAware SessionAware
	OverrideFunc *OverrideFunc
}

type Context struct {
	Host     string
	Style    string
	Device   string
	Method   string
	Headers  map[string]string
	Params   map[string]interface{}
	Session  Session
	Response *Response
}

type Response struct {
	ContentEncoding string
	ContentType     string
	RespEntity      interface{}
	TemplDir        string
	RespView        string
}

type OverrideFunc struct {
	GetHeaderFunc       func(input interface{}) error
	GetParamsFunc       func(input interface{}) error
	PreHandleFunc       func(ctx *Context) error
	PostHandleFunc      func(resp *Response, err error) error
	AfterCompletionFunc func(ctx *Context, resp *Response, err error) error
	RenderErrorFunc     func(err error) error
	LoginFunc           func(ctx *Context) error
	LogoutFunc          func(ctx *Context) error
}

func (self *Context) GetParam(k string) interface{} {
	if len(k) == 0 || len(self.Params) == 0 {
		return nil
	}
	if v, b := self.Params[k]; b {
		return v
	}
	return nil
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

func (self *Context) IsLogin() bool {
	session := self.Session
	if session != nil && session.IsValid() {
		return true
	}
	return false
}
