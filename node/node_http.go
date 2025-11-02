package node

import (
	"context"
	"fmt"
	"github.com/mailru/easyjson"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/buaazp/fasthttprouter"
	"github.com/godaddy-x/freego/cache"
	"github.com/godaddy-x/freego/ex"
	"github.com/godaddy-x/freego/ormx/sqld"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/utils/crypto"
	"github.com/godaddy-x/freego/utils/jwt"
	"github.com/godaddy-x/freego/zlog"
	"github.com/valyala/fasthttp"
)

var emptyMap = map[string]string{}

type CacheAware func(ds ...string) (cache.Cache, error)

type HttpNode struct {
	HookNode
	mu       sync.Mutex
	ctxPool  sync.Pool
	server   *fasthttp.Server
	listener net.Listener
	cancel   context.CancelFunc
}

type PostHandle func(*Context) error

type ErrorHandle func(ctx *Context, throw ex.Throw) error

func (self *HttpNode) doRequest(handle PostHandle, request *fasthttp.RequestCtx) error {
	ctx := self.ctxPool.Get().(*Context)
	ctx.reset(self.Context, handle, request, self.filters)
	if err := ctx.filterChain.DoFilter(ctx.filterChain, ctx); err != nil {
		self.ctxPool.Put(ctx)
		return err
	}
	self.ctxPool.Put(ctx)
	return nil
}

func (self *HttpNode) proxy(handle PostHandle, ctx *fasthttp.RequestCtx) {
	if err := self.doRequest(handle, ctx); err != nil {
		zlog.Error("doRequest failed", 0, zlog.AddError(err))
	}
}

var (
	defaultTimeout = 30
)

func (self *HttpNode) StartServerByTimeout(addr string, timeout int) {
	defaultTimeout = timeout
	self.StartServer(addr)
}

func (self *HttpNode) StartServer(addr string) {
	// 防止重复启动
	if self.server != nil {
		zlog.Printf("http server has already been started")
		return
	}

	// 在启动前创建filter chain，确保初始化顺序正确
	fs, err := createFilterChain(self.filters)
	if err != nil {
		panic("http service create filter chain failed")
	}
	self.filters = fs
	if len(self.filters) == 0 {
		panic("filter chain is nil")
	}

	if self.Context.CacheAware != nil {
		zlog.Printf("cache service has been started successful")
	}
	if self.Context.RSA != nil {
		if self.Context.System.enableECC {
			zlog.Printf("ECC certificate service has been started successful")
		} else {
			zlog.Printf("RSA certificate service has been started successful")
		}
	}

	// 创建上下文用于优雅关闭
	_, self.cancel = context.WithCancel(context.Background())

	// 创建服务器实例
	self.server = &fasthttp.Server{
		Handler:            self.Context.router.Handler,
		MaxRequestBodySize: MAX_BODY_LEN,
	}

	// 启动服务器
	self.listener = NewGracefulListener(addr, time.Second*time.Duration(defaultTimeout))
	go func() {
		zlog.Printf("http【%s】service has been started successful", addr)
		if err := self.server.Serve(self.listener); err != nil {
			// 忽略已关闭的错误
			if err.Error() != "use of closed network connection" {
				zlog.Error("server serve failed", 0, zlog.AddError(err))
			}
		}
	}()

	// 监听系统信号实现优雅关闭
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)
	<-quit
	zlog.Printf("http server is shutting down...")

	// 关闭数据库连接
	sqld.MysqlClose()

	// 关闭服务器（fasthttp的Shutdown已实现优雅关闭）
	if err := self.server.Shutdown(); err != nil {
		zlog.Error("server shutdown failed", 0, zlog.AddError(err))
	}

	// 关闭listener（确保所有连接正确关闭）
	if self.listener != nil {
		if err := self.listener.Close(); err != nil {
			zlog.Error("listener close failed", 0, zlog.AddError(err))
		}
	}

	// 取消上下文
	if self.cancel != nil {
		self.cancel()
	}

	zlog.Printf("http server has been stopped")
}

func (self *HttpNode) checkContextReady(path string, routerConfig *RouterConfig) {
	self.readyContext()
	self.AddCache(nil)
	self.AddCipher(nil)
	self.addRouterConfig(path, routerConfig)
	self.newRouter()
}

func (self *HttpNode) addRouter(method, path string, handle PostHandle, routerConfig *RouterConfig) {
	self.checkContextReady(path, routerConfig)
	self.Context.router.Handle(method, path, fasthttp.TimeoutHandler(
		func(ctx *fasthttp.RequestCtx) {
			self.proxy(handle, ctx)
		},
		time.Duration(self.Context.System.AcceptTimeout)*time.Second,
		fmt.Sprintf(`{"c":408,"m":"server actively disconnects the client","d":null,"t":%d,"n":"%s","p":0,"s":""}`, utils.UnixMilli(), utils.RandNonce())))
}

