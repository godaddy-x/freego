package main

import (
	"github.com/godaddy-x/freego/cache"
	"github.com/godaddy-x/freego/component/consul"
	"github.com/godaddy-x/freego/component/jwt"
	"github.com/godaddy-x/freego/component/limiter"
	"github.com/godaddy-x/freego/node/test"
	"github.com/godaddy-x/freego/util"
)

func http_test() {
	http_web.StartHttpNode()
}

func initConsul() {
	new(consul.ConsulManager).InitConfig(consul.ConsulOption{
		JwtConfig: func() jwt.JwtConfig {
			return jwt.JwtConfig{
				TokenTyp: jwt.JWT,
				TokenAlg: jwt.HS256,
				TokenKey: "123456",
			}
		},
		UnauthorizedUrl: []string{"/idworker.IdWorker/GenerateId"},
		RateOption: func(method string) (rate.Option, error) {
			return rate.Option{}, nil
		},
	}, consul.ConsulConfig{
		Host: "consulx.com:8500",
		Node: "dc/consul",
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
