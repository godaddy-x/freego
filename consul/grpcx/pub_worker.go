package grpcx

import (
	"context"
	"github.com/godaddy-x/freego/consul/grpcx/pb"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/utils/jwt"
)

type PubWorker struct {
	pb.UnimplementedPubWorkerServer
}

func (self *PubWorker) GenerateId(ctx context.Context, req *pb.GenerateIdReq) (*pb.GenerateIdRes, error) {
	return &pb.GenerateIdRes{Value: utils.GetSnowFlakeIntID(req.Node)}, nil
}

func (self *PubWorker) RPCLogin(ctx context.Context, req *pb.RPCLoginReq) (*pb.RPCLoginRes, error) {
	if len(req.Appid) != 32 {
		return nil, utils.Error("appid invalid")
	}
	if !utils.CheckLen(req.Nonce, 16, 32) {
		return nil, utils.Error("nonce invalid")
	}
	if utils.MathAbs(utils.TimeSecond()-req.Time) > jwt.FIVE_MINUTES { // 判断绝对时间差超过5分钟
		return nil, utils.Error("time invalid")
	}
	appConfig, err := GetGRPCAppConfig(req.Appid)
	if err != nil {
		return nil, err
	}
	if len(req.Signature) != 44 || utils.HMAC_SHA256(utils.AddStr(req.Appid, req.Nonce, req.Time), appConfig.Appkey, true) != req.Signature {
		return nil, utils.Error("signature invalid")
	}
	jwtConfig, err := GetGRPCJwtConfig()
	if err != nil {
		return nil, err
	}
	subject := &jwt.Subject{}
	subject.Create(req.Appid).Dev("GRPC").Expired(jwtConfig.TokenExp)
	token := subject.Generate(jwt.JwtConfig{TokenTyp: jwtConfig.TokenTyp, TokenAlg: jwtConfig.TokenAlg, TokenKey: jwtConfig.TokenKey})
	return &pb.RPCLoginRes{Token: token, Expired: subject.Payload.Exp}, nil
}
