package consul

import (
	"bufio"
	"encoding/gob"
	"errors"
	"fmt"
	"github.com/godaddy-x/freego/component/log"
	"github.com/godaddy-x/freego/util"
	consulapi "github.com/hashicorp/consul/api"
	"net"
	"net/http"
	"net/rpc"
	"reflect"
	"strings"
	"time"
)

var (
	DefaultHost     = "consulx.com:8500"
	consul_sessions = make(map[string]*ConsulManager)
)

type ConsulManager struct {
	Host    string
	Consulx *consulapi.Client
	Config  *ConsulConfig
}

// Consulx配置参数
type ConsulConfig struct {
	Node         string
	Host         string
	Domain       string
	CheckPort    int
	RpcPort      int
	ListenProt   int
	Protocol     string
	Logger       string
	Timeout      string
	Interval     string
	DestroyAfter string
}

// RPC日志
type MonitorLog struct {
	ConsulHost  string
	RpcHost     string
	RpcPort     int
	Protocol    string
	ServiceName string
	MethodName  string
	BeginTime   int64
	CostTime    int64
	Errors      []string
}

func (self *ConsulManager) InitConfig(input ...ConsulConfig) (*ConsulManager, error) {
	for _, v := range input {
		if _, b := consul_sessions[v.Host]; b {
			return nil, util.Error("consul数据源[", v.Host, "]已存在")
		}
		config := consulapi.DefaultConfig()
		config.Address = v.Host
		client, err := consulapi.NewClient(config)
		if err != nil {
			return nil, util.Error("连接", v.Host, "consul配置中心失败: ", err)

		}
		manager := &ConsulManager{Consulx: client, Host: v.Host}
		data, err := manager.GetKV(v.Node, client)
		if err != nil {
			return nil, util.Error("读取节点[", v.Node, "]数据失败", err)
		}
		result := &ConsulConfig{}
		if err := util.ReadJsonConfig(data, result); err != nil {
			return nil, util.Error("解析节点[", v.Node, "]数据失败", err)
		}
		result.Node = v.Node
		manager.Config = result
		consul_sessions[v.Host] = manager
	}
	if len(consul_sessions) == 0 {
		return nil, util.Error("consul连接初始化失败: 数据源为0")
	}
	return self, nil
}

func (self *ConsulManager) Client(dsname ...string) (*ConsulManager, error) {
	var ds string
	if len(dsname) > 0 && len(dsname[0]) > 0 {
		ds = dsname[0]
	} else {
		ds = DefaultHost
	}
	if manager := consul_sessions[ds]; manager == nil {
		return nil, util.Error("consul数据源[", ds, "]未找到,请检查...")
	} else {
		return manager, nil
	}
}

// 通过Consul中心获取指定配置数据
func (self *ConsulManager) GetKV(key string, consulx ...*consulapi.Client) ([]byte, error) {
	client := self.Consulx
	if len(consulx) > 0 {
		client = consulx[0]
	}
	kv := client.KV()
	if kv == nil {
		return nil, util.Error("consul配置[", key, "]没找到")
	}
	k, _, err := kv.Get(key, nil)
	if err != nil {
		return nil, util.Error("consul配置数据[", key, "]读取异常: ", err)
	}
	if k == nil || k.Value == nil || len(k.Value) <= 0 {
		return nil, util.Error("consul配置数据[", key, "]读取为空")
	}
	return k.Value, nil
}

// 读取节点JSON配置
func (self *ConsulManager) ReadJsonConfig(node string, result interface{}) error {
	if data, err := self.GetKV(node); err != nil {
		return err
	} else {
		return util.ReadJsonConfig(data, result)
	}
}

// 中心注册接口服务
func (self *ConsulManager) AddRegistration(name string, iface interface{}) error {
	tof := reflect.TypeOf(iface)
	vof := reflect.ValueOf(iface)
	sname := reflect.Indirect(vof).Type().Name()
	methods := ","
	for m := 0; m < tof.NumMethod(); m++ {
		method := tof.Method(m)
		methods += method.Name + ","
	}
	if len(methods) <= 0 {
		return util.Error("服务对象[", sname, "]没有注册方法")
	}
	registration := new(consulapi.AgentServiceRegistration)
	registration.ID = sname
	registration.Name = sname
	registration.Tags = []string{name}
	ip := util.GetLocalIP()
	if ip == "" {
		return util.Error("[", name, "]服务注册失败,内网IP读取失败")
	}
	registration.Address = ip
	registration.Port = self.Config.RpcPort
	meta := make(map[string]string)
	meta["host"] = ip + ":" + util.AnyToStr(self.Config.ListenProt)
	meta["protocol"] = self.Config.Protocol
	meta["version"] = "1.0.0"
	meta["methods"] = methods
	registration.Meta = meta
	registration.Check = &consulapi.AgentServiceCheck{HTTP: fmt.Sprintf("http://%s:%d%s", registration.Address, self.Config.CheckPort, "/check"), Timeout: self.Config.Timeout, Interval: self.Config.Interval, DeregisterCriticalServiceAfter: self.Config.DestroyAfter,}
	// 启动RPC服务
	log.Info(util.AddStr("[", util.GetLocalIP(), "]", " - [", name, "] - 启动成功"), 0)
	if err := self.Consulx.Agent().ServiceRegister(registration); err != nil {
		return util.Error("注册[", name, "]服务失败: ", err)
	}
	rpc.Register(iface)
	return nil
}

