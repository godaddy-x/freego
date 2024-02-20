package main

import (
	"fmt"
	"github.com/godaddy-x/freego/node"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/utils/jwt"
	"testing"
	"time"
)

type MsgReply struct {
	Id   string      `json:"id"`
	Ack  bool        `json:"ack"`
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

func TestWsServer(t *testing.T) {
	server := node.WsServer{}
	server.AddJwtConfig(jwt.JwtConfig{
		TokenTyp: jwt.JWT,
		TokenAlg: jwt.HS256,
		TokenKey: "123456" + utils.CreateLocalSecretKey(12, 45, 23, 60, 58, 30),
		TokenExp: jwt.TWO_WEEK,
	})
	handle := func(ctx *node.Context, message []byte) (interface{}, error) {
		result := map[string]interface{}{}
		if err := utils.JsonUnmarshal(message, &result); err != nil {
			return nil, err
		}
		fmt.Println("receive ack:", result)
		return nil, nil
	}
	server.AddRouter("/query", handle, nil)
	go func() {
		for {
			reply := MsgReply{Id: utils.NextSID(), Type: "transfer", Data: "我爱中国"}
			server.SendMessage(&reply, "1756510920302919681", "APP")
			time.Sleep(5 * time.Second)
		}
	}()
	server.StartWebsocket(":8080")
}
