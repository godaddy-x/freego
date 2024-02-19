package main

import (
	"fmt"
	"github.com/godaddy-x/freego/node"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/utils/jwt"
	"github.com/godaddy-x/freego/zlog"
	"testing"
	"time"
)

type MsgReply struct {
	Id   string      `json:"id"`
	Ack  bool        `json:"ack"`
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

func TestWS2(t *testing.T) {
	ws := node.WsNode{}
	ws.AddJwtConfig(jwt.JwtConfig{
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
	ws.AddRouter("/query", handle, nil)
	go func() {
		for {
			reply := MsgReply{Id: utils.NextSID(), Type: "transfer", Data: "我爱中国"}
			ws.SendMessage(&reply, "1756510920302919681", "APP")
			time.Sleep(5 * time.Second)
		}
	}()
	ws.StartWebsocket(":8080")
}

func TestWS5(t *testing.T) {

	authCall := func(object interface{}) (string, string, error) {
		//mySdk := &sdk.HttpSDK{
		//	Domain:     "http://localhost:8091",
		//	AuthDomain: "http://localhost:8090",
		//	KeyPath:    "/key",
		//	LoginPath:  "/login",
		//}
		//responseData := sdk.AuthToken{}
		//if err := mySdk.PostByECC("/login", object, &responseData); err != nil {
		//	return "", "", err
		//}
		//return responseData.Token, responseData.Secret, nil
		jwtToken := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxNzU2NTEwOTIwMzAyOTE5NjgxIiwiYXVkIjoiIiwiaXNzIjoiIiwiaWF0IjowLCJleHAiOjE3MDg4Mjk0MTIsImRldiI6IkFQUCIsImp0aSI6InEyTEtYTG1uYXJ5MGhlM0FmVE9ZcFE9PSIsImV4dCI6IiJ9.T5o3LYyncHp2H7yWhj1S+MoWCj68KrcjfpZzqf0qxL8="
		jwtSecret := "liEnMu77ysCaU4BHy*kT^j#lKWM1JqTPhjSHNtEl#lK!ZC@diQRl04GSsDVIQnU="
		return jwtToken, jwtSecret, nil
	}

	receiveCall := func(message []byte, err error) (interface{}, error) {
		reply := MsgReply{}
		if err := utils.JsonUnmarshal(message, &reply); err != nil {
			return nil, err
		}
		fmt.Println("receive:", reply)
		reply.Ack = true
		return &reply, nil
	}

	client := node.ClientAuth{
		Addr:        "ws://localhost:8080",
		Path:        "/query",
		Origin:      "*",
		AuthCall:    authCall,
		ReceiveCall: receiveCall,
	}
	authObject := map[string]string{"username": "1234567890123456", "password": "1234567890123456"}
	for {
		if err := node.StartWebsocketClient(client, &authObject); err != nil {
			zlog.Error("websocket client error", 0, zlog.AddError(err))
		}
		time.Sleep(10 * time.Second)
	}
}
