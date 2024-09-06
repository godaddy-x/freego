package rpcx

import (
	"errors"
	"fmt"
	"github.com/godaddy-x/freego/cache/limiter"
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

//func (self *GRPCManager) ServerInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
//	//if err := self.rateLimit(info.FullMethod); err != nil {
//	//	return nil, err
//	//}
//	if err := self.checkToken(ctx, info.FullMethod); err != nil {
//		return nil, status.Error(ex.BIZ, err.Error())
//	}
//	res, err := handler(ctx, req)
//	if err != nil {
//		return nil, status.Error(ex.GRPC, err.Error())
//	}
//	return res, nil
//}

//func (self *GRPCManager) ClientInterceptor(ctx context.Context, method string, req, reply interface{}, conn *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) (err error) {
//	//if err := self.rateLimit(method); err != nil {
//	//	return err
//	//}
//	ctx, err = self.createToken(ctx, method)
//	if err != nil {
//		return err
//	}
//	start := utils.UnixMilli()
//	if err := invoker(ctx, method, req, reply, conn, opts...); err != nil {
//		//rpcErr := status.Convert(err)
//		//zlog.Error("grpc call failed", start, zlog.String("service", method), zlog.AddError(rpcErr.Err()))
//		return utils.Error(status.Convert(err).Message())
//	}
//	cost := utils.UnixMilli() - start
//	if self.consul != nil && self.consul.Config.SlowQuery > 0 && cost > self.consul.Config.SlowQuery {
//		l := self.consul.GetSlowLog()
//		if l != nil {
//			l.Warn("grpc call slow query", zlog.Int64("cost", cost), zlog.Any("service", method))
//		}
//	}
//	return nil
//}
