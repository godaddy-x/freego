package consul

import (
	"fmt"
	"github.com/godaddy-x/freego/cache"
	DIC "github.com/godaddy-x/freego/common"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/zlog"
	consulapi "github.com/hashicorp/consul/api"
	"go.uber.org/zap"
	"net/http"
	"time"
)

var (
	consulSessions = make(map[string]*ConsulManager, 0)
	consulSlowlog  *zap.Logger
	queryOptions   = &consulapi.QueryOptions{
		UseCache:     true,
		MaxAge:       7200 * time.Second,
		StaleIfError: 14400 * time.Second,
	}
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
		panic(utils.AddStr("consul [", conf.Host, "] init failed: ", err))
	}
	return &ConsulManager{Consulx: client, Host: conf.Host}
}

func (self *ConsulManager) InitConfig(input ...ConsulConfig) (*ConsulManager, error) {
	for _, conf := range input {
		if len(conf.Host) == 0 {
			panic("consul host is nil")
		}
		if len(conf.DsName) == 0 {
			conf.DsName = DIC.MASTER
		}
		manager := getConsulClient(conf)
		manager.Config = &conf
		consulSessions[manager.Config.DsName] = manager
		manager.initSlowLog()
		zlog.Printf("consul service %s【%s】has been started successful", conf.Host, conf.DsName)
	}
	if len(consulSessions) == 0 {
		zlog.Printf("consul init failed: sessions is nil")
	}
	return self, nil
}

func NewConsul(ds ...string) (*ConsulManager, error) {
	return new(ConsulManager).Client(ds...)
}

func (self *ConsulManager) initSlowLog() {
	if self.Config.SlowQuery == 0 || len(self.Config.SlowLogPath) == 0 {
		return
	}
	if consulSlowlog == nil {
		consulSlowlog = zlog.InitNewLog(&zlog.ZapConfig{
			Level:   "warn",
			Console: false,
			FileConfig: &zlog.FileConfig{
				Compress:   true,
				Filename:   self.Config.SlowLogPath,
				MaxAge:     7,
				MaxBackups: 7,
				MaxSize:    512,
			}})
		fmt.Println("consul monitoring service started successful...")
	}
}

func (self *ConsulManager) GetSlowLog() *zap.Logger {
	return consulSlowlog
}

func (self *ConsulManager) Client(ds ...string) (*ConsulManager, error) {
	dsName := DIC.MASTER
	if len(ds) > 0 && len(ds[0]) > 0 {
		dsName = ds[0]
	}
	manager := consulSessions[dsName]
	if manager == nil {
		return nil, utils.Error("consul session [", ds, "] not found...")
	}
	return manager, nil
}

// 通过Consul中心获取指定JSON配置数据
func (self *ConsulManager) GetJsonValue(key string, result interface{}) error {
	client := self.Consulx
	kv := client.KV()
	if kv == nil {
		return utils.Error("consul node [", key, "] not found...")
	}
	k, _, err := kv.Get(key, nil)
	if err != nil {
		return utils.Error("consul node [", key, "] read failed...")
	}
	if k == nil || k.Value == nil || len(k.Value) == 0 {
		return utils.Error("consul node [", key, "] read is nil...")
	}
	if err := utils.JsonUnmarshal(k.Value, result); err != nil {
		return utils.Error("consul node [", key, "] parse failed...")
	}
	return nil
}

func (self *ConsulManager) GetTextValue(key string) ([]byte, error) {
	client := self.Consulx
	kv := client.KV()
	if kv == nil {
		return nil, utils.Error("consul node [", key, "] not found...")
	}
	k, _, err := kv.Get(key, nil)
	if err != nil {
		return nil, utils.Error("consul node [", key, "] read failed...")
	}
	if k == nil || k.Value == nil || len(k.Value) == 0 {
		return nil, utils.Error("consul node [", key, "] read is nil...")
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
						zlog.Println(err)
					}
					zlog.Println("remove grpc service successful: ", v.Service, " - ", v.ID)
				}
			}
		}
		return
	}
	for _, v := range services {
		if err := self.Consulx.Agent().ServiceDeregister(v.ID); err != nil {
			zlog.Println(err)
		}
		zlog.Println("remove grpc service successful: ", v.Service, " - ", v.ID)
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

var localCache = cache.NewLocalCache(30, 5)

func (self *ConsulManager) GetHealthService(service, tag string, cacheSecond int) ([]*consulapi.ServiceEntry, error) {
	if cacheSecond > 0 {
		obj, has, err := localCache.Get("consul.grpc."+service, nil)
		if err != nil {
			return nil, err
		}
		if has && obj != nil {
			return obj.([]*consulapi.ServiceEntry), nil
		}
	}
	serviceEntry, _, err := self.Consulx.Health().Service(service, tag, false, queryOptions)
	if err != nil {
		return nil, err
	}
	if len(serviceEntry) == 0 {
		return nil, utils.Error("no available services found: [", service, "]")
	}
	if cacheSecond > 0 {
		if err := localCache.Put("consul.grpc."+service, serviceEntry, cacheSecond); err != nil {
			return nil, err
		}
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
	if _, err := fmt.Fprintln(w, "consulCheck"); err != nil {
		zlog.Error("consul check output failed", 0, zlog.AddError(err))
	}
}
