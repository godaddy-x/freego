package rpcx

import (
	"context"
	"testing"
	"time"

	"github.com/godaddy-x/freego/rpcx/pb"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/utils/crypto"
	"google.golang.org/protobuf/proto"
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

const (
	//服务端私钥
	serverPrk = "Z4WmI28ILmpqTWM4OISPwzF10BcGF7hsPHoaiH3J1vw="
	//服务端公钥
	serverPub = "BO6XQ+PD66TMDmQXSEHl2xQarWE0HboB4LazrznThhr6Go5SvpjXJqiSe2fX+sup5OQDOLPkLdI1gh48jOmAq+k="
	//客户端私钥
	clientPrk = "rnX5ykQivfbLHtcbPR68CP636usTNC03u8OD1KeoDPg="
	//客户端公钥
	clientPub = "BEZkPpdLSQiUvkaObyDz0ya0figOLphr6L8hPEHbPzpc7sEMtq1lBTfG6IwZdd7WuJmMkP1FRt+GzZgnqt+DRjs="
)

// TestGRPCManager_StartServer 测试GRPC服务启动
func TestGRPCManager_StartServer(t *testing.T) {
	// 创建GRPC管理器
	manager := NewRPCManager()

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

	// 验证服务器已启动
	if manager.server == nil {
		t.Error("Server should be started")
	}

	// 停止服务器
	err = manager.StopServer()
	if err != nil {
		t.Fatalf("Failed to stop server: %v", err)
	}

	// 验证服务器已停止
	if manager.server != nil {
		t.Error("Server should be stopped")
	}
}
