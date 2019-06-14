package main

import (
	"errors"
	"fmt"
	"github.com/godaddy-x/freego/cache"
	"github.com/godaddy-x/freego/component/auth"
	"github.com/godaddy-x/freego/component/log"
	"github.com/godaddy-x/freego/ex"
	"github.com/godaddy-x/freego/sqlc"
	"github.com/godaddy-x/freego/sqld"
	"github.com/godaddy-x/freego/util"
	"github.com/gorilla/websocket"
	"net/url"
	"testing"
	"time"
)

type User struct {
	Id       int64  `json:"id" bson:"_id" tb:"rbac_user"`
	Username string `json:"username" bson:"username"`
	Password string `json:"password" bson:"password"`
	Uid      string `json:"uid" bson:"uid"`
	Utype    int8   `json:"utype" bson:"utype"`
}

type DxApp struct {
	Id        int64  `json:"id" bson:"_id" tb:"dx_app"`
	Name      string `json:"name" bson:"name"`
	Signature string `json:"signature" bson:"signature"`
	Secretkey string `json:"secretkey" bson:"secretkey"`
	Balance   int64  `json:"balance" bson:"balance"`
	Usestate  int64  `json:"usestate" bson:"usestate"`
	Ctime     int64  `json:"ctime" bson:"ctime"`
	Utime     int64  `json:"utime" bson:"utime"`
	State     int64  `json:"state" bson:"state"`
}

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

func init() {
	// 注册对象
	sqld.ModelDriver(
		sqld.Hook{
			func() interface{} { return &OwWallet{} },
			func() interface{} { return &[]*OwWallet{} },
		},
	)
	sqld.ModelDriver(
		sqld.Hook{
			func() interface{} { return &DxApp{} },
			func() interface{} { return &[]*DxApp{} },
		},
	)
	//redis := cache.RedisConfig{}
	//if err := util.ReadLocalJsonConfig("resource/redis.json", &redis); err != nil {
	//	panic(util.AddStr("读取redis配置失败: ", err.Error()))
	//}
	//manager, err := new(cache.RedisManager).InitConfig(redis)
	//if err != nil {
	//	panic(err.Error())
	//}
	//manager, err = manager.Client()
	//if err != nil {
	//	panic(err.Error())
	//}
	mysql := sqld.MysqlConfig{}
	if err := util.ReadLocalJsonConfig("resource/mysql.json", &mysql); err != nil {
		panic(util.AddStr("读取mysql配置失败: ", err.Error()))
	}
	new(sqld.MysqlManager).InitConfigAndCache(nil, mysql)
	mongo1 := sqld.MGOConfig{}
	if err := util.ReadLocalJsonConfig("resource/mongo.json", &mongo1); err != nil {
		panic(util.AddStr("读取mongo配置失败: ", err.Error()))
	}
	new(sqld.MGOManager).InitConfigAndCache(nil, mongo1)
	//opts := &options.ClientOptions{Hosts: []string{"192.168.27.124:27017"}}
	//// opts.SetAuth(options.Credential{AuthMechanism: "SCRAM-SHA-1", AuthSource: "test", Username: "test", Password: "123456"})
	//client, err := mongo.Connect(context.Background(), opts)
	//if err != nil {
	//	fmt.Println(err)
	//	return
	//}
	//MyClient = client
}

func TestMysqlSave(t *testing.T) {
	db, err := new(sqld.MysqlManager).Get(sqld.Option{OpenTx: &sqld.TRUE})
	if err != nil {
		panic(err)
	}
	defer db.Close()
	vs := []interface{}{}
	for i := 0; i < 1; i++ {
		wallet := OwWallet{
			// AppID:        map[string]interface{}{"test": 1},
			WalletID:     util.GetSnowFlakeStrID(),
			PasswordType: 1,
			Password:     "123456",
			RootPath:     "m/44'/88'/0'",
			Alias:        "hello_qtum",
			IsTrust:      0,
			AuthKey:      "",
		}
		vs = append(vs, &wallet)
	}
	l := util.Time()
	if err := db.Save(vs...); err != nil {
		fmt.Println(err)
	}
	fmt.Println("cost: ", util.Time()-l)
}

