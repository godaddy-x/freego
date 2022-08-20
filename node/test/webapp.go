package http_web

import (
	"fmt"
	"github.com/godaddy-x/freego/cache"
	"github.com/godaddy-x/freego/component/consul"
	"github.com/godaddy-x/freego/component/jwt"
	"github.com/godaddy-x/freego/component/limiter"
	"github.com/godaddy-x/freego/ex"
	"github.com/godaddy-x/freego/node"
	"github.com/godaddy-x/freego/node/common"
	"github.com/godaddy-x/freego/util"
)

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

type GetUserReq struct {
	common.BaseReq
	Uid  int    `json:"uid"`
	Name string `json:"name"`
}

func (self *MyWebNode) test(ctx *node.Context) error {
	req := &GetUserReq{}
	if err := ctx.Parser(req); err != nil {
		return err
	}
	//return self.Html(ctx, "/resource/index.html", map[string]interface{}{"tewt": 1})
	return self.Json(ctx, map[string]interface{}{"test": 1})
	//return ex.Throw{Code: ex.BIZ, Msg: "测试错误"}
}

func (self *MyWebNode) getUser(ctx *node.Context) error {
	req := &GetUserReq{}
	if err := ctx.Parser(req); err != nil {
		return err
	}
	return self.Json(ctx, map[string]interface{}{"test": "我爱中国+-/+_=/1df"})
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
	//self.LoginBySubject(subject, exp)
	config := ctx.JwtConfig()
	token := subject.Create(util.GetSnowFlakeIntID()).Dev("APP").Generate(config)
	secret := jwt.GetTokenSecret(token, config.TokenKey)
	return self.Json(ctx, map[string]interface{}{"token": token, "secret": secret})
	//return self.Html(ctx, "/web/index.html", map[string]interface{}{"tewt": 1})
}

func (self *MyWebNode) pubkey(ctx *node.Context) error {
	//test_callrpc()
	return self.Text(ctx, ctx.ServerCert.PubkeyBase64)
}

var tokenKey = "123456" + util.CreateLocalSecretKey(12, 45, 23, 60, 58, 30)

func GetJwtConfig() jwt.JwtConfig {
	return jwt.JwtConfig{
		TokenTyp: jwt.JWT,
		TokenAlg: jwt.HS256,
		TokenKey: tokenKey,
	}
}

var local = new(cache.LocalMapManager).NewCache(30, 10)
var limiter = rate.NewRateLimiter(rate.Option{Limit: 2, Bucket: 10, Expire: 30, Distributed: true})

func GetCacheAware(ds ...string) (cache.ICache, error) {
	return local, nil
}

func StartHttpNode() {
	my := &MyWebNode{}
	my.Context = &node.Context{
		Host:      "0.0.0.0",
		Port:      8090,
		JwtConfig: GetJwtConfig,
		//PermConfig: func(url string) (node.Permission, error) {
		//	return node.Permission{}, nil
		//},
	}
	//my.DisconnectTimeout = 10
	my.GatewayLimiter = rate.NewRateLimiter(rate.Option{Limit: 50, Bucket: 50, Expire: 30, Distributed: true})
	my.CacheAware = GetCacheAware
	my.OverrideFunc = &node.OverrideFunc{
		PreHandleFunc: func(ctx *node.Context) error {
			if b := limiter.Allow(ctx.Method); !b {
				return ex.Throw{Code: 429, Msg: "the method request is full, please try again later"}
			}
			if ctx.Authenticated() {
				if b := limiter.Allow(util.AnyToStr(ctx.Subject.Sub)); !b {
					return ex.Throw{Code: 429, Msg: "the access frequency is too fast, please try again later"}
				}
			}
			return nil
		},
		LogHandleFunc: func(ctx *node.Context) (node.LogHandleRes, error) {
			res := node.LogHandleRes{
				LogNo:    util.GetSnowFlakeStrID(),
				CreateAt: util.Time(),
			}
			// TODO send log to rabbitmq
			//fmt.Println("LogHandleFunc: ", res)
			return res, nil
		},
		PostHandleFunc: func(resp *node.Response, err error) error {
			return err
		},
		AfterCompletionFunc: func(ctx *node.Context, res node.LogHandleRes, err error) error {
			res.UpdateAt = util.Time()
			res.CostMill = res.UpdateAt - res.CreateAt
			// TODO send log to rabbitmq
			//fmt.Println("AfterCompletionFunc: ", res)
			return err
		},
	}
	my.Router("/test1", my.test, nil)
	my.Router("/test2", my.getUser, &node.RouterConfig{})
	my.Router("/pubkey", my.pubkey, &node.RouterConfig{Original: true, Guest: true})
	my.Router("/login", my.login, &node.RouterConfig{Login: true})
	my.StartServer()
}
