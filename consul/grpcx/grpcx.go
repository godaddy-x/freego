package grpcx

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"github.com/godaddy-x/freego/cache/limiter"
	"github.com/godaddy-x/freego/consul"
	"github.com/godaddy-x/freego/consul/grpcx/pb"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/utils/gorsa"
	"github.com/godaddy-x/freego/utils/jwt"
	"github.com/godaddy-x/freego/zlog"
	consulapi "github.com/hashicorp/consul/api"
	"github.com/shimingyah/pool"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"time"
)

var (
	serverDialTLS   grpc.ServerOption
	clientDialTLS   grpc.DialOption
	jwtConfig       *jwt.JwtConfig
	rateLimiterCall func(string) (rate.Option, error)
	selectionCall   func([]*consulapi.ServiceEntry, *GRPC) *consulapi.ServiceEntry
	appConfigCall   func(string) (AppConfig, error)
	authorizeTLS    *gorsa.RsaObj
	accessToken     = ""
	clientOptions   []grpc.DialOption
	clientPools     = make(map[string]pool.Pool, 0)
)

type GRPCManager struct {
	consul       *consul.ConsulManager
	consulDs     string
	authenticate bool
}

type ClientConfig struct {
	Appid   string
	Addrs   []string
	Timeout int
}

type TlsConfig struct {
	UseTLS    bool
	UseMTLS   bool
	CACrtFile string
	CAKeyFile string
	KeyFile   string
	CrtFile   string
	HostName  string
}

type AppConfig struct {
	Appid    string
	Appkey   string
	Status   int64
	LastTime int64
}

type GRPC struct {
	Ds          string                                                                // consul数据源ds
	Tags        []string                                                              // 服务标签名称
	Address     string                                                                // 服务地址,为空时自动填充内网IP
	Service     string                                                                // 服务名称
	CacheSecond int                                                                   // 服务缓存时间/秒
	TimeoutMill int                                                                   // RPC请求超时/毫秒
	AddRPC      func(server *grpc.Server)                                             // grpc注册proto服务
	CallRPC     func(conn *grpc.ClientConn, ctx context.Context) (interface{}, error) // grpc回调proto服务
}

type AuthObject struct {
	Appid     string
	Nonce     string
	Time      int64
	Signature string
}

func GetGRPCJwtConfig() (*jwt.JwtConfig, error) {
	if len(jwtConfig.TokenKey) == 0 {
		return nil, utils.Error("grpc jwt key is nil")
	}
	return jwtConfig, nil
}

func GetAuthorizeTLS() (*gorsa.RsaObj, error) {
	if authorizeTLS == nil {
		return nil, utils.Error("authorize tls is nil")
	}
	return authorizeTLS, nil
}

func GetGRPCAppConfig(appid string) (AppConfig, error) {
	if appConfigCall == nil {
		return AppConfig{}, utils.Error("grpc app config call is nil")
	}
	return appConfigCall(appid)
}

func (self *GRPCManager) CreateJwtConfig(tokenKey string, tokenExp ...int64) {
	if jwtConfig != nil {
		return
	}
	if len(tokenKey) < 32 {
		panic("jwt tokenKey length should be >= 32")
	}
	var exp = int64(3600)
	if len(tokenExp) > 0 && tokenExp[0] >= 3600 {
		exp = tokenExp[0]
	}
	jwtConfig = &jwt.JwtConfig{
		TokenTyp: jwt.JWT,
		TokenAlg: jwt.HS256,
		TokenKey: tokenKey,
		TokenExp: exp,
	}
}

func (self *GRPCManager) CreateAppConfigCall(fun func(appid string) (AppConfig, error)) {
	if appConfigCall != nil {
		return
	}
	appConfigCall = fun
}

func (self *GRPCManager) CreateRateLimiterCall(fun func(method string) (rate.Option, error)) {
	if rateLimiterCall != nil {
		return
	}
	rateLimiterCall = fun
}

func (self *GRPCManager) CreateSelectionCall(fun func([]*consulapi.ServiceEntry, *GRPC) *consulapi.ServiceEntry) {
	if selectionCall != nil {
		return
	}
	selectionCall = fun
}

// CreateAuthorizeTLS If server TLS is used, the certificate server.key is used by default
// Otherwise, the method needs to be explicitly called to set the certificate
func (self *GRPCManager) CreateAuthorizeTLS(keyPath string) {
	if authorizeTLS != nil {
		return
	}
	if len(keyPath) == 0 {
		panic("authorize tls key path is nil")
	}
	obj := &gorsa.RsaObj{}
	if err := obj.LoadRsaFile(keyPath); err != nil {
		panic(err)
	}
	authorizeTLS = obj
}

