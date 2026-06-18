package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/godaddy-x/freego/rpcx"
	"github.com/godaddy-x/freego/rpcx/pb"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/utils/sdk"
	"google.golang.org/protobuf/proto"

	"github.com/godaddy-x/freego/utils/crypto"
)

// ML-DSA 与 test_pq_keys.go 一致：服务端 (pqServerPrk, pqClientPub)，客户端 (pqClientPrk, pqServerPub)

func rpcxTestServerCipherHook(usr int64) (crypto.Cipher, error) {
	return crypto.CreateMLDSA87WithBase64(pqServerPrk, pqClientPub)
}

func rpcxTestClientCipherHook(usr int64) (crypto.Cipher, error) {
	return crypto.CreateMLDSA87WithBase64(pqClientPrk, pqServerPub)
}

// TestHandler 测试业务处理器
func testHandle(ctx context.Context, req *pb.TestRequest) (*pb.TestResponse, error) {
	// 处理业务逻辑
	reply := &pb.TestResponse{
		Reply:      "Hello, " + req.Message,
		ServerTime: utils.UnixSecond(),
	}

	return reply, nil
}

// TestGRPCManager_StartServer 测试GRPC服务启动
func TestGRPCManager_StartServer(t *testing.T) {
	// 创建GRPC管理器
	manager := rpcx.NewRPCManager()

	if err := manager.AddCipherHook(rpcxTestServerCipherHook); err != nil {
		t.Fatalf("Failed to add cipher hook: %v", err)
	}

	// 注册业务处理器
	manager.AddHandler("test.hello", rpcx.Wrap(testHandle), func() proto.Message { return &pb.TestRequest{} })

	if err := manager.StartServer(":9090"); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	time.Sleep(20000 * time.Millisecond)
	if err := manager.StopServer(); err != nil {
		t.Fatalf("Failed to stop server: %v", err)
	}
}

// TestRpcSDK_Basic 基础功能测试
func TestRpcSDK_Basic(t *testing.T) {

	rpcClient := sdk.NewRPC("localhost:9090").
		SetSSL(false).
		SetClientNo(1).
		AddCipherHook(rpcxTestClientCipherHook)
	if err := rpcClient.Connect(); err != nil {
		panic(err)
	}

	defer rpcClient.Close()

	testReq := &pb.TestRequest{
		Message: "鲨鱼宝宝嘟嘟嘟嘟！！！",
	}
	testRes := &pb.TestResponse{}

	for i := 0; i < 10; i++ {
		if err := rpcClient.Call("test.hello", testReq, testRes, false); err != nil {
			fmt.Println(err)
		}

		fmt.Println("result: ", i, testRes)
	}

	t.Log("✅ RpcSDK basic configuration test passed")
}
