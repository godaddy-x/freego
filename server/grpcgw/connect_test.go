package grpcgw

import (
	"context"
	"testing"
	"time"

	"github.com/godaddy-x/freego/core/crypto"
	"github.com/godaddy-x/freego/core/jwt"
	utils "github.com/godaddy-x/freego/core/str"
	pb "github.com/godaddy-x/freego/protocol/grpcgw"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
)

func TestConnectPush(t *testing.T) {
	const tokenKey = "123456"
	const usr int64 = 7
	const device = "phone"
	g := NewGrpcGateway()
	g.SetBroadcastKey("push-key")
	g.AddJwtConfig(jwt.JwtConfig{TokenTyp: jwt.JWT, TokenAlg: jwt.HS256, TokenKey: tokenKey, TokenExp: jwt.TWO_WEEK})
	g.AddCipherHook(func(int64) (crypto.Cipher, error) { return nil, utils.Error("unused") })
	ended := make(chan struct{}, 1)
	g.SetOnStreamEnd(func(u int64, dev string) {
		if u == usr && dev == device {
			ended <- struct{}{}
		}
	})

	api, stop := dialBuf(t, g)
	defer stop()

	sub := &jwt.Subject{}
	sub.Create("u7").Dev("APP")
	sub.Payload.Ext = "7"
	token := sub.Generate(g.jwtConfig)
	secret := sub.GetTokenSecret(token, tokenKey)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs(
		pb.MetadataTransportVersionKey, pb.TransportVersion,
		pb.MetadataDeviceKey, device,
		pb.MetadataAuthorizationKey, token,
	))
	stream, err := api.Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	bindPlain, err := proto.Marshal(&pb.StreamBindRequest{})
	if err != nil {
		t.Fatal(err)
	}
	req := &pb.GatewayRequest{R: pb.StreamBindRoute, D: bindPlain, N: pb.NewNonce(), T: utils.UnixSecond(), P: 0, U: usr}
	pb.SignRequest(req, secret[:32])
	if err := stream.Send(req); err != nil {
		t.Fatal(err)
	}
	ack, err := stream.Recv()
	if err != nil {
		t.Fatal(err)
	}
	if ack.C != 200 {
		t.Fatalf("ack c=%d m=%s", ack.C, ack.M)
	}
	plain := []byte(`{"n":1}`)
	if err := g.PushToSubject(usr, device, "notify", plain); err != nil {
		t.Fatal(err)
	}
	push, err := stream.Recv()
	if err != nil {
		t.Fatal(err)
	}
	if push.C != pb.PushCode || string(push.D) != string(plain) {
		t.Fatalf("push c=%d d=%s", push.C, push.D)
	}
	canonical := pb.CanonicalResponse(push.R, push.D, push.N, push.T, push.P, usr, push.C, push.M)
	expect := pb.HMACSHA256(utils.Str2Bytes("push-key"), canonical)
	if !pb.ConstantTimeEqual(expect, push.S) {
		t.Fatal("push mac invalid")
	}
	cancel()
	select {
	case <-ended:
	case <-time.After(2 * time.Second):
		t.Fatal("OnStreamEnd not called")
	}
}

func TestConnectRejectDoesNotRegister(t *testing.T) {
	const usr int64 = 7
	const device = "phone"
	g := NewGrpcGateway()
	g.SetBroadcastKey("push-key")
	g.AddJwtConfig(jwt.JwtConfig{TokenTyp: jwt.JWT, TokenAlg: jwt.HS256, TokenKey: "123456", TokenExp: jwt.TWO_WEEK})
	g.AddCipherHook(func(int64) (crypto.Cipher, error) { return nil, utils.Error("unused") })

	api, stop := dialBuf(t, g)
	defer stop()

	ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(
		pb.MetadataTransportVersionKey, pb.TransportVersion,
		pb.MetadataDeviceKey, device,
		pb.MetadataAuthorizationKey, "not-a-jwt",
	))
	stream, err := api.Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	req := &pb.GatewayRequest{R: pb.StreamBindRoute, D: []byte{1}, N: pb.NewNonce(), T: utils.UnixSecond(), P: 0, U: usr}
	pb.SignRequest(req, []byte("0123456789abcdef0123456789abcdef"))
	if err := stream.Send(req); err != nil {
		t.Fatal(err)
	}
	ack, err := stream.Recv()
	if err != nil {
		t.Fatal(err)
	}
	if ack.C == 200 {
		t.Fatal("invalid bind must not succeed")
	}
	if err := g.PushToSubject(usr, device, "notify", []byte("x")); err == nil {
		t.Fatal("rejected bind must not occupy the push stream")
	}
}

func TestConnectReplaceDoesNotPanic(t *testing.T) {
	const tokenKey = "123456"
	const usr int64 = 7
	const device = "phone"
	g := NewGrpcGateway()
	g.SetBroadcastKey("push-key")
	g.AddJwtConfig(jwt.JwtConfig{TokenTyp: jwt.JWT, TokenAlg: jwt.HS256, TokenKey: tokenKey, TokenExp: jwt.TWO_WEEK})
	g.AddCipherHook(func(int64) (crypto.Cipher, error) { return nil, utils.Error("unused") })

	api, stop := dialBuf(t, g)
	defer stop()

	sub := &jwt.Subject{}
	sub.Create("u7").Dev("APP")
	sub.Payload.Ext = "7"
	token := sub.Generate(g.jwtConfig)
	secret := sub.GetTokenSecret(token, tokenKey)

	open := func() pb.Gateway_ConnectClient {
		t.Helper()
		ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(
			pb.MetadataTransportVersionKey, pb.TransportVersion,
			pb.MetadataDeviceKey, device,
			pb.MetadataAuthorizationKey, token,
		))
		stream, err := api.Connect(ctx)
		if err != nil {
			t.Fatal(err)
		}
		req := &pb.GatewayRequest{R: pb.StreamBindRoute, D: []byte{1}, N: pb.NewNonce(), T: utils.UnixSecond(), P: 0, U: usr}
		pb.SignRequest(req, secret[:32])
		if err := stream.Send(req); err != nil {
			t.Fatal(err)
		}
		ack, err := stream.Recv()
		if err != nil {
			t.Fatal(err)
		}
		if ack.C != 200 {
			t.Fatalf("ack c=%d m=%s", ack.C, ack.M)
		}
		return stream
	}

	first := open()
	second := open()
	plain := []byte("next")
	if err := g.PushToSubject(usr, device, "notify", plain); err != nil {
		t.Fatal(err)
	}
	push, err := second.Recv()
	if err != nil {
		t.Fatal(err)
	}
	if string(push.D) != string(plain) {
		t.Fatalf("push d=%s", push.D)
	}
	req := &pb.GatewayRequest{R: pb.StreamBindRoute, D: []byte{1}, N: pb.NewNonce(), T: utils.UnixSecond(), P: 0, U: usr}
	pb.SignRequest(req, secret[:32])
	_ = first.Send(req)
}
