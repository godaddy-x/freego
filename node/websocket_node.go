package node

import (
	"github.com/godaddy-x/freego/component/log"
	"github.com/godaddy-x/freego/ex"
	"github.com/godaddy-x/freego/util"
	"github.com/gorilla/websocket"
	"net/http"
	"strings"
	"sync"
)

type WebsocketNode struct {
	HookNode
	WSClient  *WSClient
	WSManager *WSManager
}

func (self *WebsocketNode) GetHeader() error {
	return nil
}

func (self *WebsocketNode) GetParams() error {
	return nil
}

func (self *WebsocketNode) InitContext(ptr *NodePtr) error {
	output := ptr.Output
	input := ptr.Input
	node := ptr.Node.(*WebsocketNode)
	if self.OverrideFunc == nil {
		node.OverrideFunc = &OverrideFunc{}
	} else {
		node.OverrideFunc = self.OverrideFunc
	}
	if self.WSManager == nil {
		return ex.Try{Code: http.StatusInternalServerError, Msg: "WS管理器尚未初始化"}
	}
	node.SessionAware = self.SessionAware
	node.CacheAware = self.CacheAware
	node.WSManager = self.WSManager
	node.Context = &Context{
		Host:      util.GetClientIp(input),
		Port:      self.Context.Port,
		Style:     WEBSOCKET,
		Method:    ptr.Pattern,
		Anonymous: ptr.Anonymous,
		Version:   self.Context.Version,
		Response: &Response{
			ContentEncoding: UTF8,
			ContentType:     APPLICATION_JSON,
		},
		Input:    input,
		Output:   output,
		Security: self.Context.Security,
	}
	return nil
}

func (self *WebsocketNode) InitWebsocket(ptr *NodePtr) error {
	if ws, err := self.newWSClient(self.Context.Output, self.Context.Input, util.GetSnowFlakeStrID(), self.wsReadHandle, ptr.Handle); err != nil {
		return ex.Try{Code: http.StatusInternalServerError, Msg: "建立websocket连接失败", Err: err}
	} else {
		self.WSClient = ws
	}
	return nil
}

func (self *WebsocketNode) newWSClient(w http.ResponseWriter, r *http.Request, uid string, rcvd_handle func(c *WSClient, rcvd []byte) error, biz_handle func(ctx *Context) error) (*WSClient, error) {
	conn, err := (&websocket.Upgrader{
		ReadBufferSize:    4096,
		WriteBufferSize:   4096,
		EnableCompression: true,
		CheckOrigin:       func(r *http.Request) bool { return true }}).Upgrade(w, r, nil)
	if err != nil {
		return nil, err
	}
	client := &WSClient{
		id:          uid,
		socket:      conn,
		send:        make(chan WSMessage),
		access:      util.Time(),
		biz_handle:  biz_handle,
		rcvd_handle: rcvd_handle,
	}
	self.WSManager.register <- client
	return client, nil
}

func (self *WebsocketNode) wsReadHandle(c *WSClient, rcvd []byte) error {
	if rcvd == nil || len(rcvd) == 0 {
		return self.RenderError(ex.Try{Code: http.StatusBadRequest, Msg: "获取参数失败"})
	}
	// 1.获取请求数据
	req := &ReqDto{}
	if err := util.JsonUnmarshal(rcvd, req); err != nil {
		return self.RenderError(ex.Try{Code: http.StatusBadRequest, Msg: "参数解析失败", Err: err})
	}
	if err := self.Context.SecurityCheck(req, self.Context.Security().SecretKey); err != nil {
		return err
	}
	self.Context.Params = req
	// false.已有会话 true.会话为空
	state := false
	// 2.判定或校验会话
	if self.Context.Session == nil { // 如无会话则校验以及填充会话,如存在会话则跳过
		if err := self.ValidSession(); err != nil {
			return self.RenderError(err)
		}
		state = true
	} else if !self.Context.Session.IsValid() {
		return self.RenderError(ex.Try{Code: http.StatusUnauthorized, Msg: "会话已失效"})
	} else if self.Context.Session.IsTimeout() {
		return self.RenderError(ex.Try{Code: http.StatusUnauthorized, Msg: "会话已超时"})
	}
	err := func() error {
		// 3.上下文前置检测方法
		if err := self.PreHandle(self.OverrideFunc.PreHandleFunc); err != nil {
			return err
		}
		// 4.执行业务方法
		r1 := c.biz_handle(self.Context) // r1异常格式,建议使用ex模式
		// 5.执行视图控制方法
		r2 := self.PostHandle(self.OverrideFunc.PostHandleFunc, r1)
		// 6.执行释放资源,记录日志方法
		if err := self.AfterCompletion(self.OverrideFunc.AfterCompletionFunc, r2); err != nil {
			return err
		}
		return nil
	}()
	// 7.更新会话有效性
	return self.LastAccessTouch(state, err)
}

