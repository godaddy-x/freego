package node

import (
	"fmt"
	"github.com/godaddy-x/freego/ex"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/utils/gorsa"
	"github.com/godaddy-x/freego/utils/jwt"
	"github.com/godaddy-x/freego/zlog"
	"io/ioutil"
	"net/http"
	"strings"
	"time"
)

var routerConfigs = make(map[string]*RouterConfig)

type HttpNode struct {
	HookNode
}

func (self *HttpNode) getHeader() error {
	r := self.Context.Input
	headers := map[string]string{}
	if self.Context.RouterConfig.Original {
		if len(r.Header) > MAX_HEADER_SIZE {
			return ex.Throw{Code: http.StatusLengthRequired, Msg: utils.AddStr("too many header parameters: ", len(r.Header))}
		}
		for k, v := range r.Header {
			if len(k) > MAX_FIELD_LEN {
				return ex.Throw{Code: http.StatusLengthRequired, Msg: utils.AddStr("header name length is too long: ", len(k))}
			}
			v0 := v[0]
			if len(v0) > MAX_VALUE_LEN {
				return ex.Throw{Code: http.StatusLengthRequired, Msg: utils.AddStr("header value length is too long: ", len(v0))}
			}
			headers[k] = v0
		}
	} else {
		headers[USER_AGENT] = r.Header.Get(USER_AGENT)
		headers[Authorization] = r.Header.Get(Authorization)
	}
	self.Context.Token = headers[Authorization]
	self.Context.Headers = headers
	return nil
}

func (self *HttpNode) getParams() error {
	r := self.Context.Input
	if r.Method == POST { // only body json parameter is accepted
		r.ParseForm()
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "failed to read body parameters", Err: err}
		}
		r.Body.Close()
		if self.Context.RouterConfig.Original { //
			data := map[string]interface{}{}
			if len(body) == 0 {
				self.Context.Params = &ReqDto{Data: data}
				return nil
			}
			if len(body) > (MAX_VALUE_LEN * 5) {
				return ex.Throw{Code: http.StatusLengthRequired, Msg: "body parameters length is too long"}
			}
			if err := utils.JsonUnmarshal(body, &data); err != nil {
				return ex.Throw{Code: http.StatusBadRequest, Msg: "body parameters JSON parsing failed", Err: err}
			}
			self.Context.Params = &ReqDto{Data: data}
			return nil
		}
		if len(body) == 0 {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "body parameters is nil"}
		}
		if len(body) > (MAX_VALUE_LEN * 5) {
			return ex.Throw{Code: http.StatusLengthRequired, Msg: "body parameters length is too long"}
		}
		req := &ReqDto{}
		if err := utils.JsonUnmarshal(body, req); err != nil {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "body parameters JSON parsing failed", Err: err}
		}
		if err := self.validator(req); err != nil { // TODO important
			return err
		}
		return nil
	} else if r.Method == GET { // only url key/value parameter is accepted
		if !self.Context.RouterConfig.Original {
			return ex.Throw{Code: http.StatusUnsupportedMediaType, Msg: "GET type is not supported"}
		}
		r.ParseForm()
		if len(r.Form) > MAX_FIELD_LEN {
			return ex.Throw{Code: http.StatusLengthRequired, Msg: utils.AddStr("get url key name length is too long: ", len(r.Form))}
		}
		data := map[string]interface{}{}
		for k, v := range r.Form {
			if len(v) == 0 {
				continue
			}
			v0 := v[0]
			if len(v0) > MAX_VALUE_LEN {
				return ex.Throw{Code: http.StatusLengthRequired, Msg: utils.AddStr("get url value length is too long: ", len(v0))}
			}
			data[k] = v[0]
		}
		self.Context.Params = &ReqDto{Data: data}
	} else if r.Method == PUT {
		return ex.Throw{Code: http.StatusUnsupportedMediaType, Msg: "PUT type is not supported"}
	} else if r.Method == PATCH {
		return ex.Throw{Code: http.StatusUnsupportedMediaType, Msg: "PATCH type is not supported"}
	} else if r.Method == DELETE {
		return ex.Throw{Code: http.StatusUnsupportedMediaType, Msg: "DELETE type is not supported"}
	} else {
		return ex.Throw{Code: http.StatusUnsupportedMediaType, Msg: "unknown request type"}
	}
	return nil
}