func (self *HttpNode) Json(ctx *Context, data interface{}) error {
	return ctx.Json(data)
}

func (self *HttpNode) Empty(ctx *Context) error {
	return ctx.NoBody()
}

func (self *HttpNode) Text(ctx *Context, data string) error {
	return ctx.Text(data)
}

func (self *HttpNode) Bytes(ctx *Context, data []byte) error {
	return ctx.Bytes(data)
}

func (self *HttpNode) AddFilter(object *FilterObject) {
	self.readyContext()
	if object == nil {
		panic("filter object is nil")
	}
	if len(object.Name) == 0 || object.Filter == nil {
		panic("filter object name/filter is nil")
	}
	self.filters = append(self.filters, object)
	zlog.Printf("add filter [%s] successful", object.Name)
}

func (self *HttpNode) createCtxPool() sync.Pool {
	return sync.Pool{New: func() interface{} {
		ctx := &Context{}
		// 设置静态配置（启动时确定）
		ctx.configs = self.Context.configs
		ctx.router = self.Context.router
		ctx.filterChain = &filterChain{filters: self.filters} // 直接设置过滤器列表

		// 预创建可重用对象
		ctx.System = &System{}
		ctx.JsonBody = &JsonBody{}
		ctx.Subject = &jwt.Subject{Header: &jwt.Header{}, Payload: &jwt.Payload{}}
		ctx.Response = &Response{Encoding: UTF8, ContentType: APPLICATION_JSON, ContentEntity: nil}

		// 延迟初始化Storage map（在需要时创建）
		// ctx.Storage = nil // 不预创建，在reset中按需创建

		return ctx
	}}
}

func (self *HttpNode) readyContext() {
	self.mu.Lock()
	defer self.mu.Unlock()
	if self.Context == nil {
		self.Context = &Context{}
		self.Context.configs = &Configs{}
		self.Context.configs.routerConfigs = make(map[string]*RouterConfig)
		self.Context.configs.langConfigs = make(map[string]map[string]string)
		self.Context.configs.jwtConfig = jwt.JwtConfig{}
		self.Context.System = &System{}
		self.ctxPool = self.createCtxPool()
		self.Context.System.enableECC = true
	}
}

func (self *HttpNode) AddCache(cacheAware CacheAware) {
	self.readyContext()
	if self.Context.CacheAware == nil {
		if cacheAware == nil {
			cacheAware = func(ds ...string) (cache.Cache, error) {
				return cache.NewLocalCache(30, 2), nil
			}
		}
		self.Context.CacheAware = cacheAware
	}
}

func (self *HttpNode) AddCipher(cipher crypto.Cipher) {
	self.readyContext()
	if self.Context.RSA == nil {
		if cipher == nil {
			if self.Context.System.enableECC {
				defaultECC := &crypto.EccObj{}
				if err := defaultECC.CreateS256ECC(); err != nil {
					panic("ECC certificate generation failed")
				}
				cipher = defaultECC
			} else {
				defaultRSA := &crypto.RsaObj{}
				if err := defaultRSA.CreateRsa2048(); err != nil {
					panic("RSA certificate generation failed")
				}
				cipher = defaultRSA
			}
		}
		self.Context.RSA = cipher
	}
}

func (self *HttpNode) AddLanguage(langDs, filePath string) error {
	self.readyContext()
	if len(langDs) == 0 || len(filePath) == 0 {
		return nil
	}
	bs, err := utils.ReadFile(filePath)
	if err != nil {
		return err
	}
	return self.AddLanguageByJson(langDs, bs)
}

func (self *HttpNode) AddLanguageByJson(langDs string, bs []byte) error {
	self.readyContext()
	if !utils.JsonValid(bs) {
		panic("lang json config invalid: " + langDs)
	}
	kv := map[string]string{}
	if err := utils.JsonUnmarshal(bs, &kv); err != nil {
		panic("lang json unmarshal failed: " + err.Error())
	}
	self.Context.configs.langConfigs[langDs] = kv
	if len(self.Context.configs.defaultLang) == 0 {
		self.Context.configs.defaultLang = langDs
	}
	zlog.Printf("add lang [%s] successful", langDs)
	return nil
}

func (self *HttpNode) AddRoleRealm(roleRealm func(ctx *Context, onlyRole bool) (*Permission, error)) error {
	self.readyContext()
	self.Context.roleRealm = roleRealm
	zlog.Printf("add permission realm successful")
	return nil
}

