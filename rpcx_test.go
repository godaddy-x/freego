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

// RPCX 专用 X25519（与 utils/crypto 测试向量一致；与 Ed25519 身份密钥独立）
const (
	rpcSrvXPrk = "TRQj6bHELNdsZadbqSJlGcvjEsW6vBREQv8FmKMO+qU="
	rpcCliXPrk = "LArtWyUb9zfeAtiZA/jyZkmSru/MdjA54Q7d9TzhApA="
	rpcSrvXPub = "YzwF+m0YHnGF/DxJVhTscu2s4rd1P2zhTmOmVSikrAg="
	rpcCliXPub = "4pDVI9QmdGmgv02tMtBVyPS+H3OtNayM0CPYkzuPkH4="
)

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

	cipher, _ := crypto.CreateX25519RPCWithBase64(rpcSrvXPrk, rpcCliXPub)
	// 添加RSA cipher
	err := manager.AddCipher(1, cipher)
	if err != nil {
		t.Fatalf("Failed to add cipher: %v", err)
	}

	// 注册业务处理器
	manager.AddHandler("test.hello", rpcx.Wrap(testHandle), func() proto.Message { return &pb.TestRequest{} })
	if err != nil {
		t.Fatalf("Failed to register handler: %v", err)
	}

	// 启动服务器
	serverAddr := ":9090" // 使用固定端口
	err = manager.StartServer(serverAddr)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	// 等待一秒让服务器完全启动
	time.Sleep(1000 * time.Second)

	// 停止服务器
	err = manager.StopServer()
	if err != nil {
		t.Fatalf("Failed to stop server: %v", err)
	}
}

// TestRpcSDK_Basic 基础功能测试
func TestRpcSDK_Basic(t *testing.T) {

	cipher, _ := crypto.CreateX25519RPCWithBase64(rpcCliXPrk, rpcSrvXPub)

	// 创建RPC客户端SDK
	rpcClient := sdk.NewRPC("localhost:9090").
		SetSSL(false).
		SetClientNo(1).
		AddCipher(1, cipher)
	if err := rpcClient.Connect(); err != nil {
		panic(err)
	}

	defer rpcClient.Close()

	testReq := &pb.TestRequest{
		Message: "鲨鱼宝宝嘟嘟嘟嘟！！！",
	}
	testRes := &pb.TestResponse{}

	for i := 0; i < 10; i++ {
		if err := rpcClient.Call("test.hello", testReq, testRes, true); err != nil {
			fmt.Println(err)
		}

		fmt.Println("result: ", i, testRes)
	}

	t.Log("✅ RpcSDK basic configuration test passed")
}
