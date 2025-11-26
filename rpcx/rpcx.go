package rpcx

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/godaddy-x/freego/cache"
	"github.com/godaddy-x/freego/rpcx/impl"
	"github.com/godaddy-x/freego/rpcx/pb"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/utils/crypto"
	"github.com/godaddy-x/freego/zlog"
	"google.golang.org/grpc"
)

type RPCManager struct {
	mu         sync.Mutex
	server     *grpc.Server
	listener   net.Listener
	cancel     context.CancelFunc
	RSA        []crypto.Cipher
	redisCache cache.Cache
	localCache cache.Cache
}

type AuthToken struct {
	Token  string `json:"token"`
	Secret string `json:"secret"`
	Expire int64  `json:"expire"`
}

// NewRPCManager 创建GRPC管理器
func NewRPCManager() *RPCManager {
	return &RPCManager{}
}

// AddCipher 增加RSA加密器
func (g *RPCManager) AddCipher(cipher crypto.Cipher) error {
	if cipher == nil {
		return utils.Error("cipher is nil")
	}
	g.RSA = append(g.RSA, cipher)
	return nil
}

// AddRedisCache 增加Redis缓存实例
func (g *RPCManager) AddRedisCache(cacheAware cache.Cache) *RPCManager {
	g.redisCache = cacheAware
	return g
}

// AddLocalCache 增加本地缓存实例
func (g *RPCManager) AddLocalCache(cacheAware cache.Cache) *RPCManager {
	g.localCache = cacheAware
	return g
}

// RegisterHandler 注册业务处理器
func (g *RPCManager) RegisterHandler(router string, handler impl.BusinessHandler) error {
	impl.SetHandler(router, handler)
	return nil
}

// StartServer 启动GRPC服务
func (g *RPCManager) StartServer(addr string) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	// 防止重复启动
	if g.server != nil {
		return fmt.Errorf("grpc server has already been started")
	}

	// 验证必要配置
	if len(g.RSA) == 0 {
		return fmt.Errorf("RSA cipher must be set before starting server")
	}

	// 验证至少有一个业务处理器已注册
	if len(impl.GetAllHandlers()) == 0 {
		return fmt.Errorf("at least one business handler must be registered before starting server")
	}

	// 记录服务状态
	if g.redisCache != nil {
		zlog.Printf("redis cache service has been started successful")
	}
	if g.localCache != nil {
		zlog.Printf("local cache service has been started successful")
	}
	if g.RSA != nil {
		zlog.Printf("ECC certificate service has been started successful")
	}

	// 创建上下文用于优雅关闭
	_, g.cancel = context.WithCancel(context.Background())

	// 创建GRPC服务器
	g.server = grpc.NewServer()

	// 注册通用服务
	commonWorker := &impl.CommonWorker{
		ConfigProvider: g, // 传递配置提供者
	}
	pb.RegisterCommonWorkerServer(g.server, commonWorker)

	// 启动监听
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %v", addr, err)
	}
	g.listener = listener

	// 异步启动服务
	go func() {
		zlog.Printf("grpc【%s】service has been started successful", addr)
		if err := g.server.Serve(g.listener); err != nil {
			// 忽略已关闭的错误
			if err.Error() != "use of closed network connection" {
				zlog.Error("grpc server serve failed", 0, zlog.AddError(err))
			}
		}
	}()

	return nil
}

// StartServerByTimeout 带超时的启动GRPC服务
func (g *RPCManager) StartServerByTimeout(addr string, timeout int) error {
	// 设置超时（这里可以添加超时逻辑）
	_ = timeout
	return g.StartServer(addr)
}

// StopServer 停止GRPC服务
func (g *RPCManager) StopServer() error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.server == nil {
		return nil
	}

	// 优雅关闭
	done := make(chan struct{})
	go func() {
		g.server.GracefulStop()
		close(done)
	}()

	select {
	case <-done:
		zlog.Printf("grpc server stopped gracefully")
	case <-time.After(10 * time.Second):
		zlog.Warn("grpc server graceful shutdown timeout, forcing stop", 0)
		g.server.Stop()
	}

	// 关闭监听器
	if g.listener != nil {
		g.listener.Close()
	}

	// 取消上下文
	if g.cancel != nil {
		g.cancel()
	}

	g.server = nil
	g.listener = nil
	g.cancel = nil

	return nil
}

// StopServerByTimeout 带超时的停止GRPC服务
func (g *RPCManager) StopServerByTimeout(timeout time.Duration) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.server == nil {
		return nil
	}

	// 优雅关闭
	done := make(chan struct{})
	go func() {
		g.server.GracefulStop()
		close(done)
	}()

	select {
	case <-done:
		zlog.Printf("grpc server stopped gracefully")
	case <-time.After(timeout):
		zlog.Warn("grpc server graceful shutdown timeout, forcing stop", 0)
		g.server.Stop()
	}

	// 关闭监听器
	if g.listener != nil {
		g.listener.Close()
	}

	// 取消上下文
	if g.cancel != nil {
		g.cancel()
	}

	g.server = nil
	g.listener = nil
	g.cancel = nil

	return nil
}

// GetRSA 获取RSA密钥列表 (实现ConfigProvider接口)
func (g *RPCManager) GetRSA() []crypto.Cipher {
	g.mu.Lock()
	defer g.mu.Unlock()
	// 返回副本以避免外部修改
	rsaCopy := make([]crypto.Cipher, len(g.RSA))
	copy(rsaCopy, g.RSA)
	return rsaCopy
}

// GetLocalCache 获取本地缓存
func (g *RPCManager) GetLocalCache() cache.Cache {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.localCache
}

// GetRedisCache 获取Redis缓存
func (g *RPCManager) GetRedisCache() cache.Cache {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.redisCache
}
