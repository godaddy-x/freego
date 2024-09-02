package rpcx

import (
	"crypto/tls"
	"fmt"
	"github.com/godaddy-x/freego/cache/limiter"
	"github.com/godaddy-x/freego/rpcx/pool"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/utils/crypto"
	"github.com/godaddy-x/freego/utils/jwt"
	"github.com/godaddy-x/freego/zlog"
	consulapi "github.com/hashicorp/consul/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"net"
	"net/http"
	"sync"
	"time"
)

var (
	serverDialTLS   grpc.ServerOption
	clientDialTLS   grpc.DialOption
	jwtConfig       *jwt.JwtConfig
	rateLimiterCall func(string) (rate.Option, error)
	selectionCall   func([]*consulapi.ServiceEntry, GRPC) *consulapi.ServiceEntry
	authorizeTLS    *crypto.RsaObj
	accessToken     = ""
	clientConnPools = ClientConnPool{pools: make(map[string]pool.Pool)}
)

type ClientOptions []grpc.DialOption

type ClientConnPool struct {
	m sync.Mutex
	//once  concurrent.Once
	pools map[string]pool.Pool
}

type GRPCManager struct {
	consul       *ConsulManager
	consulDs     string
	authenticate bool
}

type GRPC struct {
	Ds      string                    // consul数据源ds
	Tags    []string                  // 服务标签名称
	Address string                    // 服务地址,为空时自动填充内网IP
	RpcPort int                       // 服务地址端口
	Service string                    // 服务名称
	Cache   int                       // 服务缓存时间/秒
	Timeout int                       // context timeout/毫秒
	AddRPC  func(server *grpc.Server) // grpc注册proto服务
	Center  bool                      // false: 非注册中心 true: consul注册中心获取
}

func (s *GRPCManager) CreateRateLimiterCall(fun func(method string) (rate.Option, error)) {
	if rateLimiterCall != nil {
		return
	}
	rateLimiterCall = fun
}

func (s *GRPCManager) CreateSelectionCall(fun func([]*consulapi.ServiceEntry, GRPC) *consulapi.ServiceEntry) {
	if selectionCall != nil {
		return
	}
	selectionCall = fun
}

//func (s *GRPCManager) CreateServerTLS(tlsConfig TlsConfig) {
//	if serverDialTLS != nil {
//		return
//	}
//	if tlsConfig.UseTLS && tlsConfig.UseMTLS {
//		panic("only one UseTLS/UseMTLS can be used")
//	}
//	if len(tlsConfig.CrtFile) == 0 {
//		panic("server.crt file is nil")
//	}
//	if len(tlsConfig.KeyFile) == 0 {
//		panic("server.key file is nil")
//	}
//	if tlsConfig.UseTLS {
//		creds, err := credentials.NewServerTLSFromFile(tlsConfig.CrtFile, tlsConfig.KeyFile)
//		if err != nil {
//			panic(err)
//		}
//		serverDialTLS = grpc.Creds(creds)
//		s.CreateAuthorizeTLS(tlsConfig.KeyFile)
//	}
//	if tlsConfig.UseMTLS {
//		if len(tlsConfig.CACrtFile) == 0 {
//			panic("ca.crt file is nil")
//		}
//		certPool := x509.NewCertPool()
//		ca, err := ioutil.ReadFile(tlsConfig.CACrtFile)
//		if err != nil {
//			panic(err)
//		}
//		if ok := certPool.AppendCertsFromPEM(ca); !ok {
//			panic("failed to append certs")
//		}
//		cert, err := tls.LoadX509KeyPair(tlsConfig.CrtFile, tlsConfig.KeyFile)
//		if err != nil {
//			panic(err)
//		}
//		// 构建基于 TLS 的 TransportCredentials
//		creds := credentials.NewTLS(&tls.Config{
//			// 设置证书链，允许包含一个或多个
//			Certificates: []tls.Certificate{cert},
//			// 要求必须校验客户端的证书 可以根据实际情况选用其他参数
//			ClientAuth: tls.RequireAndVerifyClientCert, // NOTE: this is optional!
//			// 设置根证书的集合，校验方式使用 ClientAuth 中设定的模式
//			ClientCAs: certPool,
//		})
//		serverDialTLS = grpc.Creds(creds)
//		s.CreateAuthorizeTLS(tlsConfig.KeyFile)
//	}
//}

