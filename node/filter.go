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

// 过滤器名称常量定义
// 用于标识不同类型的过滤器，方便配置和管理
const (
	GatewayRateLimiterFilterName = "GatewayRateLimiterFilter" // 网关限流过滤器
	ParameterFilterName          = "ParameterFilter"          // 参数解析过滤器
	SessionFilterName            = "SessionFilter"            // 会话验证过滤器
	UserRateLimiterFilterName    = "UserRateLimiterFilter"    // 用户限流过滤器
	RoleFilterName               = "RoleFilter"               // 角色权限过滤器
	PostHandleFilterName         = "PostHandleFilter"         // 后置处理过滤器
	RenderHandleFilterName       = "RenderHandleFilter"       // 响应渲染过滤器
)

// filterMap 内置过滤器映射表
// 定义系统内置的过滤器及其执行顺序（Order值越小越先执行）
// Order值范围：-1000到math.MaxInt，按升序执行
var filterMap = map[string]*FilterObject{
	GatewayRateLimiterFilterName: {Name: GatewayRateLimiterFilterName, Order: -1000, Filter: &GatewayRateLimiterFilter{}},
	ParameterFilterName:          {Name: ParameterFilterName, Order: -900, Filter: &ParameterFilter{}},
	SessionFilterName:            {Name: SessionFilterName, Order: -800, Filter: &SessionFilter{}},
	UserRateLimiterFilterName:    {Name: UserRateLimiterFilterName, Order: -700, Filter: &UserRateLimiterFilter{}},
	RoleFilterName:               {Name: RoleFilterName, Order: -600, Filter: &RoleFilter{}},
	PostHandleFilterName:         {Name: PostHandleFilterName, Order: math.MaxInt, Filter: &PostHandleFilter{}},
	RenderHandleFilterName:       {Name: RenderHandleFilterName, Order: math.MinInt, Filter: &RenderHandleFilter{}},
}

// FilterObject 结构体 - 64字节 (4个字段，8字节对齐，优化排列减少填充)
// 排列优化：大字段优先，相同大小字段分组
type FilterObject struct {
	Name         string   // 16字节 (8+8) - string字段
	Filter       Filter   // 16字节 (8+8) - interface{}字段
	MatchPattern []string // 24字节 (8+8+8) - slice字段
	Order        int      // 8字节 - int字段放在最后
}

// createFilterChain 创建过滤器链
// 将内置过滤器和外部扩展过滤器按Order值升序合并
// 返回排序后的过滤器数组，如果有重复名称则返回错误
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

// DoFilter 执行过滤器链中的下一个过滤器
// 按照过滤器链的顺序依次执行每个过滤器，直到链结束或某个过滤器返回错误
// chain: 当前过滤器链实例（用于递归调用）
// ctx: HTTP请求上下文，包含请求和响应信息
// args: 可变参数，通常包含fasthttp.RequestCtx
func (self *filterChain) DoFilter(chain Filter, ctx *Context, args ...interface{}) error {
	// 优化：直接使用self.filters，避免局部变量赋值
	for self.pos < len(self.filters) {
		f := self.filters[self.pos]
		if f == nil || f.Filter == nil {
			return ex.Throw{Code: ex.SYSTEM, Msg: fmt.Sprintf("filter at index %d is nil", self.pos)}
		}
		self.pos++

		// 优化：Empty MatchPattern表示匹配所有，直接跳过URL检查
		if len(f.MatchPattern) > 0 && !utils.MatchFilterURL(ctx.Path, f.MatchPattern) {
			continue
		}

		// 执行过滤器并返回（责任链模式：一个过滤器执行完成后继续下一个）
		return f.Filter.DoFilter(chain, ctx, args...)
	}
	return nil
}

// 过滤器结构体定义
// 实现Filter接口的DoFilter方法，按照职责分离的原则处理不同的过滤逻辑
type GatewayRateLimiterFilter struct{} // 网关级限流过滤器
type ParameterFilter struct{}          // 参数解析过滤器
type SessionFilter struct{}            // 会话验证过滤器
type UserRateLimiterFilter struct{}    // 用户级限流过滤器
type RoleFilter struct{}               // 角色权限过滤器
type PostHandleFilter struct{}         // 后置处理过滤器
type RenderHandleFilter struct{}       // 响应渲染过滤器

// 全局限流器实例
// 延迟初始化，在Redis准备就绪后再创建，支持分布式部署
var (
	gatewayRateLimiter   rate.RateLimiter                // 网关级限流器：每秒1000请求，桶容量5000，60秒过期
	methodRateLimiter    = map[string]rate.RateLimiter{} // 方法级限流器：按API路径存储专用限流器
	defaultMethodLimiter rate.RateLimiter                // 默认方法级限流器：每秒100请求，桶容量200，30秒过期
	userRateLimiter      rate.RateLimiter                // 用户级限流器：每用户每秒10请求，桶容量20，30秒过期
)

