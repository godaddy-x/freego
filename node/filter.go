package node

import (
	rate "github.com/godaddy-x/freego/cache/limiter"
	"github.com/godaddy-x/freego/ex"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/utils/concurrent"
	"github.com/godaddy-x/freego/utils/jwt"
	"math"
	"net/http"
)

const (
	GatewayRateLimiterFilterName = "GatewayRateLimiterFilter"
	ParameterFilterName          = "ParameterFilter"
	SessionFilterName            = "SessionFilter"
	UserRateLimiterFilterName    = "UserRateLimiterFilter"
	RoleFilterName               = "RoleFilter"
	ReplayFilterName             = "ReplayFilter"
	PostHandleFilterName         = "PostHandleFilter"
	RenderHandleFilterName       = "RenderHandleFilter"
)

var filters []Filter

type filterSortBy struct {
	order  int
	filter Filter
}

var filterMap = map[string]filterSortBy{
	GatewayRateLimiterFilterName: {order: 10, filter: &GatewayRateLimiterFilter{}},
	ParameterFilterName:          {order: 20, filter: &ParameterFilter{}},
	SessionFilterName:            {order: 30, filter: &SessionFilter{}},
	UserRateLimiterFilterName:    {order: 40, filter: &UserRateLimiterFilter{}},
	RoleFilterName:               {order: 50, filter: &RoleFilter{}},
	ReplayFilterName:             {order: 60, filter: &ReplayFilter{}},
	PostHandleFilterName:         {order: math.MaxInt, filter: &PostHandleFilter{}},
	RenderHandleFilterName:       {order: math.MinInt, filter: &RenderHandleFilter{}},
}

func doFilterChain(ob *HttpNode, args ...interface{}) error {
	chain := &filterChain{pos: 0}
	return chain.DoFilter(chain, &FilterObject{HttpNode: ob, PostHandle: ob.Context.RouterConfig.postHandle}, args...)
}

func createFilterChain() error {
	var fs []interface{}
	for _, v := range filterMap {
		fs = append(fs, v)
	}
	fs = concurrent.NewSorter(fs, func(a, b interface{}) bool {
		o1 := a.(filterSortBy)
		o2 := b.(filterSortBy)
		return o1.order < o2.order
	}).Sort()
	for _, f := range fs {
		filters = append(filters, f.(filterSortBy).filter)
	}
	if len(filters) == 0 {
		return utils.Error("filter chain is nil")
	}
	return nil
}

type FilterObject struct {
	HttpNode   *HttpNode
	PostHandle func(*Context) error
}

type Filter interface {
	DoFilter(chain Filter, object *FilterObject, args ...interface{}) error
}

type filterChain struct {
	pos int
}

func (self *filterChain) getFilters() []Filter {
	return filters
}

func (self *filterChain) DoFilter(chain Filter, object *FilterObject, args ...interface{}) error {
	fs := self.getFilters()
	if self.pos == len(fs) {
		return nil
	}
	f := fs[self.pos]
	self.pos++
	return f.DoFilter(chain, object, args...)
}

type GatewayRateLimiterFilter struct{}
type ParameterFilter struct{}
type SessionFilter struct{}
type UserRateLimiterFilter struct{}
type RoleFilter struct{}
type ReplayFilter struct{}
type PostHandleFilter struct{}
type RenderHandleFilter struct{}

var (
	gatewayRateLimiter = rate.NewRateLimiter(rate.Option{Limit: 50, Bucket: 1000, Expire: 30, Distributed: true})
	methodRateLimiter  = rate.NewRateLimiter(rate.Option{Limit: 50, Bucket: 500, Expire: 30, Distributed: true})
	userRateLimiter    = rate.NewRateLimiter(rate.Option{Limit: 5, Bucket: 10, Expire: 30, Distributed: true})
)

func SetGatewayRateLimiter(option rate.Option) {
	gatewayRateLimiter = rate.NewRateLimiter(option)
}

func SetMethodRateLimiter(option rate.Option) {
	methodRateLimiter = rate.NewRateLimiter(option)
}

func SetUserRateLimiter(option rate.Option) {
	userRateLimiter = rate.NewRateLimiter(option)
}

func (self *GatewayRateLimiterFilter) DoFilter(chain Filter, object *FilterObject, args ...interface{}) error {
	if b := gatewayRateLimiter.Allow("HttpThreshold"); !b {
		return ex.Throw{Code: 429, Msg: "the gateway request is full, please try again later"}
	}
	return chain.DoFilter(chain, object, args...)
}

