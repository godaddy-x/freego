package node

import (
	"fmt"
	"github.com/godaddy-x/freego/utils/concurrent"
	"github.com/godaddy-x/freego/zlog"
)

const (
	PostHandleInterceptorName = "PostHandleInterceptor"
)

var interceptors []Interceptor

type InterceptorSortBy struct {
	order       int
	interceptor Interceptor
}

var interceptorMap = map[string]InterceptorSortBy{
	PostHandleInterceptorName: {order: 10, interceptor: &PostHandleInterceptor{}},
}

func createInterceptorChain() error {
	var fs []interface{}
	for _, v := range interceptorMap {
		fs = append(fs, v)
	}
	fs = concurrent.NewSorter(fs, func(a, b interface{}) bool {
		o1 := a.(InterceptorSortBy)
		o2 := b.(InterceptorSortBy)
		return o1.order < o2.order
	}).Sort()
	for _, f := range fs {
		interceptors = append(interceptors, f.(InterceptorSortBy).interceptor)
	}
	return nil
}

type Interceptor interface {
	PreHandle(ctx *Context) (bool, error)
	PostHandle(ctx *Context) error
	AfterCompletion(ctx *Context, err error) error
}

type InterceptorChain struct {
	ptr *NodePtr
	ctx *Context
	pos int
}

func (self *InterceptorChain) execute() error {
	if b, err := self.ApplyPreHandle(); !b || err != nil {
		return err
	}
	if err := self.ApplyAfterCompletion(self.ApplyPostHandle()); err != nil {
		return err
	}
	return nil
}

func (self *InterceptorChain) getInterceptors() []Interceptor {
	return interceptors
}

func (self *InterceptorChain) ApplyPreHandle() (bool, error) {
	ors := self.getInterceptors()
	for i := 0; i < len(ors); i++ {
		or := ors[i]
		if b, err := or.PreHandle(self.ctx); !b || err != nil {
			return false, self.ApplyAfterCompletion(err)
		}
		self.pos = i
	}
	return true, nil
}

func (self *InterceptorChain) ApplyPostHandle() error {
	fmt.Println(" --- ptr.PostHandle ---")
	if err := self.ptr.PostHandle(self.ctx); err != nil {
		return err
	}
	ors := self.getInterceptors()
	for i := len(ors) - 1; i >= 0; i-- {
		if err := ors[i].PostHandle(self.ctx); err != nil {
			return err
		}
	}
	return nil
}

func (self *InterceptorChain) ApplyAfterCompletion(err error) error {
	ors := self.getInterceptors()
	for i := self.pos; i >= 0; i-- {
		if err := ors[i].AfterCompletion(self.ctx, err); err != nil {
			zlog.Error("interceptor.ApplyAfterCompletion failed", 0, zlog.AddError(err))
		}
	}
	return err
}

type PostHandleInterceptor struct{}

func (self *PostHandleInterceptor) PreHandle(ctx *Context) (bool, error) {
	return true, nil
}

func (self *PostHandleInterceptor) PostHandle(ctx *Context) error {
	return nil
}

func (self *PostHandleInterceptor) AfterCompletion(ctx *Context, err error) error {
	return err
}
