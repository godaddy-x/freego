package http_web

import (
	"fmt"
	rate "github.com/godaddy-x/freego/cache/limiter"
	"github.com/godaddy-x/freego/ex"
	"github.com/godaddy-x/freego/geetest"
	"github.com/godaddy-x/freego/node"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/utils/crypto"
	"github.com/godaddy-x/freego/utils/jwt"
	"github.com/godaddy-x/freego/utils/sdk"
)

type MyWebNode struct {
	node.HttpNode
}

func (self *MyWebNode) test(ctx *node.Context) error {
	req := &GetUserReq{}
	if err := ctx.Parser(req); err != nil {
		return err
	}
	//return self.Html(ctx, "/resource/index.html", map[string]interface{}{"tewt": 1})
	return self.Json(ctx, req)
	//return ex.Throw{Code: ex.BIZ, Msg: "测试错误"}
}

func (self *MyWebNode) getUser(ctx *node.Context) error {
	req := &GetUserReq{}
	if err := ctx.Parser(req); err != nil {
		return err
	}
	return self.Json(ctx, req)
}

func (self *MyWebNode) login(ctx *node.Context) error {
	//fmt.Println("-----", ctx.GetHeader("Language"))
	//fmt.Println("-----", ctx.GetPostBody())
	//// {"test":"测试$1次 我是$4岁"}
	//return ex.Throw{Msg: "${test}", Arg: []string{"1", "2", "123", "99"}}
	//self.LoginBySubject(subject, exp)
	config := ctx.GetJwtConfig()
	token := ctx.Subject.Create(utils.NextSID()).Dev("APP").Generate(config)
	secret := ctx.Subject.GetTokenSecret(token, config.TokenKey)
	bs, err := utils.JsonMarshal(&sdk.AuthToken{
		Token:   token,
		Secret:  utils.Base64Encode(secret),
		Expired: ctx.Subject.Payload.Exp,
	})
	if err != nil {
		return err
	}
	return self.Json(ctx, bs)
	//return self.Html(ctx, "/web/index.html", map[string]interface{}{"tewt": 1})
}

func (self *MyWebNode) publicKey(ctx *node.Context) error {
	pub, err := ctx.CreatePublicKey()
	if err != nil {
		return err
	}
	return self.Json(ctx, pub)
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
	//服务端私钥
	serverPrk = "Z4WmI28ILmpqTWM4OISPwzF10BcGF7hsPHoaiH3J1vw="
	//服务端公钥
	serverPub = "BO6XQ+PD66TMDmQXSEHl2xQarWE0HboB4LazrznThhr6Go5SvpjXJqiSe2fX+sup5OQDOLPkLdI1gh48jOmAq+k="
	//客户端私钥
	clientPrk = "rnX5ykQivfbLHtcbPR68CP636usTNC03u8OD1KeoDPg="
	//客户端公钥
	clientPub = "BEZkPpdLSQiUvkaObyDz0ya0figOLphr6L8hPEHbPzpc7sEMtq1lBTfG6IwZdd7WuJmMkP1FRt+GzZgnqt+DRjs="
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
		TokenExp: jwt.TWO_WEEK * 10,
	})

	// 增加双向验签的ECDSA
	cipher, _ := crypto.CreateS256ECDSAWithBase64(serverPrk, clientPub)
	my.AddCipher(1, cipher)

	//my.AddRedisCache(func(ds ...string) (cache.Cache, error) {
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
	// go geetest.CheckServerStatus(geetest.Config{})
	my := NewHTTP()
	my.SetLengthCheck(node.MAX_BODY_LEN*5, 0, 0)
	my.POST("/test1", my.test, nil)
	my.POST("/getUser", my.getUser, &node.RouterConfig{AesRequest: true, AesResponse: true})
	my.POST("/testGuestPost", my.testGuestPost, &node.RouterConfig{Guest: true})
	my.POST("/key", my.publicKey, &node.RouterConfig{Guest: true})
	my.POST("/login", my.login, &node.RouterConfig{UseRSA: true})

	my.POST("/geetest/register", my.FirstRegister, &node.RouterConfig{UseRSA: true})
	my.POST("/geetest/validate", my.SecondValidate, &node.RouterConfig{UseRSA: true})

	// 配置Rate Limiter示例
	configureRateLimiters(my)

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