func TestMysqlUpdate(t *testing.T) {
	db, err := new(sqld.MysqlManager).Get(sqld.Option{OpenTx: &sqld.TRUE})
	if err != nil {
		panic(err)
	}
	defer db.Close()
	vs := []interface{}{}
	for i := 0; i < 10; i++ {
		wallet := OwWallet{
			Id: util.GetSnowFlakeIntID(),
			// AppID:        "123456",
			WalletID:     util.GetSnowFlakeStrID(),
			PasswordType: 1,
			Password:     "123456",
			RootPath:     "m/44'/88'/0'",
			Alias:        "hello_qtum111333",
			IsTrust:      0,
			AuthKey:      "",
		}
		vs = append(vs, &wallet)
	}
	l := util.Time()
	if err := db.Update(vs...); err != nil {
		fmt.Println(err)
	}
	fmt.Println("cost: ", util.Time()-l)
}

func TestMysqlUpdateByCnd(t *testing.T) {
	db, err := new(sqld.MysqlManager).Get(sqld.Option{OpenTx: &sqld.TRUE})
	if err != nil {
		panic(err)
	}
	defer db.Close()
	l := util.Time()
	if err := db.UpdateByCnd(sqlc.M(&OwWallet{}).UpdateKeyValue([]string{"password"}, "1111").Eq("id", 1110371778615574533)); err != nil {
		fmt.Println(err)
	}
	fmt.Println("cost: ", util.Time()-l)
}

func TestMysqlDetele(t *testing.T) {
	db, err := new(sqld.MysqlManager).Get(sqld.Option{OpenTx: &sqld.TRUE})
	if err != nil {
		panic(err)
	}
	defer db.Close()
	vs := []interface{}{}
	for i := 0; i < 2000; i++ {
		wallet := OwWallet{
			Id: util.GetSnowFlakeIntID(),
			//AppID:        map[string]interface{}{"test": 1},
			WalletID:     util.GetSnowFlakeStrID(),
			PasswordType: 1,
			Password:     "123456",
			RootPath:     "m/44'/88'/0'",
			Alias:        "hello_qtum111333",
			IsTrust:      0,
			AuthKey:      "",
		}
		vs = append(vs, &wallet)
	}
	l := util.Time()
	if err := db.Delete(vs...); err != nil {
		fmt.Println(err)
	}
	fmt.Println("cost: ", util.Time()-l)
}

func TestMysqlFindById(t *testing.T) {
	db, err := new(sqld.MysqlManager).Get(sqld.Option{OpenTx: &sqld.FALSE})
	if err != nil {
		panic(err)
	}
	defer db.Close()
	l := util.Time()
	wallet := OwWallet{
		Id: 1124853348080549889,
	}
	if err := db.FindById(&wallet); err != nil {
		fmt.Println(err)
	}
	fmt.Println("cost: ", util.Time()-l)
}

func TestMysqlFindOne(t *testing.T) {
	db, err := new(sqld.MysqlManager).Get(sqld.Option{OpenTx: &sqld.FALSE})
	if err != nil {
		panic(err)
	}
	defer db.Close()
	l := util.Time()
	for i := 0; i < 1; i++ {

	}
	wallet := DxApp{

	}
	if err := db.FindOne(sqlc.M().Eq("id", 8266), &wallet); err != nil {
		fmt.Println(err)
	}
	fmt.Println("cost: ", util.Time()-l)
}

func TestMysqlFindList(t *testing.T) {
	db, err := new(sqld.MysqlManager).Get(sqld.Option{OpenTx: &sqld.FALSE})
	if err != nil {
		panic(err)
	}
	defer db.Close()
	l := util.Time()
	result := []*OwWallet{}
	if err := db.FindList(sqlc.M().Eq("id", 1124853348080549889).Groupby("id").Orderby("id", sqlc.DESC_).Limit(1, 5), &result); err != nil {
		fmt.Println(err)
	}
	fmt.Println("cost: ", util.Time()-l)
}

func TestMysqlFindListComplex(t *testing.T) {
	db, err := new(sqld.MysqlManager).Get(sqld.Option{OpenTx: &sqld.FALSE})
	if err != nil {
		panic(err)
	}
	defer db.Close()
	l := util.Time()
	result := []*OwWallet{}
	if err := db.FindListComplex(sqlc.M(&OwWallet{}).Fields("count(a.id) as id").From("ow_wallet a").Orderby("a.id", sqlc.DESC_).Limit(1, 5), &result); err != nil {
		fmt.Println(err)
	}
	fmt.Println("cost: ", util.Time()-l)
}

