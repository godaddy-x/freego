package main

import (
	"github.com/godaddy-x/freego/cache"
	"github.com/godaddy-x/freego/cache/limiter"
	"github.com/godaddy-x/freego/consul"
	"github.com/godaddy-x/freego/consul/grpcx"
	"github.com/godaddy-x/freego/node/test"
	"github.com/godaddy-x/freego/utils"
	"net/http"
	_ "net/http/pprof"
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
	client.CreateAuthorizeTLS("./consul/grpcx/cert/server.key")
}

func init() {
	initConsul()
	initRedis()
	initGRPC()
}

func main() {
	go func() {
		_ = http.ListenAndServe(":8849", nil)
	}()
	grpcx.RunClient(APPID)
	http_test()
	//router := fasthttprouter.New()
	//router.GET("/pubkey", func(ctx *fasthttp.RequestCtx) {
	//	ctx.WriteString("LS0tLS1CRUdJTiBSU0EgUFVCTElDSyBLRVktLS0tLQpNSUdKQW9HQkFMK2hpYkw5S3hpb2JNOVRPbmx6cXN0WnhPSy9rU2JQQzMzSmpoVTdjbklUbXlRaThuaXZiUG5wCncwOUo5N0p4aDdxY0tOWVhpakxRdTZxei9xUFNXZ0pYaU9qOWhoc2E0bEdlNVVkRkJtaFpxZ2V3R1J6ckJJNEkKRFNqZk1xcDNCM3puV1h1VnBaSFZNYStJOFBDc1A5dEd3dzdPS2hzRFI0bmp3L3Z2UXdERkFnTUJBQUU9Ci0tLS0tRU5EIFJTQSBQVUJMSUNLIEtFWS0tLS0tCg==")
	//})
	//fasthttp.ListenAndServe(":8090", router.Handler)
}
