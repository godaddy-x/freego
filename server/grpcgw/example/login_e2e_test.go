package grpcgw_example

import (
	"testing"

	cgrpc "github.com/godaddy-x/freego/client/grpcgw"
	pb "github.com/godaddy-x/freego/protocol/grpcgw"
)

// TestGRPCExampleServer：GRPCGW_E2E_SERVER=1 时启动与 main 相同的示例节点（默认 127.0.0.1:19090）。
func TestGRPCExampleServer(t *testing.T) {
	if err := StartGrpcGatewayNode(defaultE2EListen); err != nil {
		t.Fatal(err)
	}
}

func TestGRPCExampleClientLogin(t *testing.T) {
	cipher, err := ExampleClientCipher()
	if err != nil {
		t.Fatal(err)
	}
	cli, err := cgrpc.Dial(defaultE2EListen)
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Close()

	const usr int64 = 10001
	plain, err := plan2LoginWithRetry(cli, usr, cipher, demoLoginBody())
	if err != nil {
		t.Fatal(err)
	}
	tok := &pb.AuthToken{}
	if err := pb.UnmarshalProto(plain, tok); err != nil {
		t.Fatal(err)
	}
	if len(tok.Token) == 0 || len(tok.Secret) == 0 || tok.Expired <= 0 {
		t.Fatalf("auth token incomplete: %+v", tok)
	}
	t.Logf("login ok token_len=%d exp=%d", len(tok.Token), tok.Expired)
}

func TestGRPCExampleClientGetUserPlan0(t *testing.T) {
	const usr int64 = 10002
	cli := e2eLoginSession(t, usr)
	defer cli.Close()

	res := &pb.GetUserResponse{}
	err := cgrpc.CallPlan0Proto(cli, "getUser", &pb.GetUserRequest{Name: "plan0-e2e"}, res, 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Name != "plan0-e2e" || res.Echo != "1" {
		t.Fatalf("getUser plan0: %+v", res)
	}
	t.Logf("getUser plan0 ok name=%s", res.Name)
}

func TestGRPCExampleClientGetUserPlan1(t *testing.T) {
	const usr int64 = 10003
	cli := e2eLoginSession(t, usr)
	defer cli.Close()

	res := &pb.GetUserResponse{}
	err := cgrpc.CallPlan1Proto(cli, "getUser", &pb.GetUserRequest{Name: "plan1-e2e"}, res, 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Name != "plan1-e2e" || res.Echo != "1" {
		t.Fatalf("getUser plan1: %+v", res)
	}
	t.Logf("getUser plan1 ok name=%s", res.Name)
}
