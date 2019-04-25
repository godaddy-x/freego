package node

import (
	"github.com/godaddy-x/freego/component/log"
	"github.com/godaddy-x/freego/ex"
	"github.com/godaddy-x/freego/util"
	"go.uber.org/zap"
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

func (self *HttpNode) GetHeader(input interface{}) error {
	if self.OverrideFunc.GetHeaderFunc == nil {
		r := input.(*http.Request)
		headers := map[string]string{}
		if len(r.Header) > 0 {
			i := 0
			for k, v := range r.Header {
				i++
				if i > MAX_HEADER_SIZE {
					return ex.Try{Code: http.StatusLengthRequired, Msg: "请求头数量溢出: " + util.AnyToStr(i)}
				}
				if len(k) > MAX_FIELD_LEN {
					return ex.Try{Code: http.StatusLengthRequired, Msg: "参数名长度溢出: " + util.AnyToStr(len(k))}
				}
				v0 := v[0]
				if len(v0) > MAX_VALUE_LEN {
					return ex.Try{Code: http.StatusLengthRequired, Msg: "参数值长度溢出: " + util.AnyToStr(len(v0))}
				}
				headers[k] = v0
			}
		}
		self.Context.Headers = headers
		return nil
	}
	return self.OverrideFunc.GetHeaderFunc(input)
}

func (self *HttpNode) GetParams(input interface{}) error {
	if self.OverrideFunc.GetParamsFunc == nil {
		r := input.(*http.Request)
		r.ParseForm()
		params := map[string]interface{}{}
		if r.Method == GET {
			i := 0
			for k, v := range r.Form {
				i++
				if i > MAX_PARAMETER_SIZE {
					return ex.Try{Code: http.StatusLengthRequired, Msg: "请求参数数量溢出: " + util.AnyToStr(i)}
				}
				if len(k) > MAX_FIELD_LEN {
					return ex.Try{Code: http.StatusLengthRequired, Msg: "参数名长度溢出: " + util.AnyToStr(len(k))}
				}
				v0 := strings.Join(v, "")
				if len(v0) > MAX_VALUE_LEN {
					return ex.Try{Code: http.StatusLengthRequired, Msg: "参数值长度溢出: " + util.AnyToStr(len(v0))}
				}
				params[k] = v0
			}
		} else if r.Method == POST {
			result, err := ioutil.ReadAll(r.Body)
			if err != nil {
				return ex.Try{Code: http.StatusBadRequest, Msg: "获取请求参数失败", Err: err}
			}
			r.Body.Close()
			if len(result) > (MAX_VALUE_LEN * 2) {
				return ex.Try{Code: http.StatusLengthRequired, Msg: "参数值长度溢出: " + util.AnyToStr(len(result))}
			}
			if err := util.JsonUnmarshal(result, &params); err != nil {
				return ex.Try{Code: http.StatusBadRequest, Msg: "请求参数读取失败", Err: err}
			}
			for k, _ := range params {
				if len(k) > MAX_FIELD_LEN {
					return ex.Try{Code: http.StatusBadRequest, Msg: "参数名长度溢出: " + util.AnyToStr(len(k))}
				}
			}
		} else if r.Method == PUT {
			return ex.Try{Code: http.StatusUnsupportedMediaType, Msg: "暂不支持PUT类型"}
		} else if r.Method == PATCH {
			return ex.Try{Code: http.StatusUnsupportedMediaType, Msg: "暂不支持PATCH类型"}
		} else if r.Method == DELETE {
			return ex.Try{Code: http.StatusUnsupportedMediaType, Msg: "暂不支持DELETE类型"}
		} else {
			return ex.Try{Code: http.StatusUnsupportedMediaType, Msg: "未知的请求类型"}
		}
		self.Context.Params = params
		return nil
	}
	return self.OverrideFunc.GetParamsFunc(input)
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
	node.Context = &Context{
		Host:      util.GetClientIp(input),
		Style:     HTTP,
		Method:    ptr.Pattern,
		Anonymous: ptr.Anonymous,
		Version:   self.Context.Version,
		Response: &Response{
			ContentEncoding: UTF8,
			ContentType:     APPLICATION_JSON,
			TemplDir:        self.TemplDir,
		},
		Input:  input,
		Output: output,
	}
	if err := node.GetHeader(input); err != nil {
		return err
	}
	if err := node.GetParams(input); err != nil {
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
	if self.SessionAware == nil {
		return ex.Try{Code: http.StatusInternalServerError, Msg: "会话管理器尚未初始化"}
	}
	// header获取token
	accessToken := self.Context.GetHeader(Global.SessionIdName)
	// header为空尝试通过参数获取token
	if len(accessToken) == 0 {
		if v := self.Context.GetParam(Global.SessionIdName); v != nil {
			var b bool
			accessToken, b = v.(string)
			if !b {
				return ex.Try{Code: http.StatusUnauthorized, Msg: "授权令牌读取失败"}
			}
		}
	}
	if len(accessToken) == 0 {
		if !self.Context.Anonymous {
			return ex.Try{Code: http.StatusUnauthorized, Msg: "授权令牌读取失败"}
		}
		return nil
	}
	var sessionId string
	spl := strings.Split(accessToken, ".")
	if len(spl) == 3 {
		sessionId = spl[2]
	} else {
		sessionId = accessToken
	}
	session, err := self.SessionAware.ReadSession(sessionId)
	if err != nil || session == nil {
		return ex.Try{Code: http.StatusUnauthorized, Msg: "获取会话失败", Err: err}
	}
	session.SetAccessToken(accessToken)
	if !session.IsValid() {
		self.SessionAware.DeleteSession(session)
		return ex.Try{Code: http.StatusUnauthorized, Msg: "会话已失效"}
	}
	if err := session.Validate(); err != nil {
		return ex.Try{Code: http.StatusUnauthorized, Msg: "会话校验失败或已失效", Err: err}
	}
	self.Context.Session = session
	return nil
}

func (self *HttpNode) TouchSession() error {
	if self.Context == nil || self.Context.Session == nil {
		return nil
	}
	session := self.Context.Session
	if session.IsValid() {
		if err := self.SessionAware.UpdateSession(session); err != nil {
			return ex.Try{Code: http.StatusInternalServerError, Msg: "更新会话失败", Err: err}
		}
	} else {
		if err := self.SessionAware.DeleteSession(session); err != nil {
			return ex.Try{Code: http.StatusInternalServerError, Msg: "删除会话失败", Err: err}
		}
	}
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
		// 3.上下文前置检测方法
		if err := ob.PreHandle(ob.OverrideFunc.PreHandleFunc); err != nil {
			return err
		}
		// 4.执行业务方法
		r1 := ptr.Handle(ob.Context) // r1异常格式,建议使用ex模式
		// 5.执行视图控制方法
		r2 := ob.PostHandle(ob.OverrideFunc.PostHandleFunc, r1)
		// 6.执行释放资源,记录日志方法
		if err := ob.AfterCompletion(ob.OverrideFunc.AfterCompletionFunc, r2); err != nil {
			return err
		}
		return nil
	}()
	// 7.更新会话有效性
	ob.LastAccessTouch(err)
}

func (self *HttpNode) LastAccessTouch(err error) {
	if err := self.TouchSession(); err != nil {
		log.Error(err.Error())
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
	if handle != nil {
		if err := handle(self.Context, self.Context.Response, err); err != nil {
			return err
		}
	} else if err != nil {
		return err
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
			out = ex.Try{Code: out.Code, Msg: out.Msg}
		}
		if result, err := util.JsonMarshal(out); err != nil {
			self.Context.Output.WriteHeader(http.StatusInternalServerError)
			self.Context.Output.Write(util.Str2Bytes("系统发生未知错误"))
			log.Error("系统发生未知错误", zap.String("error", err.Error()))
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
		if result, err := util.JsonMarshal(self.Context.Response.RespEntity); err != nil {
			return err
		} else {
			self.SetContentType(APPLICATION_JSON)
			self.Context.Output.Write(result)
		}
	default:
		return ex.Try{Code: http.StatusUnsupportedMediaType, Msg: "无效的响应格式"}
	}
	return nil
}

func (self *HttpNode) StartServer() {
	go func() {
		if err := http.ListenAndServe(self.Context.Host, nil); err != nil {
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
		return ex.Try{Code: http.StatusNotFound, Msg: "模版目录尚未设置"}
	}
	if len(view) == 0 {
		return ex.Try{Code: http.StatusNotFound, Msg: "模版文件尚未设置"}
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
	if err := self.SessionAware.CreateSession(s); err != nil {
		return err
	}
	ctx.Session = s
	return nil
}

func (self *HttpNode) Release(ctx *Context) error {
	ctx.Session.Stop()
	return nil
}
