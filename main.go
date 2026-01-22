package main

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"

	rabbitmq "github.com/godaddy-x/freego/amqp"
	"github.com/godaddy-x/freego/cache"
	"github.com/godaddy-x/freego/ormx/sqlc"
	"github.com/godaddy-x/freego/ormx/sqld"

	ballast "github.com/godaddy-x/freego/gc"
	http_web "github.com/godaddy-x/freego/node/test"
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
	Password     []byte `json:"password" bson:"password" ignore:"true"`
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
	return "test_wallet2"
	// return "ow_wallet"
}

func (o *OwWallet) NewObject() sqlc.Object {
	return &OwWallet{}
}

func (o *OwWallet) AppendObject(data interface{}, target sqlc.Object) {
	*data.(*[]*OwWallet) = append(*data.(*[]*OwWallet), target.(*OwWallet))
}

func (o *OwWallet) NewIndex() []sqlc.Index {
	appID := sqlc.Index{Name: "appID", Key: []string{"appID"}}
	return []sqlc.Index{appID}
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

func initMongoDB() {
	conf := sqld.MGOConfig{}
	if err := utils.ReadLocalJsonConfig("resource/mongo.json", &conf); err != nil {
		panic(utils.AddStr("读取mongo配置失败: ", err.Error()))
	}
	new(sqld.MGOManager).InitConfigAndCache(nil, conf)
	fmt.Println("init mongo success")

	initDriver()
}

func initDriver() {
	//sqld.ModelTime(time.UTC, utils.TimeFmt2, utils.DateFmt)
	sqld.ModelDriver(
		&OwWallet{},
	)
}

func initRabbitMQ() {
	conf := rabbitmq.AmqpConfig{}
	if err := utils.ReadLocalJsonConfig("resource/rabbitmq.json", &conf); err != nil {
		panic(utils.AddStr("读取rabbitmq配置失败: ", err.Error()))
	}
	// 创建全局RabbitMQ管理器实例
	mgr := &rabbitmq.PublishManager{}
	if err := mgr.InitConfig(conf); err != nil {
		panic(utils.AddStr("初始化rabbitmq管理器失败: ", err.Error()))
	}
	fmt.Println("init rabbitmq success")
}

func main() {
	initMysqlDB()
	initRedis()
	initRabbitMQ()
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