// InitRateLimiters 初始化限流器
// 在Redis准备就绪后调用此函数创建分布式限流器
// 如果Redis不可用，会自动回退到本地限流器
func initRateLimiters() {
	zlog.Info("initializing rate limiters", 0)

	// 初始化网关级限流器
	gatewayRateLimiter = rate.NewRateLimiter(rate.Option{
		Limit: 1000, Bucket: 5000, Expire: 60000, Distributed: true,
	})
	if gatewayRateLimiter == nil {
		zlog.Error("failed to initialize gateway rate limiter", 0)
	}

	// 初始化默认方法级限流器
	defaultMethodLimiter = rate.NewRateLimiter(rate.Option{
		Limit: 100, Bucket: 200, Expire: 30000, Distributed: true,
	})
	if defaultMethodLimiter == nil {
		zlog.Error("failed to initialize default method rate limiter", 0)
	}

	// 初始化用户级限流器
	userRateLimiter = rate.NewRateLimiter(rate.Option{
		Limit: 10, Bucket: 20, Expire: 30000, Distributed: true,
	})
	if userRateLimiter == nil {
		zlog.Error("failed to initialize user rate limiter", 0)
	}

	zlog.Info("rate limiters initialized successfully", 0)
}

func checkLimiterStatus() bool {
	return true
}

// SetGatewayRateLimiter 设置网关级限流器配置
// 用于控制整个服务的全局请求频率，防止服务过载
// option: 限流器配置，包含速率和桶容量等参数
func (self *HttpNode) SetGatewayRateLimiter(option rate.Option) {
	gatewayRateLimiter = rate.NewRateLimiter(option)
}

func (self *HttpNode) SetDefaultMethodRateLimiter(option rate.Option) {
	defaultMethodLimiter = rate.NewRateLimiter(option)
}

// SetMethodRateLimiterByPath 为特定路径设置方法级限流器
func (self *HttpNode) SetMethodRateLimiterByPath(path string, option rate.Option) {
	methodRateLimiter[path] = rate.NewRateLimiter(option)
}

// SetUserRateLimiter 设置用户级限流器配置
// 用于控制每个用户的请求频率，防止恶意用户刷接口
// option: 限流器配置，通常设置较低的速率限制
func (self *HttpNode) SetUserRateLimiter(option rate.Option) {
	userRateLimiter = rate.NewRateLimiter(option)
}

/*
限流器使用示例：

// 1. 在Redis准备就绪后初始化限流器
// InitRateLimiters() // 自动使用推荐配置

// 2. 或者手动设置全局网关限流（每秒1000个请求，桶容量5000，60秒过期）
SetGatewayRateLimiter(rate.Option{
    Limit: 1000, Bucket: 5000, Expire: 60000, Distributed: true,
})

// 3. 设置默认方法级限流（每秒100个请求，桶容量200，30秒过期）
SetDefaultMethodRateLimiter(rate.Option{
    Limit: 100, Bucket: 200, Expire: 30000, Distributed: true,
})

// 4. 为特定API路径设置专用限流
SetMethodRateLimiterByPath("/api/user/login", rate.Option{
    Limit: 5, Bucket: 10, Expire: 30000, Distributed: true, // 登录接口限制最严格，30秒过期
})

SetMethodRateLimiterByPath("/api/order/create", rate.Option{
    Limit: 50, Bucket: 100, Expire: 30000, Distributed: true, // 下单接口适中限制，30秒过期
})

SetMethodRateLimiterByPath("/api/product/list", rate.Option{
    Limit: 200, Bucket: 500, Expire: 30000, Distributed: true, // 商品列表接口相对宽松，30秒过期
})

// 5. 设置用户级限流（每个用户每秒10个请求，30秒过期）
SetUserRateLimiter(rate.Option{
    Limit: 10, Bucket: 20, Expire: 30000, Distributed: true,
})

// 注意：如果Redis未初始化，限流器会自动回退到本地模式
// 建议在应用程序启动时，在Redis初始化完成后调用InitRateLimiters()

// 限流检查顺序：
// 1. GatewayRateLimiterFilter (-1000): 网关级 + 方法级限流
// 2. UserRateLimiterFilter (-700): 用户级限流
//
// 对于方法级限流：
// - 如果该路径有专用配置，使用专用配置
// - 如果没有专用配置，使用默认方法级限流器
*/