func (self *WebsocketNode) ValidSession() error {
	if self.SessionAware == nil {
		return ex.Try{Code: http.StatusInternalServerError, Msg: "会话管理器尚未初始化"}
	}
	accessToken := self.Context.Params.Token
	if len(accessToken) == 0 {
		if !self.Context.Anonymous {
			return ex.Try{Code: http.StatusUnauthorized, Msg: "获取授权令牌失败"}
		}
		return nil
	}
	var sessionId string
	spl := strings.Split(accessToken, ".")
	if len(spl) == 3 {
		sessionId = spl[2]
	} else {
		sessionId = accessToken
	}
	session, err := self.SessionAware.ReadSession(sessionId)
	if err != nil || session == nil {
		return ex.Try{Code: http.StatusUnauthorized, Msg: "获取会话失败", Err: err}
	}
	session.SetHost(self.Context.Host)
	if !session.IsValid() {
		self.SessionAware.DeleteSession(session)
		return ex.Try{Code: http.StatusUnauthorized, Msg: "会话已失效"}
	}
	sub, err := session.Validate(accessToken, self.Context.Security().SecretKey)
	if err != nil {
		return ex.Try{Code: http.StatusUnauthorized, Msg: "会话校验失败或已失效", Err: err}
	}
	if sig, b, err := self.CacheAware.Get(util.AddStr(JWT_SUB_, sub), nil); err != nil {
		return ex.Try{Code: http.StatusInternalServerError, Msg: "会话缓存服务异常"}
	} else if !b || sig != util.SHA256(accessToken) {
		return ex.Try{Code: http.StatusUnauthorized, Msg: "会话已被踢出", Err: err}
	}
	userId, _ := util.StrToInt64(sub)
	self.Context.UserId = userId
	self.Context.Session = session
	return nil
}

func (self *WebsocketNode) TouchSession() error {
	if self.Context == nil || self.Context.Session == nil {
		return nil
	}
	session := self.Context.Session
	if session.IsValid() {
		if err := self.SessionAware.UpdateSession(session); err != nil {
			return ex.Try{Code: http.StatusInternalServerError, Msg: "更新会话失败", Err: err}
		}
	} else {
		if err := self.SessionAware.DeleteSession(session); err != nil {
			return ex.Try{Code: http.StatusInternalServerError, Msg: "删除会话失败", Err: err}
		}
	}
	return nil
}

func (self *WebsocketNode) Proxy(ptr *NodePtr) {
	ob := &WebsocketNode{}
	ptr.Node = ob
	if err := self.InitContext(ptr); err != nil {
		return
	}
	if err := ob.InitWebsocket(ptr); err != nil {
		return
	}
}

func (self *WebsocketNode) LastAccessTouch(state bool, err error) error {
	if state {
		if err := self.TouchSession(); err != nil {
			log.Error(err.Error())
		}
	}
	if err != nil {
		self.RenderError(err)
	}
	return nil
}

func (self *WebsocketNode) PreHandle(handle func(ctx *Context) error) error {
	if handle == nil {
		return nil
	}
	return handle(self.Context)
}

