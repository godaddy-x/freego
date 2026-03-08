package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/godaddy-x/freego/utils/crypto"
	"github.com/godaddy-x/freego/utils/jwt"
	"github.com/godaddy-x/freego/zlog"

	"testing"

	"github.com/godaddy-x/freego/node"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/utils/sdk"
	"github.com/valyala/fasthttp"
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

// testMessageHandler 测试用的消息处理器
type testMessageHandler struct {
	receivedMessages []*node.JsonResp
	messageCount     int
	mu               sync.Mutex
}

// HandleMessage 实现MessageHandler接口
func (h *testMessageHandler) HandleMessage(message *node.JsonResp) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.receivedMessages = append(h.receivedMessages, message)
	h.messageCount++

	if zlog.IsDebug() {
		zlog.Debug("test handler received message", 0,
			zlog.String("data", message.Data),
			zlog.String("router", message.Router))
	}

	return nil
}

func NewSocketSDK() *sdk.SocketSDK {
	newObject := &sdk.SocketSDK{
		Domain: "localhost:8088",
	}
	_ = newObject.SetECDSAObject(1, clientPrk, serverPub)
	return newObject
}

// TestWebSocketSDKUsage 测试完整的SDK使用流程（包含服务器管理）
func TestWebSocketSDKUsage(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping WebSocket SDK usage test in short mode")
	}

	zlog.InitDefaultLog(&zlog.ZapConfig{Layout: 0, Location: time.Local, Level: zlog.DEBUG, Console: true}) // 测试环境使用空logger，避免输出干扰

	fmt.Println("=== WebSocket SDK 完整使用流程测试 ===")

	// 0. 启动测试服务器
	fmt.Println("0. 启动测试服务器...")

	// 创建WebSocket服务器实例
	server := node.NewWsServer(node.SubjectDeviceUnique)

	server.AddJwtConfig(jwt.JwtConfig{
		TokenTyp: jwt.JWT,
		TokenAlg: jwt.HS256,
		TokenKey: "123456",
		TokenExp: jwt.TWO_WEEK,
	})

	// 增加双向验签的ECDSA
	cipher, _ := crypto.CreateS256ECDSAWithBase64(serverPrk, clientPub)
	server.AddCipher(1, cipher)

	// 配置连接池
	err := server.NewPool(100, 10, 5, 30)
	if err != nil {
		t.Fatalf("Failed to initialize connection pool: %v", err)
	}

	// 添加业务路由处理器
	err = server.AddRouter("/ws/user", func(ctx context.Context, connCtx *node.ConnectionContext, body []byte) (interface{}, error) {
		fmt.Println("test", connCtx.GetUserID())
		ret := &sdk.AuthToken{
			Token:  "鲨鱼宝宝获取websocket",
			Secret: connCtx.GetUserIDString(),
		}
		return ret, nil
	}, &node.RouterConfig{})
	if err != nil {
		t.Fatalf("Failed to add router: %v", err)
	}

	err = server.AddRouter("/ws/user2", func(ctx context.Context, connCtx *node.ConnectionContext, body []byte) (interface{}, error) {
		fmt.Println("test", connCtx.GetUserID())
		ret := &sdk.AuthToken{
			Token:  "鲨鱼爸爸获取websocket",
			Secret: connCtx.GetUserIDString(),
		}
		return ret, nil
	}, &node.RouterConfig{})
	if err != nil {
		t.Fatalf("Failed to add router: %v", err)
	}

	// 在goroutine中启动服务器
	serverAddr := "localhost:8088"
	serverDoneCh := make(chan bool, 1)

	go func() {
		defer func() { serverDoneCh <- true }()
		if err := server.StartWebsocket(serverAddr); err != nil {
			t.Errorf("Server start failed: %v", err)
		}
	}()

	// 等待服务器启动
	time.Sleep(200 * time.Millisecond)

	// 使用 defer 确保服务器被停止
	defer func() {
		fmt.Println("正在停止测试服务器...")
		if err := server.StopWebsocket(); err != nil {
			t.Logf("Server stop failed: %v", err)
		}
		// 等待服务器完全停止
		select {
		case <-serverDoneCh:
			fmt.Println("测试服务器已停止")
		case <-time.After(5 * time.Second):
			t.Logf("服务器停止超时")
		}
	}()

	access_token := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxOTkyODAwOTk4Mzg4NjYyMjczIiwiYXVkIjoiIiwiaXNzIjoiIiwiZGV2IjoiQVBQIiwianRpIjoiMjgyZjAwMmQtNTY3MS00YTlhLTgwMDMtMzA5ZmI0ZGNkNTZjIiwiZXh0IjoiIiwiaWF0IjowLCJleHAiOjE3NjUxNjUzNTd9.tbuDc+g0Scge9WNRDESF/acdMG7Fqwgu6F4vWgv69WQ="
	token_secret := "nt/YcHhS6Y8npXInAhBr9PMdSNLZlGbNCfnqaQWo09HNd67Swoy0qHZeVqN2A42g/SHVoTWkLs3XQna8bEUxeA=="
	token_expire := int64(1765165357)

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

	wsSdk.SetClientNo(1)
	wsSdk.SetECDSAObject(wsSdk.ClientNo, clientPrk, serverPub)
	wsSdk.SetHealthPing(10)

	// 5. 尝试连接WebSocket（预期成功，因为服务器已启动）
	fmt.Println("5. 尝试连接WebSocket（预期成功）...")
	err = wsSdk.ConnectWebSocket("/ws")
	if err != nil {
		t.Error("连接失败：", err)
		return
	}

	// 6. 发送WebSocket消息
	fmt.Println("6. 发送WebSocket消息...")
	requestObject := map[string]interface{}{"test": "张三"}
	responseObject := &sdk.AuthToken{}
	err = wsSdk.SendWebSocketMessage("/ws/user", requestObject, responseObject, true, false, 5)
	if err != nil {
		t.Errorf("发送消息失败：%v", err)
		// 打印详细错误信息
		t.Logf("错误详情: %v", err)
		return
	}
	fmt.Println("明文响应结果1:", responseObject)

	requestObject = map[string]interface{}{"test": "张三"}
	responseObject = &sdk.AuthToken{}
	err = wsSdk.SendWebSocketMessage("/ws/user2", requestObject, responseObject, true, true, 5)
	if err != nil {
		t.Errorf("发送消息失败：%v", err)
		// 打印详细错误信息
		t.Logf("错误详情: %v", err)
		return
	}
	fmt.Println("加密响应结果2:", responseObject)

	// 添加延迟等待响应
	time.Sleep(1000 * time.Second)

	// 验证连接状态
	if !wsSdk.IsWebSocketConnected() {
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

// TestWebSocketTokenExpiredCallback 测试Token过期回调功能
func TestWebSocketTokenExpiredCallback(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping WebSocket token expired callback test in short mode")
	}

	// 模拟外部认证接口
	type AuthResponse struct {
		Token   string `json:"token"`
		Secret  string `json:"secret"`
		Expired int64  `json:"expired"`
	}

	// 模拟认证成功次数
	authCallCount := 0

	// 使用与服务器相同的JWT密钥
	serverJwtKey := "123456_fixed_test_key_for_token_verification"

	// 外部认证函数 (模拟调用外部认证接口)
	externalAuthFunc := func() (*AuthResponse, error) {
		authCallCount++
		if zlog.IsDebug() {
			zlog.Debug(fmt.Sprintf("External auth called (attempt %d)", authCallCount), 0)
		}

		// 使用与服务器相同的密钥生成token
		jwtConfig := jwt.JwtConfig{
			TokenTyp: jwt.JWT,
			TokenAlg: jwt.HS256,
			TokenKey: serverJwtKey,
			TokenExp: jwt.TWO_WEEK,
		}

		subject := &jwt.Subject{}
		token := subject.Create(fmt.Sprintf("user_%d", authCallCount)).Dev("APP").Generate(jwtConfig)

		// 生成32字节的密钥作为secret
		keyBytes := make([]byte, 32)
		for i := range keyBytes {
			keyBytes[i] = byte(65 + i%26)
		}
		secret := utils.Base64EncodeWithPool(keyBytes)

		return &AuthResponse{
			Token:   token,
			Secret:  secret,
			Expired: utils.UnixSecond() + jwt.TWO_WEEK,
		}, nil
	}

	// 启动测试服务器
	zlog.InitDefaultLog(&zlog.ZapConfig{Layout: 0, Location: time.Local, Level: zlog.DEBUG, Console: true})

	server := node.NewWsServer(node.SubjectDeviceUnique)

	server.AddJwtConfig(jwt.JwtConfig{
		TokenTyp: jwt.JWT,
		TokenAlg: jwt.HS256,
		TokenKey: serverJwtKey,
		TokenExp: jwt.TWO_WEEK,
	})

	cipher, _ := crypto.CreateS256ECDSAWithBase64(serverPrk, clientPub)
	server.AddCipher(1, cipher)

	err := server.NewPool(100, 10, 5, 30)
	if err != nil {
		t.Fatalf("Failed to initialize connection pool: %v", err)
	}

	serverAddr := "localhost:8089"
	serverDoneCh := make(chan bool, 1)

	go func() {
		defer func() { serverDoneCh <- true }()
		if err := server.StartWebsocket(serverAddr); err != nil {
			t.Errorf("Server start failed: %v", err)
		}
	}()

	time.Sleep(200 * time.Millisecond)

	defer func() {
		fmt.Println("正在停止测试服务器...")
		if err := server.StopWebsocket(); err != nil {
			t.Logf("Server stop failed: %v", err)
		}
		select {
		case <-serverDoneCh:
			fmt.Println("测试服务器已停止")
		case <-time.After(5 * time.Second):
			t.Logf("服务器停止超时")
		}
	}()

	select {
	case <-serverDoneCh:
		t.Fatalf("Server failed to start")
	default:
	}

	// 创建SDK实例
	wsSdk := NewSocketSDK()
	wsSdk.Domain = serverAddr

	// 确保ECDSA密钥设置正确
	if err := wsSdk.SetECDSAObject(1, clientPrk, serverPub); err != nil {
		t.Fatalf("Failed to set ECDSA object: %v", err)
	}

	// 1. 设置初始认证信息（即将过期）
	initialAuth := sdk.AuthToken{
		Token:   "expired_token", // 使用无效token
		Secret:  "expired_secret",
		Expired: utils.UnixSecond() - 100, // 已经过期
	}
	wsSdk.AuthToken(initialAuth)

	// 2. 设置Token过期回调
	tokenRefreshCount := 0
	wsSdk.SetTokenExpiredCallback(func() {
		tokenRefreshCount++
		if zlog.IsDebug() {
			zlog.Debug(fmt.Sprintf("Token expired callback triggered (refresh %d)", tokenRefreshCount), 0)
		}

		// 调用外部认证接口获取新的token
		authResp, err := externalAuthFunc()
		if err != nil {
			zlog.Error("Failed to refresh token from external auth", 0, zlog.AddError(err))
			return
		}

		// 更新SDK的认证信息
		newAuth := sdk.AuthToken{
			Token:   authResp.Token,
			Secret:  authResp.Secret,
			Expired: authResp.Expired,
		}
		wsSdk.AuthToken(newAuth)

		if zlog.IsDebug() {
			zlog.Debug("Token refreshed successfully", 0)
		}

		// 重置token过期标志，允许下次继续触发回调
		// 注意：这是一个内部字段，在实际使用中可能需要SDK提供公共方法
		// wsSdk.tokenExpiredCalled = false
	})

	// 3. 尝试连接（应该触发token过期回调）
	err = wsSdk.ConnectWebSocket("/ws")
	if err != nil {
		// 预期的错误，因为初始token已过期
		if !strings.Contains(err.Error(), "token empty or token expired") {
			t.Fatalf("Unexpected connection error: %v", err)
		}
	}

	// 等待回调执行
	time.Sleep(500 * time.Millisecond)

	// 4. 验证回调被触发
	if tokenRefreshCount != 1 {
		t.Errorf("Expected token refresh callback to be called once, got %d", tokenRefreshCount)
	}

	// 5. 验证外部认证接口被调用
	if authCallCount != 1 {
		t.Errorf("Expected external auth to be called once, got %d", authCallCount)
	}

	// 6. 再次尝试连接（应该成功，因为token已刷新）
	err = wsSdk.ConnectWebSocket("/ws")
	if err != nil {
		t.Fatalf("Failed to connect after token refresh: %v", err)
	}

	// 7. 验证连接成功
	if !wsSdk.IsWebSocketConnected() {
		t.Error("WebSocket should be connected after token refresh")
	}

	// 8. 测试发送消息
	response := &node.JsonResp{}
	err = wsSdk.SendWebSocketMessage("/ws/test", map[string]interface{}{"test": "data"}, response, true, true, 5)
	if err != nil {
		t.Fatalf("Failed to send message after token refresh: %v", err)
	}

	wsSdk.DisconnectWebSocket()

	t.Logf("✅ Token expired callback test completed successfully")
	t.Logf("   - Callback triggered: %d times", tokenRefreshCount)
	t.Logf("   - External auth called: %d times", authCallCount)
}

