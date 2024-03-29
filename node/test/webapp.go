package http_web

import (
	"fmt"
	"github.com/godaddy-x/freego/ex"
	"github.com/godaddy-x/freego/geetest"
	"github.com/godaddy-x/freego/node"
	"github.com/godaddy-x/freego/node/common"
	"github.com/godaddy-x/freego/rpcx"
	"github.com/godaddy-x/freego/rpcx/pb"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/utils/crypto"
	"github.com/godaddy-x/freego/utils/jwt"
	"github.com/godaddy-x/freego/utils/sdk"
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
	conn, err := rpcx.NewClientConn(rpcx.GRPC{Service: "PubWorker", Cache: 30})
	if err != nil {
		fmt.Println(err)
		return
	}
	defer conn.Close()
	res, err := pb.NewPubWorkerClient(conn.Value()).GenerateId(conn.Context(), &pb.GenerateIdReq{})
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("call rpc:", res)
}

func (self *MyWebNode) login(ctx *node.Context) error {
	//fmt.Println("-----", ctx.GetHeader("Language"))
	//fmt.Println("-----", ctx.GetPostBody())
	//// {"test":"测试$1次 我是$4岁"}
	//return ex.Throw{Msg: "${test}", Arg: []string{"1", "2", "123", "99"}}
	//self.LoginBySubject(subject, exp)
	config := ctx.GetJwtConfig()
	token := ctx.Subject.Create(utils.NextSID()).Dev("APP").Generate(config)
	secret := jwt.GetTokenSecret(token, config.TokenKey)
	return self.Json(ctx, &sdk.AuthToken{
		Token:   token,
		Secret:  secret,
		Expired: ctx.Subject.Payload.Exp,
	})
	//return self.Html(ctx, "/web/index.html", map[string]interface{}{"tewt": 1})
}

func (self *MyWebNode) publicKey(ctx *node.Context) error {
	//testCallRPC()
	_, publicKey := ctx.RSA.GetPublicKey()
	return self.Text(ctx, publicKey)
}

func (self *MyWebNode) testGuestPost(ctx *node.Context) error {
	fmt.Println(string(ctx.JsonBody.RawData()))
	return self.Json(ctx, map[string]string{"res": "中文测试下Guest响应"})
}

func (self *MyWebNode) testHAX(ctx *node.Context) error {
	fmt.Println(string(ctx.JsonBody.RawData()))
	return self.Json(ctx, map[string]string{"res": "中文测试下HAX响应"})
}

func (self *MyWebNode) FirstRegister(ctx *node.Context) error {
	res, err := geetest.FirstRegister(ctx)
	if err != nil {
		return err
	}
	return self.Json(ctx, res)
}

func (self *MyWebNode) SecondValidate(ctx *node.Context) error {
	res, err := geetest.SecondValidate(ctx)
	if err != nil {
		return err
	}
	return self.Json(ctx, res)
}

type NewPostFilter struct{}

func (self *NewPostFilter) DoFilter(chain node.Filter, ctx *node.Context, args ...interface{}) error {
	//fmt.Println(" --- NewFilter.DoFilter before ---")
	ctx.AddStorage("httpLog", node.HttpLog{Method: ctx.Path, LogNo: utils.NextSID(), CreateAt: utils.UnixMilli()})
	if err := chain.DoFilter(chain, ctx, args...); err != nil {
		return err
	}
	//fmt.Println(" --- NewFilter.DoFilter after ---")
	v := ctx.GetStorage("httpLog")
	if v == nil {
		return utils.Error("httpLog is nil")
	}
	httpLog, _ := v.(node.HttpLog)
	httpLog.UpdateAt = utils.UnixMilli()
	httpLog.CostMill = httpLog.UpdateAt - httpLog.CreateAt
	//zlog.Info("http log", 0, zlog.Any("data", httpLog))
	return nil
}

type GeetestFilter struct{}

func (self *GeetestFilter) DoFilter(chain node.Filter, ctx *node.Context, args ...interface{}) error {
	// TODO 读取自定义需要拦截的方法名+手机号码或账号
	username := utils.GetJsonString(ctx.JsonBody.RawData(), "username")
	filterObject := geetest.CreateFilterObject(ctx.Method, username)
	if !geetest.ValidSuccess(filterObject) {
		return utils.Error("geetest invalid")
	}
	if err := chain.DoFilter(chain, ctx, args...); err != nil {
		return err
	}
	return geetest.CleanStatus(filterObject)
}

