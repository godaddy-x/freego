package main

import (
	"github.com/godaddy-x/freego/cache"
	"github.com/godaddy-x/freego/cache/limiter"
	ballast "github.com/godaddy-x/freego/gc"
	"github.com/godaddy-x/freego/node/test"
	"github.com/godaddy-x/freego/rpcx"
	"github.com/godaddy-x/freego/utils"
	"net/http"
	_ "net/http/pprof"
)

func http_test() {
	http_web.StartHttpNode()
}

func initConsul() {
	conf := rpcx.ConsulConfig{}
	if err := utils.ReadLocalJsonConfig("resource/consul.json", &conf); err != nil {
		panic(utils.AddStr("读取consul配置失败: ", err.Error()))
	}
	new(rpcx.ConsulManager).InitConfig(conf)
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
	client := &rpcx.GRPCManager{}
	client.CreateJwtConfig(APPKEY)
	client.CreateAppConfigCall(func(appid string) (rpcx.AppConfig, error) {
		if appid == APPKEY {
			return rpcx.AppConfig{Appid: APPID, Appkey: APPKEY}, nil
		}
		return rpcx.AppConfig{}, utils.Error("appid invalid")
	})
	client.CreateRateLimiterCall(func(method string) (rate.Option, error) {
		return rate.Option{}, nil
	})
	client.CreateServerTLS(rpcx.TlsConfig{
		UseMTLS:   true,
		CACrtFile: "./rpcx/cert/ca.crt",
		KeyFile:   "./rpcx/cert/server.key",
		CrtFile:   "./rpcx/cert/server.crt",
	})
	client.CreateClientTLS(rpcx.TlsConfig{
		UseMTLS:   true,
		CACrtFile: "./rpcx/cert/ca.crt",
		KeyFile:   "./rpcx/cert/client.key",
		CrtFile:   "./rpcx/cert/client.crt",
		HostName:  "localhost",
	})
	client.CreateAuthorizeTLS("./rpcx/cert/server.key")
}

func init() {
	initConsul()
	initRedis()
	initGRPC()
}

func main() {
	ballast.GC(512*ballast.MB, 30)
	go func() {
		_ = http.ListenAndServe(":8849", nil)
	}()
	rpcx.RunClient(APPID)
	http_test()
	//router := fasthttprouter.New()
	//router.GET("/pubkey", func(ctx *fasthttp.RequestCtx) {
	//	ctx.WriteString("LS0tLS1CRUdJTiBSU0EgUFVCTElDSyBLRVktLS0tLQpNSUdKQW9HQkFMK2hpYkw5S3hpb2JNOVRPbmx6cXN0WnhPSy9rU2JQQzMzSmpoVTdjbklUbXlRaThuaXZiUG5wCncwOUo5N0p4aDdxY0tOWVhpakxRdTZxei9xUFNXZ0pYaU9qOWhoc2E0bEdlNVVkRkJtaFpxZ2V3R1J6ckJJNEkKRFNqZk1xcDNCM3puV1h1VnBaSFZNYStJOFBDc1A5dEd3dzdPS2hzRFI0bmp3L3Z2UXdERkFnTUJBQUU9Ci0tLS0tRU5EIFJTQSBQVUJMSUNLIEtFWS0tLS0tCg==")
	//})
	//fasthttp.ListenAndServe(":8090", router.Handler)
}
