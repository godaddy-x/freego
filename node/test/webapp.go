package http_web

import (
	"context"
	"fmt"
	"github.com/godaddy-x/freego/cache"
	"github.com/godaddy-x/freego/cache/limiter"
	"github.com/godaddy-x/freego/consul/grpcx"
	"github.com/godaddy-x/freego/consul/grpcx/pb"
	"github.com/godaddy-x/freego/node"
	"github.com/godaddy-x/freego/node/common"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/utils/jwt"
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
	config := self.GetJwtConfig()
	token := subject.Create(utils.GetSnowFlakeStrID()).Dev("APP").Generate(config)
	secret := jwt.GetTokenSecret(token, config.TokenKey)
	return self.Json(ctx, map[string]interface{}{"token": token, "secret": secret})
	//return self.Html(ctx, "/web/index.html", map[string]interface{}{"tewt": 1})
}

func (self *MyWebNode) pubkey(ctx *node.Context) error {
	//testCallRPC()
	return self.Text(ctx, ctx.ServerTLS.PubkeyBase64)
}

var local = new(cache.LocalMapManager).NewCache(30, 10)
var limiter = rate.NewRateLimiter(rate.Option{Limit: 100, Bucket: 200, Expire: 30, Distributed: true})

func GetCacheAware(ds ...string) (cache.Cache, error) {
	return local, nil
}

type NewPostFilter struct{}

func (self *NewPostFilter) DoFilter(chain node.Filter, ob *node.HttpNode, handle func(*node.Context) error, args ...interface{}) error {
	//fmt.Println(" --- NewFilter.DoFilter before ---")
	ctx := ob.Context
	ctx.AddStorage("httpLog", node.HttpLog{Method: ctx.Path, LogNo: utils.GetSnowFlakeStrID(), CreateAt: utils.Time()})
	if err := chain.DoFilter(chain, ob, handle, args...); err != nil {
		return err
	}
	//fmt.Println(" --- NewFilter.DoFilter after ---")
	v := ctx.GetStorage("httpLog")
	if v == nil {
		return utils.Error("httpLog is nil")
	}
	httpLog, _ := v.(node.HttpLog)
	httpLog.UpdateAt = utils.Time()
	httpLog.CostMill = httpLog.UpdateAt - httpLog.CreateAt
	//zlog.Info("http log", 0, zlog.Any("data", httpLog))
	return nil
}

func NewHTTP() *MyWebNode {
	var my = &MyWebNode{}
	my.CacheAware = GetCacheAware
	my.AddJwtConfig(jwt.JwtConfig{
		TokenTyp: jwt.JWT,
		TokenAlg: jwt.HS256,
		TokenKey: "123456" + utils.CreateLocalSecretKey(12, 45, 23, 60, 58, 30),
		TokenExp: jwt.TWO_WEEK,
	})
	my.AddFilter(&node.FilterObject{Name: "NewPostFilter", Order: 100, Filter: &NewPostFilter{}})
	return my
}

func StartHttpNode() {
	my := NewHTTP()
	my.POST("/test1", my.test, nil)
	my.POST("/test2", my.getUser, &node.RouterConfig{})
	my.GET("/pubkey", my.pubkey, &node.RouterConfig{Guest: true})
	my.POST("/login", my.login, &node.RouterConfig{Login: true})
	my.StartServer(":8090")
}