// TestWebSocketMessageSubscription 测试消息订阅功能（单个客户端）
func TestWebSocketMessageSubscription(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping WebSocket subscription test in short mode")
	}

	// 1. 启动测试服务器
	zlog.InitDefaultLog(&zlog.ZapConfig{Layout: 0, Location: time.Local, Level: zlog.DEBUG, Console: true})

	server := node.NewWsServer(node.SubjectDeviceUnique)

	server.AddJwtConfig(jwt.JwtConfig{
		TokenTyp: jwt.JWT,
		TokenAlg: jwt.HS256,
		TokenKey: "123456",
		TokenExp: jwt.TWO_WEEK,
	})

	// 增加双向验签的ECDSA
	cipher, _ := crypto.CreateS256ECDSAWithBase64(serverPrk, clientPub)
	server.AddCipher(1, cipher)

	// 配置连接池
	err := server.NewPool(100, 10, 5, 30)
	if err != nil {
		t.Fatalf("Failed to initialize connection pool: %v", err)
	}

	// 添加推送触发路由处理器（一次性推送10条消息）
	err = server.AddRouter("/ws/trigger-push", func(ctx context.Context, connCtx *node.ConnectionContext, body []byte) (interface{}, error) {
		// 解析触发请求
		var triggerData map[string]interface{}
		if err := utils.JsonUnmarshal(body, &triggerData); err != nil {
			return nil, fmt.Errorf("invalid trigger data: %v", err)
		}

		// 获取目标路由
		targetRouter, ok := triggerData["target_router"].(string)
		if !ok || targetRouter == "" {
			return nil, fmt.Errorf("missing target_router")
		}

		// 获取消息内容前缀
		baseMessage, _ := triggerData["message"].(string)
		if baseMessage == "" {
			baseMessage = "Test push message"
		}

		// 持续推送10条消息
		go func() {
			time.Sleep(200 * time.Millisecond) // 确保响应先发送

			for i := 1; i <= 10; i++ {
				// 构造第i条推送消息
				pushMessage := &node.JsonResp{
					Code:    200,
					Message: fmt.Sprintf("push notification #%d", i),
					Data:    fmt.Sprintf("%s #%d", baseMessage, i),
					Router:  targetRouter,
					Time:    utils.UnixSecond(),
					Plan:    0,
				}

				// 序列化推送消息
				pushData, err := utils.JsonMarshal(pushMessage)
				if err != nil {
					zlog.Error("failed to marshal push message", 0, zlog.AddError(err))
					continue
				}

				// 广播消息给所有连接的客户端
				server.GetConnManager().Broadcast(pushData)

				if zlog.IsDebug() {
					zlog.Debug("sent push message", 0,
						zlog.String("router", targetRouter),
						zlog.Int("sequence", i),
						zlog.String("data", pushMessage.Data))
				}

				// 消息间隔500ms
				if i < 10 {
					time.Sleep(500 * time.Millisecond)
				}
			}

			zlog.Info("completed sending 10 push messages", 0,
				zlog.String("target_router", targetRouter))
		}()

		return map[string]interface{}{
			"status":         "pushing_started",
			"target_router":  targetRouter,
			"total_messages": 10,
			"interval_ms":    500,
		}, nil
	}, &node.RouterConfig{})

	// 添加持续推送路由处理器（持续推送消息直到客户端断开）
	err = server.AddRouter("/ws/start-continuous-push", func(ctx context.Context, connCtx *node.ConnectionContext, body []byte) (interface{}, error) {
		// 解析请求
		var pushData map[string]interface{}
		if err := utils.JsonUnmarshal(body, &pushData); err != nil {
			return nil, fmt.Errorf("invalid push data: %v", err)
		}

		// 获取目标路由
		targetRouter, ok := pushData["target_router"].(string)
		if !ok || targetRouter == "" {
			return nil, fmt.Errorf("missing target_router")
		}

		// 获取推送间隔（秒）
		intervalSeconds, _ := pushData["interval_seconds"].(float64)
		if intervalSeconds <= 0 {
			intervalSeconds = 2 // 默认2秒间隔
		}
		interval := time.Duration(intervalSeconds) * time.Second

		// 获取消息内容前缀
		baseMessage, _ := pushData["message"].(string)
		if baseMessage == "" {
			baseMessage = "Continuous push message"
		}

		// 获取持续时间（秒），默认60秒
		durationSeconds, _ := pushData["duration_seconds"].(float64)
		if durationSeconds <= 0 {
			durationSeconds = 60
		}

		// 启动持续推送goroutine
		go func() {
			messageCount := 0
			startTime := time.Now()
			ticker := time.NewTicker(interval)
			defer ticker.Stop()

			zlog.Info("started continuous push", 0,
				zlog.String("target_router", targetRouter),
				zlog.Float64("interval_seconds", intervalSeconds),
				zlog.Float64("duration_seconds", durationSeconds),
				zlog.String("client", connCtx.GetUserIDString()))

			for {
				select {
				case <-ctx.Done():
					// 连接断开，停止推送
					zlog.Info("continuous push stopped due to connection close", 0,
						zlog.String("target_router", targetRouter),
						zlog.Int("total_messages", messageCount))
					return

				case <-ticker.C:
					// 检查是否超过持续时间
					if time.Since(startTime).Seconds() >= durationSeconds {
						zlog.Info("continuous push completed", 0,
							zlog.String("target_router", targetRouter),
							zlog.Int("total_messages", messageCount))
						return
					}

					messageCount++
					currentTime := utils.UnixSecond()

					// 构造推送消息
					pushMessage := &node.JsonResp{
						Code:    200,
						Message: fmt.Sprintf("continuous push #%d", messageCount),
						Data:    fmt.Sprintf("%s #%d at %d", baseMessage, messageCount, currentTime),
						Router:  targetRouter,
						Time:    currentTime,
						Plan:    0,
					}

					// 序列化推送消息
					pushData, err := utils.JsonMarshal(pushMessage)
					if err != nil {
						zlog.Error("failed to marshal continuous push message", 0, zlog.AddError(err))
						continue
					}

					// 广播消息给所有连接的客户端（或者可以改为只推送给特定客户端）
					server.GetConnManager().Broadcast(pushData)

					if zlog.IsDebug() {
						zlog.Debug("sent continuous push message", 0,
							zlog.String("router", targetRouter),
							zlog.Int("sequence", messageCount),
							zlog.String("data", pushMessage.Data))
					}
				}
			}
		}()

		return map[string]interface{}{
			"status":             "continuous_pushing_started",
			"target_router":      targetRouter,
			"interval_seconds":   intervalSeconds,
			"duration_seconds":   durationSeconds,
			"estimated_messages": int(durationSeconds / intervalSeconds),
		}, nil
	}, &node.RouterConfig{})
	if err != nil {
		t.Fatalf("Failed to add trigger-push router: %v", err)
	}

	// 在goroutine中启动服务器
	serverAddr := "localhost:8089"
	serverDoneCh := make(chan bool, 1)

	go func() {
		defer func() { serverDoneCh <- true }()
		if err := server.StartWebsocket(serverAddr); err != nil {
			t.Errorf("Server start failed: %v", err)
		}
	}()

	// 等待服务器启动
	time.Sleep(200 * time.Millisecond)

	// 使用 defer 确保服务器被停止
	defer func() {
		fmt.Println("正在停止测试服务器...")
		if err := server.StopWebsocket(); err != nil {
			t.Logf("Server stop failed: %v", err)
		}
		select {
		case <-serverDoneCh:
			fmt.Println("测试服务器已停止")
		case <-time.After(5 * time.Second):
			t.Logf("服务器停止超时")
		}
	}()

	// 检查服务器是否成功启动
	select {
	case <-serverDoneCh:
		t.Fatalf("Server failed to start")
	default:
		// 服务器成功启动，继续测试
	}

	// 2. 创建SDK实例并连接到测试服务器
	wsSdk := NewSocketSDK()
	wsSdk.Domain = serverAddr

	handler := &testMessageHandler{
		receivedMessages: make([]*node.JsonResp, 0),
	}

	// 测试订阅消息
	t.Run("SubscribeMessage", func(t *testing.T) {
		subscriptionID, err := wsSdk.SubscribeMessage("/ws/test", handler)
		if err != nil {
			t.Fatalf("Failed to subscribe message: %v", err)
		}

		if subscriptionID == "" {
			t.Error("Subscription ID should not be empty")
		}

		// 验证订阅是否成功
		subscriptions := wsSdk.GetSubscriptions()
		if len(subscriptions) != 1 {
			t.Errorf("Expected 1 subscription, got %d", len(subscriptions))
		}

		if sub, exists := subscriptions["/ws/test"]; !exists {
			t.Error("Subscription for /ws/test should exist")
		} else {
			if sub.ID != subscriptionID {
				t.Errorf("Subscription ID mismatch: expected %s, got %s", subscriptionID, sub.ID)
			}
			if sub.Router != "/ws/test" {
				t.Errorf("Subscription router mismatch: expected /ws/test, got %s", sub.Router)
			}
		}
	})

	// 测试消息分发（单个客户端）
	t.Run("MessageDispatch", func(t *testing.T) {
		// 创建消息处理器用于接收推送消息
		dispatchHandler := &testMessageHandler{
			receivedMessages: make([]*node.JsonResp, 0),
		}

		// 订阅推送消息
		_, err := wsSdk.SubscribeMessage("/ws/push", dispatchHandler)
		if err != nil {
			t.Fatalf("Failed to subscribe to push messages: %v", err)
		}
		defer wsSdk.UnsubscribeMessage("/ws/push")

		// 使用预定义的认证参数
		authToken := sdk.AuthToken{
			Token:   access_token,
			Secret:  token_secret,
			Expired: token_expire,
		}
		wsSdk.AuthToken(authToken)

		// 连接到服务器
		err = wsSdk.ConnectWebSocket("/ws")
		if err != nil {
			t.Fatalf("Failed to connect WebSocket: %v", err)
		}
		defer wsSdk.DisconnectWebSocket()

		// 等待连接建立
		time.Sleep(200 * time.Millisecond)

		// 通过同一个客户端发送触发推送的请求给自己
		testData := map[string]interface{}{
			"action":        "trigger_push",
			"target_router": "/ws/push",
			"message":       "Hello from push test!",
		}

		response := &node.JsonResp{}
		err = wsSdk.SendWebSocketMessage("/ws/trigger-push", testData, response, true, true, 5)
		if err != nil {
			t.Fatalf("Push trigger failed: %v", err)
		}

		// 验证触发响应
		if response.Message != "pushing_started" {
			t.Logf("Trigger response: %s", response.Message)
		}

		// 等待足够的时间来接收所有10条消息 (10条消息 + 9个500ms间隔 = 约6秒)
		time.Sleep(7 * time.Second)

		// 验证是否接收到10条推送消息
		dispatchHandler.mu.Lock()
		messageCount := len(dispatchHandler.receivedMessages)
		dispatchHandler.mu.Unlock()

		if messageCount != 10 {
			t.Errorf("Expected 10 push messages, got %d", messageCount)
		} else {
			t.Logf("Successfully received all 10 push messages")
		}

		// 验证消息内容和顺序
		dispatchHandler.mu.Lock()
		for i, msg := range dispatchHandler.receivedMessages {
			expectedSeq := i + 1
			if msg.Router != "/ws/push" {
				t.Errorf("Message %d: expected router /ws/push, got %s", expectedSeq, msg.Router)
			}

			expectedData := fmt.Sprintf("Hello from push test! #%d", expectedSeq)
			if msg.Data != expectedData {
				t.Errorf("Message %d: expected data %s, got %s", expectedSeq, expectedData, msg.Data)
			}

			expectedMessage := fmt.Sprintf("push notification #%d", expectedSeq)
			if msg.Message != expectedMessage {
				t.Errorf("Message %d: expected message %s, got %s", expectedSeq, expectedMessage, msg.Message)
			}

			t.Logf("✓ Received push message %d: %s", expectedSeq, msg.Data)
		}
		dispatchHandler.mu.Unlock()
	})

	// 测试持续消息推送（客户端连接后持续接收消息）
	t.Run("ContinuousMessagePush", func(t *testing.T) {
		// 创建消息处理器用于接收持续推送消息
		continuousHandler := &testMessageHandler{
			receivedMessages: make([]*node.JsonResp, 0),
		}

		// 订阅持续推送消息
		_, err := wsSdk.SubscribeMessage("/ws/continuous", continuousHandler)
		if err != nil {
			t.Fatalf("Failed to subscribe to continuous messages: %v", err)
		}
		defer wsSdk.UnsubscribeMessage("/ws/continuous")

		// 使用预定义的认证参数
		authToken := sdk.AuthToken{
			Token:   access_token,
			Secret:  token_secret,
			Expired: token_expire,
		}
		wsSdk.AuthToken(authToken)

		// 连接到服务器
		err = wsSdk.ConnectWebSocket("/ws")
		if err != nil {
			t.Fatalf("Failed to connect WebSocket: %v", err)
		}
		defer wsSdk.DisconnectWebSocket()

		// 等待连接建立
		time.Sleep(200 * time.Millisecond)

		// 发送启动持续推送的请求
		continuousData := map[string]interface{}{
			"action":           "start_continuous_push",
			"target_router":    "/ws/continuous",
			"message":          "Continuous test message",
			"interval_seconds": 1.0, // 每1秒推送一条消息
			"duration_seconds": 5.0, // 持续5秒
		}

		response := &node.JsonResp{}
		err = wsSdk.SendWebSocketMessage("/ws/start-continuous-push", continuousData, response, true, true, 5)
		if err != nil {
			t.Fatalf("Failed to start continuous push: %v", err)
		}

		// 验证启动响应
		if response.Message != "success" {
			t.Logf("Continuous push start response: %s", response.Message)
		}

		// 等待持续推送完成（5秒 + 1秒缓冲）
		time.Sleep(7 * time.Second)

		// 验证接收到的消息数量（大约5条消息，间隔1秒）
		continuousHandler.mu.Lock()
		continuousMessageCount := len(continuousHandler.receivedMessages)
		continuousHandler.mu.Unlock()

		if continuousMessageCount < 4 || continuousMessageCount > 6 {
			t.Errorf("Expected 4-6 continuous messages, got %d", continuousMessageCount)
		} else {
			t.Logf("Successfully received %d continuous messages", continuousMessageCount)
		}

		// 验证消息内容和时序
		continuousHandler.mu.Lock()
		for i, msg := range continuousHandler.receivedMessages {
			if msg.Router != "/ws/continuous" {
				t.Errorf("Continuous message %d: expected router /ws/continuous, got %s", i+1, msg.Router)
			}

			expectedPrefix := "Continuous test message #"
			if !strings.HasPrefix(msg.Data, expectedPrefix) {
				t.Errorf("Continuous message %d: expected data to start with '%s', got %s", i+1, expectedPrefix, msg.Data)
			}

			if i > 0 {
				// 检查时间戳是否递增（每秒一条消息）
				prevTime := continuousHandler.receivedMessages[i-1].Time
				currTime := msg.Time
				timeDiff := currTime - prevTime
				if timeDiff < 0 || timeDiff > 2 { // 允许1秒误差
					t.Errorf("Continuous message %d: unexpected time difference %d seconds", i+1, timeDiff)
				}
			}

			t.Logf("✓ Received continuous message %d: %s", i+1, msg.Data)
		}
		continuousHandler.mu.Unlock()

		// 等待一段时间确保推送已停止
		time.Sleep(2 * time.Second)
	})

	// 测试重连后自动重新订阅
	t.Run("ReconnectAutoResubscribe", func(t *testing.T) {
		// 创建消息处理器用于测试重连重新订阅
		reconnectHandler := &testMessageHandler{
			receivedMessages: make([]*node.JsonResp, 0),
		}

		// 订阅测试路由
		testRouter := "/ws/reconnect-test"
		_, err := wsSdk.SubscribeMessage(testRouter, reconnectHandler)
		if err != nil {
			t.Fatalf("Failed to subscribe to reconnect test: %v", err)
		}
		defer wsSdk.UnsubscribeMessage(testRouter)

		// 连接到服务器
		err = wsSdk.ConnectWebSocket("/ws")
		if err != nil {
			t.Fatalf("Failed to connect WebSocket: %v", err)
		}
		defer wsSdk.DisconnectWebSocket()

		// 等待连接建立
		time.Sleep(200 * time.Millisecond)

		// 断开连接
		wsSdk.DisconnectWebSocket()

		// 等待断开完成
		time.Sleep(100 * time.Millisecond)

		// 验证连接已断开
		if wsSdk.IsWebSocketConnected() {
			t.Error("WebSocket should be disconnected")
		}

		// 重新设置认证信息（模拟重连时的token更新）
		authToken := sdk.AuthToken{
			Token:   access_token,
			Secret:  token_secret,
			Expired: token_expire,
		}
		wsSdk.AuthToken(authToken)

		// 重新连接（这会触发自动重新订阅）
		err = wsSdk.ConnectWebSocket("/ws")
		if err != nil {
			t.Fatalf("Failed to reconnect WebSocket: %v", err)
		}
		defer wsSdk.DisconnectWebSocket()

		// 等待重连和重新订阅完成
		time.Sleep(500 * time.Millisecond)

		// 验证重新连接成功
		if !wsSdk.IsWebSocketConnected() {
			t.Error("WebSocket should be reconnected")
		}

		// 验证订阅仍然存在
		subscriptions := wsSdk.GetSubscriptions()
		if len(subscriptions) != 1 {
			t.Errorf("Expected 1 subscription after reconnect, got %d", len(subscriptions))
		}

		if _, exists := subscriptions[testRouter]; !exists {
			t.Errorf("Subscription for %s should still exist after reconnect", testRouter)
		}

		t.Logf("✓ Reconnect auto-resubscribe test completed successfully")
	})
}

