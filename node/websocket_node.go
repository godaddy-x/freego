package node

import (
	"github.com/godaddy-x/freego/component/log"
	"github.com/godaddy-x/freego/ex"
	"github.com/godaddy-x/freego/util"
	"github.com/gorilla/websocket"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"
)

type WebsocketNode struct {
	HookNode
	Input     *http.Request
	Output    http.ResponseWriter
	WSClient  *WSClient
	WSManager *WSManager
}

func (self *WebsocketNode) GetHeader(input interface{}) error {
	return nil
}

func (self *WebsocketNode) GetParams(input interface{}) error {
	if self.OverrideFunc.GetParamsFunc == nil {
		r := input.(*http.Request)
		r.ParseForm()
		params := map[string]interface{}{}
		if r.Method == GET || r.Method == POST {
			result, err := ioutil.ReadAll(r.Body)
			if err != nil {
				return ex.Try{Code: http.StatusBadRequest, Msg: "获取请求参数失败", Err: err}
			}
			r.Body.Close()
			if len(result) > (MAX_VALUE_LEN * 2) {
				return ex.Try{Code: http.StatusLengthRequired, Msg: "参数值长度溢出: " + util.AnyToStr(len(result))}
			}
			if err := util.JsonToObject(result, &params); err != nil {
				return ex.Try{Code: http.StatusBadRequest, Msg: "请求参数读取失败", Err: err}
			}
			for k, _ := range params {
				if len(k) > MAX_FIELD_LEN {
					return ex.Try{Code: http.StatusLengthRequired, Msg: "参数名长度溢出: " + util.AnyToStr(len(k))}
				}
			}
		} else if r.Method == PUT {
			return ex.Try{Code: http.StatusUnsupportedMediaType, Msg: "暂不支持PUT类型"}
		} else if r.Method == PATCH {
			return ex.Try{Code: http.StatusUnsupportedMediaType, Msg: "暂不支持PATCH类型"}
		} else if r.Method == DELETE {
			return ex.Try{Code: http.StatusUnsupportedMediaType, Msg: "暂不支持DELETE类型"}
		} else {
			return ex.Try{Code: http.StatusUnsupportedMediaType, Msg: "未知的请求类型"}
		}
		self.Context.Params = params
		return nil
	}
	return self.OverrideFunc.GetParamsFunc(input)
}

func (self *WebsocketNode) InitContext(ob, output, input interface{}, pattern string) error {
	w := output.(http.ResponseWriter)
	r := input.(*http.Request)
	o := ob.(*WebsocketNode)
	if self.OverrideFunc == nil {
		o.OverrideFunc = &OverrideFunc{}
	} else {
		o.OverrideFunc = self.OverrideFunc
	}
	o.SessionAware = self.SessionAware
	o.WSManager = self.WSManager
	o.Output = w
	o.Input = r
	o.Context = &Context{
		Host:   util.GetClientIp(r),
		Style:  WEBSOCKET,
		Method: pattern,
		Response: &Response{
			ContentEncoding: UTF8,
			ContentType:     APPLICATION_JSON,
		},
	}
	return nil
}

func (self *WebsocketNode) InitWebsocket(handle func(ctx *Context) error) error {
	if ws, err := self.newWSClient(self.Output, self.Input, util.GetUUID(), self.wsReadHandle, handle); err != nil {
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
		return self.RenderError(ex.Try{Code: http.StatusBadRequest, Msg: "请求数据不能为空"})
	}
	// 1.获取请求数据
	params := map[string]interface{}{}
	if err := util.JsonToObject(rcvd, &params); err != nil {
		return self.RenderError(ex.Try{Code: http.StatusBadRequest, Msg: "请求数据读取失败", Err: err})
	}
	self.Context.Params = params
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
	accessToken := ""
	// 通过参数获取token
	if v := self.Context.GetParam(Global.SessionIdName); v != nil {
		var b bool
		accessToken, b = v.(string)
		if !b {
			return ex.Try{Code: http.StatusUnauthorized, Msg: "授权令牌读取失败"}
		}
	}
	if len(accessToken) == 0 {
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
	session.SetAccessToken(accessToken)
	if !session.IsValid() {
		self.SessionAware.DeleteSession(session)
		return ex.Try{Code: http.StatusUnauthorized, Msg: "会话已失效"}
	}
	if err := session.Validate(); err != nil {
		return ex.Try{Code: http.StatusUnauthorized, Msg: "会话校验失败或已失效", Err: err}
	}
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

func (self *WebsocketNode) Proxy(output, input interface{}, pattern string, handle func(ctx *Context) error) {
	ob := &WebsocketNode{}
	if err := self.InitContext(ob, output, input, pattern); err != nil {
		return
	}
	if err := ob.InitWebsocket(handle); err != nil {
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
	if handle != nil {
		if err := handle(self.Context, self.Context.Response, err); err != nil {
			return err
		}
	} else if err != nil {
		return err
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
		if result, err := util.ObjectToJson(self.Context.Response.RespEntity); err != nil {
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
		if err := http.ListenAndServe(self.Context.Host, nil); err != nil {
			panic(err)
		}
	}()
}

func (self *WebsocketNode) Router(pattern string, handle func(ctx *Context) error) {
	http.DefaultServeMux.HandleFunc(pattern, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		self.Proxy(w, r, pattern, handle)
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
	self.Output.Header().Set("Content-Type", contentType)
}

func (self *WebsocketNode) Connect(ctx *Context, s Session) error {
	if err := self.SessionAware.CreateSession(s); err != nil {
		return err
	}
	ctx.Session = s
	return nil
}

func (self *WebsocketNode) Release(ctx *Context) error {
	ctx.Session.Stop()
	return nil
}
