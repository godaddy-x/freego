package node

import (
	"context"
	"testing"
	"time"

	"github.com/godaddy-x/freego/ex"
	"github.com/godaddy-x/freego/utils"
	"go.uber.org/zap"
)

// TestWebSocketServerExample WebSocket服务器完整启动示例测试
func TestWebSocketServerExample(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping WebSocket server example test in short mode")
	}

	// 1. 创建WebSocket服务器实例
	server := NewWsServer(true) // debug模式

	// 2. 配置连接池参数
	// maxConn: 最大连接数 (1000)
	// limit: 每秒连接限制 (100)
	// bucket: 令牌桶大小 (10)
	// ping: 心跳间隔(秒) (30)
	err := server.NewPool(1000, 100, 10, 30)
	if err != nil {
		t.Fatalf("Failed to initialize connection pool: %v", err)
	}

	// 3. 添加路由处理器
	err = server.AddRouter("/ws/chat", handleChatMessageExample)
	if err != nil {
		t.Fatalf("Failed to add chat router: %v", err)
	}

	err = server.AddRouter("/ws/notify", handleNotificationExample)
	if err != nil {
		t.Fatalf("Failed to add notification router: %v", err)
	}

	// 4. 启动服务器 (异步启动)
	serverAddr := "localhost:8081" // 使用不同的端口避免冲突
	server.logger.Info("Starting WebSocket server", zap.String("addr", serverAddr))

	go func() {
		if err := server.StartWebsocket(serverAddr); err != nil {
			server.logger.Error("Failed to start server", zap.Error(err))
			t.Errorf("Server start failed: %v", err)
		}
	}()

	// 等待服务器启动
	time.Sleep(100 * time.Millisecond)

	// 5. 验证服务器状态
	if server.connManager.Count() != 0 {
		t.Errorf("Expected 0 connections, got %d", server.connManager.Count())
	}

	server.logger.Info("WebSocket server example test completed successfully")
}

// TestWebSocketServerConfiguration 测试服务器配置
func TestWebSocketServerConfiguration(t *testing.T) {
	server := NewWsServer(false) // 非debug模式

	// 测试连接池配置
	err := server.NewPool(100, 10, 5, 60)
	if err != nil {
		t.Fatalf("Failed to configure connection pool: %v", err)
	}

	// 测试路由添加
	err = server.AddRouter("/test", func(ctx context.Context, connCtx *ConnectionContext, body []byte) (interface{}, error) {
		return map[string]string{"status": "ok"}, nil
	})
	if err != nil {
		t.Fatalf("Failed to add test router: %v", err)
	}

	// 验证配置
	if server.maxConn != 100 {
		t.Errorf("Expected maxConn=100, got %d", server.maxConn)
	}
	if server.ping != 60 {
		t.Errorf("Expected ping=60, got %d", server.ping)
	}
}

// TestConnectionManagerOperations 测试连接管理器操作
func TestConnectionManagerOperations(t *testing.T) {
	manager := NewConnectionManager(10)

	// 测试初始状态
	if count := manager.Count(); count != 0 {
		t.Errorf("Expected count=0, got %d", count)
	}

	// 创建测试连接
	devConn := &DevConn{
		Sub:  "user123",
		Dev:  "device001",
		Life: utils.UnixSecond() + 3600,
		Last: utils.UnixSecond(),
	}

	// 测试添加连接
	err := manager.Add(devConn)
	if err != nil {
		t.Fatalf("Failed to add connection: %v", err)
	}

	if count := manager.Count(); count != 1 {
		t.Errorf("Expected count=1 after add, got %d", count)
	}

	// 测试获取连接 (deviceKey格式: dev_sub)
	retrieved := manager.Get("user123", "device001_user123")
	if retrieved != devConn {
		t.Error("Failed to retrieve added connection")
	}

	// 测试清理过期连接
	devConn.Life = utils.UnixSecond() - 100 // 设置为过期
	cleaned := manager.CleanupExpired(50)
	if cleaned != 1 {
		t.Errorf("Expected cleaned=1, got %d", cleaned)
	}

	if count := manager.Count(); count != 0 {
		t.Errorf("Expected count=0 after cleanup, got %d", count)
	}
}

