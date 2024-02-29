package node

import (
	"github.com/godaddy-x/freego/cache"
	rate "github.com/godaddy-x/freego/cache/limiter"
	"github.com/godaddy-x/freego/ex"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/utils/jwt"
	"github.com/godaddy-x/freego/zlog"
	cache2 "github.com/patrickmn/go-cache"
	"golang.org/x/net/websocket"
	"net/http"
	"sync"
	"time"
)

var (
	localCache = cache.NewLocalCache(10, 10)
)

const (
	pingCmd = "ws-health-check"
)

type Handle func(*Context, []byte) (interface{}, error) // 如响应数据为nil则不回复

type WsServer struct {
	Debug bool
	HookNode
	mu sync.RWMutex
	//pool    ConnPool
	ping    int           // 长连接心跳间隔
	max     int           // 连接池总数量
	limiter *rate.Limiter // 每秒限定连接数量
}

type DevConn struct {
	mu    sync.Mutex
	Ready bool
	Dev   string
	Life  int64
	Last  int64
	Ctx   *Context
	Conn  *websocket.Conn
}

func (self *WsServer) readyContext() {
	self.mu.Lock()
	if self.Context == nil {
		self.Context = &Context{}
		self.Context.configs = &Configs{}
		self.Context.configs.routerConfigs = make(map[string]*RouterConfig)
		self.Context.configs.langConfigs = make(map[string]map[string]string)
		self.Context.configs.jwtConfig = jwt.JwtConfig{}
		self.Context.System = &System{}
	}
	self.mu.Unlock()
}

func (self *WsServer) checkContextReady(path string, routerConfig *RouterConfig) {
	self.readyContext()
	self.addRouterConfig(path, routerConfig)
}