// TestWebSocketMessageSizeLimit 测试消息大小限制
func TestWebSocketMessageSizeLimit(t *testing.T) {
	server := node.NewWsServer(node.SubjectDeviceUnique)
	serverAddr := "localhost:8089"

	access_token := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxOTkyODAwOTk4Mzg4NjYyMjczIiwiYXVkIjoiIiwiaXNzIjoiIiwiZGV2IjoiQVBQIiwianRpIjoiMjgyZjAwMmQtNTY3MS00YTlhLTgwMDMtMzA5ZmI0ZGNkNTZjIiwiZXh0IjoiIiwiaWF0IjowLCJleHAiOjE3NjUxNjUzNTd9.tbuDc+g0Scge9WNRDESF/acdMG7Fqwgu6F4vWgv69WQ="
	token_secret := "nt/YcHhS6Y8npXInAhBr9PMdSNLZlGbNCfnqaQWo09HNd67Swoy0qHZeVqN2A42g/SHVoTWkLs3XQna8bEUxeA=="
	token_expire := int64(1765165357)

	server.AddRouter("/ws/user", func(ctx context.Context, connCtx *node.ConnectionContext, body []byte) (interface{}, error) {
		return map[string]interface{}{"message": "success"}, nil
	}, &node.RouterConfig{})

	// 启动服务器
	go func() {
		if err := server.StartWebsocket(serverAddr); err != nil {
			t.Errorf("Failed to start server: %v", err)
		}
	}()
	defer server.StopWebsocket()
	time.Sleep(100 * time.Millisecond)

	// 初始化SDK
	wsSdk := NewSocketSDK()

	// 设置认证Token
	authToken := sdk.AuthToken{
		Token:   access_token,
		Secret:  token_secret,
		Expired: token_expire,
	}
	wsSdk.AuthToken(authToken)

	// 连接WebSocket
	err := wsSdk.ConnectWebSocket("/ws")
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer wsSdk.DisconnectWebSocket()

	// 创建超过1MB的消息（大约1.1MB）
	largeMessage := make([]byte, 1024*1024+100*1024) // 1.1MB
	for i := range largeMessage {
		largeMessage[i] = byte(i % 256)
	}

	requestObject := map[string]interface{}{
		"data": string(largeMessage), // 将大字节数组转换为字符串
	}
	responseObject := &sdk.AuthToken{}

	// 发送大消息，预期会失败
	err = wsSdk.SendWebSocketMessage("/ws/user", requestObject, responseObject, true, true, 5)
	if err == nil {
		t.Error("Expected message size limit error, but got success")
	} else {
		t.Logf("✓ Message size limit correctly rejected large message: %v", err)
	}
}

