package node

import (
	"github.com/buaazp/fasthttprouter"
	"github.com/godaddy-x/freego/ex"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/utils/gorsa"
	"github.com/godaddy-x/freego/utils/jwt"
	"github.com/godaddy-x/freego/zlog"
	"github.com/valyala/fasthttp"
	"net/http"
	"strings"
)

var routerConfigs = make(map[string]*RouterConfig)

type HttpNode struct {
	HookNode
}

func (self *HttpNode) readParams() error {
	agent := utils.Bytes2Str(self.Context.RequestCtx.Request.Header.Peek("User-Agent"))
	if utils.HasStr(agent, "Android") || utils.HasStr(agent, "Adr") {
		self.Context.Device = ANDROID
	} else if utils.HasStr(agent, "iPad") || utils.HasStr(agent, "iPhone") || utils.HasStr(agent, "Mac") {
		self.Context.Device = IOS
	} else {
		self.Context.Device = WEB
	}
	method := utils.Bytes2Str(self.Context.RequestCtx.Method())
	if method != POST {
		return nil
	}
	self.Context.Token = utils.Bytes2Str(self.Context.RequestCtx.Request.Header.Peek(Authorization))
	body := self.Context.RequestCtx.PostBody()
	if len(body) == 0 {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "body parameters is nil"}
	}
	if len(body) > (MAX_VALUE_LEN * 5) {
		return ex.Throw{Code: http.StatusLengthRequired, Msg: "body parameters length is too long"}
	}
	req := &JsonBody{}
	if err := utils.JsonUnmarshal(body, req); err != nil {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "body parameters JSON parsing failed", Err: err}
	}
	if err := self.validJsonBody(req); err != nil { // TODO important
		return err
	}
	return nil
}

func (self *HttpNode) validJsonBody(req *JsonBody) error {
	d, b := req.Data.(string)
	if !b || len(d) == 0 {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request data is nil"}
	}
	if !utils.CheckInt64(req.Plan, 0, 1, 2) {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request plan invalid"}
	}
	if !utils.CheckLen(req.Nonce, 8, 32) {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request nonce invalid"}
	}
	if req.Time <= 0 {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request time must be > 0"}
	}
	if utils.MathAbs(utils.TimeSecond()-req.Time) > 3000 { // 判断绝对时间差超过5分钟
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request time invalid"}
	}
	if self.Context.RouterConfig.AesRequest && req.Plan != 1 {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request parameters must use AES encryption"}
	}
	if self.Context.RouterConfig.Login && req.Plan != 2 {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request parameters must use RSA encryption"}
	}
	if !utils.CheckStrLen(req.Sign, 32, 64) {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request signature length invalid"}
	}
	var key string
	if self.Context.RouterConfig.Login {
		key = self.Context.ServerTLS.PubkeyBase64
	}
	if self.Context.GetHmac256Sign(d, req.Nonce, req.Time, req.Plan, key) != req.Sign {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request signature invalid"}
	}
	data := make(map[string]interface{}, 0)
	if req.Plan == 1 && !self.Context.RouterConfig.Login { // AES
		dec, err := utils.AesDecrypt(d, self.Context.GetTokenSecret(), utils.AddStr(req.Nonce, req.Time))
		if err != nil {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "AES failed to parse data", Err: err}
		}
		if err := utils.JsonUnmarshal(utils.Str2Bytes(dec), &data); err != nil {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "parameter JSON parsing failed"}
		}
	} else if req.Plan == 2 && self.Context.RouterConfig.Login { // RSA
		//a := "To0IaxMwcWbX2CsQRv6jmUcPiNcPHn-73708NG8n99WAr5AS3ry7zEBtNRcDUuqhMjHS6NbQQrBOGVMKCfA1Mig2cgCh4wSq50p4omyAExEf1mDDA4bRo2_yPLCDBp63ERC3FJSJY_7ru07darWH6sZbymLigEjA4CWrpxmBQGKkr0gs6nYPIZg3eMuJj_RYmoIPYQtBU5BdPpKqPvtRWOAJBMtZbpSrxDBcCoA_0m3MbNYC4vvb1ivkABp_RXT_SlQqr9IEOqyBQpWpm5FBsoMZkXnMxFBhL1syaZwTK5Fr6Vj-85D0UsTXVPJmdLOBSirlJTgHLPKMMh70PxlEGQ=="
		dec, err := self.Context.ServerTLS.Decrypt(d)
		if err != nil {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "server private-key decrypt failed", Err: err}
		}
		if len(dec) == 0 {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "server private-key decrypt data is nil", Err: err}
		}
		if err := utils.JsonUnmarshal(utils.Str2Bytes(dec), &data); err != nil {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "parameter JSON parsing failed"}
		}
		pubkey, b := data[CLIENT_PUBKEY]
		if !b {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "client public-key not found"}
		}
		pubkey_v, b := pubkey.(string)
		if !b {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "client public-key not string type"}
		}
		if len(pubkey_v) != 24 {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "client public-key length invalid"}
		}
		delete(data, CLIENT_PUBKEY)
		self.Context.AddStorage(CLIENT_PUBKEY, pubkey_v)
	} else if req.Plan == 0 && !self.Context.RouterConfig.Login && !self.Context.RouterConfig.AesRequest {
		if err := utils.ParseJsonBase64(d, &data); err != nil {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "parameter JSON parsing failed"}
		}
	} else {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request parameters plan invalid"}
	}
	req.Data = data
	self.Context.JsonBody = req
	return nil
}