func (self *ParameterFilter) DoFilter(chain Filter, object *FilterObject, args ...interface{}) error {
	if err := object.HttpNode.getHeader(); err != nil {
		return err
	}
	if err := object.HttpNode.getParams(); err != nil {
		return err
	}
	if err := object.HttpNode.paddDevice(); err != nil {
		return err
	}
	return chain.DoFilter(chain, object, args...)
}

func (self *SessionFilter) DoFilter(chain Filter, object *FilterObject, args ...interface{}) error {
	if object.HttpNode.Context.RouterConfig.Login || object.HttpNode.Context.RouterConfig.Guest { // 登录接口和游客模式跳过会话认证
		return chain.DoFilter(chain, object, args...)
	}
	if len(object.HttpNode.Context.Token) == 0 {
		return ex.Throw{Code: http.StatusUnauthorized, Msg: "AccessToken is nil"}
	}
	subject := &jwt.Subject{}
	if err := subject.Verify(object.HttpNode.Context.Token, object.HttpNode.Context.JwtConfig().TokenKey); err != nil {
		return ex.Throw{Code: http.StatusUnauthorized, Msg: "AccessToken is invalid or expired", Err: err}
	}
	object.HttpNode.Context.Roles = subject.GetTokenRole()
	object.HttpNode.Context.Subject = subject.Payload
	return chain.DoFilter(chain, object, args...)
}

func (self *UserRateLimiterFilter) DoFilter(chain Filter, object *FilterObject, args ...interface{}) error {
	ctx := object.HttpNode.Context
	if b := userRateLimiter.Allow(ctx.Method); !b {
		return ex.Throw{Code: 429, Msg: "the method request is full, please try again later"}
	}
	if ctx.Authenticated() {
		if b := userRateLimiter.Allow(ctx.Subject.Sub); !b {
			return ex.Throw{Code: 429, Msg: "the access frequency is too fast, please try again later"}
		}
	}
	return chain.DoFilter(chain, object, args...)
}

func (self *RoleFilter) DoFilter(chain Filter, object *FilterObject, args ...interface{}) error {
	if object.HttpNode.Context.PermConfig == nil {
		return chain.DoFilter(chain, object, args...)
	}
	need, err := object.HttpNode.Context.PermConfig(object.HttpNode.Context.Method)
	if err != nil {
		return ex.Throw{Code: http.StatusUnauthorized, Msg: "failed to read authorization resource", Err: err}
	} else if !need.ready { // 无授权资源配置,跳过
		return chain.DoFilter(chain, object, args...)
	} else if need.NeedRole == nil || len(need.NeedRole) == 0 { // 无授权角色配置跳过
		return chain.DoFilter(chain, object, args...)
	}
	if need.NeedLogin == 0 { // 无登录状态,跳过
		return chain.DoFilter(chain, object, args...)
	} else if !object.HttpNode.Context.Authenticated() { // 需要登录状态,会话为空,抛出异常
		return ex.Throw{Code: http.StatusUnauthorized, Msg: "login status required"}
	}
	access := 0
	needAccess := len(need.NeedRole)
	for _, cr := range object.HttpNode.Context.Roles {
		for _, nr := range need.NeedRole {
			if cr == nr {
				access++
				if need.MathchAll == 0 || access == needAccess { // 任意授权通过则放行,或已满足授权长度
					return chain.DoFilter(chain, object, args...)
				}
			}
		}
	}
	return ex.Throw{Code: http.StatusUnauthorized, Msg: "access defined"}
}

func (self *ReplayFilter) DoFilter(chain Filter, object *FilterObject, args ...interface{}) error {
	//param := object.HttpNode.Context.Params
	//if param == nil || len(param.Sign) == 0 {
	//	return nil
	//}
	//key := param.Sign
	//if c, err := object.HttpNode.CacheAware(); err != nil {
	//	return err
	//} else if b, err := c.GetInt64(key); err != nil {
	//	return err
	//} else if b > 1 {
	//	return ex.Throw{Code: http.StatusForbidden, Msg: "重复请求不受理"}
	//} else {
	//	c.Put(key, 1, int((param.Time+jwt.FIVE_MINUTES)/1000))
	//}
	return chain.DoFilter(chain, object, args...)
}

func (self *PostHandleFilter) DoFilter(chain Filter, object *FilterObject, args ...interface{}) error {
	if err := object.PostHandle(object.HttpNode.Context); err != nil {
		return err
	}
	return chain.DoFilter(chain, object, args...)
}

func (self *RenderHandleFilter) DoFilter(chain Filter, object *FilterObject, args ...interface{}) error {
	err := chain.DoFilter(chain, object, args...)
	if err == nil {
		err = defaultRenderPre(object.HttpNode.Context)
	}
	if err != nil {
		defaultRenderError(object.HttpNode.Context, err)
	}
	return defaultRenderTo(object.HttpNode.Context)
}
