package node

import (
	"github.com/godaddy-x/freego/ex"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/utils/jwt"
	"github.com/godaddy-x/freego/zlog"
	"golang.org/x/net/websocket"
	"net/http"
	"sync"
	"time"
)

type ConnPool map[string]map[string]*DevConn

type WsNode struct {
	HookNode
	mu   sync.RWMutex
	pool ConnPool
}

type DevConn struct {
	Life int64
	Conn *websocket.Conn
}

func (self *WsNode) readyContext() {
	self.mu.Lock()
	defer self.mu.Unlock()
	if self.Context == nil {
		self.Context = &Context{}
		self.Context.configs = &Configs{}
		self.Context.configs.routerConfigs = make(map[string]*RouterConfig)
		self.Context.configs.langConfigs = make(map[string]map[string]string)
		self.Context.configs.jwtConfig = jwt.JwtConfig{}
		self.Context.System = &System{}
	}
}

func (self *WsNode) checkContextReady(path string, routerConfig *RouterConfig) {
	self.readyContext()
	self.addRouterConfig(path, routerConfig)
}

func (self *WsNode) AddJwtConfig(config jwt.JwtConfig) {
	self.readyContext()
	if len(config.TokenKey) == 0 {
		panic("jwt config key is nil")
	}
	if config.TokenExp < 0 {
		panic("jwt config exp invalid")
	}
	self.Context.configs.jwtConfig.TokenAlg = config.TokenAlg
	self.Context.configs.jwtConfig.TokenTyp = config.TokenTyp
	self.Context.configs.jwtConfig.TokenKey = config.TokenKey
	self.Context.configs.jwtConfig.TokenExp = config.TokenExp
}

func (self *WsNode) addRouterConfig(path string, routerConfig *RouterConfig) {
	if routerConfig == nil {
		routerConfig = &RouterConfig{}
	}
	if _, b := self.Context.configs.routerConfigs[path]; !b {
		self.Context.configs.routerConfigs[path] = routerConfig
	}
}

func (self *Context) readWsToken(auth string) error {
	self.Subject.ResetTokenBytes(utils.Str2Bytes(auth))
	return nil
}

func (self *Context) readWsBody(body []byte) error {
	if body == nil || len(body) == 0 {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "body parameters is nil"}
	}
	if len(body) > (MAX_VALUE_LEN) {
		return ex.Throw{Code: http.StatusLengthRequired, Msg: "body parameters length is too long"}
	}
	self.JsonBody.Data = utils.GetJsonString(body, "d")
	self.JsonBody.Time = utils.GetJsonInt64(body, "t")
	self.JsonBody.Nonce = utils.GetJsonString(body, "n")
	self.JsonBody.Plan = utils.GetJsonInt64(body, "p")
	self.JsonBody.Sign = utils.GetJsonString(body, "s")
	if err := self.validJsonBody(); err != nil { // TODO important
		return err
	}
	return nil
}

func (ctx *Context) writeError(ws *websocket.Conn, err error) error {
	if err == nil {
		return nil
	}
	out := ex.Catch(err)
	if ctx.errorHandle != nil {
		throw, ok := err.(ex.Throw)
		if !ok {
			throw = ex.Throw{Code: out.Code, Msg: out.Msg, Err: err, Arg: out.Arg}
		}
		if err = ctx.errorHandle(ctx, throw); err != nil {
			zlog.Error("response error handle failed", 0, zlog.AddError(err))
		}
	}
	resp := &JsonResp{
		Code:    out.Code,
		Message: out.Msg,
		Time:    utils.UnixMilli(),
	}
	if !ctx.Authenticated() {
		resp.Nonce = utils.RandNonce()
	} else {
		if ctx.JsonBody == nil || len(ctx.JsonBody.Nonce) == 0 {
			resp.Nonce = utils.RandNonce()
		} else {
			resp.Nonce = ctx.JsonBody.Nonce
		}
	}
	if ctx.RouterConfig.Guest {
		if out.Code <= 600 {
			ctx.Response.StatusCode = out.Code
		}
		return nil
	}
	if resp.Code == 0 {
		resp.Code = ex.BIZ
	}
	result, _ := utils.JsonMarshal(resp)
	if err := websocket.Message.Send(ws, result); err != nil {
		zlog.Error("websocket send error", 0, zlog.AddError(err))
	}
	return nil
}