//func (s *GRPCManager) CreateClientTLS(tlsConfig TlsConfig) {
//	if clientDialTLS != nil {
//		return
//	}
//	if tlsConfig.UseTLS && tlsConfig.UseMTLS {
//		panic("only one tls mode can be used")
//	}
//	if len(tlsConfig.CrtFile) == 0 {
//		panic("server.crt file is nil")
//	}
//	if tlsConfig.UseTLS {
//		if len(tlsConfig.CrtFile) == 0 {
//			panic("server.crt file is nil")
//		}
//		if len(tlsConfig.HostName) == 0 {
//			panic("server host name is nil")
//		}
//		creds, err := credentials.NewClientTLSFromFile(tlsConfig.CrtFile, tlsConfig.HostName)
//		if err != nil {
//			panic(err)
//		}
//		clientDialTLS = grpc.WithTransportCredentials(creds)
//	}
//	if tlsConfig.UseMTLS {
//		if len(tlsConfig.CACrtFile) == 0 {
//			panic("ca.crt file is nil")
//		}
//		if len(tlsConfig.CrtFile) == 0 {
//			panic("client.crt file is nil")
//		}
//		if len(tlsConfig.KeyFile) == 0 {
//			panic("client.key file is nil")
//		}
//		if len(tlsConfig.HostName) == 0 {
//			panic("server host name is nil")
//		}
//		// 加载客户端证书
//		cert, err := tls.LoadX509KeyPair(tlsConfig.CrtFile, tlsConfig.KeyFile)
//		if err != nil {
//			panic(err)
//		}
//		// 构建CertPool以校验服务端证书有效性
//		certPool := x509.NewCertPool()
//		ca, err := ioutil.ReadFile(tlsConfig.CACrtFile)
//		if err != nil {
//			panic(err)
//		}
//		if ok := certPool.AppendCertsFromPEM(ca); !ok {
//			panic("failed to append ca certs")
//		}
//		creds := credentials.NewTLS(&tls.Config{
//			Certificates: []tls.Certificate{cert},
//			ServerName:   tlsConfig.HostName, // NOTE: this is required!
//			RootCAs:      certPool,
//		})
//		clientDialTLS = grpc.WithTransportCredentials(creds)
//	}
//}

func RunServer(consulDs string, authenticate bool, objects ...*GRPC) {
	if len(objects) == 0 {
		panic("rpc objects is nil...")
	}
	c, err := NewConsul(consulDs)
	if err != nil {
		panic(err)
	}
	self := &GRPCManager{consul: c, consulDs: consulDs, authenticate: authenticate}
	services, err := self.consul.GetAllService("")
	if err != nil {
		panic(err)
	}
	if err != nil {
		panic(err)
	}
	opts := []grpc.ServerOption{
		grpc.InitialWindowSize(pool.InitialWindowSize),
		grpc.InitialConnWindowSize(pool.InitialConnWindowSize),
		grpc.MaxSendMsgSize(pool.MaxSendMsgSize),
		grpc.MaxRecvMsgSize(pool.MaxRecvMsgSize),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			PermitWithoutStream: true,
		}),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			Time:    pool.KeepAliveTime,
			Timeout: pool.KeepAliveTimeout,
		}),
		grpc.UnaryInterceptor(self.ServerInterceptor),
	}
	if serverDialTLS != nil {
		opts = append(opts, serverDialTLS)
	}
	grpcServer := grpc.NewServer(opts...)
	for _, object := range objects {
		address := utils.GetLocalIP()
		port := self.consul.Config.RpcPort
		if object.RpcPort > 0 {
			port = object.RpcPort
		}
		if len(address) == 0 {
			panic("local address reading failed")
		}
		if len(object.Address) > 0 {
			address = object.Address
		}
		if len(object.Service) == 0 || len(object.Service) > 100 {
			panic("rpc service invalid")
		}
		if self.consul.CheckService(services, object.Service, address) {
			zlog.Println(utils.AddStr("grpc service [", object.Service, "][", address, "] exist, skip..."))
			object.AddRPC(grpcServer)
			continue
		}
		registration := new(consulapi.AgentServiceRegistration)
		registration.ID = utils.GetUUID()
		registration.Tags = object.Tags
		registration.Name = object.Service
		registration.Address = address
		registration.Port = port
		registration.Meta = make(map[string]string, 0)
		registration.Check = &consulapi.AgentServiceCheck{
			HTTP:                           fmt.Sprintf("http://%s:%d%s", registration.Address, self.consul.Config.CheckPort, self.consul.Config.CheckPath),
			Timeout:                        self.consul.Config.Timeout,
			Interval:                       self.consul.Config.Interval,
			DeregisterCriticalServiceAfter: self.consul.Config.DestroyAfter,
		}
		zlog.Println(utils.AddStr("grpc service [", registration.Name, "][", registration.Address, "] added successful"))
		if err := self.consul.Consulx.Agent().ServiceRegister(registration); err != nil {
			panic(utils.AddStr("grpc service [", object.Service, "] add failed: ", err.Error()))
		}
		object.AddRPC(grpcServer)
	}
	go func() {
		http.HandleFunc(self.consul.Config.CheckPath, self.consul.HealthCheck)
		if err := http.ListenAndServe(fmt.Sprintf(":%d", self.consul.Config.CheckPort), nil); err != nil {
			panic(err)
		}
	}()
	l, err := net.Listen(self.consul.Config.Protocol, utils.AddStr(":", utils.AnyToStr(self.consul.Config.RpcPort)))
	if err != nil {
		panic(err)
	}
	zlog.Println(utils.AddStr("grpc server【", utils.AddStr(":", utils.AnyToStr(self.consul.Config.RpcPort)), "】has been started successful"))
	if err := grpcServer.Serve(l); err != nil {
		panic(err)
	}
}

