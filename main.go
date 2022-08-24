package main

import (
	"github.com/godaddy-x/freego/cache"
	"github.com/godaddy-x/freego/cache/limiter"
	"github.com/godaddy-x/freego/consul"
	"github.com/godaddy-x/freego/consul/grpcx"
	"github.com/godaddy-x/freego/node/test"
	"github.com/godaddy-x/freego/utils"
)

func http_test() {
	http_web.StartHttpNode()
}

func initConsul() {
	conf := consul.ConsulConfig{}
	if err := utils.ReadLocalJsonConfig("resource/consul.json", &conf); err != nil {
		panic(utils.AddStr("读取consul配置失败: ", err.Error()))
	}
	new(consul.ConsulManager).InitConfig(conf)
}

func initRedis() {
	conf := cache.RedisConfig{}
	if err := utils.ReadLocalJsonConfig("resource/redis.json", &conf); err != nil {
		panic(utils.AddStr("读取redis配置失败: ", err.Error()))
	}
	new(cache.RedisManager).InitConfig(conf)
}

var APPID = utils.MD5("123456")
var APPKEY = utils.MD5("123456")

func initGRPC() {
	client := &grpcx.GRPCManager{}
	client.CreateJwtConfig(APPKEY)
	client.CreateAppConfigCall(func(appid string) (grpcx.AppConfig, error) {
		if appid == APPKEY {
			return grpcx.AppConfig{Appid: APPID, Appkey: APPKEY}, nil
		}
		return grpcx.AppConfig{}, utils.Error("appid invalid")
	})
	client.CreateRateLimiterCall(func(method string) (rate.Option, error) {
		return rate.Option{}, nil
	})
	client.CreateServerTLS(grpcx.TlsConfig{
		UseMTLS:   true,
		CACrtFile: "./consul/grpcx/cert/ca.crt",
		KeyFile:   "./consul/grpcx/cert/server.key",
		CrtFile:   "./consul/grpcx/cert/server.crt",
	})
	client.CreateClientTLS(grpcx.TlsConfig{
		UseMTLS:   true,
		CACrtFile: "./consul/grpcx/cert/ca.crt",
		KeyFile:   "./consul/grpcx/cert/client.key",
		CrtFile:   "./consul/grpcx/cert/client.crt",
		HostName:  "localhost",
	})
}

func init() {
	initConsul()
	initRedis()
	initGRPC()
}

func main() {
	grpcx.RunClient(APPID)
	http_test()
}