func createCtx(self *WsNode, path string, handle PostHandle) *Context {
	ctx := self.Context
	ctxNew := &Context{}
	ctxNew.configs = self.Context.configs
	ctxNew.filterChain = &filterChain{}
	ctxNew.System = &System{}
	ctxNew.JsonBody = &JsonBody{}
	ctxNew.Subject = &jwt.Subject{Header: &jwt.Header{}, Payload: &jwt.Payload{}}
	ctxNew.Response = &Response{Encoding: UTF8, ContentType: APPLICATION_JSON, ContentEntity: nil}
	ctxNew.Storage = map[string]interface{}{}
	if ctxNew.CacheAware == nil {
		ctxNew.CacheAware = ctx.CacheAware
	}
	if ctxNew.RSA == nil {
		ctxNew.RSA = ctx.RSA
	}
	if ctxNew.roleRealm == nil {
		ctxNew.roleRealm = ctx.roleRealm
	}
	if ctxNew.errorHandle == nil {
		ctxNew.errorHandle = ctx.errorHandle
	}
	ctxNew.System = ctx.System
	ctxNew.postHandle = handle
	//ctxNew.RequestCtx = request
	//ctxNew.Method = utils.Bytes2Str(self.RequestCtx.Method())
	ctxNew.Path = path
	ctxNew.RouterConfig = ctx.configs.routerConfigs[ctxNew.Path]
	ctxNew.postCompleted = false
	ctxNew.filterChain.pos = 0
	return ctxNew
}

func closeConn(ws *websocket.Conn) {
	defer func() {
		if err := recover(); err != nil {
			zlog.Error("ws close panic error", 0, zlog.Any("error", err))
		}
	}()
	if err := ws.Close(); err != nil {
		zlog.Error("ws close error", 0, zlog.AddError(err))
	}
}

func wsRenderTo(ws *websocket.Conn, msg []byte) error {
	if err := websocket.Message.Send(ws, msg); err != nil {
		return ex.Throw{Code: ex.WS_SEND, Msg: "websocket send error", Err: err}
	}
	return nil
}

func wsRenderPre(ws *websocket.Conn, ctx *Context) error {
	routerConfig, _ := ctx.configs.routerConfigs[ctx.Path]
	switch ctx.Response.ContentType {
	case TEXT_PLAIN:
		content := ctx.Response.ContentEntity
		if v, b := content.(string); b {
			return wsRenderTo(ws, utils.Str2Bytes(v))
		} else {
			return wsRenderTo(ws, utils.Str2Bytes(""))
		}
	case APPLICATION_JSON:
		if ctx.Response.ContentEntity == nil {
			return ex.Throw{Code: http.StatusInternalServerError, Msg: "response ContentEntity is nil"}
		}
		if routerConfig.Guest {
			if result, err := utils.JsonMarshal(ctx.Response.ContentEntity); err != nil {
				return ex.Throw{Code: http.StatusInternalServerError, Msg: "response JSON data failed", Err: err}
			} else {
				return wsRenderTo(ws, result)
			}
			break
		}
		data, err := utils.JsonMarshal(ctx.Response.ContentEntity)
		if err != nil {
			return ex.Throw{Code: http.StatusInternalServerError, Msg: "response conversion JSON failed", Err: err}
		}
		resp := &JsonResp{
			Code: http.StatusOK,
			Time: utils.UnixMilli(),
		}
		if ctx.JsonBody == nil || len(ctx.JsonBody.Nonce) == 0 {
			resp.Nonce = utils.RandNonce()
		} else {
			resp.Nonce = ctx.JsonBody.Nonce
		}
		var key string
		if routerConfig.UseRSA || routerConfig.UseHAX { // 非登录状态响应
			if ctx.JsonBody.Plan == 2 {
				v := ctx.GetStorage(RandomCode)
				if v == nil {
					return ex.Throw{Msg: "encryption random code is nil"}
				}
				key, _ = v.(string)
				data, err := utils.AesEncrypt(data, key, key)
				if err != nil {
					return ex.Throw{Code: http.StatusInternalServerError, Msg: "AES encryption response data failed", Err: err}
				}
				resp.Data = data
				resp.Plan = 2
				ctx.DelStorage(RandomCode)
			} else if ctx.JsonBody.Plan == 3 {
				resp.Data = utils.Base64Encode(data)
				_, key = ctx.RSA.GetPublicKey()
				resp.Plan = 3
			} else {
				return ex.Throw{Msg: "anonymous response plan invalid"}
			}
		} else if routerConfig.AesResponse {
			data, err := utils.AesEncrypt(data, ctx.GetTokenSecret(), utils.AddStr(resp.Nonce, resp.Time))
			if err != nil {
				return ex.Throw{Code: http.StatusInternalServerError, Msg: "AES encryption response data failed", Err: err}
			}
			resp.Data = data
			resp.Plan = 1
		} else {
			resp.Data = utils.Base64Encode(data)
		}
		resp.Sign = ctx.GetHmac256Sign(resp.Data.(string), resp.Nonce, resp.Time, resp.Plan, key)
		if result, err := utils.JsonMarshal(resp); err != nil {
			return ex.Throw{Code: http.StatusInternalServerError, Msg: "response JSON data failed", Err: err}
		} else {
			return wsRenderTo(ws, result)
		}
	default:
		return ex.Throw{Code: http.StatusUnsupportedMediaType, Msg: "invalid response ContentType"}
	}
	return nil
}

