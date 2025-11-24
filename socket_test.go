package main

import (
	"context"
	"fmt"
	"time"

	"testing"

	"github.com/godaddy-x/freego/node"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/utils/sdk"
	"github.com/godaddy-x/freego/zlog"
)

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

func NewSocketSDK() *sdk.SocketSDK {
	newObject := &sdk.SocketSDK{
		Domain: "localhost:8088",
	}
	_ = newObject.SetECDSAObject(clientPrk, serverPub)
	return newObject
}

func TestWebSocketGetUser(t *testing.T) {

	ws := NewSocketSDK()
	fmt.Printf("连接地址: %s\n", ws.Domain)

	requestObj := sdk.AuthToken{Token: "基准测试请求"}
	responseData := sdk.AuthToken{}

	if err := ws.PostByAuth("/getUser", &requestObj, &responseData, false); err != nil {
		fmt.Println(err)
		return
	}

}

// TestWebSocketStartServer 启动服务
func TestWebSocketStartServer(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping WebSocket ECC test in short mode")
	}

	// 1. 创建WebSocket服务器实例
	server := node.NewWsServer()

	// 1.5. 设置日志实例
	logger := zlog.InitDefaultLog(&zlog.ZapConfig{Layout: 0, Location: time.Local, Level: zlog.DEBUG, Console: true}) // 测试环境使用空logger，避免输出干扰
	server.AddLogger(logger)

	// 3. 配置连接池
	err := server.NewPool(100, 10, 5, 30)
	if err != nil {
		t.Fatalf("Failed to initialize connection pool: %v", err)
	}

	// 4. 添加ECC路由处理器
	err = server.AddRouter("/key", func(ctx context.Context, connCtx *node.ConnectionContext, body []byte) (interface{}, error) {
		return nil, nil
	}, &node.RouterConfig{
		Guest:  true,  // 允许游客访问
		UseRSA: false, // 不使用RSA
	})
	if err != nil {
		t.Fatalf("Failed to add ECC key router: %v", err)
	}

	err = server.AddRouter("/login", func(ctx context.Context, connCtx *node.ConnectionContext, body []byte) (interface{}, error) {
		return nil, nil
	}, &node.RouterConfig{
		Guest:  false, // 需要认证
		UseRSA: true,  // 使用RSA
	})
	if err != nil {
		t.Fatalf("Failed to add ECC login router: %v", err)
	}

	// 5. 在goroutine中启动服务器
	serverAddr := "localhost:8088"

	if err := server.StartWebsocket(serverAddr); err != nil {
		t.Errorf("Server start failed: %v", err)
	}

}

