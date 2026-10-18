package grpcgw_example

import (
	"strconv"

	"github.com/godaddy-x/freego/core/crypto"
	"github.com/godaddy-x/freego/core/ex"
	"github.com/godaddy-x/freego/core/jwt"
	utils "github.com/godaddy-x/freego/core/str"
	pb "github.com/godaddy-x/freego/protocol/grpcgw"
	grpcgw "github.com/godaddy-x/freego/server/grpcgw"
)

const (
	exampleServerLabel = "freego-grpcgw-example-server"
	exampleClientLabel = "freego-grpcgw-example-client"
)

type MyGrpcNode struct {
	*grpcgw.GrpcGateway
}

// NewExampleGrpcNode 注册与 E2E/压测一致的示例路由。
func NewExampleGrpcNode() *MyGrpcNode {
	node := newExampleGrpcGateway()
	return node
}

func newExampleGrpcGateway() *MyGrpcNode {
	g := grpcgw.NewGrpcGateway()
	g.AddJwtConfig(jwt.JwtConfig{
		TokenTyp: jwt.JWT, TokenAlg: jwt.HS256, TokenKey: "123456", TokenExp: jwt.TWO_WEEK * 10,
	})
	g.AddCipherHook(func(usr int64) (crypto.Cipher, error) {
		clientPub, err := crypto.PeerMLDSA87PublicKeyB64FromSeed(crypto.LabelSeedSHA256(exampleClientLabel))
		if err != nil {
			return nil, err
		}
		return crypto.CreateMLDSA87WithSeed(crypto.LabelSeedSHA256(exampleServerLabel), clientPub)
	})
	node := &MyGrpcNode{GrpcGateway: g}
	g.AddErrorHandle(func(ctx *grpcgw.Context, throw ex.Throw) error {
		_ = ctx.Route()
		return nil
	})
	g.AddMethod("testGuestPost", node.testGuest, &grpcgw.MethodConfig{Guest: true})
	g.AddMethod("getUser", node.getUser, &grpcgw.MethodConfig{})
	g.AddMethod("key", node.publicKey, &grpcgw.MethodConfig{Guest: true, KeyRoute: true, UsePlan2: true})
	g.AddMethod("login", node.login, &grpcgw.MethodConfig{UsePlan2: true})
	return node
}

// StartGrpcGatewayNode 阻塞运行直至进程信号退出（与 httpsvr 示例一致）。
func StartGrpcGatewayNode(addr string) error {
	return NewExampleGrpcNode().StartServer(addr)
}

func (n *MyGrpcNode) testGuest(ctx *grpcgw.Context) error {
	return ctx.MarshalResponse(&pb.GuestEchoResponse{Res: "grpcgw guest ok"})
}

func (n *MyGrpcNode) getUser(ctx *grpcgw.Context) error {
	req := &pb.GetUserRequest{}
	if err := ctx.UnmarshalRequest(req); err != nil {
		return err
	}
	return ctx.MarshalResponse(&pb.GetUserResponse{Name: req.Name, Echo: "1"})
}

func (n *MyGrpcNode) publicKey(ctx *grpcgw.Context) error {
	return ctx.CreatePublicKey()
}

func (n *MyGrpcNode) login(ctx *grpcgw.Context) error {
	req := &pb.LoginRequest{}
	if err := ctx.UnmarshalRequest(req); err != nil {
		return err
	}
	_ = req
	config := jwt.JwtConfig{TokenTyp: jwt.JWT, TokenAlg: jwt.HS256, TokenKey: "123456", TokenExp: jwt.TWO_WEEK * 10}
	sub := &jwt.Subject{}
	sub.Create(utils.NextSID()).Dev("APP")
	sub.Payload.Ext = strconv.FormatInt(ctx.User(), 10)
	token := sub.Generate(config)
	secret := sub.GetTokenSecret(token, config.TokenKey)
	return ctx.MarshalResponse(&pb.AuthToken{
		Token: token, Secret: secret, Expired: sub.Payload.Exp,
	})
}

func ExampleClientCipher() (crypto.Cipher, error) {
	serverPub, err := crypto.PeerMLDSA87PublicKeyB64FromSeed(crypto.LabelSeedSHA256(exampleServerLabel))
	if err != nil {
		return nil, err
	}
	return crypto.CreateMLDSA87WithSeed(crypto.LabelSeedSHA256(exampleClientLabel), serverPub)
}
