package consul

import (
	"context"
	"fmt"
	rate "github.com/godaddy-x/freego/component/limiter"
	"github.com/godaddy-x/freego/component/log"
	"github.com/godaddy-x/freego/util"
	consulapi "github.com/hashicorp/consul/api"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"math/rand"
	"net"
	"net/http"
	"time"
)

const (
	limiterKey       = "grpc:limiter:"
	limiterConfigKey = "grpc:limiter:config:"
	defaultHost      = "consulx.com:8500"
	defaultNode      = "dc/consul"
)

var (
	consulSessions = make(map[string]*ConsulManager, 0)
	consulSlowlog  *zap.Logger
)

type ConsulManager struct {
	Host    string
	Consulx *consulapi.Client
	Config  *ConsulConfig
	Option  *ConsulOption
}

// Consulx配置参数
type ConsulConfig struct {
	DsName       string // 数据源名
	Node         string // 配置数据节点, /dc/consul
	Host         string // consul host
	Domain       string // 自定义访问域名,为空时自动填充内网IP
	CheckPort    int    // 健康监测端口
	RpcPort      int    // RPC调用端口
	ListenPort   int    // 客户端监听端口
	Protocol     string // RPC协议, tcp
	Timeout      string // 请求超时时间, 3s
	Interval     string // 健康监测时间, 5s
	DestroyAfter string // 销毁服务时间, 600s
	CheckPath    string // 健康检测path /xxx/check
	SlowQuery    int64  // 0.不开启筛选 >0开启筛选查询 毫秒
	SlowLogPath  string // 慢查询写入地址
}

// RPC日志
type MonitorLog struct {
	ConsulHost  string
	RpcHost     string
	RpcPort     int
	Protocol    string
	AgentID     string
	ServiceName string
	MethodName  string
	BeginTime   int64
	CostTime    int64
	Error       error
}

// RPC参数对象
type CallInfo struct {
	Sub           int64       // 用户主体ID
	Tags          []string    // 服务标签名称
	Domain        string      // 自定义访问域名,为空时自动填充内网IP
	ClassInstance interface{} // 接口实现类实例
	Package       string      // RPC服务包名
	Service       string      // RPC服务名称
	Method        string      // RPC方法名称
	Protocol      string      // RPC访问协议,默认TCP
	Request       interface{} // 请求参数对象
	Response      interface{} // 响应参数对象
	Timeout       int64       // 连接请求超时,默认15秒
	Option        rate.Option // 限流配置
}

type GRPC struct {
	Tags    []string                                                              // 服务标签名称
	Address string                                                                // 服务地址,为空时自动填充内网IP
	Service string                                                                // 服务名称
	Timeout int                                                                   // 请求超时/毫秒
	AddRPC  func(server *grpc.Server)                                             // grpc注册proto服务
	CallRPC func(conn *grpc.ClientConn, ctx context.Context) (interface{}, error) // grpc回调proto服务
}

func getConsulClient(conf ConsulConfig) *ConsulManager {
	config := consulapi.DefaultConfig()
	config.Address = conf.Host
	client, err := consulapi.NewClient(config)
	if err != nil {
		panic(util.AddStr("consul [", conf.Host, "] init failed: ", err))
	}
	return &ConsulManager{Consulx: client, Host: conf.Host}
}

type ConsulOption struct {
	// TODO 服务选取算法实现
	Selection func([]*consulapi.ServiceEntry, *GRPC) *consulapi.ServiceEntry
	// TODO 限流算法实现
	LockerFunc   func(callInfo *CallInfo) error
	UnlockerFunc func(callInfo *CallInfo) error
}

func (self *ConsulManager) InitConfig(option *ConsulOption, input ...ConsulConfig) (*ConsulManager, error) {
	for _, conf := range input {
		if len(conf.Host) == 0 {
			conf.Host = defaultHost
		}
		if len(conf.Node) == 0 {
			conf.Node = defaultNode
		}
		localmgr := getConsulClient(conf)
		config := ConsulConfig{}
		if err := localmgr.GetJsonValue(conf.Node, &config, false); err != nil {
			panic(err)
		}
		config.Node = conf.Node
		onlinemgr := getConsulClient(config)
		onlinemgr.Config = &config
		if len(config.DsName) == 0 {
			consulSessions[conf.Node] = onlinemgr
		} else {
			consulSessions[config.DsName] = onlinemgr
		}
		if option == nil {
			onlinemgr.Option = &ConsulOption{}
		} else {
			onlinemgr.Option = option
		}
		onlinemgr.initSlowLog()
		log.Printf("consul service %s【%s】has been started successfully", conf.Host, conf.Node)
	}
	if len(consulSessions) == 0 {
		log.Printf("consul init failed: sessions is nil")
	}
	return self, nil
}

