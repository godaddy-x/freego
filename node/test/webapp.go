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
	"github.com/godaddy-x/freego/utils/jwt"
	"github.com/godaddy-x/freego/utils/sdk"
	"strings"
	"time"
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
	//config := ctx.GetJwtConfig()
	//token := ctx.Subject.Create(utils.NextSID()).Dev("APP").Generate(config)
	//secret := jwt.GetTokenSecret(token, config.TokenKey)
	//if ctx.Encipher == nil {
	//	return ex.Throw{Msg: "encipher is nil"}
	//}
	data, err := ctx.Encipher.TokenCreate(utils.AddStr(utils.NextSID(), ";", "WEB"))
	if err != nil {
		return ex.Throw{Msg: "create token fail", Err: err}
	}
	part := strings.Split(data, ";")
	expired, _ := utils.StrToInt64(part[2])
	return self.Json(ctx, &sdk.AuthToken{
		Token:   part[0],
		Secret:  part[1],
		Expired: expired,
	})
	//return self.Html(ctx, "/web/index.html", map[string]interface{}{"tewt": 1})
}

func (self *MyWebNode) publicKey(ctx *node.Context) error {
	//testCallRPC()
	//_, publicKey := ctx.RSA.GetPublicKey()
	publicKey, err := ctx.Encipher.Config("ecdsa")
	if err != nil {
		return ex.Throw{Msg: "publicKey is nil", Err: err}
	}
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

func createEncipher(addr string) *node.EncipherClient {
	if len(addr) == 0 {
		panic("encipher host is nil")
	}
	client := node.NewEncipherClient(addr)
	for {
		if err := client.CheckReady(); err != nil {
			time.Sleep(2 * time.Second)
			continue
		}
		break
	}
	return client
}

func NewHTTP() *MyWebNode {
	var my = &MyWebNode{}
	my.EnableECC(true)
	my.AddJwtConfig(jwt.JwtConfig{
		TokenTyp: jwt.JWT,
		TokenAlg: jwt.HS256,
		TokenKey: "1234567890",
		TokenExp: jwt.TWO_WEEK,
	})
	//encipher := node.NewEncipherClient("http://localhost:4141")
	//ecdsa, err := encipher.Config("ecdsa")
	//if err != nil {
	//	panic(err)
	//}
	//privateKey := utils.GetJsonString(utils.Str2Bytes(ecdsa), "PrivateKey")
	//if len(privateKey) == 0 {
	//	panic("ecdsa privateKey is nil")
	//}
	//cipher := &crypto.EccObj{}
	//if err := cipher.LoadS256ECC(privateKey); err != nil {
	//	panic("ECC certificate generation failed")
	//}
	//cipher := crypto.NewEccObject()
	//my.AddCipher(cipher)
	//my.AddCache(func(ds ...string) (cache.Cache, error) {
	//	rds, err := cache.NewRedis(ds...)
	//	return rds, err
	//})
	my.SetEncipher(createEncipher("http://localhost:4141"))
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
	my.POST("/getUser", my.getUser, &node.RouterConfig{AesResponse: false})
	my.POST("/testGuestPost", my.testGuestPost, &node.RouterConfig{Guest: true})
	my.GET("/key", my.publicKey, &node.RouterConfig{Guest: true})
	my.POST("/login", my.login, &node.RouterConfig{UseRSA: true})

	my.POST("/geetest/register", my.FirstRegister, &node.RouterConfig{UseRSA: true})
	my.POST("/geetest/validate", my.SecondValidate, &node.RouterConfig{UseRSA: true})

	//my.POST("/geetest/validate", func(ctx *node.Context) error {
	//	proxy := &fasthttp.HostClient{
	//		Addr: "b-service:8081",
	//	}
	//	return my.ProxyRequest(ctx, proxy, "", "")
	//}, &node.RouterConfig{Guest: true})

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
