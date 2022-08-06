package node

import (
	"fmt"
	"github.com/godaddy-x/freego/cache"
	"github.com/godaddy-x/freego/component/gorsa"
	"github.com/godaddy-x/freego/component/jwt"
	"github.com/godaddy-x/freego/component/limiter"
	"github.com/godaddy-x/freego/component/log"
	"github.com/godaddy-x/freego/ex"
	"github.com/godaddy-x/freego/util"
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
	if self.Config.Original {
		if len(r.Header) > 0 {
			i := 0
			for k, v := range r.Header {
				i++
				if i > MAX_HEADER_SIZE {
					return ex.Throw{Code: http.StatusLengthRequired, Msg: util.AddStr("请求头数量溢出: ", i)}
				}
				if len(k) > MAX_FIELD_LEN {
					return ex.Throw{Code: http.StatusLengthRequired, Msg: util.AddStr("参数名长度溢出: ", len(k))}
				}
				v0 := v[0]
				if len(v0) > MAX_VALUE_LEN {
					return ex.Throw{Code: http.StatusLengthRequired, Msg: util.AddStr("参数值长度溢出: ", len(v0))}
				}
				headers[k] = v0
			}
		}
	} else {
		headers[USER_AGENT] = r.Header.Get(USER_AGENT)
		headers[Authorization] = r.Header.Get(Authorization)
		if self.Config.IsLogin {
			pub := r.Header.Get(CLIENT_PUBKEY)
			if !util.CheckStrLen(pub, 340, 350) {
				return ex.Throw{Code: http.StatusBadRequest, Msg: "客户端公钥无效"}
			}
			sign := r.Header.Get(CLIENT_PUBKEY_SIGN)
			if !util.CheckStrLen(sign, 110, 120) {
				return ex.Throw{Code: http.StatusBadRequest, Msg: "客户端公钥签名无效"}
			}
			headers[CLIENT_PUBKEY] = pub
			headers[CLIENT_PUBKEY_SIGN] = sign
		}
	}
	self.Context.Token = headers["Authorization"]
	self.Context.Headers = headers
	return nil
}

func (self *HttpNode) ValidRsaLogin(body []byte, req *ReqDto) error {
	bs := util.Base64Decode(body)
	if bs == nil {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "参数序列化失败"}
	}
	pub, _ := self.Context.Headers[CLIENT_PUBKEY]
	pub_bs := util.Base64Decode(pub)
	if pub_bs == nil || len(pub_bs) == 0 {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "公钥参数序列化失败"}
	}
	pub_dec, err := self.Certificate.Decrypt(pub_bs)
	if err != nil {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "客户端公钥解析失败", Err: err}
	}
	if pub_dec == nil || len(pub_dec) == 0 {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "客户端公钥解析无效", Err: err}
	}
	cliRsa := &gorsa.RsaObj{}
	if err := cliRsa.LoadRsaPemFileByte(pub_dec); err != nil {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "客户端公钥加载失败", Err: err}
	}
	sign, _ := self.Context.Headers[CLIENT_PUBKEY_SIGN]
	if err := cliRsa.VerifyBySHA256(bs, sign); err != nil {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "客户端公钥验签失败", Err: err}
	}
	dec, err := self.Certificate.Decrypt(bs)
	if err != nil {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "参数解析失败", Err: err}
	}
	if dec == nil || len(dec) == 0 {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "参数解析无效", Err: err}
	}
	if err := util.ParseJsonBase64(dec, req); err != nil {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "参数序列化失败", Err: err}
	}
	self.Context.Storage[CLIENT_PUBKEY_OBJECT] = cliRsa
	return nil
}