func (self *ConsulManager) initSlowLog() {
	if self.Config.SlowQuery == 0 || len(self.Config.SlowLogPath) == 0 {
		return
	}
	if consulSlowlog == nil {
		consulSlowlog = log.InitNewLog(&log.ZapConfig{
			Level:   "warn",
			Console: false,
			FileConfig: &log.FileConfig{
				Compress:   true,
				Filename:   self.Config.SlowLogPath,
				MaxAge:     7,
				MaxBackups: 7,
				MaxSize:    512,
			}})
		fmt.Println("consul monitoring service started successfully...")
	}
}

func (self *ConsulManager) getSlowLog() *zap.Logger {
	return consulSlowlog
}

func (self *ConsulManager) Client(dsname ...string) (*ConsulManager, error) {
	var ds string
	if len(dsname) > 0 && len(dsname[0]) > 0 {
		ds = dsname[0]
	} else {
		ds = defaultNode
	}
	manager := consulSessions[ds]
	if manager == nil {
		return nil, util.Error("consul session [", ds, "] not found...")
	}
	return manager, nil
}

// 通过Consul中心获取指定JSON配置数据
func (self *ConsulManager) GetJsonValue(key string, result interface{}, isEncrypt bool) error {
	client := self.Consulx
	kv := client.KV()
	if kv == nil {
		return util.Error("consul node [", key, "] not found...")
	}
	k, _, err := kv.Get(key, nil)
	if err != nil {
		return util.Error("consul node [", key, "] read failed...")
	}
	if k == nil || k.Value == nil || len(k.Value) == 0 {
		return util.Error("consul node [", key, "] read is nil...")
	}
	if err := util.JsonUnmarshal(k.Value, result); err != nil {
		return util.Error("consul node [", key, "] parse failed...")
	}
	return nil
}

func (self *ConsulManager) GetTextValue(key string) ([]byte, error) {
	client := self.Consulx
	kv := client.KV()
	if kv == nil {
		return nil, util.Error("consul node [", key, "] not found...")
	}
	k, _, err := kv.Get(key, nil)
	if err != nil {
		return nil, util.Error("consul node [", key, "] read failed...")
	}
	if k == nil || k.Value == nil || len(k.Value) == 0 {
		return nil, util.Error("consul node [", key, "] read is nil...")
	}
	return k.Value, nil
}

func (self *ConsulManager) RemoveService(serviceIDs ...string) {
	services, err := self.Consulx.Agent().Services()
	if err != nil {
		panic(err)
	}
	if len(serviceIDs) > 0 {
		for _, v := range services {
			for _, ID := range serviceIDs {
				if ID == v.ID {
					if err := self.Consulx.Agent().ServiceDeregister(v.ID); err != nil {
						log.Println(err)
					}
					log.Println("remove grpc service successful: ", v.Service, " - ", v.ID)
				}
			}
		}
		return
	}
	for _, v := range services {
		if err := self.Consulx.Agent().ServiceDeregister(v.ID); err != nil {
			log.Println(err)
		}
		log.Println("remove grpc service successful: ", v.Service, " - ", v.ID)
	}
}

// 根据服务名获取可用列表
func (self *ConsulManager) GetAllService(service string) ([]*consulapi.AgentService, error) {
	result := make([]*consulapi.AgentService, 0)
	services, err := self.Consulx.Agent().Services()
	if err != nil {
		return result, err
	}
	if len(service) == 0 {
		for _, v := range services {
			result = append(result, v)
		}
		return result, nil
	}
	for _, v := range services {
		if service == v.Service {
			result = append(result, v)
		}
	}
	return result, nil
}

func (self *ConsulManager) GetHealthService(service, tag string) ([]*consulapi.ServiceEntry, error) {
	serviceEntry, _, err := self.Consulx.Health().Service(service, tag, false, &consulapi.QueryOptions{})
	if err != nil {
		return []*consulapi.ServiceEntry{}, err
	}
	return serviceEntry, nil
}