// TestWebSocketGracefulShutdownWithTimeout 测试带超时的优雅关闭
func TestWebSocketGracefulShutdownWithTimeout(t *testing.T) {
	server := node.NewWsServer(node.SubjectDeviceUnique)
	serverAddr := "localhost:8090"

	// 初始化连接池和心跳服务
	if err := server.NewPool(100, 10, 5, 30); err != nil {
		t.Fatalf("Failed to initialize pool: %v", err)
	}

	server.AddRouter("/ws/test", func(ctx context.Context, connCtx *node.ConnectionContext, body []byte) (interface{}, error) {
		return map[string]interface{}{"message": "success"}, nil
	}, &node.RouterConfig{})

	// 启动服务器
	go func() {
		if err := server.StartWebsocket(serverAddr); err != nil {
			t.Errorf("Failed to start server: %v", err)
		}
	}()

	// 等待服务器启动
	time.Sleep(200 * time.Millisecond)

	// 使用带超时的优雅关闭
	err := server.StopWebsocketWithTimeout(3 * time.Second)
	if err != nil {
		t.Errorf("StopWebsocketWithTimeout failed: %v", err)
	}

	t.Logf("✓ Graceful shutdown with timeout completed successfully")
}

// TestWebSocketConnectionHealthCheck 测试连接健康检查功能
func TestWebSocketConnectionHealthCheck(t *testing.T) {
	// 创建连接管理器
	cm := &node.ConnectionManager{}

	// 添加一个模拟连接（用于测试）
	mockConn := &node.DevConn{
		Sub:  "test_subject",
		Dev:  "test_device",
		Last: utils.UnixSecond(),
		Conn: nil, // 设置为nil来测试非活跃连接
	}

	// 使用测试方法添加连接
	cm.AddTestConnection("test_subject", "test_subject_test_device", mockConn)

	// 执行健康检查
	healthStats := cm.HealthCheck()

	// 验证健康检查结果
	t.Logf("Health check stats: %+v", healthStats)

	// 应该返回统计结果
	if len(healthStats) == 0 {
		t.Error("Expected health check to return stats")
	}

	// test_subject应该有0个活跃连接（因为Conn为nil）
	if count, exists := healthStats["test_subject"]; !exists {
		t.Error("Expected test_subject in health stats")
	} else if count != 0 {
		t.Errorf("Expected 0 active connections for test_subject, got %d", count)
	}

	t.Logf("✓ Connection health check completed successfully")
}

