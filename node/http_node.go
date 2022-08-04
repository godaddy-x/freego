package node

import (
	"fmt"
	"github.com/godaddy-x/freego/cache"
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
		headers["User-Agent"] = r.Header.Get("User-Agent")
		headers["Authorization"] = r.Header.Get("Authorization")
	}
	self.Context.Token = headers["Authorization"]
	self.Context.Headers = headers
	return nil
}

func (self *HttpNode) GetParams() error {
	r := self.Context.Input
	if r.Method == POST {
		r.ParseForm()
		result, err := ioutil.ReadAll(r.Body)
		if err != nil {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "获取参数失败", Err: err}
		}
		r.Body.Close()
		if len(result) > (MAX_VALUE_LEN * 5) {
			return ex.Throw{Code: http.StatusLengthRequired, Msg: "参数值长度溢出: " + util.AnyToStr(len(result))}
		}
		if !self.Config.Original {
			req := &ReqDto{}
			if err := util.JsonUnmarshal(result, req); err != nil {
				return ex.Throw{Code: http.StatusBadRequest, Msg: "参数解析失败", Err: err}
			}
			if err := self.Context.SecurityCheck(req, self.Config); err != nil {
				return err
			}
			return nil
		}
		data := map[string]interface{}{}
		if result == nil || len(result) == 0 {
			for k, v := range r.Form {
				if len(v) == 0 {
					continue
				}
				data[k] = v[0]
			}
		} else {
			if err := util.JsonUnmarshal(result, &data); err != nil {
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
	node.OverrideFunc = self.OverrideFunc
	node.CreateAt = util.Time()
	node.SessionAware = self.SessionAware
	node.CacheAware = self.CacheAware
	node.Config = ptr.Config
	node.Context = &Context{
		Host:          util.ClientIP(input),
		Port:          self.Context.Port,
		Style:         HTTP,
		Method:        ptr.Pattern,
		Version:       self.Context.Version,
		Response:      &Response{UTF8, APPLICATION_JSON, nil},
		Input:         input,
		Output:        output,
		SecretKey:     self.Context.SecretKey,
		PermissionKey: self.Context.PermissionKey,
		Storage:       make(map[string]interface{}),
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
	if self.Config.Authorization {
		if len(self.Context.Token) == 0 {
			return ex.Throw{Code: http.StatusUnauthorized, Msg: "授权令牌为空"}
		}
		subject := &jwt.Subject{}
		if err := subject.Verify(self.Context.Token, self.Context.SecretKey().TokenKey); err != nil {
			return ex.Throw{Code: http.StatusUnauthorized, Msg: "授权令牌无效或已过期", Err: err}
		}
		self.Context.Roles = subject.GetTokenRole()
		self.Context.Subject = subject.Payload
		return nil
	}
	if len(self.Context.Token) > 0 {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "非验证权限接口无需上传授权令牌"}
	}
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
	if self.Context.PermissionKey == nil {
		return nil
	}
	need, err := self.Context.PermissionKey(self.Context.Method)
	if err != nil {
		return ex.Throw{Code: http.StatusUnauthorized, Msg: "读取授权资源失败", Err: err}
	} else if need == nil { // 无授权资源配置,跳过
		return nil
	} else if need.NeedRole == nil || len(need.NeedRole) == 0 { // 无授权角色配置跳过
		return nil
	}
	if need.NeedLogin == 0 { // 无登录状态,跳过
		return nil
	} else if self.Context.Subject == nil { // 需要登录状态,会话为空,抛出异常
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
	if self.Context.Subject == nil {
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
		data, _ := util.ToJsonBase64(self.Context.Response.ContentEntity)
		resp := &RespDto{
			Code:    http.StatusOK,
			Message: "success",
			Time:    util.Time(),
			Data:    data,
			Nonce:   self.Context.Params.Nonce,
		}
		if self.Config.ResponseAesEncrypt {
			resp.Plan = 1
			data, _ = util.AesEncrypt(data, self.Context.GetTokenSecret(), util.AddStr(resp.Nonce, resp.Time))
			resp.Data = data
		} else if self.Config.ResponseRsaEncrypt {
			resp.Plan = 2
		}
		resp.Sign = self.Context.GetDataSign(data, resp.Nonce, resp.Time, resp.Plan)
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
		url := util.AddStr(self.Context.Host, ":", self.Context.Port)
		log.Printf("http service【%s】has been started successfully", url)
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
		self.Proxy(&NodePtr{self, nodeConfigs[pattern], r, w, pattern, handle})
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
