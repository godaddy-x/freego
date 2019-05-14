package node

import (
	"github.com/godaddy-x/freego/component/jwt"
	"github.com/godaddy-x/freego/component/log"
	"github.com/godaddy-x/freego/ex"
	"github.com/godaddy-x/freego/util"
	"html/template"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
)

type HttpNode struct {
	HookNode
	TemplDir string
}

func (self *HttpNode) GetHeader() error {
	if self.OverrideFunc.GetHeaderFunc == nil {
		return nil
	}
	return self.OverrideFunc.GetHeaderFunc(self.Context)
}

func (self *HttpNode) GetParams() error {
	if self.OverrideFunc.GetParamsFunc == nil {
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
	return self.OverrideFunc.GetParamsFunc(self.Context)
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
	if len(self.TemplDir) == 0 {
		if path, err := os.Getwd(); err != nil {
			return err
		} else {
			self.TemplDir = path
		}
	}
	node.SessionAware = self.SessionAware
	node.CacheAware = self.CacheAware
	node.Context = &Context{
		Host:      util.GetClientIp(input),
		Port:      self.Context.Port,
		Style:     HTTP,
		Method:    ptr.Pattern,
		Anonymous: ptr.Anonymous,
		Version:   self.Context.Version,
		Response: &Response{
			ContentEncoding: UTF8,
			ContentType:     APPLICATION_JSON,
			TemplDir:        self.TemplDir,
		},
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
	access_token := self.Context.Params.Token
	if len(access_token) == 0 {
		if !self.Context.Anonymous {
			return ex.Throw{Code: http.StatusUnauthorized, Msg: "获取授权令牌失败"}
		}
		return nil
	}
	checker, err := new(jwt.Subject).GetSubjectChecker(access_token)
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
	session := BuildJWTSession(checker)
	if session == nil {
		return ex.Throw{Code: http.StatusUnauthorized, Msg: "创建会话失败"}
	} else if session.Invalid() {
		return ex.Throw{Code: http.StatusUnauthorized, Msg: "会话已失效"}
	} else if session.IsTimeout() {
		return ex.Throw{Code: http.StatusUnauthorized, Msg: "会话已过期"}
	}
	userId, _ := util.StrToInt64(sub)
	self.Context.UserId = userId
	self.Context.Session = session
	return nil
}

func (self *HttpNode) ValidPermission() error {
	if self.Context.PermissionKey == nil {
		return nil
	}
	permission, err := self.Context.PermissionKey()
	if err != nil {
		return ex.Throw{Code: http.StatusUnauthorized, Msg: "读取授权资源失败", Err: err}
	}
	need, check := permission[self.Context.Method];
	if !check || need.NeedRole == nil || len(need.NeedRole) == 0 { // 没有查询到URL配置,则跳过
		return nil
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

func (self *HttpNode) TouchSession() error {
	return nil
}

func (self *HttpNode) Proxy(ptr *NodePtr) {
	// 1.初始化请求上下文
	ob := &HttpNode{}
	err := func() error {
		ptr.Node = ob
		if err := self.InitContext(ptr); err != nil {
			return err
		}
		// 2.校验会话有效性
		if err := ob.ValidSession(); err != nil {
			return err
		}
		// 3.校验访问权限
		if err := ob.ValidPermission(); err != nil {
			return err
		}
		// 4.上下文前置检测方法
		if err := ob.PreHandle(ob.OverrideFunc.PreHandleFunc); err != nil {
			return err
		}
		// 5.执行业务方法
		r1 := ptr.Handle(ob.Context) // r1异常格式,建议使用ex模式
		// 6.执行视图控制方法
		r2 := ob.PostHandle(ob.OverrideFunc.PostHandleFunc, r1)
		// 7.执行释放资源,记录日志方法
		if err := ob.AfterCompletion(ob.OverrideFunc.AfterCompletionFunc, r2); err != nil {
			return err
		}
		return nil
	}()
	// 8.更新会话有效性
	ob.LastAccessTouch(err)
}

func (self *HttpNode) LastAccessTouch(err error) {
	if err := self.TouchSession(); err != nil {
		log.Error("刷新会话失败", 0, log.AddError(err))
	}
	if err != nil {
		self.RenderError(err)
	}
}

func (self *HttpNode) PreHandle(handle func(ctx *Context) error) error {
	if handle == nil {
		return nil
	}
	return handle(self.Context)
}

func (self *HttpNode) PostHandle(handle func(resp *Response, err error) error, err error) error {
	if handle != nil {
		if err := handle(self.Context.Response, err); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	return self.RenderTo()
}

func (self *HttpNode) AfterCompletion(handle func(ctx *Context, resp *Response, err error) error, err error) error {
	var handle_err error
	if handle != nil {
		handle_err = handle(self.Context, self.Context.Response, err)
	}
	if err != nil {
		return err
	}
	if handle_err != nil {
		return handle_err
	}
	return nil
}

func (self *HttpNode) RenderError(err error) error {
	if self.OverrideFunc.RenderErrorFunc == nil {
		out := ex.Catch(err)
		if self.Context.Response.ContentType == APPLICATION_JSON {
			self.SetContentType(APPLICATION_JSON)
		}
		http_code := out.Code
		if http_code > http.StatusInternalServerError { // 大于500的都属于业务异常代码,重定义http错误代码为600
			http_code = 600
			out = ex.Throw{Code: out.Code, Msg: out.Msg}
		}
		resp := &RespDto{
			Status:  out.Code,
			Message: out.Msg,
			Time:    util.Time(),
			Data:    make(map[string]interface{}),
		}
		if result, err := util.JsonMarshal(resp); err != nil {
			self.Context.Output.WriteHeader(http.StatusInternalServerError)
			resp = &RespDto{
				Status:  http.StatusInternalServerError,
				Message: "系统发生未知错误",
				Time:    util.Time(),
				Data:    make(map[string]interface{}),
			}
			result, _ := util.JsonMarshal(resp)
			self.Context.Output.Write(result)
			log.Error(resp.Message, 0, log.AddError(err))
		} else {
			self.Context.Output.WriteHeader(http_code)
			self.Context.Output.Write(result)
		}
		return nil
	}
	return self.OverrideFunc.RenderErrorFunc(err)
}

func (self *HttpNode) RenderTo() error {
	switch self.Context.Response.ContentType {
	case TEXT_HTML:
		if templ, err := template.ParseFiles(self.Context.Response.TemplDir + self.Context.Response.RespView); err != nil {
			return err
		} else if err := templ.Execute(self.Context.Output, self.Context.Response.RespEntity); err != nil {
			return err
		}
	case APPLICATION_JSON:
		resp := &RespDto{
			Status:  http.StatusOK,
			Message: "success",
			Time:    util.Time(),
			Data:    self.Context.Response.RespEntity,
		}
		if resp.Data == nil {
			resp.Data = make(map[string]interface{})
		}
		if result, err := util.JsonMarshal(resp); err != nil {
			return ex.Throw{Code: http.StatusInternalServerError, Msg: "响应数据异常", Err: err}
		} else {
			self.SetContentType(APPLICATION_JSON)
			self.Context.Output.Write(result)
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

func (self *HttpNode) Router(pattern string, handle func(ctx *Context) error, anonymous ...bool) {
	if !strings.HasPrefix(pattern, "/") {
		pattern = util.AddStr("/", pattern)
	}
	if len(self.Context.Version) > 0 {
		pattern = util.AddStr("/", self.Context.Version, pattern)
	}
	if self.CacheAware == nil {
		panic("缓存服务尚未初始化")
	}
	http.DefaultServeMux.HandleFunc(pattern, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		anon := true
		if anonymous != nil && len(anonymous) > 0 {
			anon = anonymous[0]
		}
		self.Proxy(
			&NodePtr{
				self,
				r, w, pattern, anon, handle,
			},
		)
	}))
}

func (self *HttpNode) Html(ctx *Context, view string, data interface{}) error {
	if len(ctx.Response.TemplDir) == 0 {
		return ex.Throw{Code: http.StatusNotFound, Msg: "模版目录尚未设置"}
	}
	if len(view) == 0 {
		return ex.Throw{Code: http.StatusNotFound, Msg: "模版文件尚未设置"}
	}
	ctx.Response.ContentEncoding = UTF8
	ctx.Response.ContentType = TEXT_HTML
	ctx.Response.RespView = view
	ctx.Response.RespEntity = data
	return nil
}

func (self *HttpNode) Json(ctx *Context, data interface{}) error {
	if data == nil {
		data = map[string]interface{}{}
	}
	ctx.Response.ContentEncoding = UTF8
	ctx.Response.ContentType = APPLICATION_JSON
	ctx.Response.RespEntity = data
	return nil
}

func (self *HttpNode) SetContentType(contentType string) {
	self.Context.Output.Header().Set("Content-Type", contentType)
}

func (self *HttpNode) Connect(ctx *Context, s Session) error {
	//if err := self.SessionAware.CreateSession(s); err != nil {
	//	return err
	//}
	//ctx.Session = s
	return nil
}

func (self *HttpNode) Release(ctx *Context) error {
	ctx.Session.Stop()
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
