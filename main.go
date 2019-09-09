package main

import (
	"fmt"
	"github.com/godaddy-x/freego/node/test"
	"github.com/godaddy-x/freego/util"
	"time"
)

func http_test() {
	http_web.StartHttpNode()
}

func websocket_test() {
	http_web.StartWsNode()
}

func main() {
	fmt.Println(util.GetRandStr(16))
	websocket_test()
	http_test()
	time.Sleep(1 * time.Hour)
}