func (self *HttpNode) paddDevice() error {
	d := self.Context.GetHeader("User-Agent")
	if utils.HasStr(d, "Android") || utils.HasStr(d, "Adr") {
		self.Context.Device = ANDROID
	} else if utils.HasStr(d, "iPad") || utils.HasStr(d, "iPhone") || utils.HasStr(d, "Mac") {
		self.Context.Device = IOS
	} else {
		self.Context.Device = WEB
	}
	return nil
}

func (self *HttpNode) validator(req *ReqDto) error {
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
	if utils.MathAbs(utils.TimeSecond()-req.Time) > jwt.FIVE_MINUTES { // 判断绝对时间差超过5分钟
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
	self.Context.Params = req
	return nil
}

func (self *HttpNode) doRequest(pattern string, input *http.Request, output http.ResponseWriter) error {
	ob := &HttpNode{}
	ob.SessionAware = self.SessionAware
	ob.CacheAware = self.CacheAware
	ob.Context = &Context{
		CreateAt:     utils.Time(),
		Host:         utils.ClientIP(input),
		Port:         self.Context.Port,
		Style:        HTTP2,
		Method:       pattern,
		Version:      self.Context.Version,
		Response:     &Response{Encoding: UTF8, ContentType: APPLICATION_JSON, ContentEntity: nil, ContentEntityByte: nil},
		Input:        input,
		Output:       output,
		RouterConfig: routerConfigs[pattern],
		ServerTLS:    self.Context.ServerTLS,
		PermConfig:   self.Context.PermConfig,
		Storage:      make(map[string]interface{}, 0),
	}
	return doFilterChain(ob)
}

func (self *HttpNode) proxy(pattern string, input *http.Request, output http.ResponseWriter) {
	if err := self.doRequest(pattern, input, output); err != nil {
		zlog.Error("doRequest failed", 0, zlog.AddError(err))
	}
}

func (self *HttpNode) defaultHandler() http.Handler {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		self.handler.ServeHTTP(w, r)
	})
	if self.DisconnectTimeout <= 0 {
		self.DisconnectTimeout = 180
	}
	errorMsg := `{"c":408,"m":"server actively disconnects the client","d":null,"t":%d,"n":"%s","p":0,"s":""}`
	return http.TimeoutHandler(handler, time.Duration(self.DisconnectTimeout)*time.Second, fmt.Sprintf(errorMsg, utils.Time(), utils.GetSnowFlakeStrID()))
}

func (self *HttpNode) StartServer() {
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
		url := utils.AddStr(self.Context.Host, ":", self.Context.Port)
		zlog.Printf("http【%s】service has been started successful", url)
		if err := http.ListenAndServe(url, self.defaultHandler()); err != nil {
			zlog.Error("http service init failed", 0, zlog.AddError(err))
		}
	}()
	select {}
}

func (self *HttpNode) Router(pattern string, handle func(ctx *Context) error, routerConfig *RouterConfig) {
	if !strings.HasPrefix(pattern, "/") {
		pattern = "/" + pattern
	}
	if self.CacheAware == nil {
		zlog.Error("cache service hasn't been initialized", 0)
		return
	}
	if len(self.Context.Version) > 0 {
		pattern = "/" + self.Context.Version + pattern
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
	if _, b := routerConfigs[pattern]; !b {
		routerConfig.postHandle = handle
		routerConfigs[pattern] = routerConfig
	}
	if self.handler == nil {
		self.handler = http.NewServeMux()
	}
	self.handler.Handle(pattern, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		self.proxy(pattern, r, w)
	}))
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
	resp := &RespDto{
		Code:    out.Code,
		Message: out.Msg,
		Time:    utils.Time(),
	}
	if !ctx.Authenticated() {
		resp.Nonce = utils.RandNonce()
	} else {
		if ctx.Params == nil || len(ctx.Params.Nonce) == 0 {
			resp.Nonce = utils.RandNonce()
		} else {
			resp.Nonce = ctx.Params.Nonce
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
	ctx.Output.Header().Set("Content-Type", ctx.Response.ContentType)
	if ctx.Response.StatusCode == 0 {
		ctx.Output.WriteHeader(http.StatusOK)
	} else {
		ctx.Output.WriteHeader(ctx.Response.StatusCode)
	}
	ctx.Output.Write(ctx.Response.ContentEntityByte)
	return nil
}

func defaultRenderPre(ctx *Context) error {
	routerConfig, _ := routerConfigs[ctx.Method]
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
		resp := &RespDto{
			Code: http.StatusOK,
			Time: utils.Time(),
		}
		if ctx.Params == nil || len(ctx.Params.Nonce) == 0 {
			resp.Nonce = utils.RandNonce()
		} else {
			resp.Nonce = ctx.Params.Nonce
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