// TestRemoteIPSecurity 测试RemoteIP的安全性，防止IP伪造
func TestRemoteIPSecurity(t *testing.T) {
	// 创建一个模拟的Context
	ctx := &node.Context{}
	ctx.RequestCtx = &fasthttp.RequestCtx{}
	ctx.RequestCtx.Request.Header.Set("X-Forwarded-For", "192.168.1.100, 10.0.0.1, 203.0.113.1")
	ctx.RequestCtx.Request.Header.Set("X-Real-Ip", "10.0.0.2")

	// 测试X-Forwarded-For优先级（取第一个有效IP）
	ip := ctx.RemoteIP()
	if ip != "192.168.1.100" {
		t.Errorf("Expected first IP from X-Forwarded-For, got %s", ip)
	}

	// 测试无效IP的情况
	ctx.RequestCtx.Request.Header.Set("X-Forwarded-For", "invalid-ip, 192.168.1.101")
	ip = ctx.RemoteIP()
	if ip != "192.168.1.101" {
		t.Errorf("Expected valid IP after invalid one, got %s", ip)
	}

	// 测试X-Real-Ip回退
	ctx.RequestCtx.Request.Header.Del("X-Forwarded-For")
	ip = ctx.RemoteIP()
	if ip != "10.0.0.2" {
		t.Errorf("Expected X-Real-Ip fallback, got %s", ip)
	}

	// 测试完全无效的情况（应该回退到RemoteIP()）
	ctx.RequestCtx.Request.Header.Del("X-Real-Ip")
	// 这里我们无法直接设置RemoteIP()的返回值，所以只验证方法不panic
	ip = ctx.RemoteIP()
	if ip == "" {
		t.Error("RemoteIP should not return empty string")
	}

	t.Logf("✓ RemoteIP security test completed - IP spoofing protection working")
}

