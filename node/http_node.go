package node

import (
	"github.com/godaddy-x/freego/cache"
	"github.com/godaddy-x/freego/component/jwt"
	"github.com/godaddy-x/freego/component/limiter"
	"github.com/godaddy-x/freego/component/log"
	"github.com/godaddy-x/freego/ex"
	"github.com/godaddy-x/freego/util"
	"io/ioutil"
	"net/http"
	"strings"
)

type HttpNode struct {
	HookNode
}

func (self *HttpNode) GetHeader() error {
	if self.Option.Customize {
		r := self.Context.Input
		headers := map[string]string{}
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
		self.Context.Headers = headers
	}
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
		if !self.Option.Customize {
			req := &ReqDto{}
			if err := util.JsonUnmarshal(result, req); err != nil {
				return ex.Throw{Code: http.StatusBadRequest, Msg: "参数解析失败", Err: err}
			}
			if err := self.Context.SecurityCheck(req, self.Option.Textplain); err != nil {
				return err
			}
			return nil
		}
		data := map[string]interface{}{}
		if err := util.JsonUnmarshal(result, &data); err != nil {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "参数解析失败", Err: err}
		}
		self.Context.Params = &ReqDto{Data: data}
		return nil
	} else if r.Method == GET {
		if !self.Option.Customize {
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
	if self.OverrideFunc == nil {
		node.OverrideFunc = &OverrideFunc{}
	} else {
		node.OverrideFunc = self.OverrideFunc
	}
	node.CreateAt = util.Time()
	node.SessionAware = self.SessionAware
	node.CacheAware = self.CacheAware
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
	if v, b := self.OptionMap[ptr.Pattern]; b {
		node.Option = v
	} else {
		node.Option = &Option{}
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
	if util.HasStr(d, ANDROID) {
		self.Context.Device = ANDROID
	} else if util.HasStr(d, IPHONE) || util.HasStr(d, IPAD) {
		self.Context.Device = IPHONE
	} else {
		self.Context.Device = WEB
	}
	return nil
}

func (self *HttpNode) ValidSession() error {
	param := self.Context.Params
	if param == nil || len(param.Token) == 0 {
		if self.Option.Authenticate {
			return ex.Throw{Code: http.StatusUnauthorized, Msg: "授权令牌为空"}
		}
		return nil
	}
	checker, err := new(jwt.Subject).GetSubjectChecker(param.Token)
	if err != nil {
		return ex.Throw{Code: http.StatusUnauthorized, Msg: "授权令牌无效", Err: err}
	} else {
		self.Context.Roles = checker.Subject.GetRole()
	}
	// 获取缓存的sub->signature key
	sub := checker.Subject.Payload.Sub
	sub_key := util.AddStr(JWT_SUB_, sub)
	jwt_secret_key := self.Context.SecretKey().JwtSecretKey
	cacheObj, err := self.CacheAware()
	if err != nil {
		return ex.Throw{Code: http.StatusInternalServerError, Msg: "缓存服务异常", Err: err}
	}
	sigkey, err := cacheObj.GetString(sub_key)
	if err != nil {
		return ex.Throw{Code: http.StatusInternalServerError, Msg: "读取缓存数据异常", Err: err}
	}
	if len(sigkey) == 0 {
		return ex.Throw{Code: http.StatusUnauthorized, Msg: "会话获取失败或已失效"}
	}
	if err := checker.Authentication(sigkey, jwt_secret_key); err != nil {
		return ex.Throw{Code: http.StatusUnauthorized, Msg: "会话已失效或已超时", Err: err}
	}
	// 判断是否同一个IP请求
	//if self.Context.Host != checker.Subject.Payload.Aud {
	//
	//}
	if session := BuildJWTSession(checker); session == nil {
		return ex.Throw{Code: http.StatusUnauthorized, Msg: "创建会话失败"}
	} else if session.Invalid() {
		return ex.Throw{Code: http.StatusUnauthorized, Msg: "会话已失效"}
	} else if session.IsTimeout() {
		return ex.Throw{Code: http.StatusUnauthorized, Msg: "会话已过期"}
	} else {
		self.Context.UserId = sub
		self.Context.Session = session
	}
	return nil
}

func (self *HttpNode) ValidReplayAttack() error {
	param := self.Context.Params
	if param == nil || len(param.Sign) == 0 {
		return nil
	}
	key := util.AddStr(JWT_SIG_, param.Sign)
	if c, err := self.CacheAware(); err != nil {
		return err
	} else if b, err := c.GetInt64(key); err != nil {
		return err
	} else if b > 1 {
		return ex.Throw{Code: http.StatusForbidden, Msg: "重复请求不受理"}
	} else {
		c.Put(key, 1, int((param.Time+jwt.FIVE_MINUTES)/1000))
	}
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
	} else if self.Context.Session == nil { // 需要登录状态,会话为空,抛出异常
		return ex.Throw{Code: http.StatusUnauthorized, Msg: "获取授权令牌失败"}
	}
	access := 0
	need_access := len(need.NeedRole)
	for _, cr := range self.Context.Roles {
		for _, nr := range need.NeedRole {
			if cr == nr {
				access ++
				if need.MathchAll == 0 || access == need_access { // 任意授权通过则放行,或已满足授权长度
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
		// 6.执行业务方法
		biz_ret := ptr.Handle(ob.Context) // 抛出业务异常,建议使用ex模式
		// 7.执行视图控制方法
		post_ret := ob.PostHandle(biz_ret)
		// 8.执行释放资源,记录日志方法
		if err := ob.AfterCompletion(post_ret); err != nil {
			return err
		}
		return nil
	}(); err != nil {
		ob.RenderError(err)
	}
}

func (self *HttpNode) PreHandle() error {
	if self.OverrideFunc.PreHandleFunc == nil {
		return nil
	}
	return self.OverrideFunc.PreHandleFunc(self.Context)
}

func (self *HttpNode) PostHandle(err error) error {
	if self.OverrideFunc.PostHandleFunc != nil {
		if err := self.OverrideFunc.PostHandleFunc(self.Context.Response, err); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	return self.RenderTo()
}

func (self *HttpNode) AfterCompletion(err error) error {
	if self.OverrideFunc.AfterCompletionFunc != nil {
		return self.OverrideFunc.AfterCompletionFunc(self.Context, self.Context.Response, err)
	}
	return err
}

func (self *HttpNode) RenderError(err error) error {
	out := ex.Catch(err)
	resp := &RespDto{
		Code:    out.Code,
		Message: out.Msg,
		Time:    util.Time(),
		Data:    make(map[string]interface{}),
	}
	if self.Option != nil && self.Option.Customize {
		self.Context.Output.Header().Set("Content-Type", TEXT_PLAIN)
		self.Context.Output.WriteHeader(http.StatusOK)
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
		resp := &RespDto{
			Code:    http.StatusOK,
			Message: "success",
			Time:    util.Time(),
			Data:    self.Context.Response.ContentEntity,
		}
		if self.Option.Textplain == jwt.AES {
			access_key, b := self.Context.Storage[TEXTPLAIN_ACCESS_KEY]
			if !b {
				return ex.Throw{Code: http.StatusUnsupportedMediaType, Msg: "无效的签名数据"}
			}
			d := self.Context.Response.ContentEntity
			if d == nil {
				d = map[string]interface{}{}
			}
			respByte, _ := util.JsonMarshal(d)
			resp.Data = util.AesEncrypt(util.Bytes2Str(respByte), util.Substr(util.MD5(access_key.(string)), 0, 16))
		} else if self.Option.Textplain == jwt.RSA {

		}
		if resp.Data == nil {
			resp.Data = make(map[string]interface{})
		}
		if self.Option.Customize {
			if result, err := util.JsonMarshal(resp.Data); err != nil {
				return ex.Throw{Code: http.StatusInternalServerError, Msg: "响应数据异常", Err: err}
			} else {
				self.Context.Output.Header().Set("Content-Type", APPLICATION_JSON)
				self.Context.Output.Write(result)
			}
		} else {
			if result, err := util.JsonMarshal(resp); err != nil {
				return ex.Throw{Code: http.StatusInternalServerError, Msg: "响应数据异常", Err: err}
			} else {
				self.Context.Output.Header().Set("Content-Type", APPLICATION_JSON)
				self.Context.Output.Write(result)
			}
		}
	default:
		return ex.Throw{Code: http.StatusUnsupportedMediaType, Msg: "无效的响应格式"}
	}
	return nil
}

func (self *HttpNode) StartServer() {
	go func() {
		if err := http.ListenAndServe(util.AddStr(self.Context.Host, ":", self.Context.Port), self.limiterHandler()); err != nil {
			log.Error("初始化http服务失败", 0, log.AddError(err))
		}
	}()
}

func (self *HttpNode) limiterHandler() http.Handler {
	cache := new(cache.LocalMapManager).NewCache(20160, 20160)
	if self.RateOpetion == nil {
		self.RateOpetion = &rate.RateOpetion{
			Limit:  500,
			Bucket: 500,
			Expire: 1209600,
		}
	}
	limiter := rate.NewLocalLimiterByOption(cache, &rate.RateOpetion{
		"HttpThreshol",
		self.RateOpetion.Limit,
		self.RateOpetion.Bucket,
		self.RateOpetion.Expire,
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if limiter.Validate(nil) {
			node := &HttpNode{}
			node.Context = &Context{
				Output: w,
			}
			node.RenderError(ex.Throw{Code: 429, Msg: ""})
			return
		}
		self.Handler.ServeHTTP(w, r)
	})
}

func (self *HttpNode) Router(pattern string, handle func(ctx *Context) error, option *Option) {
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
	if option == nil {
		option = &Option{}
	}
	if self.OptionMap == nil {
		self.OptionMap = make(map[string]*Option)
	}
	if self.Handler == nil {
		self.Handler = http.NewServeMux()
	}
	self.OptionMap[pattern] = option
	self.Handler.Handle(pattern, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		self.Proxy(&NodePtr{self, r, w, pattern, handle})
	}))
}

func (self *HttpNode) Json(ctx *Context, data interface{}) error {
	if data == nil {
		data = map[string]interface{}{}
	}
	ctx.Response = &Response{UTF8, APPLICATION_JSON, data}
	return nil
}

func (self *HttpNode) Text(ctx *Context, data string) error {
	ctx.Response = &Response{UTF8, TEXT_PLAIN, data}
	return nil
}

func (self *HttpNode) LoginBySubject(sub, key string, exp int64) error {
	cacheObj, err := self.CacheAware()
	if err != nil {
		return ex.Throw{Code: http.StatusInternalServerError, Msg: "缓存服务异常", Err: err}
	}
	k := util.AddStr(JWT_SUB_, sub)
	if err := cacheObj.Put(k, key, int(exp/1000)); err != nil {
		return ex.Throw{Code: http.StatusInternalServerError, Msg: "初始化用户密钥失败", Err: err}
	}
	return nil
}

func (self *HttpNode) LogoutBySubject(subs ...string) error {
	if subs == nil {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "用户密钥不能为空"}
	}
	subkeys := make([]string, 0, len(subs))
	for _, v := range subs {
		subkeys = append(subkeys, util.AddStr(JWT_SUB_, v))
	}
	if cacheObj, err := self.CacheAware(); err != nil {
		return ex.Throw{Code: http.StatusInternalServerError, Msg: "缓存服务异常", Err: err}
	} else if err := cacheObj.Del(subkeys...); err != nil {
		return ex.Throw{Code: http.StatusInternalServerError, Msg: "删除用户密钥失败", Err: err}
	}
	return nil
}
