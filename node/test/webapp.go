package http_web

import (
	"fmt"
	"github.com/godaddy-x/freego/cache"
	"github.com/godaddy-x/freego/component/jwt"
	"github.com/godaddy-x/freego/ex"
	"github.com/godaddy-x/freego/node"
	"github.com/godaddy-x/freego/util"
)

var (
	local_cache = new(cache.LocalMapManager).NewCache(30, 10)
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
	return ex.Throw{Code: ex.BIZ, Msg: "测试错误"}
}

func (self *MyWsNode) test(ctx *node.Context) error {
	// return self.Json(ctx, map[string]interface{}{"tewt": 1})
	return ex.Throw{Code: ex.BIZ, Msg: "测试错误"}
}

func (self *MyWebNode) login(ctx *node.Context) error {
	time := util.Time()
	exp := jwt.TWO_WEEK
	subject := &jwt.Subject{
		Header: &jwt.Header{
			Nod: 0,
			Typ: jwt.JWT,
			Alg: jwt.SHA256,
		},
		Payload: &jwt.Payload{
			Sub: "zhangsan",
			Dev: ctx.Device,
			Aud: ctx.Host,
			Iss: "localhost",
			Iat: time,
			Exp: time + exp,
			Nbf: time,
		},
	}
	author, err := subject.GetAuthorization(GetSecretKey())
	if err != nil {
		return ex.Throw{ex.SYSTEM, "生成授权失败", err}
	}
	self.LoginBySubject(subject.Payload.Sub, author.SignatureKey, exp)
	return self.Json(ctx, map[string]interface{}{"token": ""})
	//return self.Html(ctx, "/web/index.html", map[string]interface{}{"tewt": 1})
}

func (self *MyWsNode) login(ctx *node.Context) error {
	time := util.Time()
	exp := jwt.TWO_WEEK
	subject := &jwt.Subject{
		Payload: &jwt.Payload{
			Sub: "zhangsan",
			Dev: ctx.Device,
			Aud: ctx.Host,
			Iss: "localhost",
			Iat: time,
			Exp: time + exp,
			Nbf: time,
		},
	}
	author, err := subject.GetAuthorization(GetSecretKey())
	if err != nil {
		return ex.Throw{ex.SYSTEM, "生成授权失败", err}
	}
	self.LoginBySubject(subject.Payload.Sub, author.SignatureKey, exp)
	return self.Json(ctx, map[string]interface{}{"token": ""})
}

func (self *MyWebNode) logout(ctx *node.Context) error {
	fmt.Println(ctx.Session.GetAttribute("test"))
	self.Release(ctx)
	return self.Html(ctx, "/resource/index.html", map[string]interface{}{"test": 1})
}

func (self *MyWsNode) logout(ctx *node.Context) error {
	fmt.Println(ctx.Session.GetAttribute("test"))
	self.Release(ctx)
	return self.Json(ctx, map[string]interface{}{"token": "test"})
}

func GetSecretKey() *jwt.SecretKey {
	return &jwt.SecretKey{
		ApiSecretKey: "123456",
		JwtSecretKey: "123456",
		SecretKeyAlg: jwt.SHA256,
	}
}

func GetCacheAware(ds ...string) (cache.ICache, error) {
	return local_cache, nil
}

func StartHttpNode() *MyWebNode {
	my := &MyWebNode{}
	my.Context = &node.Context{
		Host:      "0.0.0.0",
		Port:      8090,
		SecretKey: GetSecretKey,
	}
	my.CacheAware = GetCacheAware
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
	my.Router("/test1", my.test)
	my.Router("/login1", my.login)
	my.Router("/logout1", my.logout)
	my.StartServer()
	return my
}

func StartWsNode() *MyWsNode {
	my := &MyWsNode{}
	my.Context = &node.Context{
		Host:      "0.0.0.0",
		Port:      9090,
		SecretKey: GetSecretKey,
	}
	my.CacheAware = GetCacheAware
	my.Router("/test2", my.test)
	my.Router("/login2", my.login)
	my.Router("/logout2", my.logout)
	my.StartServer()
	return my
}