func TestMysqlFindOneComplex(t *testing.T) {
	db, err := new(sqld.MysqlManager).Get(sqld.Option{OpenTx: &sqld.FALSE})
	if err != nil {
		panic(err)
	}
	defer db.Close()
	l := util.Time()
	result := OwWallet{}
	if err := db.FindOneComplex(sqlc.M(&OwWallet{}).Fields("count(a.id) as id").From("ow_wallet a").Orderby("a.id", sqlc.DESC_).Limit(1, 5), &result); err != nil {
		fmt.Println(err)
	}
	fmt.Println("cost: ", util.Time()-l)
}

func TestMysqlCount(t *testing.T) {
	db, err := new(sqld.MysqlManager).Get(sqld.Option{OpenTx: &sqld.FALSE})
	if err != nil {
		panic(err)
	}
	defer db.Close()
	l := util.Time()
	if c, err := db.Count(sqlc.M(&OwWallet{}).Orderby("id", sqlc.DESC_).Limit(1, 30)); err != nil {
		fmt.Println(err)
	} else {
		fmt.Println(c)
	}
	fmt.Println("cost: ", util.Time()-l)
}

func TestMongoSave(t *testing.T) {
	db, err := new(sqld.MGOManager).Get(sqld.Option{OpenTx: &sqld.TRUE})
	if err != nil {
		panic(err)
	}
	defer db.Close()
	vs := []interface{}{}
	for i := 0; i < 3; i++ {
		wallet := OwWallet{
			Id: util.GetSnowFlakeIntID(),
			// AppID:        map[string]interface{}{"test": 1},
			WalletID:     util.GetSnowFlakeStrID(),
			PasswordType: 1,
			Password:     "123456",
			RootPath:     "m/44'/88'/0'",
			Alias:        "hello_qtum",
			IsTrust:      0,
			AuthKey:      "",
		}
		vs = append(vs, &wallet)
	}
	l := util.Time()
	if err := db.Save(vs...); err != nil {
		fmt.Println(err)
	}
	fmt.Println("cost: ", util.Time()-l)
}

func TestMongoUpdate(t *testing.T) {
	db, err := new(sqld.MGOManager).Get(sqld.Option{OpenTx: &sqld.TRUE})
	if err != nil {
		panic(err)
	}
	defer db.Close()
	vs := []interface{}{}
	for i := 0; i < 1; i++ {
		wallet := OwWallet{
			Id: 1110012978914131969,
			// AppID:        map[string]interface{}{"test": 1},
			WalletID:     util.GetSnowFlakeStrID(),
			PasswordType: 1,
			Password:     "123456",
			RootPath:     "m/44'/88'/01'",
			Alias:        "hello_qtum",
			IsTrust:      0,
			AuthKey:      "",
		}
		vs = append(vs, &wallet)
	}
	l := util.Time()
	if err := db.Update(vs...); err != nil {
		fmt.Println(err)
	}
	fmt.Println("cost: ", util.Time()-l)
}

func TestMongoDelete(t *testing.T) {
	db, err := new(sqld.MGOManager).Get(sqld.Option{OpenTx: &sqld.TRUE, Timeout: 3000})
	if err != nil {
		panic(err)
	}
	defer db.Close()
	vs := []interface{}{}
	for i := 0; i < 1; i++ {
		wallet := OwWallet{
			Id: 1136806872108498948,
			// AppID:        map[string]interface{}{"test": 1},
			WalletID:     util.GetSnowFlakeStrID(),
			PasswordType: 1,
			Password:     "123456",
			RootPath:     "m/44'/88'/0'",
			Alias:        "hello_qtum",
			IsTrust:      0,
			AuthKey:      "",
		}
		vs = append(vs, &wallet)
	}
	l := util.Time()
	if err := db.Delete(vs...); err != nil {
		fmt.Println(err)
	}
	fmt.Println("cost: ", util.Time()-l)
}

