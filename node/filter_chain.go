package node

import (
	"github.com/godaddy-x/freego/ex"
	"github.com/godaddy-x/freego/utils/jwt"
	"net/http"
)

const (
	ParameterFilterName  = "ParameterFilter"
	SessionFilterName    = "SessionFilter"
	RoleFilterName       = "RoleFilter"
	ReplayFilterName     = "ReplayFilter"
	PostHandleFilterName = "PostHandleFilter"
)

var filters []Filter

type FilterSortBy struct {
	order  int
	filter Filter
}

var filterMap = map[string]FilterSortBy{
	ParameterFilterName:  {order: 10, filter: &ParameterFilter{}},
	SessionFilterName:    {order: 20, filter: &SessionFilter{}},
	RoleFilterName:       {order: 30, filter: &RoleFilter{}},
	ReplayFilterName:     {order: 40, filter: &ReplayFilter{}},
	PostHandleFilterName: {order: 50, filter: &PostHandleFilter{}},
}

type FilterArg struct {
	NodePtr    *NodePtr
	HttpNode   *HttpNode
	postHandle func(ctx *Context) error
	Args       []interface{}
}

type Filter interface {
	DoFilter(chain Filter, args *FilterArg) error
}

type FilterChain struct {
	pos int
}

func (self *FilterChain) getFilters() []Filter {
	return filters
}

func (self *FilterChain) DoFilter(chain Filter, args *FilterArg) error {
	fs := self.getFilters()
	if args.NodePtr.Completed || self.pos == len(fs) {
		return nil
	}
	f := fs[self.pos]
	self.pos++
	return f.DoFilter(chain, args)
}

type SessionFilter struct{}
type ParameterFilter struct{}
type RoleFilter struct{}
type ReplayFilter struct{}
type PostHandleFilter struct{}

func (self *ParameterFilter) DoFilter(chain Filter, args *FilterArg) error {
	if err := args.HttpNode.getHeader(); err != nil {
		return err
	}
	if err := args.HttpNode.getParams(); err != nil {
		return err
	}
	if err := args.HttpNode.paddDevice(); err != nil {
		return err
	}
	return chain.DoFilter(chain, args)
}

func (self *SessionFilter) DoFilter(chain Filter, args *FilterArg) error {
	if args.HttpNode.RouterConfig.Login || args.HttpNode.RouterConfig.Guest { // 登录接口和游客模式跳过会话认证
		return chain.DoFilter(chain, args)
	}
	if len(args.HttpNode.Context.Token) == 0 {
		return ex.Throw{Code: http.StatusUnauthorized, Msg: "AccessToken is nil"}
	}
	subject := &jwt.Subject{}
	if err := subject.Verify(args.HttpNode.Context.Token, args.HttpNode.Context.JwtConfig().TokenKey); err != nil {
		return ex.Throw{Code: http.StatusUnauthorized, Msg: "AccessToken is invalid or expired", Err: err}
	}
	args.HttpNode.Context.Roles = subject.GetTokenRole()
	args.HttpNode.Context.Subject = subject.Payload
	return chain.DoFilter(chain, args)
}

func (self *RoleFilter) DoFilter(chain Filter, args *FilterArg) error {
	if args.HttpNode.Context.PermConfig == nil {
		return chain.DoFilter(chain, args)
	}
	need, err := args.HttpNode.Context.PermConfig(args.HttpNode.Context.Method)
	if err != nil {
		return ex.Throw{Code: http.StatusUnauthorized, Msg: "failed to read authorization resource", Err: err}
	} else if !need.ready { // 无授权资源配置,跳过
		return chain.DoFilter(chain, args)
	} else if need.NeedRole == nil || len(need.NeedRole) == 0 { // 无授权角色配置跳过
		return chain.DoFilter(chain, args)
	}
	if need.NeedLogin == 0 { // 无登录状态,跳过
		return chain.DoFilter(chain, args)
	} else if !args.HttpNode.Context.Authenticated() { // 需要登录状态,会话为空,抛出异常
		return ex.Throw{Code: http.StatusUnauthorized, Msg: "login status required"}
	}
	access := 0
	needAccess := len(need.NeedRole)
	for _, cr := range args.HttpNode.Context.Roles {
		for _, nr := range need.NeedRole {
			if cr == nr {
				access++
				if need.MathchAll == 0 || access == needAccess { // 任意授权通过则放行,或已满足授权长度
					return chain.DoFilter(chain, args)
				}
			}
		}
	}
	return ex.Throw{Code: http.StatusUnauthorized, Msg: "access defined"}
}

func (self *ReplayFilter) DoFilter(chain Filter, args *FilterArg) error {
	//param := args.HttpNode.Context.Params
	//if param == nil || len(param.Sign) == 0 {
	//	return nil
	//}
	//key := param.Sign
	//if c, err := args.HttpNode.CacheAware(); err != nil {
	//	return err
	//} else if b, err := c.GetInt64(key); err != nil {
	//	return err
	//} else if b > 1 {
	//	return ex.Throw{Code: http.StatusForbidden, Msg: "重复请求不受理"}
	//} else {
	//	c.Put(key, 1, int((param.Time+jwt.FIVE_MINUTES)/1000))
	//}
	return chain.DoFilter(chain, args)
}

func (self *PostHandleFilter) DoFilter(chain Filter, args *FilterArg) error {
	if err := args.NodePtr.PostHandle(args); err != nil {
		return err
	}
	return chain.DoFilter(chain, args)
}
