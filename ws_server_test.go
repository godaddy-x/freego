package main

import (
	"fmt"
	"log"
	"net/http"
	"testing"

	"golang.org/x/net/websocket"
)

func wsHandler(ws *websocket.Conn) {

	// 从请求头中提取 JWT token
	authHeader := ws.Request().Header.Get("Authorization")
	if authHeader != "123456" {
		log.Println("Authorization header is missing")
		ws.Write([]byte("401"))
		ws.Close()
		return
	}

	fmt.Println("token:", authHeader)

	defer ws.Close()

	for {
		// 读取消息
		var msg string
		err := websocket.Message.Receive(ws, &msg)
		if err != nil {
			log.Println("receive error:", err)
			break
		}
		log.Println("received:", msg)

		// 回复消息
		err = websocket.Message.Send(ws, msg)
		if err != nil {
			log.Println("send error:", err)
			break
		}
	}
}

func TestWS2(t *testing.T) {
	// 设置 WebSocket 路由
	http.Handle("/", websocket.Handler(wsHandler))

	// 启动 WebSocket 服务器
	addr := ":8080"
	fmt.Printf("Server is running on %s\n", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
