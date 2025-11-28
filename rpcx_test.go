package main

import (
	"context"
	"fmt"
	"github.com/godaddy-x/freego/rpcx"
	"github.com/godaddy-x/freego/rpcx/pb"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/utils/sdk"
	"google.golang.org/protobuf/proto"
	"testing"
	"time"

	"github.com/godaddy-x/freego/utils/crypto"
)

// TestHandler 测试业务处理器
type TestHandler struct{}

func (h *TestHandler) Handle(ctx context.Context, req proto.Message) (proto.Message, error) {
	testReq, ok := req.(*pb.TestRequest)
	if !ok {
		return nil, utils.Error("invalid request type")
	}

	// 处理业务逻辑
	reply := &pb.TestResponse{
		Reply:      "Hello, " + testReq.Message,
		ServerTime: utils.UnixSecond(),
	}

	return reply, nil
}

func (h *TestHandler) RequestType() proto.Message {
	return &pb.TestRequest{}
}

// TestGRPCManager_StartServer 测试GRPC服务启动
func TestGRPCManager_StartServer(t *testing.T) {
	// 创建GRPC管理器
	manager := rpcx.NewRPCManager()

	// 增加双向验签的ECDSA
	cipher, _ := crypto.CreateS256ECDSAWithBase64(serverPrk, clientPub)
	// 添加RSA cipher
	err := manager.AddCipher(cipher)
	if err != nil {
		t.Fatalf("Failed to add cipher: %v", err)
	}

	// 注册业务处理器
	testHandler := &TestHandler{}
	err = manager.RegisterHandler("test.hello", testHandler)
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

	// 增加双向验签的ECDSA
	cipher, _ := crypto.CreateS256ECDSAWithBase64(clientPrk, serverPub)

	// 创建RPC客户端SDK
	rpcClient := sdk.NewRPC("localhost:9090").
		SetSSL(false).
		AddCipher(cipher)
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
