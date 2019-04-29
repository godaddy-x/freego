package http_web

import (
	"fmt"
	"github.com/godaddy-x/freego/component/jwt"
	"github.com/godaddy-x/freego/ex"
	"github.com/godaddy-x/freego/node"
)

var (
	local_aware = node.NewLocalCacheSessionAware()
)

type MyWebNode struct {
	node.HttpNode
}

type MyWsNode struct {
	node.WebsocketNode
}

func (self *MyWebNode) test(ctx *node.Context) error {
	//return self.Html(ctx, "/resource/index.html", map[string]interface{}{"tewt": 1})
	//return self.Json(ctx, map[string]interface{}{"tewt": 1})
	return ex.Try{Code: ex.BIZ, Msg: "测试错误"}
}

func (self *MyWsNode) test(ctx *node.Context) error {
	// return self.Json(ctx, map[string]interface{}{"tewt": 1})
	return ex.Try{Code: ex.BIZ, Msg: "测试错误"}
}

func (self *MyWebNode) login(ctx *node.Context) error {
	subject := &jwt.Subject{
		Payload: &jwt.Payload{
			Sub: "zhangsan",
			Iss: "456",
			Aud: ctx.Host,
			Dev: ctx.Device,
			Exp: jwt.TWO_WEEK,
		},
	}
	self.Connect(ctx, node.BuildJWTSession(subject, nil))
	return self.Json(ctx, map[string]interface{}{"token": ""})
	//return self.Html(ctx, "/web/index.html", map[string]interface{}{"tewt": 1})
}

func (self *MyWsNode) login(ctx *node.Context) error {
	subject := &jwt.Subject{
		Payload: &jwt.Payload{
			Sub: "zhangsan",
			Iss: "456",
			Aud: ctx.Host,
			Dev: ctx.Device,
			Exp: jwt.TWO_WEEK,
		},
	}
	self.Connect(ctx, node.BuildJWTSession(subject, nil))
	return self.Json(ctx, map[string]interface{}{"token": ""})
}

func (self *MyWebNode) logout(ctx *node.Context) error {
	fmt.Println(ctx.Session.GetAttribute("test"))
	self.Release(ctx)
	return self.Html(ctx, "/resource/index.html", map[string]interface{}{"tewt": 1})
}

func (self *MyWsNode) logout(ctx *node.Context) error {
	fmt.Println(ctx.Session.GetAttribute("test"))
	self.Release(ctx)
	return self.Json(ctx, map[string]interface{}{"token": "test"})
}

func GetSecurity() *node.Security {
	subject := &jwt.Subject{
		Payload: &jwt.Payload{Iss: "http://localhost"},
	}
	return &node.Security{subject, "123456"}
}

func StartHttpNode() *MyWebNode {
	my := &MyWebNode{}
	my.Context = &node.Context{
		Host:     "0.0.0.0",
		Port:     8090,
		Security: GetSecurity,
	}
	my.SessionAware = local_aware
	my.OverrideFunc = &node.OverrideFunc{
		GetHeaderFunc: nil,
		GetParamsFunc: nil,
		//PreHandleFunc: func(ctx *node.Context) error {
		//	return nil
		//},
		//PostHandleFunc: func(resp *node.Response, err error) error {
		//	// resp.RespEntity = map[string]interface{}{"sssss": 3}
		//	return err
		//},
		//AfterCompletionFunc: func(ctx *node.Context, resp *node.Response, err error) error {
		//	return err
		//},
		//RenderErrorFunc: nil,
	}
	my.Router("/test1", my.test, false)
	my.Router("/login1", my.login)
	my.Router("/logout1", my.logout)
	my.StartServer()
	return my
}

func StartWsNode() *MyWsNode {
	my := &MyWsNode{}
	my.Context = &node.Context{
		Host:     "0.0.0.0",
		Port:     9090,
		Security: GetSecurity,
	}
	my.SessionAware = local_aware
	my.Router("/test2", my.test)
	my.Router("/login2", my.login)
	my.Router("/logout2", my.logout)
	my.StartServer()
	return my
}
