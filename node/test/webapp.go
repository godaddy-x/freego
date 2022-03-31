package http_web

import (
	"github.com/godaddy-x/freego/cache"
	"github.com/godaddy-x/freego/component/jwt"
	"github.com/godaddy-x/freego/component/limiter"
	"github.com/godaddy-x/freego/ex"
	"github.com/godaddy-x/freego/node"
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
	return self.Json(ctx, map[string]interface{}{"tewt": 1})
	//return ex.Throw{Code: ex.BIZ, Msg: "测试错误"}
}

func (self *MyWsNode) test(ctx *node.Context) error {
	// return self.Json(ctx, map[string]interface{}{"tewt": 1})
	return ex.Throw{Code: ex.BIZ, Msg: "测试错误"}
}

func (self *MyWebNode) login(ctx *node.Context) error {
	subject := &jwt.Subject{}
	subject.Create(123456).Iss("1111").Aud("22222").Extinfo("test", "11").Extinfo("test2", "222").Dev("APP")

	//self.LoginBySubject(subject, exp)

	token := subject.Generate(GetSecretKey().JwtSecretKey)
	secret := jwt.GetTokenSecret(token)

	return self.Json(ctx, map[string]interface{}{"token": token, "secret": secret})
	//return self.Html(ctx, "/web/index.html", map[string]interface{}{"tewt": 1})
}

func (self *MyWsNode) login(ctx *node.Context) error {
	//time := util.Time()
	//exp := jwt.TWO_WEEK
	//subject := &jwt.Subject{
	//	Payload: &jwt.Payload{
	//		Sub: 123456,
	//		Aud: ctx.Host,
	//		Iss: "localhost",
	//		Iat: time,
	//		Exp: time + exp,
	//	},
	//}
	//self.LoginBySubject(subject, exp)
	return self.Json(ctx, map[string]interface{}{"token": ""})
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
	my.RateOpetion = &rate.RateOpetion{"gateway", 2, 5, 30}
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
	my.Router("/test1", my.test, &node.Option{Plan: 0})
	my.Router("/login1", my.login, &node.Option{Anonymous: true, Plan: 1})
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
	my.StartServer()
	return my
}
