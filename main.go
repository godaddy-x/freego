package main

import (
	"github.com/godaddy-x/freego/cache"
	"github.com/godaddy-x/freego/component/consul"
	"github.com/godaddy-x/freego/component/consul/grpcx"
	"github.com/godaddy-x/freego/component/limiter"
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
	client := grpcx.NewClient()
	client.CreateJwtConfig("123456")
	client.CreateUnauthorizedUrl("/pub_worker.PubWorker/RPCLogin")
	client.CreateAppConfigCall(func(appid string) (grpcx.AppConfig, error) {
		return grpcx.AppConfig{Appid: "123456", Appkey: "123456"}, nil
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

func init() {
	initConsul()
	conf := cache.RedisConfig{}
	if err := util.ReadLocalJsonConfig("resource/redis.json", &conf); err != nil {
		panic(util.AddStr("读取redis配置失败: ", err.Error()))
	}
	new(cache.RedisManager).InitConfig(conf)
}

func main() {
	http_test()
}
