package main

import (
	"context"
	"fmt"
	"github.com/godaddy-x/freego/utils/crypto"
	"github.com/godaddy-x/freego/utils/jwt"
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

	server.AddJwtConfig(jwt.JwtConfig{
		TokenTyp: jwt.JWT,
		TokenAlg: jwt.HS256,
		TokenKey: "123456" + utils.CreateLocalSecretKey(12, 45, 23, 60, 58, 30),
		TokenExp: jwt.TWO_WEEK,
	})

	// 增加双向验签的ECDSA
	cipher, _ := crypto.CreateS256ECDSAWithBase64(serverPrk, clientPub)
	server.AddCipher(cipher)

	// 1.5. 设置日志实例
	logger := zlog.InitDefaultLog(&zlog.ZapConfig{Layout: 0, Location: time.Local, Level: zlog.DEBUG, Console: true}) // 测试环境使用空logger，避免输出干扰
	server.AddLogger(logger)

	// 3. 配置连接池
	err := server.NewPool(100, 10, 5, 30)
	if err != nil {
		t.Fatalf("Failed to initialize connection pool: %v", err)
	}

	// 4. 添加ECC路由处理器
	err = server.AddRouter("/ws", func(ctx context.Context, connCtx *node.ConnectionContext, body []byte) (interface{}, error) {
		return body, nil
	}, &node.RouterConfig{})
	if err != nil {
		t.Fatalf("Failed to add ECC key router: %v", err)
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

	// 2. 设置认证Token
	fmt.Println("2. 设置认证Token...")
	authToken := sdk.AuthToken{
		Token:   access_token,
		Secret:  token_secret,
		Expired: token_expire,
	}
	wsSdk.AuthToken(authToken)

	// 5. 尝试连接WebSocket（预期失败，因为没有真实服务器）
	fmt.Println("5. 尝试连接WebSocket（预期成功）...")
	err := wsSdk.ConnectWebSocket("/ws")
	if err == nil {
		t.Error("连接成功")
	}

	// 验证连接状态
	if wsSdk.IsWebSocketConnected() {
		t.Error("连接状态应该是true")
	}

	// 6. 测试Token过期回调（设置过期的token）
	fmt.Println("6. 测试Token过期场景...")
	expiredToken := sdk.AuthToken{
		Token:   "expired-token",
		Secret:  "expired-secret",
		Expired: utils.UnixSecond() - 100, // 已经过期
	}
	wsSdk.AuthToken(expiredToken)

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

	// 13. 最终清理
	fmt.Println("13. 最终清理...")
	wsSdk.DisconnectWebSocket()

	// 验证清理后状态
	if wsSdk.IsWebSocketConnected() {
		t.Error("断开连接后状态应该是false")
	}

	fmt.Println("🎉 WebSocket SDK 完整使用流程测试完成!")
}
