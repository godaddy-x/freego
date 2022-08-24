package main

import (
	"fmt"
	"github.com/godaddy-x/freego/cache"
	"github.com/godaddy-x/freego/component/consul"
	"github.com/godaddy-x/freego/component/consul/grpcx"
	"github.com/godaddy-x/freego/component/consul/grpcx/pb"
	rate "github.com/godaddy-x/freego/component/limiter"
	"github.com/godaddy-x/freego/node/test"
	"github.com/godaddy-x/freego/util"
)

func http_test() {
	http_web.StartHttpNode()
}

func initConsul() {
	new(consul.ConsulManager).InitConfig(consul.ConsulConfig{
		Host: "consulx.com:8500",
		Node: "dc/consul",
	})
}

func initRedis() {
	conf := cache.RedisConfig{}
	if err := util.ReadLocalJsonConfig("resource/redis.json", &conf); err != nil {
		panic(util.AddStr("读取redis配置失败: ", err.Error()))
	}
	new(cache.RedisManager).InitConfig(conf)
}

var APPID = util.MD5("123456")
var APPKEY = util.MD5("123456")

func initGRPC() {
	client := &grpcx.GRPCManager{}
	client.CreateJwtConfig(APPKEY)
	client.CreateUnauthorizedUrl("/pub_worker.PubWorker/RPCLogin")
	client.CreateAppConfigCall(func(appid string) (grpcx.AppConfig, error) {
		if appid == APPKEY {
			return grpcx.AppConfig{Appid: APPID, Appkey: APPKEY}, nil
		}
		return grpcx.AppConfig{}, util.Error("appid invalid")
	})
	client.CreateRateLimiterCall(func(method string) (rate.Option, error) {
		return rate.Option{}, nil
	})
	client.CreateServerTLS(grpcx.TlsConfig{
		UseMTLS:   true,
		CACrtFile: "./component/consul/grpcx/cert/ca.crt",
		KeyFile:   "./component/consul/grpcx/cert/server.key",
		CrtFile:   "./component/consul/grpcx/cert/server.crt",
	})
	client.CreateClientTLS(grpcx.TlsConfig{
		UseMTLS:   true,
		CACrtFile: "./component/consul/grpcx/cert/ca.crt",
		KeyFile:   "./component/consul/grpcx/cert/client.key",
		CrtFile:   "./component/consul/grpcx/cert/client.crt",
		HostName:  "localhost",
	})
}

func initClientTokenAuth() {
	new(grpcx.GRPCManager).CreateTokenAuth(APPID, func(res *pb.RPCLoginRes) error {
		fmt.Println("rpc token:  ", res.Token)
		http_web.RPC_TOKEN = res.Token
		return nil
	})
}

func init() {
	initConsul()
	initRedis()
	initGRPC()
}

func main() {
	initClientTokenAuth()
	http_test()
}
