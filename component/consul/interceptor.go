package consul

import (
	"context"
	"errors"
	"fmt"
	rate "github.com/godaddy-x/freego/component/limiter"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

var defaultLimiter = rate.NewRateLimiter(rate.Option{
	Limit:       2,
	Bucket:      10,
	Distributed: true,
})

func (self *ConsulManager) getRateOption(method string) (rate.Option, error) {
	if self.Option == nil || self.Option.RateOption == nil {
		return rate.Option{}, errors.New("consul manager option function is nil")
	}
	config, err := self.Option.RateOption(method)
	if err != nil {
		return rate.Option{}, err
	}
	return config, nil
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
	if b := limiter.Allow(method); !b {
		return errors.New(fmt.Sprintf("the method [%s] request is full", method))
	}
	return nil
}

//func (self *ConsulManager) checkToken(method string, ctx context.Context) error {
//	config, err := self.getMethodConfig(method)
//	if err != nil {
//		return err
//	}
//	tokenConfig := config.TokenConfig
//	if !tokenConfig.Authenticate {
//		return nil
//	}
//	if len(tokenConfig.TokenId) == 0 || len(tokenConfig.TokenKey) == 0 {
//		return errors.New("rpc token config is nil")
//	}
//	md, ok := metadata.FromIncomingContext(ctx)
//	if !ok {
//		return errors.New("rpc context key/value is nil")
//	}
//	token, b := md["token"]
//	if !b || len(token) == 0 {
//		return errors.New("rpc context token is nil")
//	}
//	return nil
//}

func (self *ConsulManager) ServerInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
	if err := self.rateLimit(info.FullMethod); err != nil {
		return nil, err
	}
	fmt.Println("接收到了一个新的请求")
	md, ok := metadata.FromIncomingContext(ctx)
	fmt.Println(md, ok)
	res, err := handler(ctx, req)
	fmt.Println("请求已完成")
	return res, err
}

func (self *ConsulManager) ClientInterceptor(ctx context.Context, method string, req, reply interface{}, conn *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	if err := self.rateLimit(method); err != nil {
		return err
	}
	err := invoker(ctx, method, req, reply, conn, opts...)
	return err
}
