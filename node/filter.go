package node

import (
	"github.com/godaddy-x/freego/ex"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/utils/concurrent"
	"github.com/godaddy-x/freego/utils/jwt"
	"net/http"
)

const (
	ParameterFilterName = "ParameterFilter"
	SessionFilterName   = "SessionFilter"
	RoleFilterName      = "RoleFilter"
	ReplayFilterName    = "ReplayFilter"
)

var filters []Filter

type filterSortBy struct {
	order  int
	filter Filter
}

var filterMap = map[string]filterSortBy{
	ParameterFilterName: {order: 10, filter: &ParameterFilter{}},
	SessionFilterName:   {order: 20, filter: &SessionFilter{}},
	RoleFilterName:      {order: 30, filter: &RoleFilter{}},
	ReplayFilterName:    {order: 40, filter: &ReplayFilter{}},
}

func doFilterChain(ptr *NodePtr, ob *HttpNode, args ...interface{}) error {
	chain := &filterChain{}
	return chain.DoFilter(chain, &FilterObject{NodePtr: ptr, HttpNode: ob, Args: args})
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
	NodePtr  *NodePtr
	HttpNode *HttpNode
	Args     []interface{}
}

type Filter interface {
	DoFilter(chain Filter, object *FilterObject) error
}

type filterChain struct {
	pos int
}

func (self *filterChain) getFilters() []Filter {
	return filters
}

func (self *filterChain) DoFilter(chain Filter, object *FilterObject) error {
	fs := self.getFilters()
	if self.pos == len(fs) {
		return doInterceptorChain(object.NodePtr, object.HttpNode.Context)
	}
	f := fs[self.pos]
	self.pos++
	return f.DoFilter(chain, object)
}

type SessionFilter struct{}
type ParameterFilter struct{}
type RoleFilter struct{}
type ReplayFilter struct{}

func (self *ParameterFilter) DoFilter(chain Filter, object *FilterObject) error {
	if err := object.HttpNode.getHeader(); err != nil {
		return err
	}
	if err := object.HttpNode.getParams(); err != nil {
		return err
	}
	if err := object.HttpNode.paddDevice(); err != nil {
		return err
	}
	return chain.DoFilter(chain, object)
}

func (self *SessionFilter) DoFilter(chain Filter, object *FilterObject) error {
	if object.HttpNode.RouterConfig.Login || object.HttpNode.RouterConfig.Guest { // 登录接口和游客模式跳过会话认证
		return chain.DoFilter(chain, object)
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
	return chain.DoFilter(chain, object)
}

func (self *RoleFilter) DoFilter(chain Filter, object *FilterObject) error {
	if object.HttpNode.Context.PermConfig == nil {
		return chain.DoFilter(chain, object)
	}
	need, err := object.HttpNode.Context.PermConfig(object.HttpNode.Context.Method)
	if err != nil {
		return ex.Throw{Code: http.StatusUnauthorized, Msg: "failed to read authorization resource", Err: err}
	} else if !need.ready { // 无授权资源配置,跳过
		return chain.DoFilter(chain, object)
	} else if need.NeedRole == nil || len(need.NeedRole) == 0 { // 无授权角色配置跳过
		return chain.DoFilter(chain, object)
	}
	if need.NeedLogin == 0 { // 无登录状态,跳过
		return chain.DoFilter(chain, object)
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
					return chain.DoFilter(chain, object)
				}
			}
		}
	}
	return ex.Throw{Code: http.StatusUnauthorized, Msg: "access defined"}
}

func (self *ReplayFilter) DoFilter(chain Filter, object *FilterObject) error {
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
	return chain.DoFilter(chain, object)
}