func (self *HttpNode) AddErrorHandle(errorHandle func(ctx *Context, throw ex.Throw) error) error {
	self.readyContext()
	self.Context.errorHandle = errorHandle
	zlog.Printf("add error handle successful")
	return nil
}

func (self *HttpNode) addRouterConfig(path string, routerConfig *RouterConfig) {
	self.readyContext()
	if routerConfig == nil {
		routerConfig = &RouterConfig{}
	}
	if _, b := self.Context.configs.routerConfigs[path]; !b {
		self.Context.configs.routerConfigs[path] = routerConfig
	}
}

func (self *HttpNode) newRouter() {
	self.readyContext()
	if self.Context.System.AcceptTimeout <= 0 {
		self.Context.System.AcceptTimeout = 60
	}
	if self.Context.router == nil {
		self.Context.router = fasthttprouter.New()
	}
}

func (self *HttpNode) AddJwtConfig(config jwt.JwtConfig) {
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

// EnableECC default: true
func (self *HttpNode) EnableECC(enable bool) {
	self.readyContext()
	self.Context.System.enableECC = enable
}

func (self *HttpNode) SetSystem(name, version string) {
	self.readyContext()
	self.Context.System.Name = name
	self.Context.System.Version = version
}

func (self *HttpNode) ClearFilterChain() {
	for k, _ := range filterMap {
		delete(filterMap, k)
	}
}

func ErrorMsgToLang(ctx *Context, msg string, args ...string) string {
	if len(msg) == 0 {
		return msg
	}
	lang := ctx.ClientLanguage()
	if len(lang) == 0 {
		if len(ctx.configs.defaultLang) == 0 {
			return msg
		}
		lang = ctx.configs.defaultLang
	}
	langKV, b := ctx.configs.langConfigs[lang]
	if !b || len(langKV) == 0 {
		if len(ctx.configs.defaultLang) == 0 {
			return msg
		}
		langKV = ctx.configs.langConfigs[ctx.configs.defaultLang]
		if len(langKV) == 0 {
			return msg
		}
	}
	find := utils.SPEL.FindAllStringSubmatch(msg, -1)
	if len(find) == 0 {
		return msg
	}
	for _, v := range find {
		if len(v) != 2 {
			continue
		}
		kv, b := langKV[v[1]]
		if !b || len(kv) == 0 {
			continue
		}
		fill := kv
		for i, arg := range args {
			fill = strings.ReplaceAll(fill, utils.AddStr("$", i+1), arg)
		}
		msg = strings.ReplaceAll(msg, v[0], fill)
	}
	return msg
}

func defaultRenderError(ctx *Context, err error) error {
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
		Message: ErrorMsgToLang(ctx, out.Msg, out.Arg...),
		Time:    utils.UnixSecond(),
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
	if ctx.RouterConfig == nil {
		ctx.Response.StatusCode = 400
		ctx.Response.ContentType = TEXT_PLAIN
		ctx.Response.ContentEntityByte.Write(utils.Str2Bytes(resp.Message))
		return nil
	}
	if ctx.RouterConfig.Guest {
		if out.Code <= 600 {
			ctx.Response.StatusCode = out.Code
		}
		ctx.Response.ContentType = TEXT_PLAIN
		ctx.Response.ContentEntityByte.Write(utils.Str2Bytes(resp.Message))
		return nil
	}
	result, err := utils.JsonMarshal(resp)
	if err != nil {
		ctx.Response.ContentType = TEXT_PLAIN
		ctx.Response.ContentEntityByte.Write(utils.Str2Bytes(err.Error()))
		return nil
	}
	ctx.Response.ContentType = APPLICATION_JSON
	ctx.Response.ContentEntityByte.Write(result)
	return nil
}

func defaultRenderTo(ctx *Context) error {
	ctx.RequestCtx.SetContentType(ctx.Response.ContentType)
	if ctx.Response.StatusCode == 0 {
		ctx.RequestCtx.SetStatusCode(http.StatusOK)
	} else {
		ctx.RequestCtx.SetStatusCode(ctx.Response.StatusCode)
	}
	if _, err := ctx.RequestCtx.Write(ctx.Response.ContentEntityByte.Bytes()); err != nil {
		zlog.Error("response failed", 0, zlog.AddError(err))
	}
	return nil
}

func defaultRenderPre(ctx *Context) error {
	routerConfig, _ := ctx.configs.routerConfigs[ctx.Path]
	switch ctx.Response.ContentType {
	case TEXT_PLAIN:
		content := ctx.Response.ContentEntity
		if v, b := content.(string); b {
			ctx.Response.ContentEntityByte.Write(utils.Str2Bytes(v))
		} else if v, b := content.([]byte); b {
			ctx.Response.ContentEntityByte.Write(v)
		} else {
			ctx.Response.ContentEntityByte.Write(utils.Str2Bytes(""))
		}
	case APPLICATION_JSON:
		if ctx.Response.ContentEntity == nil {
			return ex.Throw{Code: http.StatusInternalServerError, Msg: "response ContentEntity is nil"}
		}
		if routerConfig.Guest {
			if result, err := utils.JsonMarshal(ctx.Response.ContentEntity); err != nil {
				return ex.Throw{Code: http.StatusInternalServerError, Msg: "response JSON data failed", Err: err}
			} else {
				ctx.Response.ContentEntityByte.Write(result)
			}
			break
		}
		var err error
		var data []byte
		if v, b := ctx.Response.ContentEntity.([]byte); b {
			data = v
		} else if v, b := ctx.Response.ContentEntity.(string); b {
			data = utils.Str2Bytes(v)
		} else {
			data, err = utils.JsonMarshal(ctx.Response.ContentEntity)
			if err != nil {
				return ex.Throw{Code: http.StatusInternalServerError, Msg: "response conversion JSON failed", Err: err}
			}
		}
		resp := &JsonResp{
			Code:  http.StatusOK,
			Time:  utils.UnixSecond(),
			Nonce: utils.RandNonce(),
		}
		var key string
		if routerConfig.UseRSA { // 非登录状态响应
			if ctx.JsonBody.Plan == 2 {
				v := ctx.GetStorage(RandomCode)
				if v == nil {
					return ex.Throw{Msg: "encryption random code is nil"}
				}
				resp.Plan = 2
				key, _ = v.(string)
				resp.Data, err = utils.AesGCMEncryptWithAAD(data, key, utils.AddStr(resp.Time, resp.Nonce, resp.Plan, ctx.Path))
				if err != nil {
					return ex.Throw{Code: http.StatusInternalServerError, Msg: "AES encryption response data failed", Err: err}
				}
				ctx.DelStorage(RandomCode)
			} else {
				return ex.Throw{Msg: "anonymous response plan invalid"}
			}
		} else if routerConfig.AesResponse {
			resp.Plan = 1
			resp.Data, err = utils.AesGCMEncryptWithAAD(data, ctx.GetTokenSecret(), utils.AddStr(resp.Time, resp.Nonce, resp.Plan, ctx.Path))
			if err != nil {
				return ex.Throw{Code: http.StatusInternalServerError, Msg: "AES encryption response data failed", Err: err}
			}
		} else {
			resp.Data = utils.Base64Encode(data)
		}
		resp.Sign = ctx.GetHmac256Sign(resp.Data, resp.Nonce, resp.Time, resp.Plan, key)
		if result, err := easyjson.Marshal(resp); err != nil {
			return ex.Throw{Code: http.StatusInternalServerError, Msg: "response JSON data failed", Err: err}
		} else {
			ctx.Response.ContentEntityByte.Write(result)
		}
	case NO_BODY:
		return nil
	default:
		return ex.Throw{Code: http.StatusUnsupportedMediaType, Msg: "invalid response ContentType"}
	}
	return nil
}

func (self *HttpNode) POST(path string, handle func(ctx *Context) error, routerConfig *RouterConfig) {
	self.addRouter(POST, path, handle, routerConfig)
}

func (self *HttpNode) GET(path string, handle func(ctx *Context) error, routerConfig *RouterConfig) {
	self.addRouter(GET, path, handle, routerConfig)
}

func (self *HttpNode) DELETE(path string, handle func(ctx *Context) error, routerConfig *RouterConfig) {
	self.addRouter(DELETE, path, handle, routerConfig)
}

func (self *HttpNode) PUT(path string, handle func(ctx *Context) error, routerConfig *RouterConfig) {
	self.addRouter(PUT, path, handle, routerConfig)
}

func (self *HttpNode) PATCH(path string, handle func(ctx *Context) error, routerConfig *RouterConfig) {
	self.addRouter(PATCH, path, handle, routerConfig)
}

func (self *HttpNode) OPTIONS(path string, handle func(ctx *Context) error, routerConfig *RouterConfig) {
	self.addRouter(OPTIONS, path, handle, routerConfig)
}

func (self *HttpNode) HEAD(path string, handle func(ctx *Context) error, routerConfig *RouterConfig) {
	self.addRouter(HEAD, path, handle, routerConfig)
}
