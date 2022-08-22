package consul

import (
	"fmt"
	"github.com/godaddy-x/freego/component/log"
	"github.com/godaddy-x/freego/util"
	consulapi "github.com/hashicorp/consul/api"
	"go.uber.org/zap"
	"net/http"
)

const (
	defaultHost = "consulx.com:8500"
	defaultNode = "dc/consul"
)

var (
	consulSessions = make(map[string]*ConsulManager, 0)
	consulSlowlog  *zap.Logger
)

type ConsulManager struct {
	Host    string
	Token   string
	Consulx *consulapi.Client
	Config  *ConsulConfig
}

// Consulx配置参数
type ConsulConfig struct {
	DsName       string // 数据源名
	Node         string // 配置数据节点, /dc/consul
	Host         string // consul host
	CheckPort    int    // 健康监测端口
	RpcPort      int    // RPC调用端口
	Protocol     string // RPC协议, tcp
	Timeout      string // 请求超时时间, 3s
	Interval     string // 健康监测时间, 5s
	DestroyAfter string // 销毁服务时间, 600s
	CheckPath    string // 健康检测path /xxx/check
	SlowQuery    int64  // 0.不开启筛选 >0开启筛选查询 毫秒
	SlowLogPath  string // 慢查询写入地址
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

func (self *ConsulManager) InitConfig(input ...ConsulConfig) (*ConsulManager, error) {
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

func (self *ConsulManager) GetSlowLog() *zap.Logger {
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

func (self *ConsulManager) CheckService(services []*consulapi.AgentService, srvName, addr string) bool {
	for _, v := range services {
		if v.Service == srvName && v.Address == addr {
			return true
		}
	}
	return false
}

// 接口服务健康检查
func (self *ConsulManager) HealthCheck(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "consulCheck")
}