func TestMongoCount(t *testing.T) {
	db, err := new(sqld.MGOManager).Get(sqld.Option{OpenTx: &sqld.FALSE})
	if err != nil {
		panic(err)
	}
	defer db.Close()
	l := util.Time()
	if c, err := db.Count(sqlc.M(&OwWallet{}).Eq("_id", 1110013195356995886).Orderby("_id", sqlc.DESC_).Limit(1, 30)); err != nil {
		fmt.Println(err)
	} else {
		fmt.Println(c)
	}
	fmt.Println("cost: ", util.Time()-l)
}

func TestMongoFindOne(t *testing.T) {
	l := util.Time()
	db, err := new(sqld.MGOManager).Get(sqld.Option{OpenTx: &sqld.TRUE})
	if err != nil {
		panic(err)
	}
	defer db.Close()
	o := &OwWallet{}
	if err := db.FindOne(sqlc.M(o).Eq("_id", 8266), o); err != nil {
		fmt.Println(err)
	}
	fmt.Println("cost: ", util.Time()-l)
}

func TestMongoFindList(t *testing.T) {
	db, err := new(sqld.MGOManager).Get(sqld.Option{OpenTx: &sqld.TRUE})
	if err != nil {
		panic(err)
	}
	defer db.Close()
	l := util.Time()
	o := []*OwWallet{}
	if err := db.FindList(sqlc.M().Eq("_id", 8622).Limit(1, 10), &o); err != nil {
		fmt.Println(err)
	}
	fmt.Println("cost: ", util.Time()-l)
}

func TestRedis(t *testing.T) {
	client, err := new(cache.RedisManager).Client()
	if err != nil {
		fmt.Println(err.Error())
	} else {
		client.Put("redislock:test", "1", 60)
		//if err := client.TryLock("test", func() error {
		//	return nil
		//}); err != nil {
		//	fmt.Println(err.Error())
		//}
		//client.Del("test")
		s := ""
		fmt.Println(client.Get("redislock:test", &s))
		fmt.Println(s)
		fmt.Println(client.Size("tx.block.coin.BTC"))
	}
}

func TestEX(t *testing.T) {
	e := ex.Throw{ex.UNKNOWN, "sss", errors.New("ss")}
	s := ex.Catch(e)
	fmt.Println(s)
}

func TestGA(t *testing.T) {
	// 生成种子
	seed := auth.GenerateSeed()
	fmt.Println("种子: ", seed)
	// 通过种子生成密钥
	key, _ := auth.GenerateSecretKey(seed)
	fmt.Println("密钥: ", key)
	// 通过密钥+时间生成验证码
	rs := auth.GetNewCode(key, time.Now().Unix())
	fmt.Println("验证码: ", rs)
	fmt.Println("开始睡眠延迟中,请耐心等待...")
	time.Sleep(5 * time.Second)
	// 校验已有验证码
	fmt.Println("校验结果: ", auth.ValidCode(key, rs))
}

func TestWebsocket_client_login(t *testing.T) {
	u := url.URL{Scheme: "ws", Host: ":9090", Path: "/login2"}
	var dialer *websocket.Dialer
	conn, _, err := dialer.Dial(u.String(), nil)
	if err != nil {
		fmt.Println(err)
		return
	}
	go goLogin(conn)
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			fmt.Println("read:", err)
			return
		}
		fmt.Printf("received: %s\n", message)
	}
}

func TestWebsocket_client_logout(t *testing.T) {
	u := url.URL{Scheme: "ws", Host: ":9090", Path: "/logout2"}
	var dialer *websocket.Dialer
	conn, _, err := dialer.Dial(u.String(), nil)
	if err != nil {
		fmt.Println(err)
		return
	}
	go goLogout(conn)
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			fmt.Println("read:", err)
			return
		}
		fmt.Printf("received: %s\n", message)
	}
}

func TestWebsocket_client_test(t *testing.T) {
	u := url.URL{Scheme: "ws", Host: ":9090", Path: "/test2"}
	var dialer *websocket.Dialer
	conn, _, err := dialer.Dial(u.String(), nil)
	if err != nil {
		fmt.Println(err)
		return
	}
	go goTest(conn)
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			fmt.Println("read:", err)
			return
		}
		fmt.Printf("received: %s\n", message)
	}
}

