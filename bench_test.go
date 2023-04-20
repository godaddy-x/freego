package main

import (
	"fmt"
	"github.com/godaddy-x/freego/ormx/sqlc"
	"github.com/godaddy-x/freego/ormx/sqld"
	"github.com/godaddy-x/freego/utils"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

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
	return nil
}

type OwWallet2 struct {
	Id           primitive.ObjectID `json:"id" bson:"_id"`
	AppID        string             `json:"appID" bson:"appID"`
	WalletID     string             `json:"walletID" bson:"walletID"`
	Alias        string             `json:"alias" bson:"alias"`
	IsTrust      int64              `json:"isTrust" bson:"isTrust"`
	PasswordType int64              `json:"passwordType" bson:"passwordType"`
	Password     string             `json:"password" bson:"password"`
	AuthKey      string             `json:"authKey" bson:"authKey"`
	RootPath     string             `json:"rootPath" bson:"rootPath"`
	AccountIndex int64              `json:"accountIndex" bson:"accountIndex"`
	Keystore     string             `json:"keystore" bson:"keystore"`
	Applytime    int64              `json:"applytime" bson:"applytime"`
	Succtime     int64              `json:"succtime" bson:"succtime"`
	Dealstate    int64              `json:"dealstate" bson:"dealstate"`
	Ctime        int64              `json:"ctime" bson:"ctime"`
	Utime        int64              `json:"utime" bson:"utime"`
	State        int64              `json:"state" bson:"state"`
}

func (o *OwWallet2) GetTable() string {
	return "ow_wallet2"
}

func (o *OwWallet2) NewObject() sqlc.Object {
	return &OwWallet2{}
}

func (o *OwWallet2) NewIndex() []sqlc.Index {
	return nil
}

type OwBlock struct {
	Id                int64  `json:"id" bson:"_id"`
	Hash              string `json:"hash" bson:"hash"`
	Confirmations     string `json:"confirmations" bson:"confirmations"`
	Merkleroot        string `json:"merkleroot" bson:"merkleroot"`
	Previousblockhash string `json:"previousblockhash" bson:"previousblockhash"`
	Height            int64  `json:"height" bson:"height"`
	Version           int64  `json:"version" bson:"version"`
	Time              int64  `json:"time" bson:"time"`
	Fork              string `json:"fork" bson:"fork"`
	Symbol            string `json:"symbol" bson:"symbol"`
	Ctime             int64  `json:"ctime" bson:"ctime"`
	Utime             int64  `json:"utime" bson:"utime"`
	State             int64  `json:"state" bson:"state"`
}

func (o *OwBlock) GetTable() string {
	return "ow_block"
}

func (o *OwBlock) NewObject() sqlc.Object {
	return &OwBlock{}
}

func (o *OwBlock) NewIndex() []sqlc.Index {
	return nil
}

type OwAuth struct {
	Id     int64  `json:"id" bson:"_id" auto:"true"`
	Secret string `json:"secret" bson:"secret"`
	Seed   string `json:"seed" bson:"seed"`
	Ctime  int64  `json:"cTime" bson:"cTime" date:"true"`
	Utime  int64  `json:"uTime" bson:"uTime" date:"true"`
	State  int64  `json:"state" bson:"state"`
}

func (o *OwAuth) GetTable() string {
	return "admin_auth_test"
}

func (o *OwAuth) NewObject() sqlc.Object {
	return &OwAuth{}
}

func (o *OwAuth) NewIndex() []sqlc.Index {
	return nil
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

func initDriver() {
	sqld.ModelDriver(
		&OwWallet{},
		&OwWallet2{},
		&OwBlock{},
		&OwContract{},
		&OwAuth{},
	)
}

func init() {

	// 注册对象
	//sqld.ModelDriver(
	//	sqld.NewHook(func() interface{} { return &OwWallet{} }, func() interface{} { return &[]*OwWallet{} }),
	//)
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
//		//l := utils.UnixMilli()
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
//		//l := utils.UnixMilli()
//		wallet := OwWallet{}
//		if err := db.FindOne(sqlc.M().Eq("id", 1109819683034365953), &wallet); err != nil {
//			fmt.Println(err)
//		}
//		//fmt.Println("cost: ", utils.UnixMilli()-l)
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