func checkServiceExists(services []*consulapi.AgentService, srvName, addr string) bool {
	for _, v := range services {
		if v.Service == srvName && v.Address == addr {
			return true
		}
	}
	return false
}

// 中心注册接口服务
func (self *ConsulManager) RunGRPC(objects ...*GRPC) {
	if len(objects) == 0 {
		panic("rpc objects is nil...")
	}
	services, err := self.GetAllService("")
	if err != nil {
		panic(err)
	}
	if err != nil {
		panic(err)
	}
	grpcServer := grpc.NewServer()
	for _, object := range objects {
		address := util.GetLocalIP()
		port := self.Config.RpcPort
		if len(address) == 0 {
			panic("local address reading failed")
		}
		if len(object.Address) > 0 {
			address = object.Address
		}
		if len(object.Service) == 0 || len(object.Service) > 100 {
			panic("rpc service invalid")
		}
		if checkServiceExists(services, object.Service, address) {
			log.Println(util.AddStr("service [", object.Service, "][", address, "] exist, skip..."))
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
		registration.Check = &consulapi.AgentServiceCheck{HTTP: fmt.Sprintf("http://%s:%d%s", registration.Address, self.Config.CheckPort, self.Config.CheckPath), Timeout: self.Config.Timeout, Interval: self.Config.Interval, DeregisterCriticalServiceAfter: self.Config.DestroyAfter}
		log.Println(util.AddStr("service [", registration.Name, "][", registration.Address, "] added successful"))
		if err := self.Consulx.Agent().ServiceRegister(registration); err != nil {
			panic(util.AddStr("service [", object.Service, "] add failed: ", err.Error()))
		}
		object.AddRPC(grpcServer)
	}
	go func() {
		http.HandleFunc(self.Config.CheckPath, self.healthCheck)
		http.ListenAndServe(fmt.Sprintf(":%d", self.Config.CheckPort), nil)
	}()
	l, err := net.Listen(self.Config.Protocol, util.AddStr(":", util.AnyToStr(self.Config.RpcPort)))
	if err != nil {
		panic(err)
	}
	if err := grpcServer.Serve(l); err != nil {
		panic(err)
	}
}

// 获取RPC服务,并执行访问 args参数不可变,reply参数可变
func (self *ConsulManager) CallGRPC(object *GRPC) (interface{}, error) {
	if len(object.Service) == 0 || len(object.Service) > 100 {
		return nil, util.Error("call service invalid")
	}
	if object.Timeout <= 0 {
		object.Timeout = 15000
	}
	var tag string
	if len(object.Tags) > 0 {
		tag = object.Tags[0]
	}
	services, err := self.GetHealthService(object.Service, tag)
	if err != nil {
		return nil, util.Error("query service [", object.Service, "] failed: ", err)
	}
	if len(services) == 0 {
		return nil, util.Error("no available services found: [", object.Service, "]")
	}
	var service *consulapi.AgentService
	if self.Option.Selection == nil { // 选取规则为空则默认随机
		r := rand.New(rand.NewSource(util.GetSnowFlakeIntID()))
		service = services[r.Intn(len(services))].Service
	} else {
		service = self.Option.Selection(services, object).Service
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(object.Timeout)*time.Millisecond)
	defer cancel()
	conn, err := grpc.DialContext(ctx, util.AddStr(service.Address, ":", service.Port), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	return object.CallRPC(conn, ctx)
}

// 输出RPC监控日志
func (self *ConsulManager) rpcMonitor(monitor MonitorLog, err error, args interface{}, reply interface{}) error {
	monitor.CostTime = util.Time() - monitor.BeginTime
	if err != nil {
		monitor.Error = err
		log.Println(util.JsonMarshal(monitor))
		return nil
	}
	if self.Config.SlowQuery > 0 && monitor.CostTime > self.Config.SlowQuery {
		l := self.getSlowLog()
		if l != nil {
			l.Warn("consul monitor", log.Int64("cost", monitor.CostTime), log.Any("service", monitor), log.Any("request", args), log.Any("response", reply))
		}
	}
	return err
}

// 接口服务健康检查
func (self *ConsulManager) healthCheck(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "consulCheck")
}
