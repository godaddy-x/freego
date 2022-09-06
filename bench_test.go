package main

import (
	"fmt"
	"github.com/godaddy-x/freego/ormx/sqld"
	"github.com/godaddy-x/freego/utils"
)

type OwWallet struct {
	Id           int64  `json:"id" bson:"_id" tb:"ow_wallet" mg:"true"`
	AppID        string `json:"appID" bson:"appID"`
	WalletID     string `json:"walletID" bson:"walletID"`
	Alias        string `json:"alias" bson:"alias"`
	IsTrust      int64  `json:"isTrust" bson:"isTrust"`
	PasswordType int64  `json:"passwordType" bson:"passwordType"`
	Password     string `json:"password" bson:"password"`
	AuthKey      string `json:"authKey" bson:"authKey"`
	RootPath     string `json:"rootPath" bson:"rootPath"`
	AccountIndex int64  `json:"accountIndex" bson:"accountIndex"`
	Keystore     string `json:"keystore" bson:"keystore"`
	Applytime    int64  `json:"applytime" bson:"applytime"`
	Succtime     int64  `json:"succtime" bson:"succtime"`
	Dealstate    int64  `json:"dealstate" bson:"dealstate"`
	Ctime        int64  `json:"ctime" bson:"ctime"`
	Utime        int64  `json:"utime" bson:"utime"`
	State        int64  `json:"state" bson:"state"`
}

// go build -gcflags=-m main.go
// go tool pprof -http=":8081" .\cpuprofile.out
// go test bench_test.go -bench .  -benchmem -count=5 -cpuprofile cpuprofile.out -memprofile memprofile.out

func initMysqlDB() {
	conf := sqld.MysqlConfig{}
	if err := utils.ReadLocalJsonConfig("resource/mysql.json", &conf); err != nil {
		panic(utils.AddStr("读取mysql配置失败: ", err.Error()))
	}
	new(sqld.MysqlManager).InitConfigAndCache(nil, conf)
	fmt.Println("init mysql success")
}

func initMongoDB() {
	conf := sqld.MGOConfig{}
	if err := utils.ReadLocalJsonConfig("resource/mongo.json", &conf); err != nil {
		panic(utils.AddStr("读取mongo配置失败: ", err.Error()))
	}
	new(sqld.MGOManager).InitConfigAndCache(nil, conf)
	fmt.Println("init mongo success")
}

func init() {

	// 注册对象
	sqld.ModelDriver(
		sqld.NewHook(func() interface{} { return &OwWallet{} }, func() interface{} { return &[]*OwWallet{} }),
	)
	//initConsul()
	//initMysqlDB()
	//initMongoDB()
	//sqld.ModelDriver(
	//	sqld.Hook{
	//		func() interface{} { return &DxApp{} },
	//		func() interface{} { return &[]*DxApp{} },
	//	},
	//)
	//redis := cache.RedisConfig{}
	//if err := utils.ReadLocalJsonConfig("resource/redis.json", &redis); err != nil {
	//	panic(utils.AddStr("读取redis配置失败: ", err.Error()))
	//}
	//manager, err := new(cache.RedisManager).InitConfig(redis)
	//if err != nil {
	//	panic(err.Error())
	//}
	//manager, err = manager.Client()
	//if err != nil {
	//	panic(err.Error())
	//}
	//mongo1 := sqld.MGOConfig{}
	//if err := utils.ReadLocalJsonConfig("resource/mongo.json", &mongo1); err != nil {
	//	panic(utils.AddStr("读取mongo配置失败: ", err.Error()))
	//}
	//new(sqld.MGOManager).InitConfigAndCache(nil, mongo1)
	//opts := &options.ClientOptions{Hosts: []string{"192.168.27.124:27017"}}
	//// opts.SetAuth(options.Credential{AuthMechanism: "SCRAM-SHA-1", AuthSource: "test", Username: "test", Password: "123456"})
	//client, err := mongo.Connect(context.Background(), opts)
	//if err != nil {
	//	fmt.Println(err)
	//	return
	//}
	//MyClient = client
}

