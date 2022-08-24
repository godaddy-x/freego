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

type HttpNode struct {
	HookNode
}

func (self *HttpNode) GetHeader() error {
	r := self.Context.Input
	headers := map[string]string{}
	if self.RouterConfig.Original {
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

// 按指定规则进行数据解码,校验API参数安全
func (self *HttpNode) Authenticate(req *ReqDto) error {
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
	if self.RouterConfig.AesRequest && req.Plan != 1 {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request parameters must use AES encryption"}
	}
	if self.RouterConfig.Login && req.Plan != 2 {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request parameters must use RSA encryption"}
	}
	if !utils.CheckStrLen(req.Sign, 32, 64) {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request signature length invalid"}
	}
	var key string
	if self.RouterConfig.Login {
		key = self.Context.ServerCert.PubkeyBase64
	}
	if self.Context.GetDataSign(d, req.Nonce, req.Time, req.Plan, key) != req.Sign {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request signature invalid"}
	}
	data := make(map[string]interface{}, 0)
	if req.Plan == 1 && !self.RouterConfig.Login { // AES
		dec, err := utils.AesDecrypt(d, self.Context.GetTokenSecret(), utils.AddStr(req.Nonce, req.Time))
		if err != nil {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "AES failed to parse data", Err: err}
		}
		if err := utils.JsonUnmarshal(utils.Str2Bytes(dec), &data); err != nil {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "parameter JSON parsing failed"}
		}
	} else if req.Plan == 2 && self.RouterConfig.Login { // RSA
		//a := "To0IaxMwcWbX2CsQRv6jmUcPiNcPHn-73708NG8n99WAr5AS3ry7zEBtNRcDUuqhMjHS6NbQQrBOGVMKCfA1Mig2cgCh4wSq50p4omyAExEf1mDDA4bRo2_yPLCDBp63ERC3FJSJY_7ru07darWH6sZbymLigEjA4CWrpxmBQGKkr0gs6nYPIZg3eMuJj_RYmoIPYQtBU5BdPpKqPvtRWOAJBMtZbpSrxDBcCoA_0m3MbNYC4vvb1ivkABp_RXT_SlQqr9IEOqyBQpWpm5FBsoMZkXnMxFBhL1syaZwTK5Fr6Vj-85D0UsTXVPJmdLOBSirlJTgHLPKMMh70PxlEGQ=="
		dec, err := self.Context.ServerCert.Decrypt(d)
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
		self.Context.ClientCert = &gorsa.RsaObj{PubkeyBase64: pubkey_v}
	} else if req.Plan == 0 && !self.RouterConfig.Login && !self.RouterConfig.AesRequest {
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

func (self *HttpNode) GetParams() error {
	r := self.Context.Input
	if r.Method == POST { // only body json parameter is accepted
		r.ParseForm()
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "failed to read body parameters", Err: err}
		}
		r.Body.Close()
		if self.RouterConfig.Original { //
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
		if err := self.Authenticate(req); err != nil { // TODO important
			return err
		}
		return nil
	} else if r.Method == GET { // only url key/value parameter is accepted
		if !self.RouterConfig.Original {
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

func (self *HttpNode) InitContext(ptr *NodePtr) error {
	output := ptr.Output
	input := ptr.Input
	node := ptr.Node.(*HttpNode)
	node.RouterConfig = ptr.RouterConfig
	node.CreateAt = utils.Time()
	node.OverrideFunc = self.OverrideFunc
	node.SessionAware = self.SessionAware
	node.CacheAware = self.CacheAware
	node.Context = &Context{
		Host:       utils.ClientIP(input),
		Port:       self.Context.Port,
		Style:      HTTP,
		Method:     ptr.Pattern,
		Version:    self.Context.Version,
		Response:   &Response{UTF8, APPLICATION_JSON, nil},
		Input:      input,
		Output:     output,
		ServerCert: self.Context.ServerCert,
		JwtConfig:  self.Context.JwtConfig,
		PermConfig: self.Context.PermConfig,
		Storage:    make(map[string]interface{}, 0),
	}
	if err := node.GetHeader(); err != nil {
		return err
	}
	if err := node.GetParams(); err != nil {
		return err
	}
	if err := node.PaddDevice(); err != nil {
		return err
	}
	return nil
}

func (self *HttpNode) PaddDevice() error {
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

func (self *HttpNode) ValidSession() error {
	if self.RouterConfig.Login || self.RouterConfig.Guest { // 登录接口和游客模式跳过会话认证
		return nil
	}
	if len(self.Context.Token) == 0 {
		return ex.Throw{Code: http.StatusUnauthorized, Msg: "AccessToken is ni"}
	}
	subject := &jwt.Subject{}
	if err := subject.Verify(self.Context.Token, self.Context.JwtConfig().TokenKey); err != nil {
		return ex.Throw{Code: http.StatusUnauthorized, Msg: "AccessToken is invalid or expired", Err: err}
	}
	self.Context.Roles = subject.GetTokenRole()
	self.Context.Subject = subject.Payload
	return nil
}

func (self *HttpNode) ValidReplayAttack() error {
	//param := self.Context.Params
	//if param == nil || len(param.Sign) == 0 {
	//	return nil
	//}
	//key := utils.AddStr(JWT_SIG_, param.Sign)
	//if c, err := self.CacheAware(); err != nil {
	//	return err
	//} else if b, err := c.GetInt64(key); err != nil {
	//	return err
	//} else if b > 1 {
	//	return ex.Throw{Code: http.StatusForbidden, Msg: "重复请求不受理"}
	//} else {
	//	c.Put(key, 1, int((param.Time+jwt.FIVE_MINUTES)/1000))
	//}
	return nil
}

func (self *HttpNode) ValidPermission() error {
	if self.Context.PermConfig == nil {
		return nil
	}
	need, err := self.Context.PermConfig(self.Context.Method)
	if err != nil {
		return ex.Throw{Code: http.StatusUnauthorized, Msg: "failed to read authorization resource", Err: err}
	} else if !need.ready { // 无授权资源配置,跳过
		return nil
	} else if need.NeedRole == nil || len(need.NeedRole) == 0 { // 无授权角色配置跳过
		return nil
	}
	if need.NeedLogin == 0 { // 无登录状态,跳过
		return nil
	} else if !self.Context.Authenticated() { // 需要登录状态,会话为空,抛出异常
		return ex.Throw{Code: http.StatusUnauthorized, Msg: "login status required"}
	}
	access := 0
	needAccess := len(need.NeedRole)
	for _, cr := range self.Context.Roles {
		for _, nr := range need.NeedRole {
			if cr == nr {
				access++
				if need.MathchAll == 0 || access == needAccess { // 任意授权通过则放行,或已满足授权长度
					return nil
				}
			}
		}
	}
	return ex.Throw{Code: http.StatusUnauthorized, Msg: "access defined"}
}

func (self *HttpNode) Proxy(ptr *NodePtr) {
	ob := &HttpNode{}
	if err := func() error {
		ptr.Node = ob
		// 1.初始化请求上下文
		if err := self.InitContext(ptr); err != nil {
			return err
		}
		// 2.校验会话有效性
		if err := ob.ValidSession(); err != nil {
			return err
		}
		// 3.校验重放攻击
		if err := ob.ValidReplayAttack(); err != nil {
			return err
		}
		// 4.校验访问权限
		if err := ob.ValidPermission(); err != nil {
			return err
		}
		// 5.上下文前置检测方法
		if err := ob.PreHandle(); err != nil {
			return err
		}
		// 6.保存监听日志方法
		res, err := ob.LogHandle()
		if err != nil {
			return err
		}
		// 7.执行业务方法
		err = ptr.Handle(ob.Context) // 抛出业务异常,建议使用ex模式
		// 8.执行视图控制方法
		err = ob.PostHandle(err)
		// 9.执行释放资源方法
		if err := ob.AfterCompletion(res, err); err != nil {
			return err
		}
		return nil
	}(); err != nil {
		ob.RenderError(err)
	}
}

func (self *HttpNode) PreHandle() error {
	if self.OverrideFunc == nil || self.OverrideFunc.PreHandleFunc == nil {
		return nil
	}
	return self.OverrideFunc.PreHandleFunc(self.Context)
}

func (self *HttpNode) LogHandle() (LogHandleRes, error) {
	if self.OverrideFunc == nil || self.OverrideFunc.LogHandleFunc == nil {
		return LogHandleRes{}, nil
	}
	return self.OverrideFunc.LogHandleFunc(self.Context)
}

func (self *HttpNode) PostHandle(err error) error {
	if self.OverrideFunc == nil || self.OverrideFunc.PostHandleFunc == nil {
		return nil
	}
	if err := self.OverrideFunc.PostHandleFunc(self.Context.Response, err); err != nil {
		return err
	}
	if err != nil {
		return err
	}
	return self.RenderTo()
}

func (self *HttpNode) AfterCompletion(res LogHandleRes, err error) error {
	if self.OverrideFunc == nil || self.OverrideFunc.AfterCompletionFunc == nil {
		return nil
	}
	if err := self.OverrideFunc.AfterCompletionFunc(self.Context, res, err); err != nil {
		return err
	}
	return err
}

func (self *HttpNode) RenderError(err error) error {
	out := ex.Catch(err)
	resp := &RespDto{
		Code:    out.Code,
		Message: out.Msg,
		Time:    utils.Time(),
	}
	if !self.Context.Authenticated() {
		resp.Nonce = utils.RandNonce()
	} else {
		resp.Nonce = self.Context.Params.Nonce
	}
	if self.RouterConfig.Original {
		if out.Code > 600 {
			self.Context.Output.Header().Set("Content-Type", TEXT_PLAIN)
			self.Context.Output.WriteHeader(http.StatusOK)
		} else {
			self.Context.Output.WriteHeader(out.Code)
		}
		self.Context.Output.Write(utils.Str2Bytes(resp.Message))
	} else {
		if result, err := utils.JsonMarshal(resp); err != nil {
			zlog.Error(resp.Message, 0, zlog.AddError(err))
			return nil
		} else {
			self.Context.Output.Header().Set("Content-Type", APPLICATION_JSON)
			self.Context.Output.WriteHeader(http.StatusOK)
			self.Context.Output.Write(result)
		}
	}
	return nil
}

func (self *HttpNode) RenderTo() error {
	switch self.Context.Response.ContentType {
	case TEXT_PLAIN:
		content := self.Context.Response.ContentEntity
		if v, b := content.(string); b {
			self.Context.Output.Header().Set("Content-Type", TEXT_PLAIN)
			self.Context.Output.Write(utils.Str2Bytes(v))
		} else {
			self.Context.Output.Header().Set("Content-Type", TEXT_PLAIN)
			self.Context.Output.Write(utils.Str2Bytes(""))
		}
	case APPLICATION_JSON:
		if self.RouterConfig.Original {
			if result, err := utils.JsonMarshal(self.Context.Response.ContentEntity); err != nil {
				return ex.Throw{Code: http.StatusInternalServerError, Msg: "response JSON data failed", Err: err}
			} else {
				self.Context.Output.Header().Set("Content-Type", APPLICATION_JSON)
				self.Context.Output.Write(result)
			}
			break
		}
		data, err := utils.JsonMarshal(self.Context.Response.ContentEntity)
		if err != nil {
			return ex.Throw{Code: http.StatusInternalServerError, Msg: "response conversion JSON failed", Err: err}
		}
		resp := &RespDto{
			Code: http.StatusOK,
			//Message: "success",
			Time: utils.Time(),
			//Data:  data,
			Nonce: self.Context.Params.Nonce,
		}
		var key string
		if self.RouterConfig.Login {
			key = self.Context.ClientCert.PubkeyBase64
			//data, err := self.Context.ClientCert.Encrypt(data)
			//if err != nil {
			//	return ex.Throw{Code: http.StatusInternalServerError, Msg: "RSA encryption response data failed", Err: err}
			//}
			//resp.Data = data
			data, err := utils.AesEncrypt(data, key, key)
			if err != nil {
				return ex.Throw{Code: http.StatusInternalServerError, Msg: "AES encryption response data failed", Err: err}
			}
			resp.Data = data
			resp.Plan = 2
		} else if self.RouterConfig.AesResponse {
			data, err := utils.AesEncrypt(data, self.Context.GetTokenSecret(), utils.AddStr(resp.Nonce, resp.Time))
			if err != nil {
				return ex.Throw{Code: http.StatusInternalServerError, Msg: "AES encryption response data failed", Err: err}
			}
			resp.Data = data
			resp.Plan = 1
		} else {
			resp.Data = utils.Base64URLEncode(data)
		}
		resp.Sign = self.Context.GetDataSign(resp.Data.(string), resp.Nonce, resp.Time, resp.Plan, key)
		if result, err := utils.JsonMarshal(resp); err != nil {
			return ex.Throw{Code: http.StatusInternalServerError, Msg: "response JSON data failed", Err: err}
		} else {
			self.Context.Output.Header().Set("Content-Type", APPLICATION_JSON)
			self.Context.Output.Write(result)
		}
	default:
		return ex.Throw{Code: http.StatusUnsupportedMediaType, Msg: "invalid response format"}
	}
	return nil
}

func (self *HttpNode) StartServer() {
	go func() {
		if self.CacheAware != nil {
			zlog.Printf("cache service has been started successfully")
		}
		if self.Context.ServerCert != nil {
			zlog.Printf("RSA certificate service has been started successfully")
		}
		url := utils.AddStr(self.Context.Host, ":", self.Context.Port)
		zlog.Printf("http【%s】service has been started successfully", url)
		if err := http.ListenAndServe(url, self.limiterTimeoutHandler()); err != nil {
			zlog.Error("http service init failed", 0, zlog.AddError(err))
		}
	}()
	select {}
}

func (self *HttpNode) limiterTimeoutHandler() http.Handler {
	if self.GatewayLimiter == nil {
		panic("gateway limiter is nil")
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if b := self.GatewayLimiter.Allow("HttpThreshold"); !b {
			w.WriteHeader(429)
			w.Write([]byte("the gateway request is full, please try again later"))
			return
		}
		self.Handler.ServeHTTP(w, r)
	})
	if self.DisconnectTimeout <= 0 {
		self.DisconnectTimeout = 180
	}
	errorMsg := `{"c":408,"m":"server actively disconnects the client","d":null,"t":%d,"n":"%s","p":0,"s":""}`
	return http.TimeoutHandler(handler, time.Duration(self.DisconnectTimeout)*time.Second, fmt.Sprintf(errorMsg, utils.Time(), utils.GetSnowFlakeStrID()))
}

var routerConfigs = make(map[string]*RouterConfig)

func (self *HttpNode) Router(pattern string, handle func(ctx *Context) error, routerConfig *RouterConfig) {
	if !strings.HasPrefix(pattern, "/") {
		pattern = utils.AddStr("/", pattern)
	}
	if len(self.Context.Version) > 0 {
		pattern = utils.AddStr("/", self.Context.Version, pattern)
	}
	if self.CacheAware == nil {
		zlog.Error("cache service hasn't been initialized", 0)
		return
	}
	if self.Context.ServerCert == nil {
		cert := &gorsa.RsaObj{}
		if err := cert.CreateRsa1024(); err != nil {
			zlog.Error("RSA certificate generation failed", 0)
			return
		}
		self.Context.ServerCert = cert
	}
	if routerConfig == nil {
		routerConfig = &RouterConfig{}
	}
	if self.Handler == nil {
		self.Handler = http.NewServeMux()
	}
	if _, b := routerConfigs[pattern]; !b {
		routerConfigs[pattern] = routerConfig
	}
	self.Handler.Handle(pattern, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		self.Proxy(
			&NodePtr{
				Node:         self,
				RouterConfig: routerConfigs[pattern],
				Input:        r,
				Output:       w,
				Pattern:      pattern,
				Handle:       handle,
			})
	}))
}

func (self *HttpNode) Json(ctx *Context, data interface{}) error {
	ctx.Response = &Response{UTF8, APPLICATION_JSON, data}
	return nil
}

func (self *HttpNode) Text(ctx *Context, data string) error {
	ctx.Response = &Response{UTF8, TEXT_PLAIN, data}
	return nil
}
