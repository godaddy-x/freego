package grpcx

import (
	"context"
	"errors"
	"fmt"
	"github.com/godaddy-x/freego/component/jwt"
	rate "github.com/godaddy-x/freego/component/limiter"
	"github.com/godaddy-x/freego/component/log"
	"github.com/godaddy-x/freego/util"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const (
	token = "token"
)

var defaultLimiter = rate.NewRateLimiter(rate.Option{
	Limit:       10,
	Bucket:      100,
	Distributed: true,
})

func (self *GRPCManager) getRateOption(method string) (rate.Option, error) {
	if rateLimiterCall == nil {
		return rate.Option{}, errors.New("rateLimiterCall function is nil")
	}
	return rateLimiterCall(method)
}

func (self *GRPCManager) rateLimit(method string) error {
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

func (self *GRPCManager) checkToken(ctx context.Context, method string) error {
	if !self.Authentic {
		return nil
	}
	if util.CheckStr(method, unauthorizedUrl...) {
		return nil
	}
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return errors.New("rpc context key/value is nil")
	}
	token, b := md[token]
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

func (self *GRPCManager) createToken(ctx context.Context, method string) (context.Context, error) {
	if len(self.token) == 0 {
		return ctx, nil
	}
	if util.CheckStr(method, unauthorizedUrl...) {
		return ctx, nil
	}
	md := metadata.New(map[string]string{token: self.token})
	ctx = metadata.NewOutgoingContext(ctx, md)
	return ctx, nil
}

func (self *GRPCManager) ServerInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
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

func (self *GRPCManager) ClientInterceptor(ctx context.Context, method string, req, reply interface{}, conn *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) (err error) {
	if err := self.rateLimit(method); err != nil {
		return err
	}
	ctx, err = self.createToken(ctx, method)
	if err != nil {
		return err
	}
	start := util.Time()
	if err := invoker(ctx, method, req, reply, conn, opts...); err != nil {
		log.Error("grpc call failed", start, log.String("service", method), log.AddError(err))
		return err
	}
	cost := util.Time() - start
	if self.consul.Config.SlowQuery > 0 && cost > self.consul.Config.SlowQuery {
		l := self.consul.GetSlowLog()
		if l != nil {
			l.Warn("grpc call slow query", log.Int64("cost", cost), log.Any("service", method))
		}
	}
	return nil
}