func (self *GRPCManager) CreateServerTLS(tlsConfig TlsConfig) {
	if serverDialTLS != nil {
		return
	}
	if tlsConfig.UseTLS && tlsConfig.UseMTLS {
		panic("only one UseTLS/UseMTLS can be used")
	}
	if len(tlsConfig.CrtFile) == 0 {
		panic("server.crt file is nil")
	}
	if len(tlsConfig.KeyFile) == 0 {
		panic("server.key file is nil")
	}
	if tlsConfig.UseTLS {
		creds, err := credentials.NewServerTLSFromFile(tlsConfig.CrtFile, tlsConfig.KeyFile)
		if err != nil {
			panic(err)
		}
		serverDialTLS = grpc.Creds(creds)
		self.CreateAuthorizeTLS(tlsConfig.KeyFile)
	}
	if tlsConfig.UseMTLS {
		if len(tlsConfig.CACrtFile) == 0 {
			panic("ca.crt file is nil")
		}
		certPool := x509.NewCertPool()
		ca, err := ioutil.ReadFile(tlsConfig.CACrtFile)
		if err != nil {
			panic(err)
		}
		if ok := certPool.AppendCertsFromPEM(ca); !ok {
			panic("failed to append certs")
		}
		cert, err := tls.LoadX509KeyPair(tlsConfig.CrtFile, tlsConfig.KeyFile)
		if err != nil {
			panic(err)
		}
		// 构建基于 TLS 的 TransportCredentials
		creds := credentials.NewTLS(&tls.Config{
			// 设置证书链，允许包含一个或多个
			Certificates: []tls.Certificate{cert},
			// 要求必须校验客户端的证书 可以根据实际情况选用其他参数
			ClientAuth: tls.RequireAndVerifyClientCert, // NOTE: this is optional!
			// 设置根证书的集合，校验方式使用 ClientAuth 中设定的模式
			ClientCAs: certPool,
		})
		serverDialTLS = grpc.Creds(creds)
		self.CreateAuthorizeTLS(tlsConfig.KeyFile)
	}
}

func (self *GRPCManager) CreateClientTLS(tlsConfig TlsConfig) {
	if clientDialTLS != nil {
		return
	}
	if tlsConfig.UseTLS && tlsConfig.UseMTLS {
		panic("only one tls mode can be used")
	}
	if len(tlsConfig.CrtFile) == 0 {
		panic("server.crt file is nil")
	}
	if tlsConfig.UseTLS {
		if len(tlsConfig.CrtFile) == 0 {
			panic("server.crt file is nil")
		}
		if len(tlsConfig.HostName) == 0 {
			panic("server host name is nil")
		}
		creds, err := credentials.NewClientTLSFromFile(tlsConfig.CrtFile, tlsConfig.HostName)
		if err != nil {
			panic(err)
		}
		clientDialTLS = grpc.WithTransportCredentials(creds)
	}
	if tlsConfig.UseMTLS {
		if len(tlsConfig.CACrtFile) == 0 {
			panic("ca.crt file is nil")
		}
		if len(tlsConfig.CrtFile) == 0 {
			panic("client.crt file is nil")
		}
		if len(tlsConfig.KeyFile) == 0 {
			panic("client.key file is nil")
		}
		if len(tlsConfig.HostName) == 0 {
			panic("server host name is nil")
		}
		// 加载客户端证书
		cert, err := tls.LoadX509KeyPair(tlsConfig.CrtFile, tlsConfig.KeyFile)
		if err != nil {
			panic(err)
		}
		// 构建CertPool以校验服务端证书有效性
		certPool := x509.NewCertPool()
		ca, err := ioutil.ReadFile(tlsConfig.CACrtFile)
		if err != nil {
			panic(err)
		}
		if ok := certPool.AppendCertsFromPEM(ca); !ok {
			panic("failed to append ca certs")
		}
		creds := credentials.NewTLS(&tls.Config{
			Certificates: []tls.Certificate{cert},
			ServerName:   tlsConfig.HostName, // NOTE: this is required!
			RootCAs:      certPool,
		})
		clientDialTLS = grpc.WithTransportCredentials(creds)
	}
}

