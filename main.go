package main

import (
	"github.com/godaddy-x/freego/node/test"
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
}

func main() {
	http_test()
}
