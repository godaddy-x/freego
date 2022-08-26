package node

import (
	"github.com/godaddy-x/freego/ex"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/utils/concurrent"
	"github.com/godaddy-x/freego/utils/jwt"
	"github.com/godaddy-x/freego/zlog"
	"net/http"
)

const (
	parameterFilterName    = "parameterFilter"
	sessionFilterName      = "sessionFilter"
	roleFilterName         = "roleFilter"
	replayAttackFilterName = "replayAttackFilter"
	aroundHandleFilterName = "aroundHandleFilter"
)

var filterChainMap = map[string]FilterChainObject{
	parameterFilterName:    {order: 10, filter: parameterFilter},
	sessionFilterName:      {order: 20, filter: sessionFilter},
	roleFilterName:         {order: 30, filter: roleFilter},
	replayAttackFilterName: {order: 40, filter: replayAttackFilter},
	aroundHandleFilterName: {order: 50, filter: defaultAroundHandleFilter},
}

var filterChainArr []func(FilterObject) error

type FilterObject struct {
	Ptr     *NodePtr
	Http    *HttpNode
	Args    []interface{}
	OrderBy int
}

type FilterChainObject struct {
	order  int
	filter func(FilterObject) error
}

func createFilterChain() error {
	var arr []interface{}
	for _, v := range filterChainMap {
		arr = append(arr, v)
	}
	arr = concurrent.NewSorter(arr, func(a, b interface{}) bool {
		o1 := a.(FilterChainObject)
		o2 := b.(FilterChainObject)
		return o1.order < o2.order
	}).Sort()
	for _, v := range arr {
		filterChainArr = append(filterChainArr, v.(FilterChainObject).filter)
	}
	if len(filterChainArr) == 0 {
		return utils.Error("filter chain is nil")
	}
	return nil
}

func AddFilter(name string, object FilterChainObject) {
	if len(name) == 0 {
		panic("filter chain name invalid")
	}
	filterChainMap[name] = object
}

func parameterFilter(filter FilterObject) error {
	ob := filter.Http
	if err := ob.getHeader(); err != nil {
		return err
	}
	if err := ob.getParams(); err != nil {
		return err
	}
	if err := ob.paddDevice(); err != nil {
		return err
	}
	return nil
}

func sessionFilter(filter FilterObject) error {
	ob := filter.Http
	if ob.RouterConfig.Login || ob.RouterConfig.Guest { // 登录接口和游客模式跳过会话认证
		return nil
	}
	if len(ob.Context.Token) == 0 {
		return ex.Throw{Code: http.StatusUnauthorized, Msg: "AccessToken is ni"}
	}
	subject := &jwt.Subject{}
	if err := subject.Verify(ob.Context.Token, ob.Context.JwtConfig().TokenKey); err != nil {
		return ex.Throw{Code: http.StatusUnauthorized, Msg: "AccessToken is invalid or expired", Err: err}
	}
	ob.Context.Roles = subject.GetTokenRole()
	ob.Context.Subject = subject.Payload
	return nil
}

func roleFilter(filter FilterObject) error {
	ob := filter.Http
	if ob.Context.PermConfig == nil {
		return nil
	}
	need, err := ob.Context.PermConfig(ob.Context.Method)
	if err != nil {
		return ex.Throw{Code: http.StatusUnauthorized, Msg: "failed to read authorization resource", Err: err}
	} else if !need.ready { // 无授权资源配置,跳过
		return nil
	} else if need.NeedRole == nil || len(need.NeedRole) == 0 { // 无授权角色配置跳过
		return nil
	}
	if need.NeedLogin == 0 { // 无登录状态,跳过
		return nil
	} else if !ob.Context.Authenticated() { // 需要登录状态,会话为空,抛出异常
		return ex.Throw{Code: http.StatusUnauthorized, Msg: "login status required"}
	}
	access := 0
	needAccess := len(need.NeedRole)
	for _, cr := range ob.Context.Roles {
		for _, nr := range need.NeedRole {
			if cr == nr {
				access++
				if need.MathchAll == 0 || access == needAccess { // 任意授权通过则放行,或已满足授权长度
					return nil
				}
			}
		}
	}
	return ex.Throw{Code: http.StatusUnauthorized, Msg: "access defined"}
}

func replayAttackFilter(filter FilterObject) error {
	ob := filter.Http
	param := ob.Context.Params
	if param == nil || len(param.Sign) == 0 {
		return nil
	}
	key := param.Sign
	if c, err := ob.CacheAware(); err != nil {
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

func defaultAroundHandleFilter(filter FilterObject) error {
	if err := filter.Ptr.Handle(filter.Http.Context); err != nil {
		return err
	}
	return nil
}

func AroundInvokerFilter(handle func(invoker *NodePtr, ctx *Context) error) {
	if handle == nil {
		return
	}
	sortObj := filterChainMap[aroundHandleFilterName]
	sortObj.filter = func(filter FilterObject) error {
		return handle(filter.Ptr, filter.Http.Context)
	}
	filterChainMap[aroundHandleFilterName] = sortObj
	zlog.Println("around invoker filter loaded successful")
}
