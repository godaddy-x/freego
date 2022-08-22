package grpc

import (
	"context"
	"github.com/godaddy-x/freego/component/consul"
	"github.com/godaddy-x/freego/component/consul/grpc/pb"
	"github.com/godaddy-x/freego/component/jwt"
	"github.com/godaddy-x/freego/util"
)

type PubWorker struct {
	pb.UnimplementedPubWorkerServer
}

func (self *PubWorker) GenerateId(ctx context.Context, req *pb.GenerateIdReq) (*pb.GenerateIdRes, error) {
	return &pb.GenerateIdRes{Value: util.GetSnowFlakeIntID(req.Node)}, nil
}

func (self *PubWorker) RPCLogin(ctx context.Context, req *pb.RPCLoginReq) (*pb.RPCLoginRes, error) {
	if len(req.Appid) != 32 {
		return nil, util.Error("appid invalid")
	}
	if len(req.Nonce) != 16 {
		return nil, util.Error("nonce invalid")
	}
	if util.MathAbs(util.TimeSecond()-req.Time) > jwt.FIVE_MINUTES { // 判断绝对时间差超过5分钟
		return nil, util.Error("time invalid")
	}
	appConfig, err := consul.GetGRPCAppConfig(req.Appid)
	if err != nil {
		return nil, err
	}
	if len(req.Signature) != 44 || util.HMAC_SHA256(util.AddStr(req.Appid, req.Nonce, req.Time), appConfig.Appkey, true) != req.Signature {
		return nil, util.Error("signature invalid")
	}
	jwtConfig, err := consul.GetGRPCJwtConfig()
	if err != nil {
		return nil, err
	}
	subject := &jwt.Subject{}
	token := subject.Create(req.Appid).Expired(8640000).Generate(jwtConfig)
	return &pb.RPCLoginRes{Token: token, Expired: subject.Payload.Exp}, nil
}
