package node

import (
	"fmt"
	"math"
	"net/http"

	rate "github.com/godaddy-x/freego/cache/limiter"
	"github.com/godaddy-x/freego/ex"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/utils/concurrent"
	"github.com/godaddy-x/freego/zlog"
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
	GatewayRateLimiterFilterName: {Name: GatewayRateLimiterFilterName, Order: -1000, Filter: &GatewayRateLimiterFilter{}},
	ParameterFilterName:          {Name: ParameterFilterName, Order: -900, Filter: &ParameterFilter{}},
	SessionFilterName:            {Name: SessionFilterName, Order: -800, Filter: &SessionFilter{}},
	UserRateLimiterFilterName:    {Name: UserRateLimiterFilterName, Order: -700, Filter: &UserRateLimiterFilter{}},
	RoleFilterName:               {Name: RoleFilterName, Order: -600, Filter: &RoleFilter{}},
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
			return ex.Throw{Code: ex.SYSTEM, Msg: fmt.Sprintf("filter at index %d is nil", self.pos)}
		}
		self.pos++
		// 优化：Empty MatchPattern表示匹配所有，直接跳过URL检查
		if len(f.MatchPattern) > 0 && !utils.MatchFilterURL(ctx.Path, f.MatchPattern) {
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
	if ctx.RouterConfig == nil {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "router path invalid"}
	}
	if ctx.RouterConfig.UseRSA || ctx.RouterConfig.Guest { // 登录接口和游客模式跳过会话认证
		return chain.DoFilter(chain, ctx, args...)
	}
	// 验证JWT token（SessionFilter的核心职责）
	// 这是第一次明确验证JWT的签名和过期时间
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
	// RoleFilter 职责：进行基于角色的访问控制(RBAC)
	// 流程说明：
	// 1. 检查是否配置了roleRealm权限方法，未配置则跳过权限检查
	// 2. 检查用户是否已通过身份认证，未认证则跳过权限检查
	// 3. 通过roleRealm获取所需的角色列表(need.NeedRole)和用户拥有的角色列表(has.HasRole)
	// 4. 根据MatchAll标志判断是"全部匹配"还是"任意匹配"
	// 5. 匹配失败返回403未授权，匹配成功则继续执行后续过滤器
	// 安全增强：必须配置权限方法且已验证身份才能进行权限检查
	if ctx.roleRealm == nil {
		return chain.DoFilter(chain, ctx, args...)
	}
	// 必须通过身份认证
	if !ctx.Authenticated() {
		return chain.DoFilter(chain, ctx, args...)
	}
	// 优化：只调用一次roleRealm(false)，同时获取need和has信息，减少性能开销
	need, err := ctx.roleRealm(ctx, false) // 获取所需角色配置
	if err != nil {
		return err
	}
	if need == nil { // 无授权资源配置,跳过
		return chain.DoFilter(chain, ctx, args...)
	}
	if len(need.NeedRole) == 0 { // 无授权角色配置跳过
		return chain.DoFilter(chain, ctx, args...)
	}
	// 再次调用roleRealm(true)获取用户拥有角色（保持原有逻辑，支持分离查询）
	has, err := ctx.roleRealm(ctx, true) // 获取拥有角色配置
	if err != nil {
		return err
	}
	var hasRoles []int64
	if has != nil && len(has.HasRole) > 0 {
		hasRoles = has.HasRole
	}

	// 优化：直接遍历hasRoles，对need.NeedRole进行匹配
	if need.MatchAll {
		// MatchAll: 必须满足所有所需角色
		// 对每个needRole检查是否在hasRoles中存在
		// 时间复杂度O(m*n)，但实际应用中角色数量通常很少(≤10个)，性能开销可忽略
		for _, needRole := range need.NeedRole {
			found := false
			for _, hasRole := range hasRoles {
				if hasRole == needRole {
					found = true
					break
				}
			}
			if !found {
				return ex.Throw{Code: http.StatusUnauthorized, Msg: "access defined"}
			}
		}
		return chain.DoFilter(chain, ctx, args...)
	} else {
		// MatchAny: 只需满足任意一个所需角色
		for _, hasRole := range hasRoles {
			for _, needRole := range need.NeedRole {
				if hasRole == needRole {
					return chain.DoFilter(chain, ctx, args...)
				}
			}
		}
		return ex.Throw{Code: http.StatusUnauthorized, Msg: "access defined"}
	}
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
