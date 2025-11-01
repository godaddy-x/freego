package rpcx

import (
	"context"
	"errors"
	"fmt"
	"github.com/godaddy-x/freego/cache/limiter"
	"github.com/godaddy-x/freego/ex"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/zlog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	token          = "token"
	limiterKey     = "grpc:limiter:"
	timeDifference = 2400
)

var (
	unauthorizedUrl = []string{"/pub_worker.PubWorker/Authorize", "/pub_worker.PubWorker/PublicKey"}
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
	if !self.authenticate {
		return nil
	}
	//if utils.CheckStr(method, unauthorizedUrl...) {
	//	return nil
	//}
	//md, ok := metadata.FromIncomingContext(ctx)
	//if !ok {
	//	return errors.New("rpc context key/value is nil")
	//}
	//token, b := md[token]
	//if !b || len(token) == 0 {
	//	return errors.New("rpc context token is nil")
	//}
	//if len(jwtConfig.TokenKey) == 0 {
	//	return errors.New("rpc context jwt is nil")
	//}
	//subject := &jwt.Subject{}
	//if err := subject.Verify(token[0], jwtConfig.TokenKey, false); err != nil {
	//	return err
	//}
	return nil
}

func (self *GRPCManager) createToken(ctx context.Context, method string) (context.Context, error) {
	if len(accessToken) == 0 {
		return ctx, nil
	}
	if utils.CheckStr(method, unauthorizedUrl...) {
		return ctx, nil
	}
	md := metadata.New(map[string]string{token: accessToken})
	ctx = metadata.NewOutgoingContext(ctx, md)
	return ctx, nil
}

func (self *GRPCManager) ServerInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
	//if err := self.rateLimit(info.FullMethod); err != nil {
	//	return nil, err
	//}
	if err := self.checkToken(ctx, info.FullMethod); err != nil {
		return nil, status.Error(ex.BIZ, err.Error())
	}
	res, err := handler(ctx, req)
	if err != nil {
		return nil, status.Error(ex.GRPC, err.Error())
	}
	return res, nil
}

func (self *GRPCManager) ClientInterceptor(ctx context.Context, method string, req, reply interface{}, conn *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) (err error) {
	//if err := self.rateLimit(method); err != nil {
	//	return err
	//}
	ctx, err = self.createToken(ctx, method)
	if err != nil {
		return err
	}
	start := utils.UnixMilli()
	if err := invoker(ctx, method, req, reply, conn, opts...); err != nil {
		//rpcErr := status.Convert(err)
		//zlog.Error("grpc call failed", start, zlog.String("service", method), zlog.AddError(rpcErr.Err()))
		return utils.Error(status.Convert(err).Message())
	}
	cost := utils.UnixMilli() - start
	if self.consul != nil && self.consul.Config.SlowQuery > 0 && cost > self.consul.Config.SlowQuery {
		l := self.consul.GetSlowLog()
		if l != nil {
			l.Warn("grpc call slow query", zlog.Int64("cost", cost), zlog.Any("service", method))
		}
	}
	return nil
}