func (self *WsNode) addConn(conn *websocket.Conn, ctx *Context) error {
	self.mu.Lock()
	defer self.mu.Unlock()
	sub := ctx.Subject.Payload.Sub
	dev := ctx.Subject.GetDev()
	exp := ctx.Subject.GetExp()
	if len(dev) == 0 {
		dev = "web"
	}
	dev = utils.AddStr(dev, "_", ctx.Path)
	if self.pool == nil {
		self.pool = make(ConnPool, 50)
	}

	check, b := self.pool[sub]
	if !b {
		self.pool[sub] = map[string]*DevConn{dev: {Life: exp, Conn: conn}}
		return nil
	}
	devConn, b := check[dev]
	if b {
		closeConn(devConn.Conn) // 如果存在连接对象则先关闭
	}
	check[dev] = &DevConn{Life: exp, Conn: conn}
	return nil
}

func (self *WsNode) NewPool(initSize int) {
	self.mu.Lock()
	defer self.mu.Unlock()
	if self.pool == nil {
		self.pool = make(ConnPool, initSize)
	}
}

func (self *WsNode) AddRouter(path string, handle PostHandle, routerConfig *RouterConfig) {
	if handle == nil {
		panic("handle function is nil")
	}
	self.checkContextReady(path, routerConfig)
	http.Handle(path, websocket.Handler(func(ws *websocket.Conn) {

		defer closeConn(ws)

		ctx := createCtx(self, path, handle)
		ctx.readWsToken(ws.Request().Header.Get("Authorization"))

		if len(ctx.Subject.GetRawBytes()) == 0 {
			ctx.writeError(ws, ex.Throw{Code: http.StatusUnauthorized, Msg: "token is nil"})
			return
		}
		if err := ctx.Subject.Verify(utils.Bytes2Str(ctx.Subject.GetRawBytes()), ctx.GetJwtConfig().TokenKey, true); err != nil {
			ctx.writeError(ws, ex.Throw{Code: http.StatusUnauthorized, Msg: "token invalid or expired", Err: err})
			return
		}

		self.addConn(ws, ctx)

		for {
			// 读取消息
			var msg string
			err := websocket.Message.Receive(ws, &msg)
			if err != nil {
				zlog.Error("receive message error", 0, zlog.AddError(err))
				break
			}

			body := utils.Str2Bytes(msg)

			if body == nil || len(body) == 0 {
				ctx.writeError(ws, ex.Throw{Code: http.StatusBadRequest, Msg: "body parameters is nil"})
				continue
			}
			if len(body) > (MAX_VALUE_LEN) {
				ctx.writeError(ws, ex.Throw{Code: http.StatusLengthRequired, Msg: "body parameters length is too long"})
				continue
			}
			ctx.JsonBody.Data = utils.GetJsonString(body, "d")
			ctx.JsonBody.Time = utils.GetJsonInt64(body, "t")
			ctx.JsonBody.Nonce = utils.GetJsonString(body, "n")
			ctx.JsonBody.Plan = utils.GetJsonInt64(body, "p")
			ctx.JsonBody.Sign = utils.GetJsonString(body, "s")
			if err := ctx.validJsonBody(); err != nil { // TODO important
				ctx.writeError(ws, err)
				continue
			}

			ctxNew := createCtx(self, path, handle)
			ctxNew.JsonBody = ctx.JsonBody
			ctxNew.Subject = ctx.Subject

			if err := handle(ctxNew); err != nil {
				ctx.writeError(ws, err)
				continue
			}

			//回复消息
			if err := wsRenderPre(ws, ctxNew); err != nil {
				out := ex.Catch(err)
				if out.Code == ex.WS_SEND {
					zlog.Error("websocket render error", 0, zlog.AddError(err))
					break
				}
				continue
			}
		}
	}))
}

func (self *WsNode) StartWebsocket(addr string) {
	go func() {
		//fs, err := createFilterChain(self.filters)
		//if err != nil {
		//	panic("http service create filter chain failed")
		//}
		//self.filters = fs
		//if len(self.filters) == 0 {
		//	panic("filter chain is nil")
		//}
		zlog.Printf("websocket【%s】service has been started successful", addr)
		if err := http.Serve(NewGracefulListener(addr, time.Second*10), nil); err != nil {
			panic(err)
		}
	}()
	select {}
}