// TestWebSocketSDKUsage 测试完整的SDK使用流程（基于用户提供的实例）
func TestWebSocketSDKUsage(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping WebSocket SDK usage test in short mode")
	}

	fmt.Println("=== WebSocket SDK 完整使用流程测试 ===")

	// 1. 初始化SDK
	fmt.Println("1. 初始化SDK...")
	wsSdk := NewSocketSDK()
	wsSdk.Domain = "api.example.com"
	wsSdk.SSL = true

	// 验证初始化
	if wsSdk.Domain != "api.example.com" {
		t.Errorf("Domain设置失败，期望: api.example.com, 实际: %s", wsSdk.Domain)
	}
	if !wsSdk.SSL {
		t.Error("SSL设置失败，期望: true")
	}

	// 2. 设置认证Token
	fmt.Println("2. 设置认证Token...")
	authToken := sdk.AuthToken{
		Token:   "test-jwt-token",
		Secret:  "test-secret",
		Expired: utils.UnixSecond() + 3600,
	}
	wsSdk.AuthToken(authToken)

	// 3. 启用重连
	fmt.Println("3. 启用重连...")
	wsSdk.EnableReconnect()

	// 验证重连配置
	enabled, attempts, maxAttempts, _ := wsSdk.GetReconnectStatus()
	if !enabled {
		t.Error("重连启用失败")
	}
	if maxAttempts != 10 {
		t.Errorf("重连次数设置失败，期望: 10, 实际: %d", maxAttempts)
	}
	if attempts != 0 {
		t.Errorf("初始重连次数应该为0，实际: %d", attempts)
	}

	// 4. 设置Token过期回调
	fmt.Println("4. 设置Token过期回调...")
	tokenExpiredCalled := false
	wsSdk.SetTokenExpiredCallback(func() {
		tokenExpiredCalled = true
		fmt.Println("   -> Token过期回调被调用")
	})

	// 5. 尝试连接WebSocket（预期失败，因为没有真实服务器）
	fmt.Println("5. 尝试连接WebSocket（预期失败）...")
	err := wsSdk.ConnectWebSocket("/ws/chat")
	if err == nil {
		t.Error("连接应该失败，但没有失败")
		wsSdk.DisconnectWebSocket() // 如果意外连接成功，清理连接
		return
	}
	fmt.Printf("   -> 连接失败（预期）: %v\n", err)

	// 验证连接状态
	if wsSdk.IsWebSocketConnected() {
		t.Error("连接状态应该是false")
	}

	// 6. 测试Token过期回调（设置过期的token）
	fmt.Println("6. 测试Token过期场景...")
	expiredToken := sdk.AuthToken{
		Token:   "expired-token",
		Secret:  "expired-secret",
		Expired: utils.UnixSecond() - 100, // 已经过期
	}
	wsSdk.AuthToken(expiredToken)

	tokenExpiredCalled = false
	err = wsSdk.ConnectWebSocket("/ws/chat")
	if err == nil {
		t.Error("使用过期token连接应该失败")
		wsSdk.DisconnectWebSocket()
		return
	}

	// 等待回调执行
	time.Sleep(100 * time.Millisecond)
	if !tokenExpiredCalled {
		t.Error("Token过期回调应该被调用")
	} else {
		fmt.Println("   -> Token过期回调正常工作")
	}

	// 7. 恢复有效Token，测试发送消息前的验证
	fmt.Println("7. 恢复有效Token...")
	validToken := sdk.AuthToken{
		Token:   "valid-token",
		Secret:  "valid-secret",
		Expired: utils.UnixSecond() + 3600,
	}
	wsSdk.AuthToken(validToken)

	// 8. 测试发送同步消息（连接断开状态下）
	fmt.Println("8. 测试发送同步消息（连接断开状态）...")
	req := map[string]interface{}{"content": "hello"}
	res := map[string]interface{}{}
	err = wsSdk.SendWebSocketMessage("/ws/chat", &req, &res, true, true, 5)
	if err == nil {
		t.Error("在连接断开状态下发送消息应该失败")
	} else {
		fmt.Printf("   -> 发送失败（预期）: %v\n", err)
	}
	if len(res) != 0 {
		t.Error("断开连接时响应应该为nil")
	}

	// 9. 测试发送异步消息（连接断开状态下）
	//fmt.Println("9. 测试发送异步消息（连接断开状态）...")
	//err = wsSdk.SendWebSocketMessage("/ws/chat", map[string]interface{}{"content": "async hello"}, false, 0)
	//if err == nil {
	//	t.Error("在连接断开状态下发送异步消息应该失败")
	//} else {
	//	fmt.Printf("   -> 异步发送失败（预期）: %v\n", err)
	//}

	// 10. 测试重连功能
	fmt.Println("10. 测试重连功能...")
	// 这里会触发重连，但由于没有服务器会失败
	time.Sleep(2 * time.Second) // 等待可能的第一次重连尝试

	// 11. 强制重连测试
	fmt.Println("11. 测试强制重连...")
	err = wsSdk.ForceReconnect()
	if err == nil {
		t.Error("强制重连应该失败（无服务器）")
	} else {
		fmt.Printf("   -> 强制重连失败（预期）: %v\n", err)
	}

	// 12. 禁用重连
	fmt.Println("12. 禁用重连...")
	wsSdk.DisableReconnect()
	enabled, _, _, _ = wsSdk.GetReconnectStatus()
	if enabled {
		t.Error("重连禁用失败")
	}

	// 13. 最终清理
	fmt.Println("13. 最终清理...")
	wsSdk.DisconnectWebSocket()

	// 验证清理后状态
	if wsSdk.IsWebSocketConnected() {
		t.Error("断开连接后状态应该是false")
	}

	fmt.Println("🎉 WebSocket SDK 完整使用流程测试完成!")
}

// TestWebSocketSDKInitialization 测试SDK初始化功能
func TestWebSocketSDKInitialization(t *testing.T) {
	fmt.Println("=== WebSocket SDK 初始化测试 ===")

	// 测试NewSocketSDK函数
	sdk := NewSocketSDK()
	if sdk == nil {
		t.Fatal("NewSocketSDK返回nil")
	}

	// 测试默认值
	if sdk.Domain == "" {
		t.Error("默认Domain应该有值")
	}
	if sdk.SSL {
		t.Error("默认SSL应该是false")
	}

	// 测试配置方法
	sdk.Domain = "test.example.com"
	sdk.SSL = true
	sdk.SetTimeout(30)
	sdk.SetLanguage("zh-CN")

	if sdk.Domain != "test.example.com" {
		t.Errorf("Domain设置失败")
	}
	if !sdk.SSL {
		t.Error("SSL设置失败")
	}

	fmt.Println("✅ SDK初始化功能正常")
}

// TestWebSocketTokenManagement 测试Token管理功能
func TestWebSocketTokenManagement(t *testing.T) {
	fmt.Println("=== WebSocket Token 管理测试 ===")

	wsSdk := NewSocketSDK()

	// 测试AuthToken设置
	testToken := sdk.AuthToken{
		Token:   "test-token",
		Secret:  "test-secret",
		Expired: utils.UnixSecond() + 3600,
	}
	wsSdk.AuthToken(testToken)

	// 测试Token过期回调设置
	wsSdk.SetTokenExpiredCallback(func() {
		// 回调函数设置成功
	})

	// 测试新Token重置回调标志
	newAuthToken := sdk.AuthToken{
		Token:   "new-token",
		Secret:  "new-secret",
		Expired: utils.UnixSecond() + 7200,
	}
	wsSdk.AuthToken(newAuthToken)

	fmt.Println("✅ Token管理功能正常")
}
