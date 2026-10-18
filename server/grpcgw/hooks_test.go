package grpcgw

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/godaddy-x/freego/core/crypto"
	"github.com/godaddy-x/freego/core/ex"
	utils "github.com/godaddy-x/freego/core/str"
	pb "github.com/godaddy-x/freego/protocol/grpcgw"
	"google.golang.org/grpc/metadata"
)

func TestUnaryInterceptorBeforeHandler(t *testing.T) {
	g := NewGrpcGateway()
	g.SetBroadcastKey("test-broadcast")
	g.AddCipherHook(func(int64) (crypto.Cipher, error) { return nil, utils.Error("unused") })

	var seen int32
	g.AddInterceptor(&InterceptorObject{
		Name:  "inject",
		Order: 0,
		Interceptor: func(ctx *Context) error {
			ctx.AddStorage("tenant", "acme")
			atomic.StoreInt32(&seen, 1)
			return nil
		},
	})
	g.AddMethod("echo", func(ctx *Context) error {
		if ctx.GetStorage("tenant") != "acme" {
			return ex.Throw{Msg: "tenant missing"}
		}
		return ctx.MarshalResponse(&pb.GuestEchoResponse{Res: "ok"})
	}, &MethodConfig{Guest: true})

	api, stop := dialBuf(t, g)
	defer stop()

	req := guestSignedRequest("echo", []byte("x"), "test-broadcast")
	ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(pb.MetadataTransportVersionKey, pb.TransportVersion))
	res, err := api.Call(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if res.C != 200 || atomic.LoadInt32(&seen) != 1 {
		t.Fatalf("c=%d seen=%d", res.C, seen)
	}
}

func TestUnaryInterceptorReject(t *testing.T) {
	g := NewGrpcGateway()
	g.SetBroadcastKey("test-broadcast")
	g.AddCipherHook(func(int64) (crypto.Cipher, error) { return nil, utils.Error("unused") })
	g.AddInterceptor(&InterceptorObject{
		Name: "deny",
		Interceptor: func(ctx *Context) error {
			return ex.Throw{Code: 403, Msg: "blocked by interceptor"}
		},
	})
	g.AddMethod("echo", func(ctx *Context) error {
		return ctx.MarshalResponse(&pb.GuestEchoResponse{Res: "ok"})
	}, &MethodConfig{Guest: true})

	api, stop := dialBuf(t, g)
	defer stop()

	req := guestSignedRequest("echo", []byte("x"), "test-broadcast")
	ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(pb.MetadataTransportVersionKey, pb.TransportVersion))
	res, err := api.Call(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if res.C != 403 || res.M != "blocked by interceptor" {
		t.Fatalf("c=%d m=%q", res.C, res.M)
	}
}

func TestErrorHandleInvoked(t *testing.T) {
	g := NewGrpcGateway()
	g.SetBroadcastKey("test-broadcast")
	g.AddCipherHook(func(int64) (crypto.Cipher, error) { return nil, utils.Error("unused") })
	var hookCalls int32
	g.AddErrorHandle(func(ctx *Context, throw ex.Throw) error {
		atomic.AddInt32(&hookCalls, 1)
		if throw.Msg != "biz" {
			t.Errorf("unexpected msg %q", throw.Msg)
		}
		return nil
	})
	g.AddMethod("fail", func(ctx *Context) error {
		return ex.Throw{Code: ex.BIZ, Msg: "biz"}
	}, &MethodConfig{Guest: true})

	api, stop := dialBuf(t, g)
	defer stop()

	req := guestSignedRequest("fail", []byte("x"), "test-broadcast")
	ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(pb.MetadataTransportVersionKey, pb.TransportVersion))
	res, err := api.Call(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if res.C != int32(ex.BIZ) || res.M != "biz" {
		t.Fatalf("c=%d m=%q", res.C, res.M)
	}
	if atomic.LoadInt32(&hookCalls) != 1 {
		t.Fatalf("error handle calls=%d", hookCalls)
	}
}