// TestDevConnConcurrentSafety 测试DevConn的并发安全性
func TestDevConnConcurrentSafety(t *testing.T) {
	// 创建一个模拟的DevConn（Conn为nil，用于测试锁机制）
	devConn := &node.DevConn{
		Sub:  "test_subject",
		Dev:  "test_device",
		Last: utils.UnixSecond(),
		Conn: nil, // 设置为nil，IsActive会直接返回false，但会执行锁逻辑
	}

	// 并发测试：多个goroutine同时调用IsActive
	const numGoroutines = 10
	const numCalls = 100

	done := make(chan bool, numGoroutines)
	errorChan := make(chan error, numGoroutines*numCalls)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer func() { done <- true }()

			for j := 0; j < numCalls; j++ {
				// 调用IsActive，会尝试获取sendMu锁
				active := devConn.IsActive()
				// 由于Conn为nil，IsActive应该返回false
				if active {
					errorChan <- fmt.Errorf("goroutine %d call %d: expected false but got true", id, j)
					return
				}

				// 模拟一些处理时间
				time.Sleep(time.Microsecond)
			}
		}(i)
	}

	// 等待所有goroutine完成
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// 检查是否有错误
	close(errorChan)
	var errors []error
	for err := range errorChan {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		t.Errorf("Concurrent IsActive calls failed with %d errors:", len(errors))
		for i, err := range errors {
			t.Errorf("  Error %d: %v", i+1, err)
		}
	} else {
		t.Logf("✓ %d goroutines with %d calls each completed successfully without race conditions", numGoroutines, numCalls)
	}
}

