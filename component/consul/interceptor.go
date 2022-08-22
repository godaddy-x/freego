package consul

import (
	"context"
	"errors"
	"fmt"
	"github.com/godaddy-x/freego/component/jwt"
	rate "github.com/godaddy-x/freego/component/limiter"
	"github.com/godaddy-x/freego/util"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const (
	authorization = "authorization"
)

var defaultLimiter = rate.NewRateLimiter(rate.Option{
	Limit:       2,
	Bucket:      10,
	Distributed: true,
})

func (self *ConsulManager) getRateOption(method string) (rate.Option, error) {
	if rateLimiterCall == nil {
		return rate.Option{}, errors.New("rateLimiterCall function is nil")
	}
	return rateLimiterCall(method)
}

func (self *ConsulManager) rateLimit(method string) error {
	option, err := self.getRateOption(method)
	if err != nil {
		return err
	}
	var limiter rate.RateLimiter
	if option.Limit == 0 || option.Bucket == 0 {
		limiter = defaultLimiter
	} else {
		limiter = rate.NewRateLimiter(option)
	}
	if b := limiter.Allow(limiterKey + method); !b {
		return errors.New(fmt.Sprintf("the method [%s] request is full", method))
	}
	return nil
}

func (self *ConsulManager) checkToken(ctx context.Context, method string) error {
	if util.CheckStr(method, unauthorizedUrl...) {
		return nil
	}
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return errors.New("rpc context key/value is nil")
	}
	token, b := md[authorization]
	if !b || len(token) == 0 {
		return errors.New("rpc context token is nil")
	}
	if len(jwtConfig.TokenKey) == 0 {
		return errors.New("rpc context jwt is nil")
	}
	subject := &jwt.Subject{}
	if err := subject.Verify(token[0], jwtConfig.TokenKey); err != nil {
		return err
	}
	return nil
}

func (self *ConsulManager) createToken(ctx context.Context, method string) (context.Context, error) {
	if util.CheckStr(method, unauthorizedUrl...) {
		return ctx, nil
	}
	if len(self.Token) == 0 {
		return nil, errors.New("authorization token is nil: " + method)
	}
	md := metadata.New(map[string]string{authorization: self.Token})
	ctx = metadata.NewOutgoingContext(ctx, md)
	return ctx, nil
}

func (self *ConsulManager) ServerInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
	if err := self.rateLimit(info.FullMethod); err != nil {
		return nil, err
	}
	if err := self.checkToken(ctx, info.FullMethod); err != nil {
		return nil, err
	}
	fmt.Println("接收到了一个新的请求")
	md, ok := metadata.FromIncomingContext(ctx)
	fmt.Println(md, ok)
	res, err := handler(ctx, req)
	fmt.Println("请求已完成")
	return res, err
}

func (self *ConsulManager) ClientInterceptor(ctx context.Context, method string, req, reply interface{}, conn *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) (err error) {
	if err := self.rateLimit(method); err != nil {
		return err
	}
	ctx, err = self.createToken(ctx, method)
	if err != nil {
		return err
	}
	err = invoker(ctx, method, req, reply, conn, opts...)
	return err
}