// TestMessageProcessing 测试消息处理逻辑
func TestMessageProcessing(t *testing.T) {
	// 创建消息处理器
	messageHandler := NewMessageHandler(func(ctx context.Context, connCtx *ConnectionContext, body []byte) (interface{}, error) {
		return map[string]string{"result": "processed"}, nil
	}, true)

	// 创建模拟连接上下文
	connCtx := &ConnectionContext{
		Logger: zap.NewNop(), // 不输出日志的logger
	}

	// 测试消息处理
	testMessage := `{"data": "{\"test\": \"data\"}"}`
	result, err := messageHandler.Process(connCtx, []byte(testMessage))
	if err != nil {
		t.Fatalf("Message processing failed: %v", err)
	}

	// 验证结果
	if result == nil {
		t.Error("Expected non-nil result")
	}
}

// ==================== 业务处理器示例函数 ====================

// handleChatMessageExample WebSocket聊天消息处理器示例
func handleChatMessageExample(ctx context.Context, connCtx *ConnectionContext, body []byte) (interface{}, error) {
	connCtx.Logger.Info("received chat message", zap.ByteString("raw_data", body))

	// 解析消息
	var msg struct {
		Type    string `json:"type"`
		Content string `json:"content"`
		Target  string `json:"target,omitempty"` // 目标用户ID，可选
	}

	if err := utils.JsonUnmarshal(body, &msg); err != nil {
		connCtx.Logger.Error("failed to parse chat message", zap.Error(err))
		return nil, ex.Throw{Code: ex.BIZ, Msg: "invalid message format"}
	}

	// 处理不同类型的消息
	switch msg.Type {
	case "broadcast":
		// 广播消息给所有连接
		connCtx.Server.connManager.Broadcast([]byte(msg.Content))
		connCtx.Logger.Info("broadcast message sent", zap.String("content", msg.Content))

	case "private":
		// 发送私信给指定用户
		if msg.Target != "" {
			connCtx.Server.connManager.SendToSubject(msg.Target, []byte(msg.Content))
			connCtx.Logger.Info("private message sent",
				zap.String("target", msg.Target),
				zap.String("content", msg.Content))
		} else {
			return nil, ex.Throw{Code: ex.BIZ, Msg: "target user required for private message"}
		}

	case "room":
		// 房间消息 (可扩展实现)
		connCtx.Logger.Info("room message received", zap.String("content", msg.Content))

	default:
		connCtx.Logger.Warn("unsupported message type", zap.String("type", msg.Type))
		return nil, ex.Throw{Code: ex.BIZ, Msg: "unsupported message type: " + msg.Type}
	}

	// 返回确认消息
	return map[string]interface{}{
		"code":      200,
		"message":   "message sent successfully",
		"type":      msg.Type,
		"timestamp": utils.UnixMilli(),
	}, nil
}

// handleNotificationExample WebSocket通知处理器示例
func handleNotificationExample(ctx context.Context, connCtx *ConnectionContext, body []byte) (interface{}, error) {
	connCtx.Logger.Info("received notification request", zap.ByteString("raw_data", body))

	// 解析通知请求
	var req struct {
		UserID   string `json:"user_id"`
		Title    string `json:"title"`
		Content  string `json:"content"`
		Priority string `json:"priority,omitempty"` // high, normal, low
	}

	if err := utils.JsonUnmarshal(body, &req); err != nil {
		connCtx.Logger.Error("failed to parse notification", zap.Error(err))
		return nil, ex.Throw{Code: ex.BIZ, Msg: "invalid notification format"}
	}

	// 验证必填字段
	if req.UserID == "" || req.Title == "" || req.Content == "" {
		return nil, ex.Throw{Code: ex.BIZ, Msg: "user_id, title, and content are required"}
	}

	// 设置默认优先级
	if req.Priority == "" {
		req.Priority = "normal"
	}

	// 验证权限 (示例 - 在实际应用中需要更完善的权限检查)
	currentUser := "anonymous"
	if connCtx.Subject != nil {
		currentUser = connCtx.Subject.GetSub(nil)
	}

	connCtx.Logger.Info("sending notification",
		zap.String("sender", currentUser),
		zap.String("target", req.UserID),
		zap.String("title", req.Title),
		zap.String("priority", req.Priority))

	// 创建通知消息
	notification := map[string]interface{}{
		"type":      "notification",
		"title":     req.Title,
		"content":   req.Content,
		"priority":  req.Priority,
		"sender":    currentUser,
		"timestamp": utils.UnixMilli(),
	}

	// 发送通知给目标用户
	notificationData, err := utils.JsonMarshal(notification)
	if err != nil {
		connCtx.Logger.Error("failed to marshal notification", zap.Error(err))
		return nil, ex.Throw{Code: ex.SYSTEM, Msg: "failed to create notification"}
	}

	connCtx.Server.connManager.SendToSubject(req.UserID, notificationData)

	connCtx.Logger.Info("notification sent successfully",
		zap.String("target", req.UserID),
		zap.String("title", req.Title))

	return map[string]interface{}{
		"code":      200,
		"message":   "notification sent successfully",
		"target":    req.UserID,
		"title":     req.Title,
		"timestamp": utils.UnixMilli(),
	}, nil
}

