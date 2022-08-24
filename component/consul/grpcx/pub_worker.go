package grpcx

import (
	"context"
	pb2 "github.com/godaddy-x/freego/component/consul/grpcx/pb"
	"github.com/godaddy-x/freego/component/jwt"
	"github.com/godaddy-x/freego/util"
)

type PubWorker struct {
	pb2.UnimplementedPubWorkerServer
}

func (self *PubWorker) GenerateId(ctx context.Context, req *pb2.GenerateIdReq) (*pb2.GenerateIdRes, error) {
	return &pb2.GenerateIdRes{Value: util.GetSnowFlakeIntID(req.Node)}, nil
}

func (self *PubWorker) RPCLogin(ctx context.Context, req *pb2.RPCLoginReq) (*pb2.RPCLoginRes, error) {
	if len(req.Appid) != 32 {
		return nil, util.Error("appid invalid")
	}
	if !util.CheckLen(req.Nonce, 16, 32) {
		return nil, util.Error("nonce invalid")
	}
	if util.MathAbs(util.TimeSecond()-req.Time) > jwt.FIVE_MINUTES { // 判断绝对时间差超过5分钟
		return nil, util.Error("time invalid")
	}
	appConfig, err := GetGRPCAppConfig(req.Appid)
	if err != nil {
		return nil, err
	}
	if len(req.Signature) != 44 || util.HMAC_SHA256(util.AddStr(req.Appid, req.Nonce, req.Time), appConfig.Appkey, true) != req.Signature {
		return nil, util.Error("signature invalid")
	}
	jwtConfig, err := GetGRPCJwtConfig()
	if err != nil {
		return nil, err
	}
	subject := &jwt.Subject{}
	subject.Create(req.Appid).Expired(jwtConfig.TokenExp)
	token := subject.Generate(jwt.JwtConfig{TokenTyp: jwtConfig.TokenTyp, TokenAlg: jwtConfig.TokenAlg, TokenKey: jwtConfig.TokenKey})
	return &pb2.RPCLoginRes{Token: token, Expired: subject.Payload.Exp}, nil
}