func (self *HttpNode) doRequest(handle func(ctx *Context) error, ctx *fasthttp.RequestCtx) error {
	ob := &HttpNode{}
	ob.SessionAware = self.SessionAware
	ob.CacheAware = self.CacheAware
	ob.Context = &Context{
		CreateAt: utils.Time(),
		// Host:         utils.ClientIP(input),
		RequestCtx:   ctx,
		Path:         utils.Bytes2Str(ctx.Path()),
		Response:     &Response{Encoding: UTF8, ContentType: APPLICATION_JSON, ContentEntity: nil, ContentEntityByte: nil},
		RouterConfig: routerConfigs[utils.Bytes2Str(ctx.Path())],
		ServerTLS:    self.Context.ServerTLS,
		PermConfig:   self.Context.PermConfig,
		Storage:      make(map[string]interface{}, 0),
	}
	return doFilterChain(ob, handle)
}

func (self *HttpNode) proxy(handle func(ctx *Context) error, ctx *fasthttp.RequestCtx) {
	if err := self.doRequest(handle, ctx); err != nil {
		zlog.Error("doRequest failed", 0, zlog.AddError(err))
	}
}

func (self *HttpNode) StartServer(address string) {
	go func() {
		if self.CacheAware != nil {
			zlog.Printf("cache service has been started successful")
		}
		if self.Context.ServerTLS != nil {
			zlog.Printf("RSA certificate service has been started successful")
		}
		if err := createFilterChain(); err != nil {
			zlog.Error("http service create filter chain failed", 0, zlog.AddError(err))
			return
		}
		zlog.Printf("http【%s】service has been started successful", address)
		if err := fasthttp.ListenAndServe(address, self.router.Handler); err != nil {
			zlog.Error("http service init failed", 0, zlog.AddError(err))
		}
	}()
	select {}
}

func (self *HttpNode) Router(method, path string, handle func(ctx *Context) error, routerConfig *RouterConfig) {
	if !utils.CheckStr(method, GET, POST, DELETE, PUT, PATCH, OPTIONS, HEAD) {
		panic("http method invalid")
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if self.CacheAware == nil {
		zlog.Error("cache service hasn't been initialized", 0)
		return
	}
	if self.Context == nil {
		self.Context = &Context{}
	}
	if self.Context.ServerTLS == nil {
		tls := &gorsa.RsaObj{}
		if err := tls.CreateRsa1024(); err != nil {
			zlog.Error("RSA certificate generation failed", 0)
			return
		}
		self.Context.ServerTLS = tls
	}
	if routerConfig == nil {
		routerConfig = &RouterConfig{}
	}
	if _, b := routerConfigs[path]; !b {
		routerConfigs[path] = routerConfig
	}
	if self.router == nil {
		self.router = fasthttprouter.New()
	}
	self.router.Handle(method, path, func(ctx *fasthttp.RequestCtx) {
		self.proxy(handle, ctx)
	})
}

func (self *HttpNode) Json(ctx *Context, data interface{}) error {
	if data == nil {
		data = map[string]string{}
	}
	ctx.Response = &Response{Encoding: UTF8, ContentType: APPLICATION_JSON, ContentEntity: data, ContentEntityByte: nil}
	return nil
}

func (self *HttpNode) Text(ctx *Context, data string) error {
	ctx.Response = &Response{Encoding: UTF8, ContentType: TEXT_PLAIN, ContentEntity: data, ContentEntityByte: nil}
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

func (self *HttpNode) GetJwtConfig() jwt.JwtConfig {
	return jwtConfig
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
		Time:    utils.Time(),
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
	if ctx.RouterConfig.Original {
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
	ctx.RequestCtx.Write(ctx.Response.ContentEntityByte)
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
		if routerConfig.Original {
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
			Time: utils.Time(),
		}
		if ctx.JsonBody == nil || len(ctx.JsonBody.Nonce) == 0 {
			resp.Nonce = utils.RandNonce()
		} else {
			resp.Nonce = ctx.JsonBody.Nonce
		}
		var key string
		if routerConfig.Login {
			v := ctx.GetStorage(CLIENT_PUBKEY)
			if v == nil {
				return ex.Throw{Msg: "encryption pubkey is nil"}
			}
			key, _ = v.(string)
			data, err := utils.AesEncrypt(data, key, key)
			if err != nil {
				return ex.Throw{Code: http.StatusInternalServerError, Msg: "AES encryption response data failed", Err: err}
			}
			resp.Data = data
			resp.Plan = 2
		} else if routerConfig.AesResponse {
			data, err := utils.AesEncrypt(data, ctx.GetTokenSecret(), utils.AddStr(resp.Nonce, resp.Time))
			if err != nil {
				return ex.Throw{Code: http.StatusInternalServerError, Msg: "AES encryption response data failed", Err: err}
			}
			resp.Data = data
			resp.Plan = 1
		} else {
			resp.Data = utils.Base64URLEncode(data)
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