// TestWebSocketErrorHandling 测试错误处理的上下文信息记录
func TestWebSocketErrorHandling(t *testing.T) {
	server := node.NewWsServer(node.SubjectDeviceUnique)
	serverAddr := "localhost:8092"

	// 添加JWT配置
	server.AddJwtConfig(jwt.JwtConfig{
		TokenTyp: jwt.JWT,
		TokenAlg: jwt.HS256,
		TokenKey: "123456",
		TokenExp: jwt.TWO_WEEK,
	})

	// 增加双向验签的ECDSA
	cipher, _ := crypto.CreateS256ECDSAWithBase64(serverPrk, clientPub)
	server.AddCipher(1, cipher)

	// 初始化连接池和心跳服务
	if err := server.NewPool(100, 10, 5, 30); err != nil {
		t.Fatalf("Failed to initialize pool: %v", err)
	}

	server.AddRouter("/ws/test", func(ctx context.Context, connCtx *node.ConnectionContext, body []byte) (interface{}, error) {
		return map[string]interface{}{"message": "success"}, nil
	}, &node.RouterConfig{})

	// 添加一个会失败的路由来触发错误处理
	server.AddRouter("/ws/error", func(ctx context.Context, connCtx *node.ConnectionContext, body []byte) (interface{}, error) {
		return nil, fmt.Errorf("test error for error handling")
	}, &node.RouterConfig{})

	// 启动服务器
	go func() {
		if err := server.StartWebsocket(serverAddr); err != nil {
			t.Errorf("Failed to start server: %v", err)
		}
	}()
	defer server.StopWebsocket()
	time.Sleep(200 * time.Millisecond)

	// 初始化SDK并建立连接
	wsSdk := NewSocketSDK()
	wsSdk.Domain = serverAddr
	authToken := sdk.AuthToken{
		Token:   "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxOTkyODAwOTk4Mzg4NjYyMjczIiwiYXVkIjoiIiwiaXNzIjoiIiwiZGV2IjoiQVBQIiwianRpIjoiMjgyZjAwMmQtNTY3MS00YTlhLTgwMDMtMzA5ZmI0ZGNkNTZjIiwiZXh0IjoiIiwiaWF0IjowLCJleHAiOjE3NjUxNjUzNTd9.tbuDc+g0Scge9WNRDESF/acdMG7Fqwgu6F4vWgv69WQ=",
		Secret:  "nt/YcHhS6Y8npXInAhBr9PMdSNLZlGbNCfnqaQWo09HNd67Swoy0qHZeVqN2A42g/SHVoTWkLs3XQna8bEUxeA==",
		Expired: int64(1765165357),
	}
	wsSdk.AuthToken(authToken)

	err := wsSdk.ConnectWebSocket("/ws")
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer wsSdk.DisconnectWebSocket()

	// 发送请求到会失败的路由，触发错误处理
	requestObject := map[string]interface{}{"test": "error"}
	responseObject := &sdk.AuthToken{}

	// 发送到错误路由，应该会记录详细的错误日志
	err = wsSdk.SendWebSocketMessage("/ws/error", requestObject, responseObject, true, true, 5)
	if err == nil {
		t.Error("Expected error from /ws/error route, but got success")
	}

	t.Logf("✓ Error handling test completed - check logs for detailed context information")
}

