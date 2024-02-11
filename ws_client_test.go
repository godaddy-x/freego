package main

import (
	"fmt"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/utils/sdk"
	"log"
	"os"
	"os/signal"
	"testing"
	"time"

	"golang.org/x/net/websocket"
)

func TestWS3(t *testing.T) {
	// WebSocket 服务器地址
	serverAddr := "ws://localhost:8080/balance"

	// 创建 WebSocket 连接
	config, err := websocket.NewConfig(serverAddr, "http://localhost/")
	if err != nil {
		log.Fatal("config:", err)
	}

	// {"token":"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxNzU2NTEwOTIwMzAyOTE5NjgxIiwiYXVkIjoiIiwiaXNzIjoiIiwiaWF0IjowLCJleHAiOjE3MDg4Mjk0MTIsImRldiI6IkFQUCIsImp0aSI6InEyTEtYTG1uYXJ5MGhlM0FmVE9ZcFE9PSIsImV4dCI6IiJ9.T5o3LYyncHp2H7yWhj1S+MoWCj68KrcjfpZzqf0qxL8=","secret":"liEnMu77ysCaU4BHy*kT^j#lKWM1JqTPhjSHNtEl#lK!ZC@diQRl04GSsDVIQnU=","expired":1708829412}

	jwtToken := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxNzU2NTEwOTIwMzAyOTE5NjgxIiwiYXVkIjoiIiwiaXNzIjoiIiwiaWF0IjowLCJleHAiOjE3MDg4Mjk0MTIsImRldiI6IkFQUCIsImp0aSI6InEyTEtYTG1uYXJ5MGhlM0FmVE9ZcFE9PSIsImV4dCI6IiJ9.T5o3LYyncHp2H7yWhj1S+MoWCj68KrcjfpZzqf0qxL8="
	jwtSecret := "liEnMu77ysCaU4BHy*kT^j#lKWM1JqTPhjSHNtEl#lK!ZC@diQRl04GSsDVIQnU="
	// 设置 JWT 头部
	config.Header.Add("Authorization", jwtToken)

	// 建立 WebSocket 连接
	ws, err := websocket.DialConfig(config)
	if err != nil {
		log.Fatal("dial:", err)
	}
	defer ws.Close()

	// 启动接收消息的 goroutine
	go receiveMessages(ws)

	go func() {
		for {
			a := map[string]string{"cmd": "ping"}
			data, _ := sdk.BuildRequestObject("/balance", &a, jwtSecret, true)
			sendMessages(ws, utils.Bytes2Str(data))
			time.Sleep(5 * time.Second)
		}
	}()

	// 发送消息到服务器
	//sendMessagesForI(ws)

	// 等待中断信号，优雅关闭连接
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	<-interrupt
	fmt.Println("Closing connection...")
}

func TestWS4(t *testing.T) {
	// WebSocket 服务器地址
	serverAddr := "ws://localhost:8080/query"

	// 创建 WebSocket 连接
	config, err := websocket.NewConfig(serverAddr, "http://localhost/")
	if err != nil {
		log.Fatal("config:", err)
	}

	// {"token":"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxNzU2NTEwOTIwMzAyOTE5NjgxIiwiYXVkIjoiIiwiaXNzIjoiIiwiaWF0IjowLCJleHAiOjE3MDg4Mjk0MTIsImRldiI6IkFQUCIsImp0aSI6InEyTEtYTG1uYXJ5MGhlM0FmVE9ZcFE9PSIsImV4dCI6IiJ9.T5o3LYyncHp2H7yWhj1S+MoWCj68KrcjfpZzqf0qxL8=","secret":"liEnMu77ysCaU4BHy*kT^j#lKWM1JqTPhjSHNtEl#lK!ZC@diQRl04GSsDVIQnU=","expired":1708829412}

	jwtToken := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxNzU2NTEwOTIwMzAyOTE5NjgxIiwiYXVkIjoiIiwiaXNzIjoiIiwiaWF0IjowLCJleHAiOjE3MDg4Mjk0MTIsImRldiI6IkFQUCIsImp0aSI6InEyTEtYTG1uYXJ5MGhlM0FmVE9ZcFE9PSIsImV4dCI6IiJ9.T5o3LYyncHp2H7yWhj1S+MoWCj68KrcjfpZzqf0qxL8="
	jwtSecret := "liEnMu77ysCaU4BHy*kT^j#lKWM1JqTPhjSHNtEl#lK!ZC@diQRl04GSsDVIQnU="
	// 设置 JWT 头部
	config.Header.Add("Authorization", jwtToken)

	// 建立 WebSocket 连接
	ws, err := websocket.DialConfig(config)
	if err != nil {
		log.Fatal("dial:", err)
	}
	defer ws.Close()

	// 启动接收消息的 goroutine
	go receiveMessages(ws)

	go func() {
		for {
			a := map[string]string{"cmd": "ping"}
			data, _ := sdk.BuildRequestObject("/query", &a, jwtSecret)
			sendMessages(ws, utils.Bytes2Str(data))
			time.Sleep(5 * time.Second)
		}
	}()

	// 发送消息到服务器
	//sendMessagesForI(ws)

	// 等待中断信号，优雅关闭连接
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	<-interrupt
	fmt.Println("Closing connection...")
}

// 发送消息到服务器
func sendMessagesForI(ws *websocket.Conn) {
	for i := 0; i < 1; i++ {
		message := fmt.Sprintf("Hello, message %d", i+1)
		if err := websocket.Message.Send(ws, message); err != nil {
			log.Println("send error:", err)
			return
		}
		fmt.Println("sent:", message)
	}
}

func sendMessages(ws *websocket.Conn, message string) {
	if err := websocket.Message.Send(ws, message); err != nil {
		log.Println("send error:", err)
		return
	}
	fmt.Println("sent:", message)
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