// 开启并监听服务
func (self *ConsulManager) StartListenAndServe() {
	//rpc.HandleHTTP()
	//l, e := net.Listen(self.Config.Protocol, util.AddStr(":", util.AnyToStr(self.Config.ListenProt)))
	//if e != nil {
	//	panic("Consul监听服务异常: " + e.Error())
	//}
	//go http.Serve(l, nil)
	//http.HandleFunc("/check", self.healthCheck)
	//http.ListenAndServe(fmt.Sprintf(":%d", self.Config.CheckPort), nil)
	l, e := net.Listen(self.Config.Protocol, util.AddStr(":", util.AnyToStr(self.Config.ListenProt)))
	if e != nil {
		panic("Consul监听服务异常: " + e.Error())
	}
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				log.Error("accept rpc connection error", 0, log.AddError(err))
				continue
			}
			go func(conn net.Conn) {
				buf := bufio.NewWriter(conn)
				srv := &gobServerCodec{
					rwc:    conn,
					dec:    gob.NewDecoder(conn),
					enc:    gob.NewEncoder(buf),
					encBuf: buf,
				}
				err = rpc.ServeRequest(srv)
				if err != nil {
					log.Error("server rpc request error", 0, log.AddError(err))
				}
				srv.Close()
			}(conn)
		}
	}()
	http.HandleFunc("/check", self.healthCheck)
	http.ListenAndServe(fmt.Sprintf(":%d", self.Config.CheckPort), nil)
}

// 获取RPC服务,并执行访问 args参数不可变,reply参数可变
func (self *ConsulManager) CallService(srv string, args interface{}, reply interface{}) error {
	if srv == "" {
		return errors.New("类名+方法名不能为空")
	}
	sarr := strings.Split(srv, ".")
	if len(sarr) != 2 || len(sarr[0]) <= 0 || len(sarr[1]) <= 0 {
		return errors.New("类名+方法名格式有误")
	}
	monitor := MonitorLog{
		ConsulHost:  self.Config.Host,
		ServiceName: sarr[0],
		MethodName:  sarr[1],
		BeginTime:   util.Time(),
	}
	services, _ := self.Consulx.Agent().Services()
	if _, found := services[sarr[0]]; !found {
		return errors.New(util.AddStr("[", util.GetLocalIP(), "]", "[", sarr[0], "]无可用服务"))
	}
	meta := services[sarr[0]].Meta
	if len(meta) < 4 {
		return errors.New(util.AddStr("[", util.GetLocalIP(), "]", "[", sarr[0], "]服务参数异常,请检查..."))
	}
	methods := meta["methods"]
	s := "," + sarr[1] + ","
	if !util.HasStr(methods, s) {
		return errors.New(util.AddStr("[", util.GetLocalIP(), "]", "[", sarr[0], "][", sarr[1], "]无效,请检查..."))
	}
	host := meta["host"]
	protocol := meta["protocol"]
	if len(host) <= 0 || len(protocol) <= 0 {
		return errors.New("meta参数异常")
	} else {
		s := strings.Split(host, ":")
		port, _ := util.StrToInt(s[1])
		monitor.RpcHost = s[0]
		monitor.RpcPort = port
		monitor.Protocol = protocol
	}
	conn, err := net.DialTimeout(protocol, host, time.Second*10)
	if err != nil {
		return self.rpcMonitor(monitor, util.Error("[", host, "]", "[", srv, "]连接失败: ", err))
	}
	encBuf := bufio.NewWriter(conn)
	codec := &gobClientCodec{conn, gob.NewDecoder(conn), gob.NewEncoder(encBuf), encBuf}
	cli := rpc.NewClientWithCodec(codec)
	err1 := cli.Call(srv, args, reply)
	err2 := cli.Close()
	if err1 != nil && err2 != nil {
		return self.rpcMonitor(monitor, util.Error("[", host, "]", "[", srv, "]访问失败: ", err1, ";", err2))
	}
	if err1 != nil {
		return self.rpcMonitor(monitor, util.Error("[", host, "]", "[", srv, "]访问失败: ", err1))
	} else if err2 != nil {
		return self.rpcMonitor(monitor, util.Error("[", host, "]", "[", srv, "]访问失败: ", err2))
	}
	//client, err := rpc.DialHTTP(protocol, host)
	//if err != nil {
	//	log.Println(util.AddStr("[", host, "]", "[", srv, "]连接失败: ", err.Error()))
	//	return self.rpcMonitor(monitor, errors.New(util.AddStr("[", host, "]", "[", srv, "]连接失败: ", err.Error())))
	//}
	//defer client.Close()
	//if err := client.Call(srv, args, reply); err != nil {
	//	return self.rpcMonitor(monitor, errors.New(util.AddStr("[", host, "]", "[", srv, "]访问失败: ", err.Error())))
	//}
	//call := <-client.Go(srv, args, reply, nil).Done
	//if call.Error != nil {
	//	return self.rpcMonitor(monitor, errors.New(util.AddStr("[", host, "]", "[", srv, "]访问失败: ", call.Error.Error())))
	//}
	return self.rpcMonitor(monitor, nil)
}

// 输出RPC监控日志
func (self *ConsulManager) rpcMonitor(monitor MonitorLog, err error) error {
	monitor.CostTime = util.Time() - monitor.BeginTime
	if err != nil {
		monitor.Errors = []string{err.Error()}
		log.Error("RPC错误", 0, log.AddError(err))
	}
	if self.Config.Logger == "local" {
		log.Error("RPC监控", 0, log.Any("monitor", monitor))
	} else if self.Config.Logger == "amqp" {
	}
	return err
}

// 接口服务健康检查
func (self *ConsulManager) healthCheck(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "consulCheck")
}
