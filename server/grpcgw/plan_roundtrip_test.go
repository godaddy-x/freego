package grpcgw

import (
	"context"
	"net"
	"testing"

	"github.com/godaddy-x/freego/core/crypto"
	"github.com/godaddy-x/freego/core/ex"
	"github.com/godaddy-x/freego/core/jwt"
	utils "github.com/godaddy-x/freego/core/str"
	cgrpc "github.com/godaddy-x/freego/client/grpcgw"
	pb "github.com/godaddy-x/freego/protocol/grpcgw"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
)

func TestGuestPlan0RoundTrip(t *testing.T) {
	g := NewGrpcGateway()
	g.SetBroadcastKey("test-broadcast")
	g.AddCipherHook(func(int64) (crypto.Cipher, error) { return nil, utils.Error("unused") })
	g.AddMethod("testGuestPost", func(ctx *Context) error {
		return ctx.MarshalResponse(&pb.GuestEchoResponse{Res: "ok"})
	}, &MethodConfig{Guest: true})

	api, stop := dialBuf(t, g)
	defer stop()

	plain, err := proto.Marshal(&pb.GuestEchoRequest{X: 1})
	if err != nil {
		t.Fatal(err)
	}
	req := guestSignedRequest("testGuestPost", plain, "test-broadcast")
	ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(pb.MetadataTransportVersionKey, pb.TransportVersion))
	res, err := api.Call(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if res.C != 200 || len(res.S) != 32 {
		t.Fatalf("bad response c=%d s=%d", res.C, len(res.S))
	}
	expect := pb.HMACSHA256(utils.Str2Bytes("test-broadcast"), pb.CanonicalResponse(res.R, res.D, res.N, res.T, res.P, 0, res.C, res.M))
	if !pb.ConstantTimeEqual(expect, res.S) {
		t.Fatal("response mac invalid")
	}
}

func TestHandlerErrorPreservesCodeAndMsg(t *testing.T) {
	const broadcast = "test-broadcast"
	g := NewGrpcGateway()
	g.SetBroadcastKey(broadcast)
	g.AddCipherHook(func(int64) (crypto.Cipher, error) { return nil, utils.Error("unused") })

	cases := []struct {
		route string
		throw error
		wantC int32
		wantM string
	}{
		{"bizFail", ex.Throw{Msg: "test error"}, int32(ex.BIZ), "test error"},
		{"authFail", ex.Throw{Code: 401, Msg: "wrong password"}, 401, "wrong password"},
	}
	for _, tc := range cases {
		handlerErr := tc.throw
		g.AddMethod(tc.route, func(ctx *Context) error { return handlerErr }, &MethodConfig{Guest: true})
	}

	api, stop := dialBuf(t, g)
	defer stop()

	for _, tc := range cases {
		req := guestSignedRequest(tc.route, []byte("x"), broadcast)
		ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(pb.MetadataTransportVersionKey, pb.TransportVersion))
		res, err := api.Call(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		if res.C != tc.wantC || res.M != tc.wantM {
			t.Fatalf("%s: c=%d m=%q", tc.route, res.C, res.M)
		}
		expect := pb.HMACSHA256(utils.Str2Bytes(broadcast), pb.CanonicalResponse(res.R, res.D, res.N, res.T, res.P, 0, res.C, res.M))
		if !pb.ConstantTimeEqual(expect, res.S) {
			t.Fatalf("%s: response mac invalid", tc.route)
		}
	}
}

func guestSignedRequest(route string, d []byte, broadcastKey string) *pb.GatewayRequest {
	req := &pb.GatewayRequest{
		R: route, D: d, N: pb.NewNonce(), T: utils.UnixSecond(), P: 0, U: 0,
	}
	pb.SignRequest(req, utils.Str2Bytes(broadcastKey))
	return req
}

func TestPlan0GetUserRoundTrip(t *testing.T) {
	const tokenKey = "123456"
	const usr int64 = 7
	g := NewGrpcGateway()
	g.AddJwtConfig(jwt.JwtConfig{TokenTyp: jwt.JWT, TokenAlg: jwt.HS256, TokenKey: tokenKey, TokenExp: jwt.TWO_WEEK})
	g.AddCipherHook(func(int64) (crypto.Cipher, error) { return nil, utils.Error("unused") })
	g.AddMethod("getUser", func(ctx *Context) error {
		req := &pb.GetUserRequest{}
		if err := ctx.UnmarshalRequest(req); err != nil {
			return err
		}
		return ctx.MarshalResponse(&pb.GetUserResponse{Name: req.Name, Echo: "plan0"})
	}, &MethodConfig{})

	api, stop := dialBuf(t, g)
	defer stop()

	sub := &jwt.Subject{}
	sub.Create("u7").Dev("APP")
	sub.Payload.Ext = "7"
	token := sub.Generate(g.jwtConfig)
	secret := sub.GetTokenSecret(token, tokenKey)

	cli := cgrpc.WrapGatewayClient(api)
	cli.SetUser(usr)
	cli.SetAuthToken(token, secret, sub.Payload.Exp)

	reqMsg := &pb.GetUserRequest{Name: "a"}
	resMsg := &pb.GetUserResponse{}
	if err := cgrpc.CallPlan0Proto(cli, "getUser", reqMsg, resMsg, 0); err != nil {
		t.Fatal(err)
	}
	if resMsg.Name != "a" || resMsg.Echo != "plan0" {
		t.Fatalf("resp %+v", resMsg)
	}
}

