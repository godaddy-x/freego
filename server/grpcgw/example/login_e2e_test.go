package grpcgw_example

import (
	"testing"

	pb "github.com/godaddy-x/freego/protocol/grpcgw"
)

// TestGRPCExampleServer：GRPCGW_E2E_SERVER=1 时启动与 main 相同的示例节点（默认 127.0.0.1:19090）。
func TestGRPCExampleServer(t *testing.T) {
	if err := StartGrpcGatewayNode(defaultE2EListen); err != nil {
		t.Fatal(err)
	}
}

func TestGRPCExampleClientLogin(t *testing.T) {
	const usr int64 = 10001
	s, err := newExampleSDK(usr)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	res := &pb.AuthToken{}
	if err := sdkPlan2LoginProtoWithRetry(s, "key", "login", demoLoginReq(), res, 0); err != nil {
		t.Fatal(err)
	}
	if res.Token == "" || len(res.Secret) < 32 || res.Expired <= 0 {
		t.Fatalf("auth token incomplete: %+v", res)
	}
	t.Logf("login ok token_len=%d exp=%d", len(res.Token), res.Expired)
}

func TestGRPCExampleClientGetUserPlan0(t *testing.T) {
	const usr int64 = 10002
	s := e2eLoginSDKSession(t, usr)
	defer s.Close()

	res := &pb.GetUserResponse{}
	if err := s.CallPlan0Proto("getUser", &pb.GetUserRequest{Name: "plan0-e2e"}, res, 0); err != nil {
		t.Fatal(err)
	}
	if res.Name != "plan0-e2e" || res.Echo != "1" {
		t.Fatalf("getUser plan0: %+v", res)
	}
	t.Logf("getUser plan0 ok name=%s", res.Name)
}

func TestGRPCExampleClientGetUserPlan1(t *testing.T) {
	const usr int64 = 10003
	s := e2eLoginSDKSession(t, usr)
	defer s.Close()

	res := &pb.GetUserResponse{}
	if err := s.CallPlan1Proto("getUser", &pb.GetUserRequest{Name: "plan1-e2e"}, res, 0); err != nil {
		t.Fatal(err)
	}
	if res.Name != "plan1-e2e" || res.Echo != "1" {
		t.Fatalf("getUser plan1: %+v", res)
	}
	t.Logf("getUser plan1 ok name=%s", res.Name)
}
