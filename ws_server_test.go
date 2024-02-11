package main

import (
	"fmt"
	"github.com/godaddy-x/freego/node"
	"github.com/godaddy-x/freego/node/common"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/utils/jwt"
	"testing"
)

type Ping struct {
	common.BaseReq
}

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
		req := Ping{}
		if err := ctx.Parser(&req); err != nil {
			return err
		}
		fmt.Println(req)
		result := map[string]string{"cmd": req.Cmd, "test": utils.NextSID()}
		//return ex.Throw{Msg: "test error"}
		return ctx.Json(&result)
	}, nil)
	ws.AddRouter("/query", func(ctx *node.Context) error {
		req := Ping{}
		if err := ctx.Parser(&req); err != nil {
			return err
		}
		fmt.Println(req)
		result := map[string]string{"cmd": req.Cmd, "test222": utils.NextSID()}
		//return ex.Throw{Msg: "test error"}
		return ctx.Json(&result)
	}, nil)
	ws.StartWebsocket(":8080")
}