func (self *WsServer) AddJwtConfig(config jwt.JwtConfig) {
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

func (self *WsServer) addRouterConfig(path string, routerConfig *RouterConfig) {
	if routerConfig == nil {
		routerConfig = &RouterConfig{}
	}
	if _, b := self.Context.configs.routerConfigs[path]; !b {
		self.Context.configs.routerConfigs[path] = routerConfig
	}
}

func (self *WsServer) writeError(ctx *Context, err error) error {
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
	if err := self.SendMessage(resp, ctx.Subject.GetSub()); err != nil {
		zlog.Error("websocket send error", 0, zlog.AddError(err))
	}
	return nil
}

func (self *WsServer) validBody(ctx *Context, body []byte) bool {
	if body == nil || len(body) == 0 {
		_ = self.writeError(ctx, ex.Throw{Code: http.StatusBadRequest, Msg: "body parameters is nil"})
		return false
	}
	if len(body) > (MAX_VALUE_LEN) {
		_ = self.writeError(ctx, ex.Throw{Code: http.StatusLengthRequired, Msg: "body parameters length is too long"})
		return false
	}
	ctx.JsonBody.Data = utils.GetJsonString(body, "d")
	ctx.JsonBody.Time = utils.GetJsonInt64(body, "t")
	ctx.JsonBody.Nonce = utils.GetJsonString(body, "n")
	ctx.JsonBody.Plan = utils.GetJsonInt64(body, "p")
	ctx.JsonBody.Sign = utils.GetJsonString(body, "s")
	if err := ctx.validJsonBody(); err != nil { // TODO important
		//ctx.writeError(ws, err)
		return false
	}
	return true
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

func createCtx(self *WsServer, path string) *Context {
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
	//ctxNew.postHandle = handle
	//ctxNew.RequestCtx = request
	//ctxNew.Method = utils.Bytes2Str(self.RequestCtx.Method())
	ctxNew.Path = path
	ctxNew.RouterConfig = ctx.configs.routerConfigs[ctxNew.Path]
	ctxNew.postCompleted = false
	ctxNew.filterChain.pos = 0
	return ctxNew
}

func wsRenderTo(ws *websocket.Conn, ctx *Context, data interface{}) error {
	if data == nil {
		return nil
	}
	routerConfig, _ := ctx.configs.routerConfigs[ctx.Path]
	data, err := authReq(ctx.Path, data, ctx.GetTokenSecret(), routerConfig.AesResponse)
	if err != nil {
		return err
	}
	if err := websocket.Message.Send(ws, data); err != nil {
		return ex.Throw{Code: ex.WS_SEND, Msg: "websocket send error", Err: err}
	}
	return nil
}

func getDevConn(subject string) (*DevConn, error) {
	value, b, err := localCache.Get(subject, nil)
	if err != nil {
		return nil, err
	}
	if !b || value == nil {
		return nil, nil
	}
	conn, b := value.(*DevConn)
	if !b {
		return nil, nil
	}
	return conn, nil
}

func (self *WsServer) SendMessage(data interface{}, subject string, dev ...string) error {
	conn, err := getDevConn(subject)
	if err != nil {
		return err
	}
	if conn == nil {
		return nil
	}
	conn.mu.Lock()
	defer conn.mu.Unlock()
	if !conn.Ready {
		return nil
	}
	if err := wsRenderTo(conn.Conn, conn.Ctx, data); err != nil {
		return err
	}
	return nil
}

func (self *WsServer) addConn(conn *websocket.Conn, ctx *Context) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	size, err := localCache.Size()
	if err != nil {
		return err
	}
	if size >= self.max {
		closeConn(conn)
		return utils.Error("conn pool full: ", size)
	}
	sub := ctx.Subject.Payload.Sub
	dev := ctx.Subject.GetDev()
	exp := ctx.Subject.GetExp()
	if len(dev) == 0 {
		dev = "web"
	}

	zlog.Info("websocket client connect success", 0, zlog.String("subject", sub), zlog.String("path", ctx.Path), zlog.String("dev", dev))

	value, err := getDevConn(sub)
	if err != nil {
		return err
	}
	if value == nil {
		_ = localCache.Put(sub, &DevConn{Ready: true, Life: exp, Last: utils.UnixSecond(), Dev: dev, Ctx: ctx, Conn: conn})
		return nil
	}
	value.mu.Lock()
	closeConn(value.Conn) // 如果存在连接对象则先关闭
	value.Last = utils.UnixSecond()
	value.Life = exp
	value.Dev = dev
	value.Ctx = ctx
	value.Conn = conn
	value.Ready = true
	value.mu.Unlock()
	return nil
}

func (self *WsServer) refConn(ctx *Context) error {
	//self.mu.Lock()
	//defer self.mu.Unlock()
	//dev := ctx.Subject.GetDev()
	//if len(dev) == 0 {
	//	dev = "web"
	//}
	//dev = utils.AddStr(dev, "_", ctx.Path)

	value, err := getDevConn(ctx.Subject.Payload.Sub)
	if err != nil {
		return err
	}
	if value == nil {
		return nil
	}
	value.mu.Lock()
	if value.Ready {
		value.Last = utils.UnixSecond()
	}
	value.mu.Unlock()
	return nil
}

func (self *WsServer) NewPool(maxConn, limit, bucket, ping int) {
	if maxConn <= 0 {
		panic("maxConn is nil")
	}
	if limit <= 0 {
		panic("limit is nil")
	}
	if bucket <= 0 {
		panic("bucket is nil")
	}
	if ping <= 0 {
		panic("ping is nil")
	}
	self.mu.Lock()
	defer self.mu.Unlock()
	//if self.pool == nil {
	//	self.pool = make(ConnPool, maxConn)
	//}
	self.max = maxConn
	self.ping = ping

	// 设置每秒放入100个令牌，并允许最大突发50个令牌
	self.limiter = rate.NewLimiter(rate.Limit(limit), bucket)
}

func (self *WsServer) withConnectionLimit(handler websocket.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !self.limiter.Allow() {
			http.Error(w, "limited access", http.StatusServiceUnavailable)
			return
		}
		//if len(self.pool) >= self.max {
		//	http.Error(w, "too many connections", http.StatusServiceUnavailable)
		//	return
		//}
		handler.ServeHTTP(w, r)
	})
}

