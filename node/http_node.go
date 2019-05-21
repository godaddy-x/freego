package node

import (
	"github.com/godaddy-x/freego/component/jwt"
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
	return nil
}

func (self *HttpNode) GetParams() error {
	r := self.Context.Input
	r.ParseForm()
	req := &ReqDto{}
	if r.Method == POST {
		result, err := ioutil.ReadAll(r.Body)
		if err != nil {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "获取参数失败", Err: err}
		}
		r.Body.Close()
		if len(result) > (MAX_VALUE_LEN * 5) {
			return ex.Throw{Code: http.StatusLengthRequired, Msg: "参数值长度溢出: " + util.AnyToStr(len(result))}
		}
		if err := util.JsonUnmarshal(result, req); err != nil {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "参数解析失败", Err: err}
		}
		if err := self.Context.SecurityCheck(req); err != nil {
			return err
		}
	} else if r.Method == GET {
		// return ex.Throw{Code: http.StatusUnsupportedMediaType, Msg: "暂不支持GET类型"}
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
		Host:          util.GetClientIp(input),
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
	if self.Customize {
		node.Customize = true
		return nil
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
		return nil
	}
	checker, err := new(jwt.Subject).GetSubjectChecker(param.Token)
	if err != nil {
		return ex.Throw{Code: http.StatusUnauthorized, Msg: "授权令牌无效", Err: err}
	} else {
		self.Context.Roles = checker.GetRole()
	}
	// 获取缓存的sub->signature key
	sub := checker.Subject.Payload.Sub
	sub_key := util.AddStr(JWT_SUB_, sub)
	jwt_secret_key := self.Context.SecretKey().JwtSecretKey
	if cacheObj, err := self.CacheAware(); err != nil {
		return ex.Throw{Code: http.StatusInternalServerError, Msg: "缓存服务异常", Err: err}
	} else if sigkey, b, err := cacheObj.Get(sub_key, nil); err != nil {
		return ex.Throw{Code: http.StatusInternalServerError, Msg: "读取缓存数据异常", Err: err}
	} else if !b {
		return ex.Throw{Code: http.StatusUnauthorized, Msg: "会话获取失败或已失效"}
	} else if v, b := sigkey.(string); !b {
		return ex.Throw{Code: http.StatusUnauthorized, Msg: "会话签名密钥无效"}
	} else if err := checker.Authentication(v, jwt_secret_key); err != nil {
		return ex.Throw{Code: http.StatusUnauthorized, Msg: "会话已失效或已超时", Err: err}
	}
	if session := BuildJWTSession(checker); session == nil {
		return ex.Throw{Code: http.StatusUnauthorized, Msg: "创建会话失败"}
	} else if session.Invalid() {
		return ex.Throw{Code: http.StatusUnauthorized, Msg: "会话已失效"}
	} else if session.IsTimeout() {
		return ex.Throw{Code: http.StatusUnauthorized, Msg: "会话已过期"}
	} else {
		userId, _ := util.StrToInt64(sub)
		self.Context.UserId = userId
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
	} else if _, b, err := c.Get(key, nil); err != nil {
		return err
	} else if b {
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
	// 1.初始化请求上下文
	ob := &HttpNode{}
	if err := func() error {
		ptr.Node = ob
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
	var ret error
	if self.OverrideFunc.AfterCompletionFunc != nil {
		ret = self.OverrideFunc.AfterCompletionFunc(self.Context, self.Context.Response, err)
	} else if err != nil {
		return err
	} else if ret != nil {
		return ret
	}
	return nil
}

func (self *HttpNode) RenderError(err error) error {
	out := ex.Catch(err)
	http_code := out.Code
	if http_code > http.StatusInternalServerError { // 大于500的都属于业务异常代码,重定义http错误代码为600
		http_code = 600
		out = ex.Throw{Code: out.Code, Msg: out.Msg}
	}
	resp := &RespDto{
		Code:    out.Code,
		Message: out.Msg,
		Time:    util.Time(),
		Data:    make(map[string]interface{}),
	}
	if self.Customize {
		if result, err := util.JsonMarshal(resp.Data); err != nil {
			log.Error(resp.Message, 0, log.AddError(err))
			return nil
		} else {
			self.Context.Output.Header().Set("Content-Type", TEXT_PLAIN)
			self.Context.Output.WriteHeader(http_code)
			self.Context.Output.Write(result)
		}
	} else {
		if result, err := util.JsonMarshal(resp); err != nil {
			log.Error(resp.Message, 0, log.AddError(err))
			return nil
		} else {
			self.Context.Output.Header().Set("Content-Type", APPLICATION_JSON)
			self.Context.Output.WriteHeader(http_code)
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
		if resp.Data == nil {
			resp.Data = make(map[string]interface{})
		}
		if self.Customize {
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
		if err := http.ListenAndServe(util.AddStr(self.Context.Host, ":", self.Context.Port), nil); err != nil {
			panic(err)
		}
	}()
}

func (self *HttpNode) Router(pattern string, handle func(ctx *Context) error, customize ...bool) {
	if !strings.HasPrefix(pattern, "/") {
		pattern = util.AddStr("/", pattern)
	}
	if len(self.Context.Version) > 0 {
		pattern = util.AddStr("/", self.Context.Version, pattern)
	}
	if self.CacheAware == nil {
		panic("缓存服务尚未初始化")
	}
	if customize != nil && len(customize) > 0 {
		if customize[0] {
			self.Customize = true
		}
	}
	http.DefaultServeMux.HandleFunc(pattern, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	if cacheObj, err := self.CacheAware(); err != nil {
		return ex.Throw{Code: http.StatusInternalServerError, Msg: "缓存服务异常", Err: err}
	} else if err := cacheObj.Put(util.AddStr(JWT_SUB_, sub), key, int(exp/1000)); err != nil {
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
