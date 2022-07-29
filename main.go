package main

import (
	"github.com/godaddy-x/freego/node/test"
	"time"
)

func http_test() {
	http_web.StartHttpNode()
}

func main() {
	http_test()
	time.Sleep(1 * time.Hour)
}
