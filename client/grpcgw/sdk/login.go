// Plan2 登录路径（grpcgw）：
//
//	1. key 路由：ML-KEM 交换（CreatePublicKey，请求体为 Plan2PublicKey protobuf）
//	2. login 路由：业务 protobuf 请求 → 响应 pb.AuthToken
//	3. LoginByPlan2Auto 写入 JWT 会话，供 Plan0/1 与 Connect/StreamBind 使用
//
// JSON Plan2 登录仅存在于 client/ws；grpcgw SDK 只接受 protobuf。
package sdk

import (
	"errors"
	"strings"

	cgrpc "github.com/godaddy-x/freego/client/grpcgw"
	"github.com/godaddy-x/freego/core/crypto"
	utils "github.com/godaddy-x/freego/core/str"
	pb "github.com/godaddy-x/freego/protocol/grpcgw"
	"github.com/godaddy-x/freego/protocol/wire"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"
)

// --- 无长会话 SDK 的短连接 Plan2（压测 / 一次性） ---

func Plan2LoginWithMLDSA(target string, clientNo int64, clientPrk, serverPub, keyRoute, loginRoute string, loginReq proto.Message) (*pb.AuthToken, error) {
	in, err := proto.Marshal(loginReq)
	if err != nil {
		return nil, err
	}
	return Plan2LoginWithMLDSABody(target, clientNo, clientPrk, serverPub, keyRoute, loginRoute, in)
}

func Plan2LoginWithMLDSABody(target string, clientNo int64, clientPrk, serverPub, keyRoute, loginRoute string, loginPlain []byte) (*pb.AuthToken, error) {
	cipher, err := crypto.CreateMLDSA87WithBase64(clientPrk, serverPub)
	if err != nil {
		return nil, err
	}
	return Plan2LoginBody(target, clientNo, cipher, keyRoute, loginRoute, loginPlain)
}

func Plan2LoginBody(target string, clientNo int64, cipher crypto.Cipher, keyRoute, loginRoute string, loginPlain []byte) (*pb.AuthToken, error) {
	cli, err := cgrpc.Dial(normalizeTarget(target), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	defer cli.Close()
	cli.SetClientNo(clientNo)
	plain, err := cgrpc.Plan2Login(cli, clientNo, cipher, TrimRoute(keyRoute), TrimRoute(loginRoute), loginPlain, 0)
	if err != nil {
		return nil, err
	}
	return UnmarshalLoginAuthToken(plain)
}

// UnmarshalLoginAuthToken 解析 Plan2 login 响应（protobuf AuthToken）。
func UnmarshalLoginAuthToken(plain []byte) (*pb.AuthToken, error) {
	if len(plain) == 0 {
		return nil, errors.New("login response empty")
	}
	at := &pb.AuthToken{}
	if err := pb.UnmarshalProto(plain, at); err != nil {
		return nil, err
	}
	if at.Token == "" || len(at.Secret) < 32 {
		return nil, errors.New("login response incomplete")
	}
	if strings.TrimSpace(at.Subject) == "" {
		return nil, errors.New("login response missing subject")
	}
	return at, nil
}

// WireAuthToken pb.AuthToken → wire.AuthToken（供仍使用 client/ws 形态的调用方，如 broker 返回类型）。
func WireAuthToken(at *pb.AuthToken) wire.AuthToken {
	if at == nil {
		return wire.AuthToken{}
	}
	return wire.AuthToken{
		Token: at.Token, Secret: utils.Base64Encode(at.Secret), Expired: at.Expired, Subject: at.Subject,
	}
}

// LoginByPlan2Auto Plan2 登录：protobuf 请求/响应（resMsg 须为 *pb.AuthToken）。
func (s *SDK) LoginByPlan2Auto(keyRouter, loginRouter string, reqMsg, resMsg proto.Message, timeoutSec int64) error {
	if reqMsg == nil || resMsg == nil {
		return errors.New("login request and response required")
	}
	in, err := proto.Marshal(reqMsg)
	if err != nil {
		return err
	}
	if timeoutSec <= 0 {
		timeoutSec = s.timeoutSec
	}
	plain, err := s.plan2LoginRaw(keyRouter, loginRouter, in, timeoutSec)
	if err != nil {
		return err
	}
	parsed, err := UnmarshalLoginAuthToken(plain)
	if err != nil {
		return err
	}
	at, ok := resMsg.(*pb.AuthToken)
	if !ok {
		return errors.New("login response must be *grpcgw.AuthToken")
	}
	*at = *parsed
	s.ensureClient()
	if s.cli == nil {
		return errors.New("grpc client dial failed")
	}
	return s.cli.SetAuthTokenProto(at)
}

func (s *SDK) plan2LoginRaw(keyRoute, loginRoute string, loginPlain []byte, timeoutSec int64) ([]byte, error) {
	if s.cipher == nil {
		return nil, errors.New("ML-DSA cipher not configured")
	}
	s.ensureClient()
	if s.cli == nil {
		return nil, errors.New("grpc client dial failed")
	}
	s.cli.SetClientNo(s.clientNo)
	return cgrpc.Plan2Login(s.cli, s.clientNo, s.cipher, TrimRoute(keyRoute), TrimRoute(loginRoute), loginPlain, timeoutSec)
}
