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

func TestWS2(t *testing.T) {
	//// 设置 WebSocket 路由
	//http.Handle("/", websocket.Handler(wsHandler))
	//
	//// 启动 WebSocket 服务器
	//addr := ":8080"
	//fmt.Printf("Server is running on %s\n", addr)
	//if err := http.ListenAndServe(addr, nil); err != nil {
	//	log.Fatal("ListenAndServe: ", err)
	//}
	ws := node.WsNode{}
	ws.AddJwtConfig(jwt.JwtConfig{
		TokenTyp: jwt.JWT,
		TokenAlg: jwt.HS256,
		TokenKey: "123456" + utils.CreateLocalSecretKey(12, 45, 23, 60, 58, 30),
		TokenExp: jwt.TWO_WEEK,
	})
	ws.AddRouter("/balance", func(ctx *node.Context) error {
		//req := Ping{}
		//if err := ctx.Parser(&req); err != nil {
		//	return err
		//}
		//fmt.Println(req)
		result := map[string]string{"cmd": "123", "test": utils.NextSID()}
		//return ex.Throw{Msg: "test error"}
		return ctx.Json(&result)
	}, nil)
	ws.AddRouter("/query", func(ctx *node.Context) error {
		result := map[string]string{"cmd": "456", "test222": utils.NextSID()}
		//return ex.Throw{Msg: "test error"}
		return ctx.Json(&result)
	}, nil)
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

	receiveCall := func(message []byte, err error) error {
		fmt.Println("receive:", string(message))
		return err
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
