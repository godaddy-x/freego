package node

import (
	"fmt"
	"github.com/buaazp/fasthttprouter"
	"github.com/godaddy-x/freego/cache"
	"github.com/godaddy-x/freego/ex"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/utils/gorsa"
	"github.com/godaddy-x/freego/utils/jwt"
	"github.com/godaddy-x/freego/zlog"
	"github.com/valyala/fasthttp"
	"net/http"
	"sync"
	"time"
)

var localCache cache.Cache
var emptyMap = map[string]string{}
var routerConfigs = make(map[string]*RouterConfig)
var ctxPool = sync.Pool{New: func() interface{} {
	ctx := &Context{}
	ctx.filterChain = &filterChain{}
	ctx.Response = &Response{Encoding: UTF8, ContentType: APPLICATION_JSON, ContentEntity: nil, ContentEntityByte: nil}
	return ctx
}}

type CacheAware func(ds ...string) (cache.Cache, error)

type HttpNode struct {
	HookNode
}

type PostHandle func(*Context) error

func (self *HttpNode) doRequest(handle PostHandle, request *fasthttp.RequestCtx) error {
	ctx := ctxPool.Get().(*Context)
	ctx.reset(self.Context, handle, request)
	if err := ctx.filterChain.DoFilter(ctx.filterChain, ctx); err != nil {
		ctxPool.Put(ctx)
		return err
	}
	ctxPool.Put(ctx)
	return nil
}

func (self *HttpNode) proxy(handle PostHandle, ctx *fasthttp.RequestCtx) {
	if err := self.doRequest(handle, ctx); err != nil {
		zlog.Error("doRequest failed", 0, zlog.AddError(err))
	}
}

func (self *HttpNode) StartServer(addr string) {
	go func() {
		if self.Context.CacheAware != nil {
			zlog.Printf("cache service has been started successful")
		}
		if self.Context.RSA != nil {
			zlog.Printf("RSA certificate service has been started successful")
		}
		if err := createFilterChain(); err != nil {
			panic("http service create filter chain failed")
		}
		zlog.Printf("http【%s】service has been started successful", addr)
		if err := fasthttp.Serve(NewGracefulListener(addr, time.Second*10), self.Context.router.Handler); err != nil {
			panic(err)
		}
	}()
	select {}
}

func (self *HttpNode) checkContextReady(path string, routerConfig *RouterConfig) {
	self.readyContext()
	self.AddCache(nil)
	self.AddRSA(nil)
	self.addRouterConfig(path, routerConfig)
	self.newRouter()
}

func (self *HttpNode) addRouter(method, path string, handle PostHandle, routerConfig *RouterConfig) {
	self.checkContextReady(path, routerConfig)
	self.Context.router.Handle(method, path, fasthttp.TimeoutHandler(
		func(ctx *fasthttp.RequestCtx) {
			self.proxy(handle, ctx)
		},
		time.Duration(self.Context.AcceptTimeout)*time.Second,
		fmt.Sprintf(`{"c":408,"m":"server actively disconnects the client","d":null,"t":%d,"n":"%s","p":0,"s":""}`, utils.UnixMilli(), utils.RandNonce())))
}

func (self *HttpNode) Json(ctx *Context, data interface{}) error {
	ctx.Response.ContentType = APPLICATION_JSON
	if data == nil {
		ctx.Response.ContentEntity = emptyMap
	} else {
		ctx.Response.ContentEntity = data
	}
	return nil
}

func (self *HttpNode) Text(ctx *Context, data string) error {
	ctx.Response.ContentType = TEXT_PLAIN
	ctx.Response.ContentEntity = data
	return nil
}

func (self *HttpNode) AddFilter(object *FilterObject) {
	if object == nil {
		panic("filter object is nil")
	}
	if len(object.Name) == 0 || object.Filter == nil {
		panic("filter object name/filter is nil")
	}
	filterMap[object.Name] = object
	zlog.Printf("add filter [%s] successful", object.Name)
}

func (self *HttpNode) readyContext() {
	if self.Context == nil {
		self.Context = &Context{}
	}
}

