package impl

import (
	"context"
	"github.com/godaddy-x/freego/rpcx"
	"github.com/godaddy-x/freego/rpcx/pb"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/utils/jwt"
)

type PubWorker struct {
	pb.UnimplementedPubWorkerServer
}

func (self *PubWorker) GenerateId(ctx context.Context, req *pb.GenerateIdReq) (*pb.GenerateIdRes, error) {
	return &pb.GenerateIdRes{Value: utils.NextIID()}, nil
}

func (self *PubWorker) PublicKey(ctx context.Context, req *pb.PublicKeyReq) (*pb.PublicKeyRes, error) {
	tls, err := rpcx.GetAuthorizeTLS()
	if err != nil {
		return nil, err
	}
	_, publicKey := tls.GetPublicKey()
	return &pb.PublicKeyRes{PublicKey: publicKey}, nil
}

func (self *PubWorker) Authorize(ctx context.Context, req *pb.AuthorizeReq) (*pb.AuthorizeRes, error) {
	if len(req.Message) == 0 {
		return nil, utils.Error("message is nil")
	}
	rsaObj, err := rpcx.GetAuthorizeTLS()
	if err != nil {
		return nil, err
	}
	dec, err := rsaObj.Decrypt("", nil)
	if err != nil {
		return nil, err
	}
	authObj := &rpcx.AuthObject{}
	if err := utils.ParseJsonBase64(dec, authObj); err != nil {
		return nil, err
	}
	if len(authObj.AppId) != 32 {
		return nil, utils.Error("appid invalid")
	}
	if !utils.CheckLen(authObj.Nonce, 8, 16) {
		return nil, utils.Error("nonce invalid")
	}
	if utils.MathAbs(utils.UnixSecond()-authObj.Time) > jwt.FIVE_MINUTES { // 判断绝对时间差超过5分钟
		return nil, utils.Error("time invalid")
	}
	appConfig, err := rpcx.GetGRPCAppConfig(authObj.AppId)
	if err != nil {
		return nil, err
	}
	if len(authObj.Signature) != 44 || utils.HMAC_SHA256(utils.AddStr(authObj.AppId, authObj.Nonce, authObj.Time), appConfig.AppKey, true) != authObj.Signature {
		return nil, utils.Error("signature invalid")
	}
	jwtConfig, err := rpcx.GetGRPCJwtConfig()
	if err != nil {
		return nil, err
	}
	subject := &jwt.Subject{}
	subject.Create(authObj.AppId).Dev("GRPC").Expired(jwtConfig.TokenExp)
	token := subject.Generate(jwt.JwtConfig{TokenTyp: jwtConfig.TokenTyp, TokenAlg: jwtConfig.TokenAlg, TokenKey: jwtConfig.TokenKey})
	return &pb.AuthorizeRes{Token: token, Expired: subject.Payload.Exp}, nil
}
