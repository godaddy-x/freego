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
	PostHandleFilterName         = "PostHandleFilter"
	RenderHandleFilterName       = "RenderHandleFilter"
)

var filterMap = map[string]*FilterObject{
	GatewayRateLimiterFilterName: {Name: GatewayRateLimiterFilterName, Order: -100, Filter: &GatewayRateLimiterFilter{}},
	ParameterFilterName:          {Name: ParameterFilterName, Order: -90, Filter: &ParameterFilter{}},
	SessionFilterName:            {Name: SessionFilterName, Order: -80, Filter: &SessionFilter{}},
	UserRateLimiterFilterName:    {Name: UserRateLimiterFilterName, Order: -70, Filter: &UserRateLimiterFilter{}},
	RoleFilterName:               {Name: RoleFilterName, Order: -60, Filter: &RoleFilter{}},
	PostHandleFilterName:         {Name: PostHandleFilterName, Order: math.MaxInt, Filter: &PostHandleFilter{}},
	RenderHandleFilterName:       {Name: RenderHandleFilterName, Order: math.MinInt, Filter: &RenderHandleFilter{}},
}

type FilterObject struct {
	Name         string
	Order        int
	Filter       Filter
	MatchPattern []string
}

func createFilterChain(extFilters []*FilterObject) ([]*FilterObject, error) {
	var filters []*FilterObject
	var fs []interface{}
	for _, v := range filterMap {
		extFilters = append(extFilters, v)
	}
	for _, v := range extFilters {
		for _, check := range fs {
			if check.(*FilterObject).Name == v.Name {
				panic("filter name exist: " + v.Name)
			}
		}
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
		return nil, utils.Error("filter chain is nil")
	}
	return filters, nil
}

type Filter interface {
	DoFilter(chain Filter, ctx *Context, args ...interface{}) error
}

type filterChain struct {
	pos     int
	filters []*FilterObject
}

func (self *filterChain) DoFilter(chain Filter, ctx *Context, args ...interface{}) error {
	fs := self.filters
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
	if ctx.RouterConfig.UseRSA || ctx.RouterConfig.Guest { // 登录接口和游客模式跳过会话认证
		return chain.DoFilter(chain, ctx, args...)
	}
	if len(ctx.Subject.GetRawBytes()) == 0 {
		return ex.Throw{Code: http.StatusUnauthorized, Msg: "token is nil"}
	}
	if err := ctx.Subject.Verify(utils.Bytes2Str(ctx.Subject.GetRawBytes()), ctx.GetJwtConfig().TokenKey, true); err != nil {
		return ex.Throw{Code: http.StatusUnauthorized, Msg: "token invalid or expired", Err: err}
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
	if ctx.roleRealm == nil || !ctx.Authenticated() { // 未配置权限方法或非登录状态跳过
		return chain.DoFilter(chain, ctx, args...)
	}
	need, err := ctx.roleRealm(ctx, false)
	if err != nil {
		return err
	}
	if need == nil { // 无授权资源配置,跳过
		return chain.DoFilter(chain, ctx, args...)
	}
	if len(need.NeedRole) == 0 { // 无授权角色配置跳过
		return chain.DoFilter(chain, ctx, args...)
	}
	//if !need.NeedLogin { // 无登录状态,跳过
	//	return chain.DoFilter(chain, ctx, args...)
	//} else if !ctx.Authenticated() { // 需要登录状态,会话为空,抛出异常
	//	return ex.Throw{Code: http.StatusUnauthorized, Msg: "login status required"}
	//}
	has, err := ctx.roleRealm(ctx, true)
	if err != nil {
		return err
	}
	var hasRoles []int64
	if has != nil && len(has.HasRole) > 0 {
		hasRoles = has.HasRole
	}
	accessCount := 0
	needAccess := len(need.NeedRole)
	for _, hasRole := range hasRoles {
		for _, needRole := range need.NeedRole {
			if hasRole == needRole {
				accessCount++
				if !need.MatchAll || accessCount == needAccess { // 任意授权通过则放行,或已满足授权长度
					return chain.DoFilter(chain, ctx, args...)
				}
			}
		}
	}
	return ex.Throw{Code: http.StatusUnauthorized, Msg: "access defined"}
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
