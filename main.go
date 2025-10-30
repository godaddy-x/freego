package main

import (
	"fmt"
	"github.com/godaddy-x/freego/ormx/sqlc"
	"github.com/godaddy-x/freego/ormx/sqld"
	"net/http"
	_ "net/http/pprof"

	"github.com/godaddy-x/freego/cache"
	rate "github.com/godaddy-x/freego/cache/limiter"
	ballast "github.com/godaddy-x/freego/gc"
	http_web "github.com/godaddy-x/freego/node/test"
	"github.com/godaddy-x/freego/rpcx"
	"github.com/godaddy-x/freego/utils"
	_ "go.uber.org/automaxprocs"
)

func http_test() {
	//go http_web.StartHttpNode1()
	//go http_web.StartHttpNode2()
	// sqld.RebuildMongoDBIndex()
	http_web.StartHttpNode()
}

type OwWallet struct {
	Id           int64  `json:"id" bson:"_id"`
	AppID        string `json:"appID" bson:"appID"`
	WalletID     string `json:"walletID" bson:"walletID"`
	Alias        string `json:"alias" bson:"alias"`
	IsTrust      int64  `json:"isTrust" bson:"isTrust"`
	PasswordType int64  `json:"passwordType" bson:"passwordType"`
	Password     string `json:"password" bson:"password"`
	AuthKey      string `json:"authKey" bson:"authKey"`
	RootPath     string `json:"rootPath" bson:"rootPath"`
	AccountIndex int64  `json:"accountIndex" bson:"accountIndex"`
	Keystore     string `json:"keyJson" bson:"keyJson"`
	Applytime    int64  `json:"applytime" bson:"applytime"`
	Succtime     int64  `json:"succtime" bson:"succtime"`
	Dealstate    int64  `json:"dealstate" bson:"dealstate"`
	Ctime        int64  `json:"ctime" bson:"ctime"`
	Utime        int64  `json:"utime" bson:"utime"`
	State        int64  `json:"state" bson:"state"`
}

func (o *OwWallet) GetTable() string {
	return "ow_wallet"
}

func (o *OwWallet) NewObject() sqlc.Object {
	return &OwWallet{}
}

func (o *OwWallet) NewIndex() []sqlc.Index {
	appID := sqlc.Index{Name: "appID", Key: []string{"appID"}}
	return []sqlc.Index{appID}
}

func initConsul() {
	conf := rpcx.ConsulConfig{}
	if err := utils.ReadLocalJsonConfig("resource/consul.json", &conf); err != nil {
		panic(utils.AddStr("读取consul配置失败: ", err.Error()))
	}
	new(rpcx.ConsulManager).InitConfig(conf)
}

func initRedis() {
	conf := cache.RedisConfig{}
	if err := utils.ReadLocalJsonConfig("resource/redis.json", &conf); err != nil {
		panic(utils.AddStr("读取redis配置失败: ", err.Error()))
	}
	new(cache.RedisManager).InitConfig(conf)
}

func initMysqlDB() {
	conf := sqld.MysqlConfig{}
	if err := utils.ReadLocalJsonConfig("resource/mysql.json", &conf); err != nil {
		panic(utils.AddStr("读取mysql配置失败: ", err.Error()))
	}
	new(sqld.MysqlManager).InitConfigAndCache(nil, conf)
	fmt.Println("init mysql success")

	initDriver()
}

func initDriver() {
	//sqld.ModelTime(time.UTC, utils.TimeFmt2, utils.DateFmt)
	sqld.ModelDriver(
		&OwWallet{},
	)
}

var appConfig = rpcx.AppConfig{}

func initGRPC() {
	if err := utils.ReadLocalJsonConfig("resource/app.json", &appConfig); err != nil {
		panic(err)
	}
	client := &rpcx.GRPCManager{}
	client.CreateJwtConfig(appConfig.AppKey)
	client.CreateAppConfigCall(func(appId string) (rpcx.AppConfig, error) {
		if appId == appConfig.AppId {
			return appConfig, nil
		}
		return rpcx.AppConfig{}, utils.Error("appId invalid")
	})
	client.CreateRateLimiterCall(func(method string) (rate.Option, error) {
		return rate.Option{}, nil
	})
	client.CreateServerTLS(rpcx.TlsConfig{
		UseMTLS:   true,
		CACrtFile: "./rpcx/cert/ca.crt",
		KeyFile:   "./rpcx/cert/server.key",
		CrtFile:   "./rpcx/cert/server.crt",
	})
	client.CreateClientTLS(rpcx.TlsConfig{
		UseMTLS:   true,
		CACrtFile: "./rpcx/cert/ca.crt",
		KeyFile:   "./rpcx/cert/client.key",
		CrtFile:   "./rpcx/cert/client.crt",
		HostName:  "localhost",
	})
	client.CreateAuthorizeTLS("./rpcx/cert/server.key")
}

func init() {
	//initConsul()
	//initRedis()
	//initGRPC()
}

func main() {
	initMysqlDB()
	ballast.GC(512*ballast.MB, 30)
	go func() {
		_ = http.ListenAndServe(":8849", nil)
	}()
	//node.SetLocalSecret(utils.RandStr(24))
	//rpcx.RunClient(appConfig.AppId)
	http_test()
	//router := fasthttprouter.New()
	//router.GET("/pubkey", func(ctx *fasthttp.RequestCtx) {
	//	ctx.WriteString("LS0tLS1CRUdJTiBSU0EgUFVCTElDSyBLRVktLS0tLQpNSUdKQW9HQkFMK2hpYkw5S3hpb2JNOVRPbmx6cXN0WnhPSy9rU2JQQzMzSmpoVTdjbklUbXlRaThuaXZiUG5wCncwOUo5N0p4aDdxY0tOWVhpakxRdTZxei9xUFNXZ0pYaU9qOWhoc2E0bEdlNVVkRkJtaFpxZ2V3R1J6ckJJNEkKRFNqZk1xcDNCM3puV1h1VnBaSFZNYStJOFBDc1A5dEd3dzdPS2hzRFI0bmp3L3Z2UXdERkFnTUJBQUU9Ci0tLS0tRU5EIFJTQSBQVUJMSUNLIEtFWS0tLS0tCg==")
	//})
	//fasthttp.ListenAndServe(":8090", router.Handler)
}
