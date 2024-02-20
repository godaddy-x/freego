package main

import (
	"fmt"
	"github.com/godaddy-x/freego/node"
	"github.com/godaddy-x/freego/utils"
	"testing"
	"time"
)

func TestWsClient(t *testing.T) {

	authCall := func() (string, string, error) {
		//mySdk := &sdk.HttpSDK{
		//	Domain:     "http://localhost:8091",
		//	AuthDomain: "http://localhost:8090",
		//	KeyPath:    "/key",
		//	LoginPath:  "/login",
		//}
		//authObject := map[string]string{"username": "1234567890123456", "password": "1234567890123456"}
		//responseData := sdk.AuthToken{}
		//if err := mySdk.PostByECC("/login", authObject, &responseData); err != nil {
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

	client := node.WsClient{
		Addr:        "ws://localhost:8080",
		Path:        "/query",
		Origin:      "*",
		AuthCall:    authCall,
		ReceiveCall: receiveCall,
	}
	go func() {
		for {
			a := map[string]interface{}{"type": "test", "aaaaaaa": utils.NextSID()}
			client.SendMessage(&a)
			time.Sleep(5 * time.Second)
		}
	}()
	client.StartWebsocket()
}