//func BenchmarkSave(b *testing.B) {
//	b.StopTimer()  //调用该函数停止压力测试的时间计数go test -run="webbench_test.go" -test.bench="."*
//	b.StartTimer() //重新开始时间
//	for i := 0; i < b.N; i++ { //use b.N for looping
//		db, err := new(sqld.MGOManager).Get()
//		if err != nil {
//			panic(err)
//		}
//		defer db.Close()
//		//l := utils.Time()
//		o := OwWallet{
//			AppID:    utils.GetSnowFlakeStrID(),
//			WalletID: utils.GetSnowFlakeStrID(),
//		}
//		if err := db.Save(&o); err != nil {
//			fmt.Println(err)
//		}
//	}
//	//fmt.Println(wallet.Id)
//}

//func BenchmarkFind(b *testing.B) {
//	b.StopTimer()  //调用该函数停止压力测试的时间计数go test -run="webbench_test.go" -test.bench="."*
//	b.StartTimer() //重新开始时间
//	for i := 0; i < b.N; i++ { //use b.N for looping
//		db, err := new(sqld.MGOManager).Get(sqld.Option{OpenTx: true})
//		if err != nil {
//			panic(err)
//		}
//		defer db.Close()
//		o := OwWallet{}
//		if err := db.FindOne(sqlc.M(&OwWallet{}).Eq("id", 1110012978914131972), &o); err != nil {
//			fmt.Println(err)
//		}
//		//fmt.Println(wallet.Id)
//	}
//}
//
//func BenchmarkUpdateBy(b *testing.B) {
//	b.StopTimer()  //调用该函数停止压力测试的时间计数go test -run="webbench_test.go" -test.bench="."*
//	b.StartTimer() //重新开始时间
//	for i := 0; i < b.N; i++ { //use b.N for looping
//		db, err := new(sqld.MGOManager).Get(sqld.Option{OpenTx: true})
//		if err != nil {
//			panic(err)
//		}
//		defer db.Close()
//		if err := db.UpdateByCnd(sqlc.M(&OwWallet{}).Eq("id", 1110012978914131972).UpdateKeyValue([]string{"appID", "ctime"}, utils.GetSnowFlakeStrID(), 1)); err != nil {
//			fmt.Println(err)
//		}
//		//fmt.Println(wallet.Id)
//	}
//}

//func BenchmarkMysqlFindOne(b *testing.B) {
//	b.StopTimer()  //调用该函数停止压力测试的时间计数go test -run="webbench_test.go" -test.bench="."*
//	b.StartTimer() //重新开始时间
//	for i := 0; i < b.N; i++ { //use b.N for looping
//		db, err := new(sqld.MysqlManager).Get(sqld.Option{OpenTx: false})
//		if err != nil {
//			panic(err)
//		}
//		defer db.Close()
//		//l := utils.Time()
//		wallet := OwWallet{}
//		if err := db.FindOne(sqlc.M().Eq("id", 1109819683034365953), &wallet); err != nil {
//			fmt.Println(err)
//		}
//		//fmt.Println("cost: ", utils.Time()-l)
//	}
//}
//
//func BenchmarkMongoFindOne(b *testing.B) {
//	b.StopTimer()  //调用该函数停止压力测试的时间计数go test -run="webbench_test.go" -test.bench="."*
//	b.StartTimer() //重新开始时间
//	for i := 0; i < b.N; i++ { //use b.N for looping
//		db, err := new(sqld.MGOManager).Get()
//		if err != nil {
//			panic(err)
//		}
//		defer db.Close()
//		o := &OwWallet{}
//		if err := db.FindOne(sqlc.M().Eq("id", 1182663723102240768), o); err != nil {
//			fmt.Println(err)
//		}
//	}
//}

//func BenchmarkConsulxCallRPC(b *testing.B) {
//	mgr, err := new(consul.ConsulManager).Client()
//	if err != nil {
//		panic(err)
//	}
//
//	req := &ReqObj{AesObj{"456789"}, 123, "托尔斯泰"}
//	res := &ResObj{}
//
//	if err := mgr.CallRPC(&consul.CallInfo{
//		Package:  "mytest",
//		Service:  "UserServiceImpl",
//		Method:   "FindUser",
//		Request:  req,
//		Response: res,
//	}); err != nil {
//		fmt.Println(err)
//		return
//	}
//	//fmt.Println("grpc result: ", res)
//}