func (self *HttpNode) AddCache(aware CacheAware) {
	self.readyContext()
	if self.Context.CacheAware == nil {
		if aware == nil {
			if localCache == nil {
				localCache = cache.NewLocalCache(30, 2)
			}
			aware = func(ds ...string) (cache.Cache, error) {
				return localCache, nil
			}
		}
		self.Context.CacheAware = aware
	}
}

func (self *HttpNode) AddRSA(rsa gorsa.RSA) {
	self.readyContext()
	if self.Context.RSA == nil {
		if rsa == nil {
			defaultRSA := &gorsa.RsaObj{}
			if err := defaultRSA.CreateRsa2048(); err != nil {
				panic("RSA certificate generation failed")
			}
			rsa = defaultRSA
		}
		self.Context.RSA = rsa
	}
}

func (self *HttpNode) addRouterConfig(path string, routerConfig *RouterConfig) {
	self.readyContext()
	if routerConfig == nil {
		routerConfig = &RouterConfig{}
	}
	if _, b := routerConfigs[path]; !b {
		routerConfigs[path] = routerConfig
	}
}

func (self *HttpNode) newRouter() {
	self.readyContext()
	if self.Context.AcceptTimeout <= 0 {
		self.Context.AcceptTimeout = 60
	}
	if self.Context.router == nil {
		self.Context.router = fasthttprouter.New()
	}
}

func (self *HttpNode) AddJwtConfig(config jwt.JwtConfig) {
	if len(config.TokenKey) == 0 {
		panic("jwt config key is nil")
	}
	if config.TokenExp < 0 {
		panic("jwt config exp invalid")
	}
	jwtConfig.TokenAlg = config.TokenAlg
	jwtConfig.TokenTyp = config.TokenTyp
	jwtConfig.TokenKey = config.TokenKey
	jwtConfig.TokenExp = config.TokenExp
}

func (self *HttpNode) ClearFilterChain() {
	for k, _ := range filterMap {
		delete(filterMap, k)
	}
}

func defaultRenderError(ctx *Context, err error) error {
	if err == nil {
		return nil
	}
	out := ex.Catch(err)
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
		ctx.Response.ContentType = TEXT_PLAIN
		ctx.Response.ContentEntityByte = utils.Str2Bytes(resp.Message)
		return nil
	}
	result, err := utils.JsonMarshal(resp)
	if err != nil {
		ctx.Response.ContentType = TEXT_PLAIN
		ctx.Response.ContentEntityByte = utils.Str2Bytes(err.Error())
		return nil
	}
	ctx.Response.ContentType = APPLICATION_JSON
	ctx.Response.ContentEntityByte = result
	return nil
}

func defaultRenderTo(ctx *Context) error {
	ctx.RequestCtx.SetContentType(ctx.Response.ContentType)
	if ctx.Response.StatusCode == 0 {
		ctx.RequestCtx.SetStatusCode(http.StatusOK)
	} else {
		ctx.RequestCtx.SetStatusCode(ctx.Response.StatusCode)
	}
	if _, err := ctx.RequestCtx.Write(ctx.Response.ContentEntityByte); err != nil {
		zlog.Error("response failed", 0, zlog.AddError(err))
	}
	return nil
}

func defaultRenderPre(ctx *Context) error {
	routerConfig, _ := routerConfigs[ctx.Path]
	switch ctx.Response.ContentType {
	case TEXT_PLAIN:
		content := ctx.Response.ContentEntity
		if v, b := content.(string); b {
			ctx.Response.ContentEntityByte = utils.Str2Bytes(v)
		} else {
			ctx.Response.ContentEntityByte = utils.Str2Bytes("")
		}
	case APPLICATION_JSON:
		if ctx.Response.ContentEntity == nil {
			return ex.Throw{Code: http.StatusInternalServerError, Msg: "response ContentEntity is nil"}
		}
		if routerConfig.Guest {
			if result, err := utils.JsonMarshal(ctx.Response.ContentEntity); err != nil {
				return ex.Throw{Code: http.StatusInternalServerError, Msg: "response JSON data failed", Err: err}
			} else {
				ctx.Response.ContentEntityByte = result
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
			ctx.Response.ContentEntityByte = result
		}
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
