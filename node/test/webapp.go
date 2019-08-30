package http_web

import (
	"fmt"
	"github.com/godaddy-x/freego/cache"
	"github.com/godaddy-x/freego/component/jwt"
	"github.com/godaddy-x/freego/component/limiter"
	"github.com/godaddy-x/freego/ex"
	"github.com/godaddy-x/freego/node"
	"github.com/godaddy-x/freego/util"
)

var (
	local_cache = new(cache.LocalMapManager).NewCache(30, 10)
	limiter     = rate.NewLocalLimiter(local_cache)
)

func init() {
	//redisConf := cache.RedisConfig{
	//	Host:        "192.168.27.160",
	//	Port:        6379,
	//	Password:    "wallet828",
	//	MaxIdle:     150,
	//	MaxActive:   150,
	//	IdleTimeout: 240,
	//	Network:     "tcp",
	//	LockTimeout: 15,
	//}
	//new(cache.RedisManager).InitConfig(redisConf)
	//redis_cache, _ := new(cache.RedisManager).Client()
	//limiter = rate.NewRedisLimiter(redis_cache)
}

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
	return nil
}

func (self *MyWsNode) logout(ctx *node.Context) error {
	fmt.Println(ctx.Session.GetAttribute("test"))
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
	my.Limiter = rate.NewLocalLimiterByOption(local_cache, &rate.RateOpetion{"gateway", 2, 5, 30})
	my.CacheAware = GetCacheAware
	my.OverrideFunc = &node.OverrideFunc{
		PreHandleFunc: func(ctx *node.Context) error {
			if limiter.Validate(&rate.RateOpetion{ctx.Method, 2, 5, 30}) {
				return ex.Throw{Code: 429, Msg: "系统正繁忙,人数过多"}
			}
			return nil
		},
		//PostHandleFunc: func(resp *node.Response, err error) error {
		//	// resp.RespEntity = map[string]interface{}{"sssss": 3}
		//	return err
		//},
		//AfterCompletionFunc: func(ctx *node.Context, resp *node.Response, err error) error {
		//	return err
		//},
		//RenderErrorFunc: nil,
	}
	my.Router("/test1", my.test, &node.Option{})
	my.Router("/login1", my.login, &node.Option{Customize: true})
	my.Router("/logout1", my.logout, &node.Option{})
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
	my.Router("/test2", my.test, nil)
	my.Router("/login2", my.login, nil)
	my.Router("/logout2", my.logout, nil)
	my.StartServer()
	return my
}