type TestFilter struct{}

func (self *TestFilter) DoFilter(chain node.Filter, ctx *node.Context, args ...interface{}) error {
	ctx.Json(map[string]string{"tttt": "22222"})
	//return utils.Error("11111")
	return chain.DoFilter(chain, ctx, args...)
}

const (
	privateKey = "MHcCAQEEIEpXwxicdbb4DM+EW/cJVvoTSubRHIKB6kai/1qgaWnNoAoGCCqGSM49AwEHoUQDQgAEo2hpVqkCUrLC/mxd9qD8sdryanqx0YVfpAfN9ciMGiOSgJ8KBmDpE8FfAtRSk8PM4Le6EMLrQQLPaLURshOwZg=="
)

func roleRealm(ctx *node.Context, onlyRole bool) (*node.Permission, error) {
	permission := &node.Permission{}
	if onlyRole {
		permission.HasRole = []int64{1, 2, 3, 4}
		return permission, nil
	}
	//permission.Ready = true
	//permission.MatchAll = true
	permission.NeedRole = []int64{2, 3, 4}
	return permission, nil
}

func NewHTTP() *MyWebNode {
	var my = &MyWebNode{}
	my.EnableECC(true)
	my.AddJwtConfig(jwt.JwtConfig{
		TokenTyp: jwt.JWT,
		TokenAlg: jwt.HS256,
		TokenKey: "123456" + utils.CreateLocalSecretKey(12, 45, 23, 60, 58, 30),
		TokenExp: jwt.TWO_WEEK,
	})
	cipher := &crypto.EccObj{}
	if err := cipher.LoadS256ECC(privateKey); err != nil {
		panic("ECC certificate generation failed")
	}
	my.AddCipher(cipher)
	//my.AddCache(func(ds ...string) (cache.Cache, error) {
	//	rds, err := cache.NewRedis(ds...)
	//	return rds, err
	//})
	my.SetSystem("test", "1.0.0")
	my.AddRoleRealm(roleRealm)
	my.AddErrorHandle(func(ctx *node.Context, throw ex.Throw) error {
		fmt.Println(throw)
		return throw
	})
	my.AddFilter(&node.FilterObject{Name: "TestFilter", Order: 100, Filter: &TestFilter{}, MatchPattern: []string{"/getUser"}})
	my.AddFilter(&node.FilterObject{Name: "NewPostFilter", Order: 100, Filter: &NewPostFilter{}})
	my.AddFilter(&node.FilterObject{Name: "GeetestFilter", Order: 101, MatchPattern: []string{"/TestGeetest"}, Filter: &GeetestFilter{}})
	return my
}

func StartHttpNode() {
	go geetest.CheckServerStatus(geetest.Config{})
	my := NewHTTP()
	my.POST("/test1", my.test, nil)
	my.POST("/getUser", my.getUser, nil)
	my.POST("/testGuestPost", my.testGuestPost, &node.RouterConfig{Guest: true})
	my.POST("/testHAX", my.testHAX, &node.RouterConfig{UseHAX: true})
	my.GET("/key", my.publicKey, &node.RouterConfig{Guest: true})
	my.POST("/login", my.login, &node.RouterConfig{UseRSA: true})

	my.POST("/geetest/register", my.FirstRegister, &node.RouterConfig{UseRSA: true})
	my.POST("/geetest/validate", my.SecondValidate, &node.RouterConfig{UseRSA: true})

	my.AddLanguageByJson("en", []byte(`{"test":"测试$1次 我是$4岁"}`))
	my.StartServer(":8090")
}

func StartHttpNode1() {
	go geetest.CheckServerStatus(geetest.Config{})
	my := NewHTTP()
	my.POST("/test1", my.test, nil)
	my.StartServer(":8091")
}

func StartHttpNode2() {
	go geetest.CheckServerStatus(geetest.Config{})
	my := NewHTTP()
	my.POST("/test1", my.test, nil)
	my.StartServer(":8092")
}
