package http_web

import (
	"fmt"
	"github.com/godaddy-x/freego/cache"
	"github.com/godaddy-x/freego/component/consul"
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

type ReqObj struct {
	Uid  int
	Name string
}

type ResObj struct {
	Name   string
	Title  string
	Status int
}

func (self *MyWebNode) test(ctx *node.Context) error {
	//return self.Html(ctx, "/resource/index.html", map[string]interface{}{"tewt": 1})
	return self.Json(ctx, map[string]interface{}{"test": 1})
	//return ex.Throw{Code: ex.BIZ, Msg: "测试错误"}
}

func test_callrpc() {
	mgr, err := new(consul.ConsulManager).Client()
	if err != nil {
		panic(err)
	}

	req := &ReqObj{123, "托尔斯泰"}
	res := &ResObj{}

	if err := mgr.CallRPC(&consul.CallInfo{
		Package:  "mytest",
		Service:  "UserServiceImpl",
		Method:   "FindUser",
		Request:  req,
		Response: res,
	}); err != nil {
		fmt.Println(err)
	}
	fmt.Println("rpc result: ", res)
}

func (self *MyWebNode) login(ctx *node.Context) error {
	subject := &jwt.Subject{}
	subject.Create(123456).Iss("1111").Aud("22222").Extinfo("test", "11").Extinfo("test2", "222").Dev("APP")

	//self.LoginBySubject(subject, exp)

	token := subject.Generate(GetSecretKey().TokenKey)
	secret := jwt.GetTokenSecret(token)

	return self.Json(ctx, map[string]interface{}{"token": token, "secret": secret})
	//return self.Html(ctx, "/web/index.html", map[string]interface{}{"tewt": 1})
}

func (self *MyWebNode) callrpc(ctx *node.Context) error {
	test_callrpc()
	return self.Json(ctx, map[string]interface{}{})
}

func GetSecretKey() *jwt.JwtSecretKey {
	return &jwt.JwtSecretKey{
		TokenKey: "123456",
		TokenAlg: jwt.SHA256,
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
	my.GatewayRate = &rate.RateOpetion{Limit: 2, Bucket: 5, Expire: 30}
	my.CacheAware = GetCacheAware
	my.OverrideFunc = &node.OverrideFunc{
		PreHandleFunc: func(ctx *node.Context) error {
			if limiter.Validate(&rate.RateOpetion{Key: ctx.Method, Limit: 2, Bucket: 5, Expire: 30}) {
				return ex.Throw{Code: 429, Msg: "too many visitors, please try again later"}
			}
			if ctx.Subject != nil {
				if limiter.Validate(&rate.RateOpetion{Key: util.AnyToStr(ctx.Subject.Sub), Limit: 2, Bucket: 5, Expire: 30}) {
					return ex.Throw{Code: 429, Msg: "the access frequency is too fast, please try again later"}
				}
			}
			return nil
		},
		LogHandleFunc: func(ctx *node.Context) (*node.LogHandleRes, error) {
			res := &node.LogHandleRes{
				LogNo:    util.GetSnowFlakeStrID(),
				CreateAt: util.Time(),
			}
			// TODO send log to rabbitmq
			return res, nil
		},
		PostHandleFunc: func(resp *node.Response, err error) error {
			return err
		},
		AfterCompletionFunc: func(ctx *node.Context, res *node.LogHandleRes, err error) error {
			res.UpdateAt = util.Time()
			res.CostMill = res.UpdateAt - res.CreateAt
			// TODO send log to rabbitmq
			return err
		},
	}
	my.Router("/test1", my.test, nil)
	my.Router("/login1", my.login, &node.Config{Authorization: false, RequestAesEncrypt: true, ResponseAesEncrypt: true})
	my.Router("/callrpc", my.login, &node.Config{Authorization: false, RequestAesEncrypt: false, ResponseAesEncrypt: false})
	my.StartServer()
	return my
}