type Param struct {
	Addr              string
	Object            []*GRPC
	CaFile            string
	CertFile          string
	KeyFile           string
	ClientTimeout     int
	ClientInterceptor grpc.UnaryClientInterceptor
	ServerInterceptor grpc.UnaryServerInterceptor
	ClientOptions     ClientOptions
}

func RunOnlyServer(param Param) {
	if len(param.Object) == 0 {
		panic("rpc objects is nil...")
	}
	opts := []grpc.ServerOption{
		grpc.InitialWindowSize(pool.InitialWindowSize),
		grpc.InitialConnWindowSize(pool.InitialConnWindowSize),
		grpc.MaxSendMsgSize(pool.MaxSendMsgSize),
		grpc.MaxRecvMsgSize(pool.MaxRecvMsgSize),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			PermitWithoutStream: true,
		}),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			Time:    pool.KeepAliveTime,
			Timeout: pool.KeepAliveTimeout,
		}),
		//grpc.UnaryInterceptor(self.ServerInterceptor),
	}
	if len(param.CertFile) > 0 && len(param.KeyFile) > 0 {
		// Load server's certificate and private key
		serverCert, err := tls.LoadX509KeyPair(param.CertFile, param.KeyFile)
		if err != nil {
			panic(err)
		}
		// Create a TLS config with server's certificate
		tlsConfig := &tls.Config{
			Certificates: []tls.Certificate{serverCert},
		}
		opts = append(opts, grpc.Creds(credentials.NewTLS(tlsConfig)))
	}
	grpcServer := grpc.NewServer(opts...)
	for _, object := range param.Object {
		address := utils.GetLocalIP()
		if len(address) == 0 {
			panic("local address reading failed")
		}
		if len(object.Address) == 0 {
			object.Address = address
		}
		if len(object.Service) == 0 || len(object.Service) > 100 {
			panic("rpc service invalid")
		}
		object.AddRPC(grpcServer)
	}
	l, err := net.Listen("tcp", param.Addr)
	if err != nil {
		panic(err)
	}
	zlog.Println(utils.AddStr("grpc server【", param.Addr, "】has been started successful"))
	if err := grpcServer.Serve(l); err != nil {
		panic(err)
	}
}

func NewOnlyClient(serverAddress string, timeout int, clientOptions ClientOptions) (pool.Conn, error) {
	return clientConnPools.getClientConn(serverAddress, timeout, clientOptions)
}

// RunClient Important: ensure that the service starts only once
// JWT Token expires in 1 hour
// The remaining 1200s will be automatically renewed and detected every 15s
func RunClient(appId ...string) {
	//if len(clientOptions) == 0 {
	//	c, err := NewConsul()
	//	if err != nil {
	//		panic(err)
	//	}
	//	client := &GRPCManager{consul: c, consulDs: ""}
	//	clientOptions = append(clientOptions, grpc.WithInitialWindowSize(pool.InitialWindowSize))
	//	clientOptions = append(clientOptions, grpc.WithInitialConnWindowSize(pool.InitialConnWindowSize))
	//	clientOptions = append(clientOptions, grpc.WithDefaultCallOptions(grpc.MaxCallSendMsgSize(pool.MaxSendMsgSize)))
	//	clientOptions = append(clientOptions, grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(pool.MaxRecvMsgSize)))
	//	clientOptions = append(clientOptions, grpc.WithKeepaliveParams(keepalive.ClientParameters{
	//		Time:                pool.KeepAliveTime,
	//		Timeout:             pool.KeepAliveTimeout,
	//		PermitWithoutStream: true,
	//	}))
	//	clientOptions = append(clientOptions, grpc.WithUnaryInterceptor(client.ClientInterceptor))
	//	if clientDialTLS != nil {
	//		clientOptions = append(clientOptions, clientDialTLS)
	//	} else {
	//		clientOptions = append(clientOptions, grpc.WithTransportCredentials(insecure.NewCredentials()))
	//	}
	//}
	//if len(appId) == 0 || len(appId[0]) == 0 {
	//	return
	//}
	//var err error
	//var expired int64
	//for {
	//	accessToken, expired, err = callLogin(appId[0])
	//	if err != nil {
	//		zlog.Error("rpc login failed", 0, zlog.AddError(err))
	//		time.Sleep(5 * time.Second)
	//		continue
	//	}
	//	break
	//}
	//go renewClientToken(appId[0], expired)
}

