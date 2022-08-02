package main

import (
	"github.com/godaddy-x/freego/component/consul"
	"github.com/godaddy-x/freego/node/test"
	"time"
)

func http_test() {
	http_web.StartHttpNode()
}

func init()  {
	new(consul.ConsulManager).InitConfig(consul.ConsulConfig{
		Host: "consulx.com:8500",
		Node: "dc/consul",
	})
}

func main() {
	http_test()
	time.Sleep(1 * time.Hour)
}