// 按指定规则进行数据解码,校验API参数安全
func (self *HttpNode) Authenticate(req *ReqDto) error {
	d, b := req.Data.(string)
	if !b || len(d) == 0 {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "业务参数无效"}
	}
	if !util.CheckInt64(req.Plan, 0, 1) {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "计划参数无效"}
	}
	if !util.CheckLen(req.Nonce, 8, 32) {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "随机参数无效"}
	}
	if req.Time <= 0 {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "时间参数为空"}
	}
	if util.MathAbs(util.TimeSecond()-req.Time) > jwt.FIVE_MINUTES { // 判断绝对时间差超过5分钟
		return ex.Throw{Code: http.StatusBadRequest, Msg: "时间参数无效"}
	}
	if self.Config.RequestAesEncrypt && req.Plan != 1 {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "请求参数必须使用AES加密模式"}
	}
	if self.Config.IsLogin && req.Plan == 1 {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "请求参数必须使用RSA加密模式"}
	}
	if !self.Config.IsLogin {
		if len(req.Sign) != 32 && len(req.Sign) != 64 {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "签名参数无效"}
		}
		if self.Context.GetDataSign(d, req.Nonce, req.Time, req.Plan) != req.Sign {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "API签名校验失败"}
		}
		if req.Plan == 1 { // AES
			dec, err := util.AesDecrypt(d, self.Context.GetTokenSecret(), util.AddStr(req.Nonce, req.Time))
			if err != nil {
				return ex.Throw{Code: http.StatusBadRequest, Msg: "请求数据解码失败", Err: err}
			}
			d = dec
		}
	}
	data := make(map[string]interface{}, 0)
	if err := util.ParseJsonBase64(d, &data); err != nil {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "业务参数解析失败"}
	}
	req.Data = data
	self.Context.Params = req
	return nil
}

func (self *HttpNode) GetParams() error {
	r := self.Context.Input
	if r.Method == POST {
		r.ParseForm()
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "获取参数失败", Err: err}
		}
		r.Body.Close()
		if len(body) == 0 {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "获取参数为空", Err: nil}
		}
		if len(body) > (MAX_VALUE_LEN * 5) {
			return ex.Throw{Code: http.StatusLengthRequired, Msg: "参数值长度溢出"}
		}
		if !self.Config.Original {
			req := &ReqDto{}
			if self.Config.IsLogin { // rsa valid
				if err := self.ValidRsaLogin(body, req); err != nil {
					return err
				}
			} else if err := util.JsonUnmarshal(body, req); err != nil {
				return ex.Throw{Code: http.StatusBadRequest, Msg: "参数解析失败", Err: err}
			}
			// TODO important
			if err := self.Authenticate(req); err != nil {
				return err
			}
			return nil
		}
		data := map[string]interface{}{}
		if body == nil || len(body) == 0 {
			for k, v := range r.Form {
				if len(v) == 0 {
					continue
				}
				data[k] = v[0]
			}
		} else {
			if err := util.JsonUnmarshal(body, &data); err != nil {
				return ex.Throw{Code: http.StatusBadRequest, Msg: "参数解析失败", Err: err}
			}
		}
		self.Context.Params = &ReqDto{Data: data}
		return nil
	} else if r.Method == GET {
		if !self.Config.Original {
			return ex.Throw{Code: http.StatusUnsupportedMediaType, Msg: "暂不支持GET类型"}
		}
		r.ParseForm()
		result, err := ioutil.ReadAll(r.Body)
		if err != nil {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "获取参数失败", Err: err}
		}
		r.Body.Close()
		if len(result) > (MAX_VALUE_LEN * 5) {
			return ex.Throw{Code: http.StatusLengthRequired, Msg: "参数值长度溢出: " + util.AnyToStr(len(result))}
		}
		data := map[string]interface{}{}
		if err := util.JsonUnmarshal(result, &data); err != nil {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "参数解析失败", Err: err}
		}
		self.Context.Params = &ReqDto{Data: data}
	} else if r.Method == PUT {
		return ex.Throw{Code: http.StatusUnsupportedMediaType, Msg: "暂不支持PUT类型"}
	} else if r.Method == PATCH {
		return ex.Throw{Code: http.StatusUnsupportedMediaType, Msg: "暂不支持PATCH类型"}
	} else if r.Method == DELETE {
		return ex.Throw{Code: http.StatusUnsupportedMediaType, Msg: "暂不支持DELETE类型"}
	} else {
		return ex.Throw{Code: http.StatusUnsupportedMediaType, Msg: "未知的请求类型"}
	}
	return nil
}

