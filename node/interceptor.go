package node

import (
	"github.com/godaddy-x/freego/utils/concurrent"
	"github.com/godaddy-x/freego/zlog"
)

const (
	PostHandleInterceptorName = "PostHandleInterceptor"
)

var interceptors []Interceptor

type interceptorSortBy struct {
	order       int
	interceptor Interceptor
}

var interceptorMap = map[string]interceptorSortBy{
	PostHandleInterceptorName: {order: 10, interceptor: &PostHandleInterceptor{}},
}

func executeInterceptorChain(ptr *NodePtr, ctx *Context) error {
	o := &interceptorChain{pos: -1, ptr: ptr, ctx: ctx}
	return o.execute()
}

func createInterceptorChain() error {
	var fs []interface{}
	for _, v := range interceptorMap {
		fs = append(fs, v)
	}
	fs = concurrent.NewSorter(fs, func(a, b interface{}) bool {
		o1 := a.(interceptorSortBy)
		o2 := b.(interceptorSortBy)
		return o1.order < o2.order
	}).Sort()
	for _, f := range fs {
		interceptors = append(interceptors, f.(interceptorSortBy).interceptor)
	}
	return nil
}

type Interceptor interface {
	PreHandle(ctx *Context) (bool, error)
	PostHandle(ctx *Context) error
	AfterCompletion(ctx *Context, err error) error
}

type interceptorChain struct {
	ptr *NodePtr
	ctx *Context
	pos int
}

func (self *interceptorChain) execute() error {
	if b, err := self.ApplyPreHandle(); !b || err != nil {
		return err
	}
	if err := self.ApplyAfterCompletion(self.ApplyPostHandle()); err != nil {
		return err
	}
	return nil
}

func (self *interceptorChain) getInterceptors() []Interceptor {
	return interceptors
}

func (self *interceptorChain) ApplyPreHandle() (bool, error) {
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

func (self *interceptorChain) ApplyPostHandle() error {
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

func (self *interceptorChain) ApplyAfterCompletion(err error) error {
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
