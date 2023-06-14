package node

import (
	"fmt"
	rate "github.com/godaddy-x/freego/cache/limiter"
	"github.com/godaddy-x/freego/ex"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/utils/concurrent"
	"github.com/godaddy-x/freego/zlog"
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

var filters []*FilterObject

var filterMap = map[string]*FilterObject{
	GatewayRateLimiterFilterName: {Name: GatewayRateLimiterFilterName, Order: -100, Filter: &GatewayRateLimiterFilter{}},
	ParameterFilterName:          {Name: ParameterFilterName, Order: -90, Filter: &ParameterFilter{}},
	SessionFilterName:            {Name: SessionFilterName, Order: -80, Filter: &SessionFilter{}},
	UserRateLimiterFilterName:    {Name: UserRateLimiterFilterName, Order: -70, Filter: &UserRateLimiterFilter{}},
	RoleFilterName:               {Name: RoleFilterName, Order: -60, Filter: &RoleFilter{}},
	ReplayFilterName:             {Name: ReplayFilterName, Order: -50, Filter: &ReplayFilter{}},
	PostHandleFilterName:         {Name: PostHandleFilterName, Order: math.MaxInt, Filter: &PostHandleFilter{}},
	RenderHandleFilterName:       {Name: RenderHandleFilterName, Order: math.MinInt, Filter: &RenderHandleFilter{}},
}

type FilterObject struct {
	Name         string
	Order        int
	Filter       Filter
	MatchPattern []string
}

func createFilterChain() error {
	var fs []interface{}
	for _, v := range filterMap {
		fs = append(fs, v)
	}
	fs = concurrent.NewSorter(fs, func(a, b interface{}) bool {
		o1 := a.(*FilterObject)
		o2 := b.(*FilterObject)
		return o1.Order < o2.Order
	}).Sort()
	for _, f := range fs {
		v := f.(*FilterObject)
		filters = append(filters, v)
		zlog.Printf("add filter [%s] successful", v.Name)
	}
	if len(filters) == 0 {
		return utils.Error("filter chain is nil")
	}
	return nil
}

type Filter interface {
	DoFilter(chain Filter, ctx *Context, args ...interface{}) error
}

type filterChain struct {
	pos int
}

func (self *filterChain) getFilters() []*FilterObject {
	return filters
}

func (self *filterChain) DoFilter(chain Filter, ctx *Context, args ...interface{}) error {
	fs := self.getFilters()
	for self.pos < len(fs) {
		f := fs[self.pos]
		if f == nil || f.Filter == nil {
			return ex.Throw{Code: ex.SYSTEM, Msg: fmt.Sprintf("filter [%s] is nil", f.Name)}
		}
		self.pos++
		if !utils.MatchFilterURL(ctx.Path, f.MatchPattern) {
			continue
		}
		return f.Filter.DoFilter(chain, ctx, args...)
	}
	return nil
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
	gatewayRateLimiter = rate.NewRateLimiter(rate.Option{Limit: 200, Bucket: 2000, Expire: 30, Distributed: true})
	methodRateLimiter  = rate.NewRateLimiter(rate.Option{Limit: 200, Bucket: 2000, Expire: 30, Distributed: true})
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

func (self *GatewayRateLimiterFilter) DoFilter(chain Filter, ctx *Context, args ...interface{}) error {
	//if b := gatewayRateLimiter.Allow("HttpThreshold"); !b {
	//	return ex.Throw{Code: 429, Msg: "the gateway request is full, please try again later"}
	//}
	return chain.DoFilter(chain, ctx, args...)
}

func (self *ParameterFilter) DoFilter(chain Filter, ctx *Context, args ...interface{}) error {
	if err := ctx.readParams(); err != nil {
		return err
	}
	return chain.DoFilter(chain, ctx, args...)
}

func (self *SessionFilter) DoFilter(chain Filter, ctx *Context, args ...interface{}) error {
	if ctx.RouterConfig.UseRSA || ctx.RouterConfig.UseHAX || ctx.RouterConfig.Guest { // 登录接口和游客模式跳过会话认证
		return chain.DoFilter(chain, ctx, args...)
	}
	if len(ctx.Token) == 0 {
		return ex.Throw{Code: http.StatusUnauthorized, Msg: "AccessToken is nil"}
	}
	if err := ctx.Subject.Verify(ctx.Token, ctx.GetJwtConfig().TokenKey, true); err != nil {
		return ex.Throw{Code: http.StatusUnauthorized, Msg: "AccessToken is invalid or expired", Err: err}
	}
	return chain.DoFilter(chain, ctx, args...)
}

func (self *UserRateLimiterFilter) DoFilter(chain Filter, ctx *Context, args ...interface{}) error {
	//if b := methodRateLimiter.Allow(ctx.Path); !b {
	//	return ex.Throw{Code: 429, Msg: "the method request is full, please try again later"}
	//}
	//if ctx.Authenticated() {
	//	if b := userRateLimiter.Allow(ctx.Subject.Sub); !b {
	//		return ex.Throw{Code: 429, Msg: "the access frequency is too fast, please try again later"}
	//	}
	//}
	return chain.DoFilter(chain, ctx, args...)
}

func (self *RoleFilter) DoFilter(chain Filter, ctx *Context, args ...interface{}) error {
	if ctx.PermConfig == nil {
		return chain.DoFilter(chain, ctx, args...)
	}
	_, need, err := ctx.PermConfig(ctx.Subject.GetSub(), ctx.Path)
	if err != nil {
		return ex.Throw{Code: http.StatusUnauthorized, Msg: "failed to read authorization resource", Err: err}
	} else if !need.Ready { // 无授权资源配置,跳过
		return chain.DoFilter(chain, ctx, args...)
	} else if need.NeedRole == nil || len(need.NeedRole) == 0 { // 无授权角色配置跳过
		return chain.DoFilter(chain, ctx, args...)
	}
	if need.NeedLogin == 0 { // 无登录状态,跳过
		return chain.DoFilter(chain, ctx, args...)
	} else if !ctx.Authenticated() { // 需要登录状态,会话为空,抛出异常
		return ex.Throw{Code: http.StatusUnauthorized, Msg: "login status required"}
	}
	roles, _, err := ctx.PermConfig(ctx.Subject.GetSub(), "", true)
	if err != nil {
		return ex.Throw{Code: http.StatusUnauthorized, Msg: "user roles read failed"}
	}
	access := 0
	needAccess := len(need.NeedRole)
	for _, cr := range roles {
		for _, nr := range need.NeedRole {
			if cr == nr {
				access++
				if need.MathchAll == 0 || access == needAccess { // 任意授权通过则放行,或已满足授权长度
					return chain.DoFilter(chain, ctx, args...)
				}
			}
		}
	}
	return ex.Throw{Code: http.StatusUnauthorized, Msg: "access defined"}
}

func (self *ReplayFilter) DoFilter(chain Filter, ctx *Context, args ...interface{}) error {
	//param := ctx.Params
	//if param == nil || len(param.Sign) == 0 {
	//	return nil
	//}
	//key := param.Sign
	//if c, err := node.CacheAware(); err != nil {
	//	return err
	//} else if b, err := c.GetInt64(key); err != nil {
	//	return err
	//} else if b > 1 {
	//	return ex.Throw{Code: http.StatusForbidden, Msg: "重复请求不受理"}
	//} else {
	//	c.Put(key, 1, int((param.Time+jwt.FIVE_MINUTES)/1000))
	//}
	return chain.DoFilter(chain, ctx, args...)
}

func (self *PostHandleFilter) DoFilter(chain Filter, ctx *Context, args ...interface{}) error {
	if err := ctx.Handle(); err != nil {
		return err
	}
	return chain.DoFilter(chain, ctx, args...)
}

func (self *RenderHandleFilter) DoFilter(chain Filter, ctx *Context, args ...interface{}) error {
	err := chain.DoFilter(chain, ctx, args...)
	if err == nil {
		err = defaultRenderPre(ctx)
	}
	if err != nil {
		err = defaultRenderError(ctx, err)
	}
	return defaultRenderTo(ctx)
}