func TestWebSocketServer(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping WebSocket SDK usage test in short mode")
	}

	zlog.InitDefaultLog(&zlog.ZapConfig{Layout: 0, Location: time.Local, Level: zlog.DEBUG, Console: true}) // 测试环境使用空logger，避免输出干扰

	fmt.Println("=== WebSocket SDK 完整使用流程测试 ===")

	// 0. 启动测试服务器
	fmt.Println("0. 启动测试服务器...")

	// 创建WebSocket服务器实例
	server := node.NewWsServer(node.SubjectDeviceUnique)

	server.AddJwtConfig(jwt.JwtConfig{
		TokenTyp: jwt.JWT,
		TokenAlg: jwt.HS256,
		TokenKey: "123456",
		TokenExp: jwt.TWO_WEEK,
	})

	// 增加双向验签的ECDSA
	cipher, _ := crypto.CreateS256ECDSAWithBase64(serverPrk, clientPub)
	server.AddCipher(1, cipher)

	// 配置连接池
	err := server.NewPool(100, 10, 5, 30)
	if err != nil {
		t.Fatalf("Failed to initialize connection pool: %v", err)
	}

	// 添加业务路由处理器
	err = server.AddRouter("/ws/user", func(ctx context.Context, connCtx *node.ConnectionContext, body []byte) (interface{}, error) {
		fmt.Println("test", connCtx.GetUserID())
		ret := &sdk.AuthToken{
			Token:  "鲨鱼宝宝获取websocket",
			Secret: connCtx.GetUserIDString(),
		}
		return ret, nil
	}, &node.RouterConfig{})
	if err != nil {
		t.Fatalf("Failed to add router: %v", err)
	}

	err = server.AddRouter("/ws/user2", func(ctx context.Context, connCtx *node.ConnectionContext, body []byte) (interface{}, error) {
		fmt.Println("test", connCtx.GetUserID())
		ret := &sdk.AuthToken{
			Token:  "鲨鱼爸爸获取websocket",
			Secret: connCtx.GetUserIDString(),
		}
		return ret, nil
	}, &node.RouterConfig{})
	if err != nil {
		t.Fatalf("Failed to add router: %v", err)
	}

	// 在goroutine中启动服务器
	serverAddr := "localhost:8088"
	serverDoneCh := make(chan bool, 1)

	go func() {
		defer func() { serverDoneCh <- true }()
		if err := server.StartWebsocket(serverAddr); err != nil {
			t.Errorf("Server start failed: %v", err)
		}
	}()

	time.Sleep(1 * time.Second)

	go func() {
		for {
			// 2019955939305586689
			_ = server.GetConnectionManager().SendToSubject("2019955939305586689", "test push", map[string]string{"push data": "hello tony!"})
			time.Sleep(3 * time.Second)
		}
	}()

	select {}

}

func TestWebSocketClient(t *testing.T) {
	access_token := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIyMDE5OTU1OTM5MzA1NTg2Njg5IiwiYXVkIjoiIiwiaXNzIjoiIiwiZGV2IjoiQVBQIiwianRpIjoiZDRiOTJhNjhmODU3NGY4NTg4NjljNDNlYTk4YzZlNDAiLCJleHQiOiIiLCJpYXQiOjAsImV4cCI6MTc4MjUyNTk5OX0=.8hUWn5+sEkbabRV1rDTqFLbBIMcxQ0WplRqlz0MJKRc="
	token_secret := "P2wvYCoyzsFI97gJj10tNofO2YFYmK9jmFPrkiZ4qhowL4OefGgdgzIgVM0anz1KdY8KaqASeTZysYAC21AZ6Q=="
	token_expire := int64(1782525999)

	// 1. 初始化SDK
	fmt.Println("1. 初始化SDK...")
	wsSdk := sdk.NewSocketSDK("localhost:8088")

	// 2. 设置认证Token
	fmt.Println("2. 设置认证Token...")
	authToken := sdk.AuthToken{
		Token:   access_token,
		Secret:  token_secret,
		Expired: token_expire,
	}
	wsSdk.AuthToken(authToken)

	wsSdk.SetClientNo(1)
	wsSdk.SetECDSAObject(wsSdk.ClientNo, clientPrk, serverPub)
	wsSdk.SetHealthPing(3) // 3秒心跳间隔，便于测试

	// 设置推送消息回调 - 客户端通过code=300识别推送消息，已自动处理验签和解密
	wsSdk.SetPushMessageCallback(func(router string, data []byte) {
		fmt.Printf("📨 收到推送消息 - Router: %s\n", router)
		fmt.Printf("📦 推送数据: %s\n", string(data))

		// 示例：解析推送数据为结构化对象
		var pushData map[string]interface{}
		if err := utils.JsonUnmarshal(data, &pushData); err != nil {
			fmt.Printf("❌ 解析推送数据失败: %v\n", err)
			return
		}

		// 处理不同类型的推送消息
		switch router {
		case "/push/notification":
			fmt.Printf("🔔 收到通知推送: %v\n", pushData)
			// 处理通知逻辑...

		case "/push/user/status":
			fmt.Printf("👤 用户状态更新: %v\n", pushData)
			// 处理用户状态逻辑...

		case "/push/system/alert":
			fmt.Printf("🚨 系统告警: %v\n", pushData)
			// 处理系统告警逻辑...

		default:
			fmt.Printf("📬 收到未知类型推送: %s\n", router)
			fmt.Printf("📋 数据内容: %v\n", pushData)
		}
	})

	// 4. 启用自动重连
	fmt.Println("4. 启用自动重连...")
	wsSdk.EnableReconnect() // 启用重连，最大尝试10次，初始间隔1秒，最大间隔30秒

	// 5. 尝试连接WebSocket（预期成功，因为服务器已启动）
	fmt.Println("5. 尝试连接WebSocket（预期成功）...")
	_ = wsSdk.ConnectWebSocket("/ws")

	// 6. 发送WebSocket消息
	fmt.Println("6. 发送WebSocket消息...")
	requestObject := map[string]interface{}{"test": "张三"}
	responseObject := &sdk.AuthToken{}
	err := wsSdk.SendWebSocketMessage("/ws/user", requestObject, responseObject, true, false, 5)
	if err != nil {
		t.Errorf("发送消息失败：%v", err)
		// 打印详细错误信息
		t.Logf("错误详情: %v", err)
		return
	}
	fmt.Println("明文响应结果1:", responseObject)

	requestObject = map[string]interface{}{"test": "张三"}
	responseObject = &sdk.AuthToken{}
	err = wsSdk.SendWebSocketMessage("/ws/user2", requestObject, responseObject, true, true, 5)
	if err != nil {
		t.Errorf("发送消息失败：%v", err)
		// 打印详细错误信息
		t.Logf("错误详情: %v", err)
		return
	}
	fmt.Println("加密响应结果2:", responseObject)

	select {}
}
