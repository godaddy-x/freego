package grpcx

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"github.com/godaddy-x/freego/component/consul"
	"github.com/godaddy-x/freego/component/jwt"
	rate "github.com/godaddy-x/freego/component/limiter"
	"github.com/godaddy-x/freego/component/log"
	"github.com/godaddy-x/freego/util"
	consulapi "github.com/hashicorp/consul/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"time"
)

const (
	limiterKey = "grpc:limiter:"
)

var (
	serverDialTLS   grpc.ServerOption
	clientDialTLS   grpc.DialOption
	jwtConfig       jwt.JwtConfig
	unauthorizedUrl []string
	rateLimiterCall func(string) (rate.Option, error)
	selectionCall   func([]*consulapi.ServiceEntry, *GRPC) *consulapi.ServiceEntry
	appConfigCall   func(string) (AppConfig, error)
)

type GRPCManager struct {
	consul    *consul.ConsulManager
	token     string
	ConsulDs  string
	Authentic bool
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
	Ds      string                                                                // consul数据源ds
	Token   string                                                                // 授权token
	Tags    []string                                                              // 服务标签名称
	Address string                                                                // 服务地址,为空时自动填充内网IP
	Service string                                                                // 服务名称
	Timeout int                                                                   // 请求超时/毫秒
	AddRPC  func(server *grpc.Server)                                             // grpc注册proto服务
	CallRPC func(conn *grpc.ClientConn, ctx context.Context) (interface{}, error) // grpc回调proto服务
}

func GetGRPCJwtConfig() (jwt.JwtConfig, error) {
	if len(jwtConfig.TokenKey) == 0 {
		return jwt.JwtConfig{}, util.Error("grpc jwt key is nil")
	}
	return jwt.JwtConfig{
		TokenTyp: jwtConfig.TokenTyp,
		TokenAlg: jwtConfig.TokenAlg,
		TokenKey: jwtConfig.TokenKey,
		TokenExp: jwtConfig.TokenExp,
	}, nil
}

func GetGRPCAppConfig(appid string) (AppConfig, error) {
	if appConfigCall == nil {
		return AppConfig{}, util.Error("grpc app config call is nil")
	}
	return appConfigCall(appid)
}

func (self *GRPCManager) CreateUnauthorizedUrl(url ...string) {
	if len(unauthorizedUrl) > 0 {
		return
	}
	unauthorizedUrl = url
}

func (self *GRPCManager) CreateJwtConfig(tokenKey string, tokenExp int64) {
	if len(jwtConfig.TokenKey) > 0 {
		return
	}
	jwtConfig = jwt.JwtConfig{
		TokenTyp: jwt.JWT,
		TokenAlg: jwt.HS256,
		TokenKey: tokenKey,
		TokenExp: tokenExp,
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

func (self *GRPCManager) RunServer(objects ...*GRPC) {
	if len(objects) == 0 {
		panic("rpc objects is nil...")
	}
	if self.consul == nil {
		consul, err := new(consul.ConsulManager).Client(self.ConsulDs)
		if err != nil {
			panic(err)
		}
		self.consul = consul
	}
	services, err := self.consul.GetAllService("")
	if err != nil {
		panic(err)
	}
	if err != nil {
		panic(err)
	}
	opts := []grpc.ServerOption{
		grpc.UnaryInterceptor(self.ServerInterceptor),
	}
	if serverDialTLS != nil {
		opts = append(opts, serverDialTLS)
	}
	grpcServer := grpc.NewServer(opts...)
	for _, object := range objects {
		address := util.GetLocalIP()
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
			log.Println(util.AddStr("grpc service [", object.Service, "][", address, "] exist, skip..."))
			object.AddRPC(grpcServer)
			continue
		}
		registration := new(consulapi.AgentServiceRegistration)
		registration.ID = util.GetUUID()
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
		log.Println(util.AddStr("grpc service [", registration.Name, "][", registration.Address, "] added successful"))
		if err := self.consul.Consulx.Agent().ServiceRegister(registration); err != nil {
			panic(util.AddStr("grpc service [", object.Service, "] add failed: ", err.Error()))
		}
		object.AddRPC(grpcServer)
	}
	go func() {
		http.HandleFunc(self.consul.Config.CheckPath, self.consul.HealthCheck)
		http.ListenAndServe(fmt.Sprintf(":%d", self.consul.Config.CheckPort), nil)
	}()
	l, err := net.Listen(self.consul.Config.Protocol, util.AddStr(":", util.AnyToStr(self.consul.Config.RpcPort)))
	if err != nil {
		panic(err)
	}
	log.Println(util.AddStr("grpc server【", util.AddStr(":", util.AnyToStr(self.consul.Config.RpcPort)), "】has been started successfully"))
	if err := grpcServer.Serve(l); err != nil {
		panic(err)
	}
}

func CallRPC(object *GRPC) (interface{}, error) {
	if len(object.Service) == 0 || len(object.Service) > 100 {
		return nil, util.Error("call service invalid")
	}
	if object.Timeout <= 0 {
		object.Timeout = 60000
	}
	var tag string
	if len(object.Tags) > 0 {
		tag = object.Tags[0]
	}
	consul, err := new(consul.ConsulManager).Client(object.Ds)
	if err != nil {
		return nil, err
	}
	services, err := consul.GetHealthService(object.Service, tag)
	if err != nil {
		return nil, util.Error("query service [", object.Service, "] failed: ", err)
	}
	if len(services) == 0 {
		return nil, util.Error("no available services found: [", object.Service, "]")
	}
	var service *consulapi.AgentService
	if selectionCall == nil { // 选取规则为空则默认随机
		r := rand.New(rand.NewSource(util.GetSnowFlakeIntID()))
		service = services[r.Intn(len(services))].Service
	} else {
		service = selectionCall(services, object).Service
	}
	client := &GRPCManager{consul: consul, token: object.Token, ConsulDs: object.Ds}
	opts := []grpc.DialOption{
		grpc.WithUnaryInterceptor(client.ClientInterceptor),
	}
	if clientDialTLS != nil {
		opts = append(opts, clientDialTLS)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(object.Timeout)*time.Millisecond)
	defer cancel()
	conn, err := grpc.DialContext(ctx, util.AddStr(service.Address, ":", service.Port), opts...)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	return object.CallRPC(conn, ctx)
}
