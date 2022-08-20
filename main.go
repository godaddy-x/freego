package main

import (
	"github.com/godaddy-x/freego/cache"
	"github.com/godaddy-x/freego/node/test"
	"github.com/godaddy-x/freego/util"
)

func http_test() {
	//new(consul.ConsulManager).InitConfig(consul.ConsulConfig{})
	http_web.StartHttpNode()
}

func init() {
	//new(consul.ConsulManager).InitConfig(consul.ConsulConfig{
	//	Host: "consulx.com:8500",
	//	Node: "dc/consul",
	//})
	conf := cache.RedisConfig{}
	if err := util.ReadLocalJsonConfig("resource/redis.json", &conf); err != nil {
		panic(util.AddStr("读取redis配置失败: ", err.Error()))
	}
	new(cache.RedisManager).InitConfig(conf)
}

func main() {
	http_test()
}
