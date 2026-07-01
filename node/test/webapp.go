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
	"github.com/godaddy-x/freego/zlog"
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
	// ML-DSA-87（种子 freego-mldsa-server / freego-mldsa-client，与根目录 test_pq_keys.go 一致）
	serverPrk = "EULcPhJxYhic7aezibVaYMyw02R+F7atiY2BvECu4ho="
	serverPub = "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"
	clientPrk = "dYa9TRsihNJZDwLrjd5+C6Ezkv4rN8invwWf6TUS6V0="
	clientPub = "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"
)

func roleRealm(ctx *node.Context) (*node.Permission, error) {
	permission := &node.Permission{}
	//permission.Ready = true
	//permission.MatchAll = true
	permission.NeedRole = []int64{2, 3, 4}
	permission.HasRole = []int64{1, 2, 3, 4}
	return permission, nil
}

func NewHTTP() *MyWebNode {
	var my = &MyWebNode{}
	my.AddJwtConfig(jwt.JwtConfig{
		TokenTyp: jwt.JWT,
		TokenAlg: jwt.HS256,
		TokenKey: "123456",
		TokenExp: jwt.TWO_WEEK * 10,
	})

	_ = my.AddCipherHook(func(usr int64) (crypto.Cipher, error) {
		return crypto.CreateMLDSA87WithBase64(serverPrk, clientPub)
	})

	//my.AddRedisCache(func(ds ...string) (cache.Cache, error) {
	//	rds, err := cache.NewRedis(ds...)
	//	return rds, err
	//})
	my.SetSystem("test", "1.0.0")
	my.SetAcceptTimeout(600)

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
	my.POST("/getUser", my.getUser, &node.RouterConfig{AesRequest: false, AesResponse: false})
	my.POST("/testGuestPost", my.testGuestPost, &node.RouterConfig{Guest: true})
	my.POST("/key", my.publicKey, &node.RouterConfig{Guest: true})
	my.POST("/login", my.login, &node.RouterConfig{UsePlan2: true})

	my.POST("/geetest/register", my.FirstRegister, &node.RouterConfig{UsePlan2: true})
	my.POST("/geetest/validate", my.SecondValidate, &node.RouterConfig{UsePlan2: true})

	// 配置Rate Limiter示例
	configureRateLimiters(my)

	my.AddLanguageByJson("en", []byte(`{"test":"测试$1次 我是$4岁"}`))
	my.StartServer(":8090")
}

func StartHttpNode1() {
	my := NewHTTP()
	my.POST("/test1", my.test, nil)
	my.StartServer(":8091")
}

func StartHttpNode2() {
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
	zlog.Info("✅ Gateway rate limiter configured: 500 QPS, 2500 bucket", 0)

	// 3. 覆盖默认方法级限流器配置（API接口保护）
	my.SetDefaultMethodRateLimiter(rate.Option{
		Limit:       50,    // 每秒50个请求（适合一般API接口）
		Bucket:      100,   // 桶容量100
		Expire:      30000, // 30秒过期时间
		Distributed: true,
	})
	zlog.Info("✅ Default method rate limiter configured: 50 QPS, 100 bucket", 0)

	// 4. 为敏感接口设置专用限流器配置
	// 登录接口：限制更严格，防止暴力破解
	my.SetMethodRateLimiterByPath("/login", rate.Option{
		Limit:       10,    // 每秒10个请求（登录接口限制严格）
		Bucket:      20,    // 桶容量20
		Expire:      30000, // 30秒过期时间
		Distributed: true,
	})
	zlog.Info("✅ Login endpoint rate limiter configured: 10 QPS, 20 bucket", 0)

	// 用户信息接口：中等限制
	my.SetMethodRateLimiterByPath("/getUser", rate.Option{
		Limit:       30,    // 每秒30个请求
		Bucket:      60,    // 桶容量60
		Expire:      30000, // 30秒过期时间
		Distributed: true,
	})
	zlog.Info("✅ User info endpoint rate limiter configured: 30 QPS, 60 bucket", 0)

	// 公开接口：相对宽松
	my.SetMethodRateLimiterByPath("/key", rate.Option{
		Limit:       100,   // 每秒100个请求（公开接口相对宽松）
		Bucket:      200,   // 桶容量200
		Expire:      30000, // 30秒过期时间
		Distributed: true,
	})
	zlog.Info("✅ Public key endpoint rate limiter configured: 100 QPS, 200 bucket", 0)

	// 5. 设置用户级限流器配置（防止单个用户刷接口）
	my.SetUserRateLimiter(rate.Option{
		Limit:       5,     // 每个用户每秒5个请求
		Bucket:      10,    // 桶容量10（允许少量突发）
		Expire:      30000, // 30秒过期时间
		Distributed: true,
	})
	zlog.Info("✅ User-level rate limiter configured: 5 QPS per user, 10 bucket", 0)

	// 6. 动态调整示例（可在运行时调用）
	// 业务高峰期：提高网关限流阈值
	// my.SetGatewayRateLimiter(rate.Option{Limit: 800, Bucket: 4000, Expire: 60000, Distributed: true})

	// 活动期间：降低用户级限制
	// my.SetUserRateLimiter(rate.Option{Limit: 10, Bucket: 20, Expire: 30000, Distributed: true})

	// 维护期间：严格限制所有接口
	// my.SetGatewayRateLimiter(rate.Option{Limit: 10, Bucket: 50, Expire: 60000, Distributed: true})

	//fmt.Println("🎉 All rate limiters configured successfully!")
	//fmt.Println("📊 Rate limiting hierarchy:")
	//fmt.Println("   🌐 Gateway: 500 QPS (global protection)")
	//fmt.Println("   📍 Methods: 50 QPS default, custom limits per endpoint")
	//fmt.Println("   👤 Users: 5 QPS per user (anti-abuse)")
	//fmt.Println("   🔄 Distributed: Redis-backed (auto-fallback to local)")
}
