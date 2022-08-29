package node

import (
	"github.com/godaddy-x/freego/ex"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/utils/concurrent"
	"github.com/godaddy-x/freego/zlog"
	"net/http"
)

const (
	PostHandleInterceptorName   = "PostHandleInterceptor"
	RenderHandleInterceptorName = "RenderHandleInterceptor"
)

var interceptors []Interceptor

type interceptorSortBy struct {
	order       int
	interceptor Interceptor
}

var interceptorMap = map[string]interceptorSortBy{
	RenderHandleInterceptorName: {order: -999, interceptor: &RenderHandleInterceptor{}},
}

func doInterceptorChain(handle func(*Context) error, ctx *Context) error {
	chain := &interceptorChain{pos: -1, handle: handle, ctx: ctx}
	return chain.doInterceptor()
}

func createInterceptorChain() error {
	var fs []interface{}
	for _, v := range interceptorMap {
		fs = append(fs, v)
	}
	fs = concurrent.NewSorter(fs, func(a, b interface{}) bool {
		o1 := a.(interceptorSortBy)
		o2 := b.(interceptorSortBy)
		return o1.order < o2.order
	}).Sort()
	for _, f := range fs {
		interceptors = append(interceptors, f.(interceptorSortBy).interceptor)
	}
	return nil
}

type Interceptor interface {
	PreHandle(ctx *Context) (bool, error)
	PostHandle(ctx *Context) error
	AfterCompletion(ctx *Context, err error) error
}

type interceptorChain struct {
	handle func(*Context) error
	ctx    *Context
	pos    int
}

func (self *interceptorChain) doInterceptor() error {
	if b, err := self.ApplyPreHandle(); !b || err != nil {
		return err
	}
	if err := self.ApplyAfterCompletion(self.ApplyPostHandle()); err != nil {
		return err
	}
	return nil
}

func (self *interceptorChain) getInterceptors() []Interceptor {
	return interceptors
}

func (self *interceptorChain) ApplyPreHandle() (bool, error) {
	interceptors := self.getInterceptors()
	if len(interceptors) == 0 {
		return true, nil
	}
	for i := 0; i < len(interceptors); i++ {
		or := interceptors[i]
		if b, err := or.PreHandle(self.ctx); !b || err != nil {
			return false, self.ApplyAfterCompletion(err)
		}
		self.pos = i
	}
	return true, nil
}

func (self *interceptorChain) ApplyPostHandle() error {
	if err := self.handle(self.ctx); err != nil {
		return err
	}
	interceptors := self.getInterceptors()
	if len(interceptors) == 0 {
		return nil
	}
	for i := len(interceptors) - 1; i >= 0; i-- {
		if err := interceptors[i].PostHandle(self.ctx); err != nil {
			return err
		}
	}
	return nil
}

func (self *interceptorChain) ApplyAfterCompletion(err error) error {
	interceptors := self.getInterceptors()
	if len(interceptors) == 0 {
		return err
	}
	for i := self.pos; i >= 0; i-- {
		if err := interceptors[i].AfterCompletion(self.ctx, err); err != nil {
			zlog.Error("interceptor.ApplyAfterCompletion failed", 0, zlog.AddError(err))
		}
	}
	return err
}

type RenderHandleInterceptor struct{}

func (self *RenderHandleInterceptor) PreHandle(ctx *Context) (bool, error) {
	return true, nil
}

func (self *RenderHandleInterceptor) PostHandle(ctx *Context) error {
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
		if routerConfig.Original {
			if result, err := utils.JsonMarshal(ctx.Response.ContentEntity); err != nil {
				return ex.Throw{Code: http.StatusInternalServerError, Msg: "response JSON data failed", Err: err}
			} else {
				ctx.Response.ContentEntityByte = result
			}
			break
		}
		if ctx.Response.ContentEntity == nil {
			return ex.Throw{Code: http.StatusInternalServerError, Msg: "response ContentEntity is nil"}
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
			key = ctx.ClientCert.PubkeyBase64
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
		resp.Sign = ctx.GetDataSign(resp.Data.(string), resp.Nonce, resp.Time, resp.Plan, key)
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

func (self *RenderHandleInterceptor) AfterCompletion(ctx *Context, err error) error {
	return err
}
