package rpcx

import (
	"context"
	"testing"
	"time"

	"github.com/godaddy-x/freego/rpcx/impl"
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
	manager := NewGRPCManager()

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
	time.Sleep(time.Second)

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

// TestGRPCManager_StartServerWithoutCipher 测试没有RSA cipher时的启动失败
func TestGRPCManager_StartServerWithoutCipher(t *testing.T) {
	manager := NewGRPCManager()

	// 不添加RSA cipher，直接启动应该失败
	err := manager.StartServer(":0")
	if err == nil {
		t.Error("Expected error when starting without RSA cipher")
	}

	expectedMsg := "RSA cipher must be set before starting server"
	if err.Error() != expectedMsg {
		t.Errorf("Expected error message '%s', got '%s'", expectedMsg, err.Error())
	}
}

// TestGRPCManager_StartServerWithoutHandler 测试没有注册处理器时的启动失败
func TestGRPCManager_StartServerWithoutHandler(t *testing.T) {
	// 清理之前测试遗留的handlers
	impl.ClearAllHandlers()

	// 创建RSA cipher
	rsaObj := &crypto.RsaObj{}
	err := rsaObj.CreateRsa2048()
	if err != nil {
		t.Fatalf("Failed to create RSA key: %v", err)
	}

	manager := NewGRPCManager()

	// 添加RSA cipher但不注册处理器
	err = manager.AddCipher(rsaObj)
	if err != nil {
		t.Fatalf("Failed to add cipher: %v", err)
	}

	// 启动服务器应该失败
	err = manager.StartServer(":0")
	if err == nil {
		t.Error("Expected error when starting without handlers")
	}

	expectedMsg := "at least one business handler must be registered before starting server"
	if err.Error() != expectedMsg {
		t.Errorf("Expected error message '%s', got '%s'", expectedMsg, err.Error())
	}
}

// TestGRPCManager_DuplicateStart 测试重复启动
func TestGRPCManager_DuplicateStart(t *testing.T) {
	// 创建RSA cipher
	rsaObj := &crypto.RsaObj{}
	err := rsaObj.CreateRsa2048()
	if err != nil {
		t.Fatalf("Failed to create RSA key: %v", err)
	}

	manager := NewGRPCManager()

	// 添加RSA cipher
	err = manager.AddCipher(rsaObj)
	if err != nil {
		t.Fatalf("Failed to add cipher: %v", err)
	}

	// 注册业务处理器
	testHandler := &TestHandler{}
	err = manager.RegisterHandler("test.hello", testHandler)
	if err != nil {
		t.Fatalf("Failed to register handler: %v", err)
	}

	// 第一次启动
	err = manager.StartServer(":0")
	if err != nil {
		t.Fatalf("Failed to start server first time: %v", err)
	}

	// 第二次启动应该失败
	err = manager.StartServer(":0")
	if err == nil {
		t.Error("Expected error when starting server twice")
	}

	expectedMsg := "grpc server has already been started"
	if err.Error() != expectedMsg {
		t.Errorf("Expected error message '%s', got '%s'", expectedMsg, err.Error())
	}

	// 清理：停止服务器
	manager.StopServer()
}

// TestGRPCManager_AddCipher 测试添加多个cipher
func TestGRPCManager_AddCipher(t *testing.T) {
	// 清理之前的handlers
	impl.ClearAllHandlers()

	manager := NewGRPCManager()

	// 创建第一个RSA cipher
	rsaObj1 := &crypto.RsaObj{}
	err := rsaObj1.CreateRsa2048()
	if err != nil {
		t.Fatalf("Failed to create first RSA key: %v", err)
	}

	// 创建第二个RSA cipher
	rsaObj2 := &crypto.RsaObj{}
	err = rsaObj2.CreateRsa2048()
	if err != nil {
		t.Fatalf("Failed to create second RSA key: %v", err)
	}

	// 添加两个cipher
	err = manager.AddCipher(rsaObj1)
	if err != nil {
		t.Fatalf("Failed to add first cipher: %v", err)
	}

	err = manager.AddCipher(rsaObj2)
	if err != nil {
		t.Fatalf("Failed to add second cipher: %v", err)
	}

	// 验证RSA数组长度
	if len(manager.RSA) != 2 {
		t.Errorf("Expected 2 ciphers, got %d", len(manager.RSA))
	}
}

// TestGRPCManager_StopServerByTimeout 测试带超时的停止服务
func TestGRPCManager_StopServerByTimeout(t *testing.T) {
	// 创建RSA cipher
	rsaObj := &crypto.RsaObj{}
	err := rsaObj.CreateRsa2048()
	if err != nil {
		t.Fatalf("Failed to create RSA key: %v", err)
	}

	manager := NewGRPCManager()

	// 添加RSA cipher
	err = manager.AddCipher(rsaObj)
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
	err = manager.StartServer(":0")
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	// 使用超时停止服务器
	timeout := 5 * time.Second
	err = manager.StopServerByTimeout(timeout)
	if err != nil {
		t.Fatalf("Failed to stop server with timeout: %v", err)
	}

	// 验证服务器已停止
	if manager.server != nil {
		t.Error("Server should be stopped")
	}
}

// BenchmarkGRPCManager_StartStop 基准测试启动停止性能
func BenchmarkGRPCManager_StartStop(b *testing.B) {
	// 创建RSA cipher
	rsaObj := &crypto.RsaObj{}
	err := rsaObj.CreateRsa2048()
	if err != nil {
		b.Fatalf("Failed to create RSA key: %v", err)
	}

	testHandler := &TestHandler{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		manager := NewGRPCManager()

		// 添加配置
		manager.AddCipher(rsaObj)
		manager.RegisterHandler("test.hello", testHandler)

		// 启动和停止
		err := manager.StartServer(":0")
		if err != nil {
			b.Fatalf("Failed to start server: %v", err)
		}

		err = manager.StopServer()
		if err != nil {
			b.Fatalf("Failed to stop server: %v", err)
		}
	}
}
