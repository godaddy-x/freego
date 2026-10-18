package grpcgw

import (
	"sort"

	"github.com/godaddy-x/freego/core/ex"
	utils "github.com/godaddy-x/freego/core/str"
	"github.com/godaddy-x/freego/infra/zlog"
)

// ErrorHandle 与 httpsvr.ErrorHandle 一致：在渲染错误响应前回调（可打日志、审计；throw 为值拷贝）。
type ErrorHandle func(ctx *Context, throw ex.Throw) error

// UnaryInterceptor 在入站校验通过后、业务 handler 之前执行；返回 error 走 renderError。
type UnaryInterceptor func(ctx *Context) error

// InterceptorObject 与 httpsvr.FilterObject 类似，可按 route 匹配。
type InterceptorObject struct {
	Name          string
	Order         int
	Interceptor   UnaryInterceptor
	MatchRoutes   []string
	MatchPatterns []string
}

func (g *GrpcGateway) AddErrorHandle(h ErrorHandle) {
	g.errorHandle = h
	zlog.Info("grpcgw add error handle successful", 0)
}

func (g *GrpcGateway) AddInterceptor(obj *InterceptorObject) {
	if obj == nil || obj.Interceptor == nil {
		return
	}
	g.interceptors = append(g.interceptors, obj)
	sort.Slice(g.interceptors, func(i, j int) bool {
		return g.interceptors[i].Order < g.interceptors[j].Order
	})
	zlog.Info("grpcgw add interceptor successful", 0, zlog.String("name", obj.Name))
}

func (g *GrpcGateway) runInterceptors(ctx *Context) error {
	route := ctx.Route()
	for _, obj := range g.interceptors {
		if obj == nil || obj.Interceptor == nil {
			continue
		}
		if len(obj.MatchRoutes) > 0 {
			matched := false
			for _, r := range obj.MatchRoutes {
				if r == route {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		if len(obj.MatchPatterns) > 0 && !utils.MatchFilterURL(route, obj.MatchPatterns) {
			continue
		}
		if err := obj.Interceptor(ctx); err != nil {
			return err
		}
	}
	return nil
}