// configureRateLimiters 配置Rate Limiter示例
// 演示如何在HTTP服务器启动时配置各种级别的限流器
func configureRateLimiters(my *MyWebNode) {
	// 1. 初始化限流器（Redis准备就绪后自动创建分布式限流器）
	// 如果Redis不可用，会自动回退到本地限流器
	// 注意：这里会自动在HttpNode.StartServer()中调用

	// 2. 覆盖默认网关级限流器配置（全局保护）
	// 适用于高流量生产环境，可根据实际QPS调整
	my.SetGatewayRateLimiter(rate.Option{
		Limit:       500,   // 每秒500个请求（生产环境可设置为1000+）
		Bucket:      2500,  // 桶容量2500（支持突发流量）
		Expire:      60000, // 60秒过期时间
		Distributed: true,
	})
	fmt.Println("✅ Gateway rate limiter configured: 500 QPS, 2500 bucket")

	// 3. 覆盖默认方法级限流器配置（API接口保护）
	my.SetDefaultMethodRateLimiter(rate.Option{
		Limit:       50,    // 每秒50个请求（适合一般API接口）
		Bucket:      100,   // 桶容量100
		Expire:      30000, // 30秒过期时间
		Distributed: true,
	})
	fmt.Println("✅ Default method rate limiter configured: 50 QPS, 100 bucket")

	// 4. 为敏感接口设置专用限流器配置
	// 登录接口：限制更严格，防止暴力破解
	my.SetMethodRateLimiterByPath("/login", rate.Option{
		Limit:       10,    // 每秒10个请求（登录接口限制严格）
		Bucket:      20,    // 桶容量20
		Expire:      30000, // 30秒过期时间
		Distributed: true,
	})
	fmt.Println("✅ Login endpoint rate limiter configured: 10 QPS, 20 bucket")

	// 用户信息接口：中等限制
	my.SetMethodRateLimiterByPath("/getUser", rate.Option{
		Limit:       30,    // 每秒30个请求
		Bucket:      60,    // 桶容量60
		Expire:      30000, // 30秒过期时间
		Distributed: true,
	})
	fmt.Println("✅ User info endpoint rate limiter configured: 30 QPS, 60 bucket")

	// 公开接口：相对宽松
	my.SetMethodRateLimiterByPath("/key", rate.Option{
		Limit:       100,   // 每秒100个请求（公开接口相对宽松）
		Bucket:      200,   // 桶容量200
		Expire:      30000, // 30秒过期时间
		Distributed: true,
	})
	fmt.Println("✅ Public key endpoint rate limiter configured: 100 QPS, 200 bucket")

	// 5. 设置用户级限流器配置（防止单个用户刷接口）
	my.SetUserRateLimiter(rate.Option{
		Limit:       5,     // 每个用户每秒5个请求
		Bucket:      10,    // 桶容量10（允许少量突发）
		Expire:      30000, // 30秒过期时间
		Distributed: true,
	})
	fmt.Println("✅ User-level rate limiter configured: 5 QPS per user, 10 bucket")

	// 6. 动态调整示例（可在运行时调用）
	// 业务高峰期：提高网关限流阈值
	// my.SetGatewayRateLimiter(rate.Option{Limit: 800, Bucket: 4000, Expire: 60000, Distributed: true})

	// 活动期间：降低用户级限制
	// my.SetUserRateLimiter(rate.Option{Limit: 10, Bucket: 20, Expire: 30000, Distributed: true})

	// 维护期间：严格限制所有接口
	// my.SetGatewayRateLimiter(rate.Option{Limit: 10, Bucket: 50, Expire: 60000, Distributed: true})

	fmt.Println("🎉 All rate limiters configured successfully!")
	fmt.Println("📊 Rate limiting hierarchy:")
	fmt.Println("   🌐 Gateway: 500 QPS (global protection)")
	fmt.Println("   📍 Methods: 50 QPS default, custom limits per endpoint")
	fmt.Println("   👤 Users: 5 QPS per user (anti-abuse)")
	fmt.Println("   🔄 Distributed: Redis-backed (auto-fallback to local)")
}
