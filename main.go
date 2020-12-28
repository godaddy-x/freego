package main

import (
	"github.com/godaddy-x/freego/node/test"
	"time"
)

func http_test() {
	http_web.StartHttpNode()
}

func websocket_test() {
	http_web.StartWsNode()
}

func main() {
	//websocket_test()
	http_test()
	time.Sleep(1 * time.Hour)
}