func (self *WebsocketNode) PostHandle(handle func(resp *Response, err error) error, err error) error {
	if handle != nil {
		if err := handle(self.Context.Response, err); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	return self.RenderTo()
}

func (self *WebsocketNode) AfterCompletion(handle func(ctx *Context, resp *Response, err error) error, err error) error {
	var handle_err error
	if handle != nil {
		handle_err = handle(self.Context, self.Context.Response, err)
	}
	if err != nil {
		return err
	}
	if handle_err != nil {
		return handle_err
	}
	return nil
}

func (self *WebsocketNode) RenderError(err error) error {
	if self.OverrideFunc.RenderErrorFunc == nil {
		out := ex.Catch(err)
		self.WSClient.send <- WSMessage{MessageType: websocket.CloseMessage, Content: util.Str2Bytes(out.Error())}
		return nil
	}
	return self.OverrideFunc.RenderErrorFunc(err)
}

func (self *WebsocketNode) RenderTo() error {
	switch self.Context.Response.ContentType {
	case TEXT_HTML:
	case TEXT_PLAIN:
	case APPLICATION_JSON:
		if result, err := util.JsonMarshal(self.Context.Response.RespEntity); err != nil {
			self.sendJsonConvertError(err)
		} else {
			self.WSClient.send <- WSMessage{MessageType: websocket.TextMessage, Content: result}
		}
	default:
		return ex.Try{Code: http.StatusUnsupportedMediaType, Msg: "无效的响应格式"}
	}
	return nil
}

func (self *WebsocketNode) sendJsonConvertError(err error) error {
	out := ex.Try{Code: http.StatusUnsupportedMediaType, Msg: "系统发生未知错误", Err: util.Error("JSON对象转换失败: ", err.Error())}
	return self.RenderError(out)
}

func (self *WebsocketNode) StartServer() {
	self.WSManager = &WSManager{
		register:   make(chan *WSClient),
		unregister: make(chan *WSClient),
		clients:    sync.Map{},
	}
	go self.WSManager.start()
	go func() {
		if err := http.ListenAndServe(util.AddStr(self.Context.Host, ":", self.Context.Port), nil); err != nil {
			panic(err)
		}
	}()
}

func (self *WebsocketNode) Router(pattern string, handle func(ctx *Context) error, anonymous ...bool) {
	if !strings.HasPrefix(pattern, "/") {
		pattern = util.AddStr("/", pattern)
	}
	if len(self.Context.Version) > 0 {
		pattern = util.AddStr("/", self.Context.Version, pattern)
	}
	if self.SessionAware == nil {
		panic("会话服务尚未初始化")
	}
	if self.CacheAware == nil {
		panic("缓存服务尚未初始化")
	}
	http.DefaultServeMux.HandleFunc(pattern, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		anon := true
		if anonymous != nil && len(anonymous) > 0 {
			anon = anonymous[0]
		}
		self.Proxy(
			&NodePtr{
				self,
				r, w, pattern, anon, handle,
			},
		)
	}))
}

func (self *WebsocketNode) Html(ctx *Context, view string, data interface{}) error {
	return nil
}

func (self *WebsocketNode) Json(ctx *Context, data interface{}) error {
	if data == nil {
		data = map[string]interface{}{}
	}
	ctx.Response.ContentEncoding = UTF8
	ctx.Response.ContentType = APPLICATION_JSON
	ctx.Response.RespEntity = data
	return nil
}

func (self *WebsocketNode) SetContentType(contentType string) {
	self.Context.Output.Header().Set("Content-Type", contentType)
}

func (self *WebsocketNode) Connect(ctx *Context, s Session, sub, token string) error {
	if err := self.SessionAware.CreateSession(s); err != nil {
		return err
	}
	ctx.Session = s
	expire, _ := s.GetTimeout()
	if err := self.CacheAware.Put(util.AddStr(JWT_SUB_, sub), util.SHA256(token), int(expire/1000)); err != nil {
		return ex.Try{Code: http.StatusInternalServerError, Msg: "会话缓存服务异常"}
	}
	return nil
}

func (self *WebsocketNode) Release(ctx *Context) error {
	ctx.Session.Stop()
	return nil
}