func TestPlan1GetUserRoundTrip(t *testing.T) {
	const tokenKey = "123456"
	const usr int64 = 7
	g := NewGrpcGateway()
	g.AddJwtConfig(jwt.JwtConfig{TokenTyp: jwt.JWT, TokenAlg: jwt.HS256, TokenKey: tokenKey, TokenExp: jwt.TWO_WEEK})
	g.AddCipherHook(func(int64) (crypto.Cipher, error) { return nil, utils.Error("unused") })
	g.AddMethod("getUser", func(ctx *Context) error {
		req := &pb.GetUserRequest{}
		if err := ctx.UnmarshalRequest(req); err != nil {
			return err
		}
		return ctx.MarshalResponse(&pb.GetUserResponse{Name: req.Name, Echo: "1"})
	}, &MethodConfig{})

	api, stop := dialBuf(t, g)
	defer stop()

	sub := &jwt.Subject{}
	sub.Create("u7").Dev("APP")
	sub.Payload.Ext = "7"
	token := sub.Generate(g.jwtConfig)
	secret := sub.GetTokenSecret(token, tokenKey)

	cli := cgrpc.WrapGatewayClient(api)
	cli.SetUser(usr)
	cli.SetAuthToken(token, secret, sub.Payload.Exp)

	reqMsg := &pb.GetUserRequest{Name: "a"}
	resMsg := &pb.GetUserResponse{}
	if err := cgrpc.CallPlan1Proto(cli, "getUser", reqMsg, resMsg, 0); err != nil {
		t.Fatal(err)
	}
	if resMsg.Name != "a" || resMsg.Echo != "1" {
		t.Fatalf("resp %+v", resMsg)
	}
}

func TestPlan2KeyThenLogin(t *testing.T) {
	const usr int64 = 9
	serverSeed := crypto.LabelSeedSHA256("grpcgw-test-server")
	clientSeed := crypto.LabelSeedSHA256("grpcgw-test-client")
	clientPub, err := crypto.PeerMLDSA87PublicKeyB64FromSeed(clientSeed)
	if err != nil {
		t.Fatal(err)
	}
	serverPub, err := crypto.PeerMLDSA87PublicKeyB64FromSeed(serverSeed)
	if err != nil {
		t.Fatal(err)
	}
	clientCipher, err := crypto.CreateMLDSA87WithSeed(clientSeed, serverPub)
	if err != nil {
		t.Fatal(err)
	}

	g := NewGrpcGateway()
	g.AddJwtConfig(jwt.JwtConfig{TokenTyp: jwt.JWT, TokenAlg: jwt.HS256, TokenKey: "123456", TokenExp: jwt.TWO_WEEK})
	g.AddCipherHook(func(int64) (crypto.Cipher, error) {
		return crypto.CreateMLDSA87WithSeed(serverSeed, clientPub)
	})
	g.AddMethod("key", func(ctx *Context) error {
		return ctx.CreatePublicKey()
	}, &MethodConfig{KeyRoute: true, UsePlan2: true})
	g.AddMethod("login", func(ctx *Context) error {
		return ctx.MarshalResponse(&pb.LoginOkResponse{Ok: "1"})
	}, &MethodConfig{UsePlan2: true})

	cli, stop := dialClientBuf(t, g)
	defer stop()

	loginPlain, err := proto.Marshal(&pb.LoginRequest{Password: "x"})
	if err != nil {
		t.Fatal(err)
	}
	out, err := cgrpc.Plan2Login(cli, usr, clientCipher, "key", "login", loginPlain, 0)
	if err != nil {
		t.Fatal(err)
	}
	resp := &pb.LoginOkResponse{}
	if err := pb.UnmarshalProto(out, resp); err != nil {
		t.Fatal(err)
	}
	if resp.Ok != "1" {
		t.Fatalf("login %+v", resp)
	}
}

func serveBuf(t *testing.T, g *GrpcGateway) (*bufconn.Listener, func()) {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)
	s := grpc.NewServer()
	g.RegisterOn(s)
	go func() { _ = s.Serve(lis) }()
	return lis, func() { s.Stop() }
}

func bufDialOpts(lis *bufconn.Listener) []grpc.DialOption {
	return []grpc.DialOption{
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}
}

func dialBuf(t *testing.T, g *GrpcGateway) (pb.GatewayClient, func()) {
	t.Helper()
	lis, stopSrv := serveBuf(t, g)
	conn, err := grpc.NewClient("passthrough:///bufnet", bufDialOpts(lis)...)
	if err != nil {
		t.Fatal(err)
	}
	return pb.NewGatewayClient(conn), func() { stopSrv(); conn.Close() }
}

func dialClientBuf(t *testing.T, g *GrpcGateway) (*cgrpc.Client, func()) {
	t.Helper()
	lis, stopSrv := serveBuf(t, g)
	cli, err := cgrpc.Dial("passthrough:///bufnet", bufDialOpts(lis)...)
	if err != nil {
		t.Fatal(err)
	}
	return cli, func() { stopSrv(); cli.Close() }
}