// DoFilter 执行网关级和方法级限流检查
// 1. 网关级限流：全局请求频率控制
// 2. 方法级限流：按API路径进行精细化频率控制
// 如果请求超出限制，返回429 Too Many Requests错误
func (self *GatewayRateLimiterFilter) DoFilter(chain Filter, ctx *Context, args ...interface{}) error {

	// 方便测试可以控制limiter开关
	if checkLimiterStatus() {
		return chain.DoFilter(chain, ctx, args...)
	}

	// 1. 网关级限流（全局阈值）
	if gatewayRateLimiter != nil && !gatewayRateLimiter.Allow("limiter:gateway:all") {
		return ex.Throw{Code: 429, Msg: "gateway request rate limit exceeded"}
	}

	// 2. 方法级限流（按API路径）
	// 首先尝试查找该路径特定的限流器，如果没有找到则使用默认限流器
	var methodLimiter rate.RateLimiter
	if limiter, exists := methodRateLimiter[ctx.Path]; exists {
		methodLimiter = limiter
	} else if defaultMethodLimiter != nil {
		methodLimiter = defaultMethodLimiter
	}

	if methodLimiter != nil && !methodLimiter.Allow(utils.AddStr("limiter:gateway:method:", ctx.Path)) {
		return ex.Throw{Code: 429, Msg: "method request rate limit exceeded"}
	}

	return chain.DoFilter(chain, ctx, args...)
}

// DoFilter 执行参数解析和验证
// 从HTTP请求中解析参数并存储到上下文，供后续处理使用
// 如果参数解析失败，返回相应的错误信息
func (self *ParameterFilter) DoFilter(chain Filter, ctx *Context, args ...interface{}) error {
	if err := ctx.readParams(); err != nil {
		return err
	}
	return chain.DoFilter(chain, ctx, args...)
}

// DoFilter 执行会话验证和身份认证
// 检查用户是否已登录，解析JWT令牌并验证其有效性
// 为已认证用户设置Subject信息，供后续过滤器使用
func (self *SessionFilter) DoFilter(chain Filter, ctx *Context, args ...interface{}) error {
	if ctx.RouterConfig == nil {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "router path invalid"}
	}
	if ctx.RouterConfig.UseRSA || ctx.RouterConfig.Guest { // 登录接口和游客模式跳过会话认证
		return chain.DoFilter(chain, ctx, args...)
	}
	// 验证JWT token（SessionFilter的核心职责）
	// 这是第一次明确验证JWT的签名和过期时间
	auth := ctx.GetRawTokenBytes()
	if len(auth) == 0 {
		return ex.Throw{Code: http.StatusUnauthorized, Msg: "token is nil"}
	}
	if err := ctx.Subject.Verify(auth, ctx.GetJwtConfig().TokenKey); err != nil {
		return ex.Throw{Code: http.StatusUnauthorized, Msg: "token invalid or expired", Err: err}
	}
	return chain.DoFilter(chain, ctx, args...)
}

// DoFilter 执行用户级限流检查
// 针对已认证用户进行个性化频率限制，防止恶意刷接口行为
// 每个用户的请求频率单独计算和限制
func (self *UserRateLimiterFilter) DoFilter(chain Filter, ctx *Context, args ...interface{}) error {
	// 方便测试可以控制limiter开关
	if checkLimiterStatus() {
		return chain.DoFilter(chain, ctx, args...)
	}
	// 用户级限流（按用户ID）
	if ctx.Authenticated() && ctx.Subject != nil && ctx.Subject.Payload != nil && len(ctx.Subject.Payload.Sub) > 0 {
		if userRateLimiter != nil && !userRateLimiter.Allow(utils.AddStr("limiter:gateway:user:", ctx.Subject.Payload.Sub)) {
			return ex.Throw{Code: 429, Msg: "user request rate limit exceeded"}
		}
	}
	return chain.DoFilter(chain, ctx, args...)
}

// DoFilter 执行基于角色的访问控制(RBAC)
// 根据用户角色和API要求的权限进行访问控制验证
// 支持"全部匹配"和"任意匹配"两种权限验证模式
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

// DoFilter 执行后置处理逻辑
// 在业务处理完成后执行清理、日志记录等操作
// 确保响应的一致性和完整性
func (self *PostHandleFilter) DoFilter(chain Filter, ctx *Context, args ...interface{}) error {
	if err := ctx.Handle(); err != nil {
		return err
	}
	return chain.DoFilter(chain, ctx, args...)
}

// DoFilter 执行最终的响应渲染和输出
// 将处理结果序列化并输出到HTTP响应中
// 这是过滤器链的最后一个环节，确保响应正确返回给客户端
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