// CreateClientOpts serverAddr: 服务端地址 interceptor: 客户端拦截器
func CreateClientOpts(param Param) ClientOptions {
	var clientOptions []grpc.DialOption
	clientOptions = append(clientOptions, grpc.WithInitialWindowSize(pool.InitialWindowSize))
	clientOptions = append(clientOptions, grpc.WithInitialConnWindowSize(pool.InitialConnWindowSize))
	clientOptions = append(clientOptions, grpc.WithDefaultCallOptions(grpc.MaxCallSendMsgSize(pool.MaxSendMsgSize)))
	clientOptions = append(clientOptions, grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(pool.MaxRecvMsgSize)))
	clientOptions = append(clientOptions, grpc.WithKeepaliveParams(keepalive.ClientParameters{
		Time:                pool.KeepAliveTime,
		Timeout:             pool.KeepAliveTimeout,
		PermitWithoutStream: true,
	}))
	if param.ClientInterceptor != nil {
		clientOptions = append(clientOptions, grpc.WithUnaryInterceptor(param.ClientInterceptor))
	}
	if len(param.CertFile) > 0 {
		tls, err := credentials.NewClientTLSFromFile(param.CertFile, "")
		if err != nil {
			panic(err)
		}
		clientOptions = append(clientOptions, grpc.WithTransportCredentials(tls))
	} else {
		clientOptions = append(clientOptions, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	return clientOptions
}

func NewClientConn(object GRPC) (pool.Conn, error) {
	if len(object.Service) == 0 || len(object.Service) > 100 {
		return nil, utils.Error("call service invalid")
	}
	var tag string
	var timeout int
	if object.Timeout <= 0 {
		timeout = 60000
	}
	if len(object.Tags) > 0 {
		tag = object.Tags[0]
	}
	c, err := NewConsul(object.Ds)
	if err != nil {
		return nil, err
	}
	services, err := c.GetCacheService(object.Service, tag, object.Cache)
	if err != nil {
		return nil, utils.Error("query service [", object.Service, "] failed: ", err)
	}
	var service *consulapi.AgentService
	if selectionCall == nil { // 选取规则为空则默认随机
		if len(services) == 1 {
			service = services[0].Service
		} else {
			service = services[utils.ModRand(len(services))].Service
		}
	} else {
		service = selectionCall(services, object).Service
	}
	return clientConnPools.getClientConn(utils.AddStr(service.Address, ":", service.Port), timeout, nil)
}

func (s *ClientConnPool) getClientConn(host string, timeout int, clientOptions ClientOptions) (conn pool.Conn, err error) {
	p, b := s.pools[host]
	if !b || p == nil {
		zlog.Warn("client pool host creating", 0, zlog.String("host", host))
		p, err = s.readyPool(host, clientOptions)
		if err != nil {
			zlog.Error("client pool ready failed", 0, zlog.AddError(err))
			return nil, err
		}
		if p == nil {
			return nil, utils.Error("init client pool failed")
		}
	}
	conn, err = p.Get()
	if err != nil {
		return nil, err
	}
	conn.NewContext(time.Duration(timeout) * time.Millisecond)
	return conn, nil
}

func (s *ClientConnPool) readyPool(host string, clientOptions []grpc.DialOption) (pool.Pool, error) {
	s.m.Lock()
	defer s.m.Unlock()
	p, b := s.pools[host]
	if !b || p == nil {
		pool, err := pool.NewPool(pool.DefaultOptions, pool.ConnConfig{Address: host, Timeout: 10, Opts: clientOptions})
		if err != nil {
			return nil, err
		}
		s.pools[host] = pool
		zlog.Info("client connection pool create successful", 0, zlog.String("host", host))
		return pool, nil
	}
	p, b = s.pools[host]
	if !b || p == nil {
		return nil, utils.Error("pool connection create failed")
		//panic("pool connection not found")
	}
	return p, nil
}