// ==================== 客户端连接示例 ====================

// ExampleWebSocketClientConnection 客户端连接示例 (非测试函数，仅供参考)
func ExampleWebSocketClientConnection() {
	// 注意: 这个函数不是测试函数，只是一个使用示例
	// 要运行这个示例，需要先启动服务器，然后取消注释以下代码

	/*
		// 创建客户端
		client := &WsClient{
			Addr:    "localhost:8081",
			Path:    "/ws/chat",
			Origin:  "http://localhost:8081",
			AuthCall: func() (string, string, error) {
				// 返回token和secret (示例)
				return "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...", "secret123", nil
			},
			ReceiveCall: func(data []byte) (interface{}, error) {
				fmt.Printf("Received: %s\n", string(data))
				return nil, nil // 不回复
			},
		}

		// 启动客户端
		go client.StartWebsocket(false, 10) // 自动重连，10秒间隔

		// 等待连接建立
		time.Sleep(2 * time.Second)

		// 发送消息
		chatMsg := map[string]interface{}{
			"type":    "broadcast",
			"content": "Hello from client!",
		}

		msgData, _ := utils.JsonMarshal(chatMsg)
		client.SendMessage(msgData)

		// 运行一段时间
		time.Sleep(30 * time.Second)
	*/
}

// ==================== 高级用法示例 ====================

// TestAdvancedWebSocketFeatures 测试高级WebSocket功能
func TestAdvancedWebSocketFeatures(t *testing.T) {
	server := NewWsServer(true)
	err := server.NewPool(100, 10, 5, 30)
	if err != nil {
		t.Fatalf("Failed to init pool: %v", err)
	}

	// 添加带权限验证的路由
	err = server.AddRouter("/ws/admin", handleAdminMessage)
	if err != nil {
		t.Fatalf("Failed to add admin router: %v", err)
	}

	// 验证路由数量
	server.routesMu.RLock()
	routeCount := len(server.routes)
	server.routesMu.RUnlock()

	if routeCount != 1 {
		t.Errorf("Expected 1 route, got %d", routeCount)
	}
}

// handleAdminMessage 管理员消息处理器示例 (需要权限验证)
func handleAdminMessage(ctx context.Context, connCtx *ConnectionContext, body []byte) (interface{}, error) {
	// 权限检查 (简化版 - 在实际项目中需要更完善的权限系统)
	if connCtx.Subject == nil {
		connCtx.Logger.Warn("access denied: authentication required")
		return nil, ex.Throw{Code: ex.BIZ, Msg: "authentication required"}
	}

	connCtx.Logger.Info("admin command executed", zap.ByteString("command", body))

	// 获取服务器统计信息
	stats := map[string]interface{}{
		"total_connections": connCtx.Server.connManager.Count(),
		"server_uptime":     "example", // 可以添加实际的运行时间统计
		"active_routes":     len(connCtx.Server.routes),
	}

	return map[string]interface{}{
		"code":    200,
		"message": "admin command executed",
		"stats":   stats,
	}, nil
}

// ==================== 压力测试示例 ====================

// BenchmarkWebSocketMessageProcessing 消息处理性能基准测试
func BenchmarkWebSocketMessageProcessing(b *testing.B) {
	messageHandler := NewMessageHandler(func(ctx context.Context, connCtx *ConnectionContext, body []byte) (interface{}, error) {
		return map[string]string{"result": "ok"}, nil
	}, false)

	connCtx := &ConnectionContext{
		Logger: zap.NewNop(),
	}

	testData := []byte(`{"type":"test","content":"benchmark data"}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := messageHandler.Process(connCtx, testData)
		if err != nil {
			b.Fatalf("Message processing failed: %v", err)
		}
	}
}

// BenchmarkConnectionManagerOperations 连接管理器操作性能基准测试
func BenchmarkConnectionManagerOperations(b *testing.B) {
	manager := NewConnectionManager(10000)

	b.Run("AddConnection", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			conn := &DevConn{
				Sub:  "user" + string(rune(i%1000)),
				Dev:  "dev" + string(rune(i%100)),
				Life: utils.UnixSecond() + 3600,
				Last: utils.UnixSecond(),
			}
			manager.Add(conn)
		}
	})

	b.Run("GetConnection", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			userID := "user" + string(rune(i%1000))
			devKey := userID + "_" + userID
			manager.Get(userID, devKey)
		}
	})
}
