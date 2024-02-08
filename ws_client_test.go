package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"testing"

	"golang.org/x/net/websocket"
)

func TestWS3(t *testing.T) {
	// WebSocket 服务器地址
	serverAddr := "ws://localhost:8080"

	// 创建 WebSocket 连接
	config, err := websocket.NewConfig(serverAddr, "http://localhost/")
	if err != nil {
		log.Fatal("config:", err)
	}

	// 设置 JWT 头部
	config.Header.Add("Authorization", "1234567")

	// 建立 WebSocket 连接
	ws, err := websocket.DialConfig(config)
	if err != nil {
		log.Fatal("dial:", err)
	}
	defer ws.Close()

	// 启动接收消息的 goroutine
	go receiveMessages(ws)

	// 发送消息到服务器
	sendMessages(ws)

	// 等待中断信号，优雅关闭连接
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	<-interrupt
	fmt.Println("Closing connection...")
}

// 发送消息到服务器
func sendMessages(ws *websocket.Conn) {
	for i := 0; i < 5; i++ {
		message := fmt.Sprintf("Hello, message %d", i+1)
		if err := websocket.Message.Send(ws, message); err != nil {
			log.Println("send error:", err)
			return
		}
		fmt.Println("sent:", message)
	}
}

// 接收服务器消息
func receiveMessages(ws *websocket.Conn) {
	for {
		var message string
		if err := websocket.Message.Receive(ws, &message); err != nil {
			log.Println("receive error:", err)
			return
		}
		fmt.Println("received:", message)
	}
}