func RunServer(consulDs string, authenticate bool, objects ...*GRPC) {
	if len(objects) == 0 {
		panic("rpc objects is nil...")
	}
	c, err := consul.NewConsul(consulDs)
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

// RunClient Important: ensure that the service starts only once
// JWT Token expires in 1 hour
// The remaining 1200s will be automatically renewed and detected every 15s
func RunClient(config ClientConfig) {
	if len(config.Appid) == 0 {
		panic("appid is nil")
	}
	if config.Timeout <= 0 {
		panic("timeout invalid")
	}
	if len(config.Addrs) == 0 {
		panic("addrs is nil")
	}
	if len(clientOptions) == 0 {
		c, err := consul.NewConsul()
		if err != nil {
			panic(err)
		}
		client := &GRPCManager{consul: c, consulDs: ""}
		clientOptions = append(clientOptions, grpc.WithInitialWindowSize(pool.InitialWindowSize))
		clientOptions = append(clientOptions, grpc.WithInitialConnWindowSize(pool.InitialConnWindowSize))
		clientOptions = append(clientOptions, grpc.WithDefaultCallOptions(grpc.MaxCallSendMsgSize(pool.MaxSendMsgSize)))
		clientOptions = append(clientOptions, grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(pool.MaxRecvMsgSize)))
		clientOptions = append(clientOptions, grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                pool.KeepAliveTime,
			Timeout:             pool.KeepAliveTimeout,
			PermitWithoutStream: true,
		}))
		clientOptions = append(clientOptions, grpc.WithUnaryInterceptor(client.ClientInterceptor))
		if clientDialTLS != nil {
			clientOptions = append(clientOptions, clientDialTLS)
		} else {
			clientOptions = append(clientOptions, grpc.WithTransportCredentials(insecure.NewCredentials()))
		}
		if len(config.Addrs) == 0 {
			panic("client addrs is nil")
		}
		for _, v := range config.Addrs {
			clientPool, err := pool.NewPool(pool.DefaultOptions, v, config.Timeout, clientOptions)
			if err != nil {
				panic(err)
			}
			clientPools[v] = clientPool
		}
	}
	var loginRes *pb.AuthorizeRes
	for {
		res, err := CallRPC(&GRPC{
			Service:     "PubWorker",
			CacheSecond: 30,
			CallRPC: func(conn *grpc.ClientConn, ctx context.Context) (interface{}, error) {
				appConfig, err := GetGRPCAppConfig(config.Appid)
				if err != nil {
					return nil, err
				}
				if len(appConfig.Appkey) == 0 {
					return nil, utils.Error("rpc appConfig key is nil")
				}
				authObject := &AuthObject{
					Appid: config.Appid,
					Nonce: utils.RandStr(16),
					Time:  utils.TimeSecond(),
				}
				authObject.Signature = utils.HMAC_SHA256(utils.AddStr(authObject.Appid, authObject.Nonce, authObject.Time), appConfig.Appkey, true)
				b64, err := utils.ToJsonBase64(authObject)
				if err != nil {
					return nil, err
				}
				// load public key
				pub, err := pb.NewPubWorkerClient(conn).PublicKey(ctx, &pb.PublicKeyReq{})
				if err != nil {
					return nil, err
				}
				rsaObj := &gorsa.RsaObj{}
				if err := rsaObj.LoadRsaPemFileBase64(pub.PublicKey); err != nil {
					return nil, err
				}
				content, err := rsaObj.Encrypt(utils.Str2Bytes(b64))
				if err != nil {
					return nil, err
				}
				req := &pb.AuthorizeReq{
					Message: content,
				}
				return pb.NewPubWorkerClient(conn).Authorize(ctx, req)
			}})
		if err != nil {
			zlog.Error("rpc login failed", 0, zlog.AddError(err))
			time.Sleep(5 * time.Second)
			continue
		}
		loginRes, _ = res.(*pb.AuthorizeRes)
		break
	}
	accessToken = loginRes.Token
	go renewClientToken(config, loginRes.Expired)
}

func renewClientToken(config ClientConfig, expired int64) {
	for {
		//zlog.Warn("detecting rpc token expiration", 0, zlog.Int64("countDown", expired-utils.TimeSecond()-timeDifference))
		if expired-utils.TimeSecond() > timeDifference { // TODO token过期时间大于2400s则忽略,每15s检测一次
			time.Sleep(15 * time.Second)
			continue
		}
		RunClient(config)
		zlog.Info("rpc token renewal succeeded", 0)
		return
	}
}

func CallRPC(object *GRPC) (interface{}, error) {
	if len(object.Service) == 0 || len(object.Service) > 100 {
		return nil, utils.Error("call service invalid")
	}
	if object.TimeoutMill <= 0 {
		object.TimeoutMill = 60000
	}
	var tag string
	if len(object.Tags) > 0 {
		tag = object.Tags[0]
	}
	c, err := consul.NewConsul(object.Ds)
	if err != nil {
		return nil, err
	}
	services, err := c.GetHealthService(object.Service, tag, object.CacheSecond)
	if err != nil {
		return nil, utils.Error("query service [", object.Service, "] failed: ", err)
	}
	var service *consulapi.AgentService
	if selectionCall == nil { // 选取规则为空则默认随机
		r := rand.New(rand.NewSource(utils.GetSnowFlakeIntID()))
		service = services[r.Intn(len(services))].Service
	} else if len(services) == 1 {
		service = services[0].Service
	} else {
		service = selectionCall(services, object).Service
	}
	clientPool, b := clientPools[utils.AddStr(service.Address, ":", service.Port)]
	if !b || clientPool == nil {
		return nil, utils.Error("client pool is nil: ", utils.AddStr(service.Address, ":", service.Port))
	}
	conn, err := clientPool.Get()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(object.TimeoutMill)*time.Millisecond)
	defer cancel()
	res, err := object.CallRPC(conn.Value(), ctx)
	if err := conn.Close(); err != nil {
		zlog.Error("rpc connection close failed", 0, zlog.AddError(err))
	}
	return res, err
}