func (self *WsServer) wsHandler(path string, handle Handle) websocket.Handler {
	return func(ws *websocket.Conn) {

		defer closeConn(ws)

		ctx := createCtx(self, path)
		_ = ctx.readWsToken(ws.Request().Header.Get("Authorization"))

		if len(ctx.Subject.GetRawBytes()) == 0 {
			//_ = self.writeError(ctx, ex.Throw{Code: http.StatusUnauthorized, Msg: "token is nil"})
			return
		}
		if err := ctx.Subject.Verify(utils.Bytes2Str(ctx.Subject.GetRawBytes()), ctx.GetJwtConfig().TokenKey, true); err != nil {
			//_ = self.writeError(ctx, ex.Throw{Code: http.StatusUnauthorized, Msg: "token invalid or expired", Err: err})
			zlog.Error("subject token invalid or expired", 0, zlog.AddError(err), zlog.String("data", utils.Bytes2Str(ctx.Subject.GetRawBytes())))
			return
		}

		if err := self.addConn(ws, ctx); err != nil {
			zlog.Error("add conn error", 0, zlog.AddError(err))
			return
		}

		for {
			// 读取消息
			var body []byte
			err := websocket.Message.Receive(ws, &body)
			if err != nil {
				zlog.Error("receive message error", 0, zlog.AddError(err))
				break
			}

			if self.Debug {
				zlog.Info("websocket receive message", 0, zlog.String("data", string(body)))
			}

			if !self.validBody(ctx, body) {
				if self.Debug {
					zlog.Info("websocket receive message invalid", 0, zlog.String("data", string(body)))
				}
				continue
			}

			dec, b := ctx.JsonBody.Data.([]byte)

			if b && utils.GetJsonString(dec, "healthCheck") == pingCmd {
				_ = self.refConn(ctx)
				continue
			}

			reply, err := handle(ctx, dec)
			if err != nil {
				_ = self.writeError(ctx, err)
				continue
			}

			if self.Debug && reply != nil {
				zlog.Info("websocket reply message", 0, zlog.Any("data", reply))
			}

			// 回复消息
			if err := self.SendMessage(reply, ctx.Subject.GetSub()); err != nil {
				zlog.Error("receive message reply error", 0, zlog.AddError(err))
				break
			}

		}
	}
}

func (self *WsServer) AddRouter(path string, handle Handle, routerConfig *RouterConfig) {
	if handle == nil {
		panic("handle function is nil")
	}

	self.checkContextReady(path, routerConfig)

	http.Handle(path, self.withConnectionLimit(self.wsHandler(path, handle)))
}

func (self *WsServer) GetAllConn() map[string]cache2.Item {
	items, _ := localCache.Values()
	if len(items) == 0 {
		return nil
	}
	return items[0].(map[string]cache2.Item)
}

func (self *WsServer) StartWebsocket(addr string) {
	go func() {
		for {
			time.Sleep(time.Duration(self.ping) * time.Second)
			s := utils.UnixMilli()
			index := 0
			current := utils.UnixSecond()
			items, _ := localCache.Values()
			if len(items) == 0 {
				continue
			}
			var del []string
			item := items[0].(map[string]cache2.Item)
			for k, v := range item {
				conn := v.Object.(*DevConn)
				conn.mu.Lock()
				if current-conn.Last > int64(self.ping*2) || current > conn.Life {
					conn.Ready = false
					closeConn(conn.Conn)
					del = append(del, k)
				}
				index++
				conn.mu.Unlock()
			}
			if len(del) > 0 {
				_ = localCache.Del(del...)
			}
			if self.Debug {
				zlog.Info("websocket check pool", 0, zlog.String("cost", utils.AddStr(utils.UnixMilli()-s, " ms")), zlog.Int("count", index))
			}
		}
	}()
	go func() {
		zlog.Printf("websocket【%s】service has been started successful", addr)
		if err := http.Serve(NewGracefulListener(addr, time.Second*10), nil); err != nil {
			panic(err)
		}
	}()
	select {}
}
