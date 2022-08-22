package main

import (
	"github.com/godaddy-x/freego/cache"
	"github.com/godaddy-x/freego/component/consul"
	"github.com/godaddy-x/freego/component/limiter"
	"github.com/godaddy-x/freego/node/test"
	"github.com/godaddy-x/freego/util"
)

func http_test() {
	http_web.StartHttpNode()
}

func initConsul() {
	client, _ := new(consul.ConsulManager).InitConfig(consul.ConsulConfig{
		Host: "consulx.com:8500",
		Node: "dc/consul",
	})
	client.CreateJwtConfig("123456")
	client.CreateUnauthorizedUrl("/pub_worker.PubWorker/RPCLogin")
	client.CreateRateLimiterCall(func(method string) (rate.Option, error) {
		return rate.Option{}, nil
	})
	client.CreateServerTLS(consul.TlsConfig{
		UseTLS:    true,
		CACrtFile: "./component/consul/grpc/cert/ca.crt",
		KeyFile:   "./component/consul/grpc/cert/server.key",
		CrtFile:   "./component/consul/grpc/cert/server.crt",
	})
	client.CreateClientTLS(consul.TlsConfig{
		UseTLS:    true,
		CACrtFile: "./component/consul/grpc/cert/ca.crt",
		KeyFile:   "./component/consul/grpc/cert/client.key",
		CrtFile:   "./component/consul/grpc/cert/client.crt",
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