var token = "eyJub2QiOjAsInR5cCI6IkpXVCIsImFsZyI6IlNIQTI1NiJ9.eyJzdWIiOiJ6aGFuZ3NhbiIsImRldiI6IiIsImF1ZCI6IjEyNy4wLjAuMSIsImlzcyI6IjQ1NiIsImlhdCI6MTU0ODc1MzQ2NjY0NCwiZXhwIjoxNTQ4NzU1MjY2NjQ0LCJyeHAiOjE1NDg3NTUyNjY2NDQsIm5iZiI6MTU0ODc1MzQ2NjY0NCwianRpIjoiNWVlOWMzZmM1ODUzNWRlOWMwMWIzZDIyOTAyNDExZTIxYWI0ODQ5NDRhMzAwYzkxOTg5NzI3Mjk1ZDYwNWI3NSIsImV4dCI6e319.c66ccbfb3eb7a95aaf2a981570990a645a77f7e89df8b46b0c744355f06bdb59"

func goTest(conn *websocket.Conn) {
	for {
		time.Sleep(time.Second * 1)
		data := map[string]interface{}{"token": token}
		send, _ := util.JsonMarshal(&data)
		conn.WriteMessage(websocket.TextMessage, send)
	}
}

func goLogin(conn *websocket.Conn) {
	time.Sleep(time.Second * 1)
	data := map[string]interface{}{"user": "zhangsan"}
	send, _ := util.JsonMarshal(&data)
	conn.WriteMessage(websocket.TextMessage, send)
}

func goLogout(conn *websocket.Conn) {
	time.Sleep(time.Second * 1)
	data := map[string]interface{}{"token": token}
	send, _ := util.JsonMarshal(&data)
	conn.WriteMessage(websocket.TextMessage, send)
}

func TestWS(t *testing.T) {
	u := url.URL{Scheme: "ws", Host: ":8080", Path: "/echo"}
	var dialer *websocket.Dialer
	conn, _, err := dialer.Dial(u.String(), nil)
	if err != nil {
		fmt.Println(err)
		return
	}
	go timeWriter(conn)
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			fmt.Println("read:", err)
			return
		}
		fmt.Printf("received: %s\n", message)
	}
}

func timeWriter(conn *websocket.Conn) {
	//for {
	time.Sleep(time.Second * 2)
	conn.WriteMessage(websocket.TextMessage, []byte(time.Now().Format("2006-01-02 15:04:05")))
	//}
}

func TestA(t *testing.T) {
	fmt.Println(sqlc.ASC_)
}

func TestB(t *testing.T) {
	l := util.Time()
	s := "mydata4vipday.720.datx"
	for i := 0; i < 1000000000; i++ {
		a := util.Str2Bytes(s)
		_ = util.Bytes2Str(a)
	}
	fmt.Println(util.Time() - l)
}

func TestRGX(t *testing.T) {
	a := `["1","2","3","4","5"]`
	for i := 0; i < 2000000; i++ {
		b := []string{}
		util.JsonUnmarshal(util.Str2Bytes(a), &b);
		util.Str2Bytes(a)
	}
}

func TestRGX1(t *testing.T) {
	var a int64
	defer log.Info("耗时", util.Time(), log.Int64("a", a))
	for i := 0; i < 20000000; i++ {
		a = util.Time()
	}
}

func TestRGX2(t *testing.T) {
	//start := util.Time()
	//for i := 0; i < 20000; i++ {
	//	// MyClient.Connect(context.Background())
	//	c := MyClient.Database("openwallet").Collection("ow_wallet")
	//	pipeline := []map[string]interface{}{{"$match": map[string]interface{}{"_id": 8266}}}
	//	//pipeline := []map[string]interface{}{}
	//	batchSize := int32(5)
	//	cursor, err := c.Aggregate(context.Background(), pipeline, &options.AggregateOptions{BatchSize: &batchSize})
	//	if err != nil {
	//		fmt.Println(err)
	//		return
	//	}
	//	for cursor.Next(context.Background()) {
	//		//fmt.Println(cursor.Current.String())
	//		o := OwWallet{}
	//		cursor.Decode(&o)
	//		//fmt.Println(o)
	//	}
	//	// MyClient.Disconnect(context.Background())
	//}
	//fmt.Println("cost: ", util.Time()-start)
}

func BenchmarkLoopsParallel(b *testing.B) {
	i := float64(1)
	// b.SetParallelism(1000)
	b.N = 50000
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) { //并发
		for pb.Next() {
			s:= 145647.454564
			s = s+i
			util.Shift(s, 10,true)
		}
	})
}
