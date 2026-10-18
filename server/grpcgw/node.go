package grpcgw

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"encoding/hex"

	"github.com/godaddy-x/freego/core/jwt"
	utils "github.com/godaddy-x/freego/core/str"
	cache "github.com/godaddy-x/freego/infra/cache/contract"
	"github.com/godaddy-x/freego/infra/cachelocal"
	"github.com/godaddy-x/freego/infra/zlog"
	pb "github.com/godaddy-x/freego/protocol/grpcgw"
	fwsign "github.com/godaddy-x/freego/protocol/sign"
	"google.golang.org/grpc"
)

type UnaryHandler func(ctx *Context) error

type CipherHook = fwsign.CipherHook

// GrpcGateway is the grpcgw-v1 server entry (parallel to httpsvr.HttpNode).
type GrpcGateway struct {
	mu            sync.Mutex
	server        *grpc.Server
	listener      net.Listener
	jwtConfig     jwt.JwtConfig
	methods       map[string]UnaryHandler
	methodConfigs map[string]*MethodConfig
	cipherHook    CipherHook
	broadcastKey  string
	localCache    cache.Cache
	redisCache    cache.Cache
	hub           *StreamHub
	onStreamEnd   func(u int64, device string)
	errorHandle   ErrorHandle
	interceptors  []*InterceptorObject
}

func NewGrpcGateway() *GrpcGateway {
	g := &GrpcGateway{
		methods:       make(map[string]UnaryHandler),
		methodConfigs: make(map[string]*MethodConfig),
		localCache:    cachelocal.NewLocalCache(),
		broadcastKey:  hex.EncodeToString(utils.GetRandomSecure(32)),
		hub:           newStreamHub(),
	}
	// Reserved: Gateway.Connect 首帧；需 JWT Plan0/1（与 getUser 相同 mac_key）。
	g.AddMethod(pb.StreamBindRoute, func(ctx *Context) error {
		ctx.SetPlain(pb.StreamBindAckPlain())
		return nil
	}, &MethodConfig{})
	return g
}

func (g *GrpcGateway) AddJwtConfig(c jwt.JwtConfig) {
	g.jwtConfig = c
}

func (g *GrpcGateway) AddCipherHook(hook CipherHook) {
	g.cipherHook = hook
}

func (g *GrpcGateway) AddRedisCache(c cache.Cache) {
	g.redisCache = c
}

// cacheObject：与 HTTP Context.GetCacheObject 一致，优先 Redis（AddRedisCache），否则进程内 local。
func (g *GrpcGateway) cacheObject() cache.Cache {
	if g.redisCache != nil {
		return g.redisCache
	}
	return g.localCache
}

func (g *GrpcGateway) SetBroadcastKey(key string) {
	g.broadcastKey = key
}

// RegisterOn attaches Gateway service to an existing grpc.Server (tests / custom listeners).
func (g *GrpcGateway) RegisterOn(s *grpc.Server) {
	pb.RegisterGatewayServer(s, &gatewayService{gw: g})
}

func (g *GrpcGateway) AddMethod(route string, h UnaryHandler, cfg *MethodConfig) {
	if cfg == nil {
		cfg = &MethodConfig{}
	}
	if err := validateMethodConfig(cfg); err != nil {
		panic(err.Error())
	}
	g.methods[route] = h
	g.methodConfigs[route] = cfg
}

// StartServer 与 httpsvr.HttpNode.StartServer 一致：后台 Serve，阻塞等待退出信号后 GracefulStop。
func (g *GrpcGateway) StartServer(addr string) error {
	if err := g.ListenAndServe(addr); err != nil {
		return err
	}
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)
	<-quit
	zlog.Info("grpcgw server is shutting down...", 0)
	g.GracefulStop()
	zlog.Info("grpcgw server stopped", 0)
	return nil
}

// ListenAndServe 仅启动监听与 gRPC Serve（不阻塞）；单测或嵌入其它进程时使用。
func (g *GrpcGateway) ListenAndServe(addr string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.server != nil {
		return fmt.Errorf("grpcgw server already started")
	}
	if g.cipherHook == nil {
		return fmt.Errorf("cipher hook required (even for Plan0/1 example)")
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	s := grpc.NewServer()
	g.RegisterOn(s)
	g.server = s
	g.listener = ln
	zlog.Info(fmt.Sprintf("grpcgw service has been started successful on %s transport=%s", ln.Addr().String(), pb.TransportVersion), 0)
	go func() {
		if err := s.Serve(ln); err != nil {
			zlog.Error("grpcgw serve failed", 0, zlog.String("err", err.Error()))
		}
	}()
	return nil
}

// Addr returns the bound listen address after StartServer (e.g. 127.0.0.1:54321 when addr used :0).
func (g *GrpcGateway) Addr() string {
	g.mu.Lock()
	ln := g.listener
	g.mu.Unlock()
	if ln == nil {
		return ""
	}
	return ln.Addr().String()
}

func (g *GrpcGateway) GracefulStop() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.server != nil {
		g.server.GracefulStop()
		g.server = nil
	}
	if g.listener != nil {
		_ = g.listener.Close()
		g.listener = nil
	}
}

func (g *GrpcGateway) methodConfig(route string) *MethodConfig {
	if c, ok := g.methodConfigs[route]; ok {
		return c
	}
	return &MethodConfig{}
}

func (g *GrpcGateway) handleCall(ctx context.Context, req *pb.GatewayRequest) (*pb.GatewayResponse, error) {
	if req == nil {
		return g.signedError("", 400, "request is nil", 0), nil
	}
	if req.R == "" {
		return g.signedError("", 400, "r is empty", 0), nil
	}
	route := req.R
	h := g.methods[route]
	if h == nil {
		return g.signedError(route, 404, "route not found", req.U), nil
	}
	cfg := g.methodConfig(route)
	gctx := newContext(ctx, g, req, cfg)
	if err := gctx.validateInbound(); err != nil {
		zlog.Info("grpcgw inbound rejected", 0, zlog.String("route", route), zlog.String("err", err.Error()))
		return g.signedError(route, 400, "request rejected", req.U), nil
	}
	if err := g.runInterceptors(gctx); err != nil {
		return gctx.renderError(err)
	}
	if err := h(gctx); err != nil {
		return gctx.renderError(err)
	}
	return gctx.renderSuccess()
}

type gatewayService struct {
	pb.UnimplementedGatewayServer
	gw *GrpcGateway
}

func (s *gatewayService) Call(ctx context.Context, req *pb.GatewayRequest) (*pb.GatewayResponse, error) {
	return s.gw.handleCall(ctx, req)
}