func (self *HttpNode) InitContext(ptr *NodePtr) error {
	output := ptr.Output
	input := ptr.Input
	node := ptr.Node.(*HttpNode)
	node.Config = ptr.Config
	node.CreateAt = util.Time()
	node.OverrideFunc = self.OverrideFunc
	node.SessionAware = self.SessionAware
	node.CacheAware = self.CacheAware
	node.Certificate = self.Certificate
	node.JwtConfig = self.JwtConfig
	node.PermConfig = self.PermConfig
	node.Context = &Context{
		Host:     util.ClientIP(input),
		Port:     self.Context.Port,
		Style:    HTTP,
		Method:   ptr.Pattern,
		Version:  self.Context.Version,
		Response: &Response{UTF8, APPLICATION_JSON, nil},
		Input:    input,
		Output:   output,
		Storage:  make(map[string]interface{}, 0),
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
	if util.HasStr(d, "Android") || util.HasStr(d, "Adr") {
		self.Context.Device = ANDROID
	} else if util.HasStr(d, "iPad") || util.HasStr(d, "iPhone") || util.HasStr(d, "Mac") {
		self.Context.Device = IOS
	} else {
		self.Context.Device = WEB
	}
	return nil
}

func (self *HttpNode) ValidSession() error {
	if !self.Config.Authorization { // 非必须权限类型
		return nil
	}
	if len(self.Context.Token) == 0 {
		return ex.Throw{Code: http.StatusUnauthorized, Msg: "授权令牌为空"}
	}
	subject := &jwt.Subject{}
	if err := subject.Verify(self.Context.Token, self.JwtConfig().TokenKey); err != nil {
		return ex.Throw{Code: http.StatusUnauthorized, Msg: "授权令牌无效或已过期", Err: err}
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
	//key := util.AddStr(JWT_SIG_, param.Sign)
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
	if self.PermConfig == nil {
		return nil
	}
	need, err := self.PermConfig(self.Context.Method)
	if err != nil {
		return ex.Throw{Code: http.StatusUnauthorized, Msg: "读取授权资源失败", Err: err}
	} else if !need.ready { // 无授权资源配置,跳过
		return nil
	} else if need.NeedRole == nil || len(need.NeedRole) == 0 { // 无授权角色配置跳过
		return nil
	}
	if need.NeedLogin == 0 { // 无登录状态,跳过
		return nil
	} else if !self.Context.Authenticated() { // 需要登录状态,会话为空,抛出异常
		return ex.Throw{Code: http.StatusUnauthorized, Msg: "获取授权主体失败"}
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
	return ex.Throw{Code: http.StatusUnauthorized, Msg: "访问权限不足"}
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
		Time:    util.Time(),
	}
	if !self.Context.Authenticated() {
		resp.Nonce = util.GetSnowFlakeStrID()
	} else {
		resp.Nonce = self.Context.Params.Nonce
	}
	if self.Config.Original {
		if out.Code > 600 {
			self.Context.Output.Header().Set("Content-Type", TEXT_PLAIN)
			self.Context.Output.WriteHeader(http.StatusOK)
		} else {
			self.Context.Output.WriteHeader(out.Code)
		}
		self.Context.Output.Write(util.Str2Bytes(resp.Message))
	} else {
		if result, err := util.JsonMarshal(resp); err != nil {
			log.Error(resp.Message, 0, log.AddError(err))
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
			self.Context.Output.Write(util.Str2Bytes(v))
		} else {
			self.Context.Output.Header().Set("Content-Type", TEXT_PLAIN)
			self.Context.Output.Write(util.Str2Bytes(""))
		}
	case APPLICATION_JSON:
		if self.Config.Original {
			if result, err := util.JsonMarshal(self.Context.Response.ContentEntity); err != nil {
				return ex.Throw{Code: http.StatusInternalServerError, Msg: "响应数据异常", Err: err}
			} else {
				self.Context.Output.Header().Set("Content-Type", APPLICATION_JSON)
				self.Context.Output.Write(result)
			}
			break
		}
		data, err := util.ToJsonBase64(self.Context.Response.ContentEntity)
		if err != nil {
			return ex.Throw{Code: http.StatusInternalServerError, Msg: "响应数据序列化失败", Err: err}
		}
		resp := &RespDto{
			Code:    http.StatusOK,
			Message: "success",
			Time:    util.Time(),
			Data:    data,
			Nonce:   self.Context.Params.Nonce,
		}
		if self.Config.ResponseAesEncrypt {
			resp.Plan = 1
			data, err := util.AesEncrypt(data, self.Context.GetTokenSecret(), util.AddStr(resp.Nonce, resp.Time))
			if err != nil {
				return ex.Throw{Code: http.StatusInternalServerError, Msg: "响应数据加密失败", Err: err}
			}
			resp.Data = data
		}
		if self.Config.IsLogin {
			sign, err := self.Context.GetDataRsaSign(self.Certificate, data, resp.Nonce, resp.Time, resp.Plan)
			if err != nil {
				return ex.Throw{Code: http.StatusInternalServerError, Msg: "响应数据加密失败", Err: err}
			}
			resp.Sign = sign
		} else {
			resp.Sign = self.Context.GetDataSign(data, resp.Nonce, resp.Time, resp.Plan)
		}
		if result, err := util.JsonMarshal(resp); err != nil {
			return ex.Throw{Code: http.StatusInternalServerError, Msg: "响应数据异常", Err: err}
		} else {
			self.Context.Output.Header().Set("Content-Type", APPLICATION_JSON)
			self.Context.Output.Write(result)
		}
	default:
		return ex.Throw{Code: http.StatusUnsupportedMediaType, Msg: "无效的响应格式"}
	}
	return nil
}

func (self *HttpNode) StartServer() {
	go func() {
		if self.CacheAware != nil {
			log.Printf("cache service has been started successfully")
		}
		if self.Certificate != nil {
			log.Printf("server【rsa2048/sha256】-> client【rsa668/sha256】certificate service has been started successfully")
		}
		url := util.AddStr(self.Context.Host, ":", self.Context.Port)
		log.Printf("http【%s】service has been started successfully", url)
		if err := http.ListenAndServe(url, self.limiterTimeoutHandler()); err != nil {
			log.Error("初始化http服务失败", 0, log.AddError(err))
		}
	}()
	select {}
}

func (self *HttpNode) limiterTimeoutHandler() http.Handler {
	if self.GatewayRate == nil {
		self.GatewayRate = &rate.RateOpetion{
			Key:    "HttpThreshold",
			Limit:  1000,
			Bucket: 1000,
			Expire: 1209600,
		}
	}
	limiter := rate.NewLocalLimiterByOption(new(cache.LocalMapManager).NewCache(20160, 20160), self.GatewayRate)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if limiter.Validate(nil) {
			w.WriteHeader(429)
			return
		}
		self.Handler.ServeHTTP(w, r)
	})
	if self.DisconnectTimeout <= 0 {
		self.DisconnectTimeout = 180
	}
	errmsg := `{"c":408,"m":"服务端主动断开客户端连接","d":null,"t":%d,"n":"%s","p":0,"s":""}`
	return http.TimeoutHandler(handler, time.Duration(self.DisconnectTimeout)*time.Second, fmt.Sprintf(errmsg, util.Time(), util.GetSnowFlakeStrID()))
}

var nodeConfigs = make(map[string]*Config)

func (self *HttpNode) Router(pattern string, handle func(ctx *Context) error, config *Config) {
	if !strings.HasPrefix(pattern, "/") {
		pattern = util.AddStr("/", pattern)
	}
	if len(self.Context.Version) > 0 {
		pattern = util.AddStr("/", self.Context.Version, pattern)
	}
	if self.CacheAware == nil {
		log.Error("缓存服务尚未初始化", 0)
		return
	}
	if self.Certificate == nil {
		cert := &gorsa.RsaObj{}
		_, _, err := cert.CreateRsaFileBase64()
		if err != nil {
			log.Error("RSA证书生成失败", 0)
			return
		}
		self.Certificate = cert
	}
	if config == nil {
		config = &Config{Authorization: true}
	}
	if self.Handler == nil {
		self.Handler = http.NewServeMux()
	}
	if _, b := nodeConfigs[pattern]; !b {
		nodeConfigs[pattern] = config
	}
	self.Handler.Handle(pattern, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		self.Proxy(
			&NodePtr{
				Node:    self,
				Config:  nodeConfigs[pattern],
				Input:   r,
				Output:  w,
				Pattern: pattern,
				Handle:  handle,
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
