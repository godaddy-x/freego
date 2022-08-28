package http_web

import (
	"context"
	"fmt"
	"github.com/godaddy-x/freego/cache"
	"github.com/godaddy-x/freego/cache/limiter"
	"github.com/godaddy-x/freego/consul/grpcx"
	"github.com/godaddy-x/freego/consul/grpcx/pb"
	"github.com/godaddy-x/freego/ex"
	"github.com/godaddy-x/freego/node"
	"github.com/godaddy-x/freego/node/common"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/utils/jwt"
	"github.com/godaddy-x/freego/zlog"
	"google.golang.org/grpc"
)

type MyWebNode struct {
	node.HttpNode
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

func testCallRPC() {
	res, err := grpcx.CallRPC(&grpcx.GRPC{
		Service: "PubWorker",
		CallRPC: func(conn *grpc.ClientConn, ctx context.Context) (interface{}, error) {
			return pb.NewPubWorkerClient(conn).GenerateId(ctx, &pb.GenerateIdReq{})
		}})
	if err != nil {
		fmt.Println(err)
	} else {
		fmt.Println("grpc:", res)
	}
}

func (self *MyWebNode) login(ctx *node.Context) error {
	subject := &jwt.Subject{}
	//self.LoginBySubject(subject, exp)
	config := ctx.JwtConfig()
	token := subject.Create(utils.GetSnowFlakeStrID()).Dev("APP").Generate(config)
	secret := jwt.GetTokenSecret(token, config.TokenKey)
	return self.Json(ctx, map[string]interface{}{"token": token, "secret": secret})
	//return self.Html(ctx, "/web/index.html", map[string]interface{}{"tewt": 1})
}

func (self *MyWebNode) pubkey(ctx *node.Context) error {
	testCallRPC()
	return self.Text(ctx, ctx.ServerCert.PubkeyBase64)
}

func GetJwtConfig() jwt.JwtConfig {
	return jwt.JwtConfig{
		TokenTyp: jwt.JWT,
		TokenAlg: jwt.HS256,
		TokenKey: "123456" + utils.CreateLocalSecretKey(12, 45, 23, 60, 58, 30),
		TokenExp: jwt.TWO_WEEK,
	}
}

var local = new(cache.LocalMapManager).NewCache(30, 10)
var limiter = rate.NewRateLimiter(rate.Option{Limit: 100, Bucket: 200, Expire: 30, Distributed: true})

func GetCacheAware(ds ...string) (cache.ICache, error) {
	return local, nil
}

type NewPostFilter struct{}

func (self *NewPostFilter) DoFilter(chain node.Filter, object *node.FilterObject) error {
	fmt.Println(" --- NewFilter.DoFilter ---")
	return chain.DoFilter(chain, object)
}

type NewPostHandleInterceptor struct{}

func (self *NewPostHandleInterceptor) PreHandle(ctx *node.Context) (bool, error) {
	fmt.Println(" --- NewPostHandleInterceptor PreHandle -- ")
	if b := limiter.Allow(ctx.Method); !b {
		return false, ex.Throw{Code: 429, Msg: "the method request is full, please try again later"}
	}
	if ctx.Authenticated() {
		if b := limiter.Allow(utils.AnyToStr(ctx.Subject.Sub)); !b {
			return false, ex.Throw{Code: 429, Msg: "the access frequency is too fast, please try again later"}
		}
	}
	ctx.AddStorage("httpLog", node.HttpLog{Method: ctx.Method, LogNo: utils.GetSnowFlakeStrID(), CreateAt: utils.Time()})
	return true, nil
}

func (self *NewPostHandleInterceptor) PostHandle(ctx *node.Context) error {
	fmt.Println(" --- NewPostHandleInterceptor PostHandle -- ")
	return nil
}

func (self *NewPostHandleInterceptor) AfterCompletion(ctx *node.Context, err error) error {
	fmt.Println(" --- NewPostHandleInterceptor AfterCompletion -- ")
	v := ctx.GetStorage("httpLog")
	if v == nil {
		return utils.Error("httpLog is nil")
	}
	httpLog, _ := v.(node.HttpLog)
	httpLog.UpdateAt = utils.Time()
	httpLog.CostMill = httpLog.UpdateAt - httpLog.CreateAt
	zlog.Info("http log", 0, zlog.Any("data", httpLog))
	if err := node.RenderData(ctx); err != nil {
		return err
	}
	return err
}

func StartHttpNode() {
	var my = &MyWebNode{}
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
	my.AddFilter("NewPostFilter", 100, &NewPostFilter{})
	my.AddInterceptor(node.PostHandleInterceptorName, 50, &NewPostHandleInterceptor{})
	my.Router("/test1", my.test, nil)
	my.Router("/test2", my.getUser, &node.RouterConfig{})
	my.Router("/pubkey", my.pubkey, &node.RouterConfig{Original: true, Guest: true})
	my.Router("/login", my.login, &node.RouterConfig{Login: true})
	my.StartServer()
}
