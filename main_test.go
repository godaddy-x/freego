package main

import (
	"fmt"
	"github.com/godaddy-x/freego/cache"
	"github.com/godaddy-x/freego/goquery"
	"github.com/godaddy-x/freego/ormx/sqlc"
	"github.com/godaddy-x/freego/ormx/sqld"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/utils/concurrent"
	"github.com/godaddy-x/freego/utils/gauth"
	"github.com/gorilla/websocket"
	"net/url"
	"testing"
	"time"
)

func TestMysql(t *testing.T) {
	db, err := new(sqld.MysqlManager).Get(sqld.Option{OpenTx: true})
	if err != nil {
		panic(err)
	}
	defer db.Close()
	o1 := OwWallet{
		Id:    123,
		AppID: "123",
	}
	o2 := OwWallet{
		Id:    1234,
		AppID: "1234",
	}
	db.Update(&o1, &o2)
}

func TestMysqlSave(t *testing.T) {
	db, err := new(sqld.MysqlManager).Get(sqld.Option{OpenTx: true})
	if err != nil {
		panic(err)
	}
	defer db.Close()
	vs := []interface{}{}
	for i := 0; i < 1; i++ {
		wallet := OwWallet{
			// AppID:        map[string]interface{}{"test": 1},
			WalletID:     utils.GetSnowFlakeStrID(),
			PasswordType: 1,
			Password:     "123456",
			RootPath:     "m/44'/88'/0'",
			Alias:        "hello_qtum",
			IsTrust:      0,
			AuthKey:      "",
		}
		vs = append(vs, &wallet)
	}
	l := utils.Time()
	if err := db.Save(vs...); err != nil {
		fmt.Println(err)
	}
	fmt.Println("cost: ", utils.Time()-l)
}

func TestMysqlUpdate(t *testing.T) {
	db, err := new(sqld.MysqlManager).Get(sqld.Option{OpenTx: true})
	if err != nil {
		panic(err)
	}
	defer db.Close()
	vs := []interface{}{}
	for i := 0; i < 10; i++ {
		wallet := OwWallet{
			Id: utils.GetSnowFlakeIntID(),
			// AppID:        "123456",
			WalletID:     utils.GetSnowFlakeStrID(),
			PasswordType: 1,
			Password:     "123456",
			RootPath:     "m/44'/88'/0'",
			Alias:        "hello_qtum111333",
			IsTrust:      0,
			AuthKey:      "",
		}
		vs = append(vs, &wallet)
	}
	l := utils.Time()
	if err := db.Update(vs...); err != nil {
		fmt.Println(err)
	}
	fmt.Println("cost: ", utils.Time()-l)
}

func TestMysqlUpdateByCnd(t *testing.T) {
	db, err := new(sqld.MysqlManager).Get(sqld.Option{OpenTx: true})
	if err != nil {
		panic(err)
	}
	defer db.Close()
	l := utils.Time()
	if err := db.UpdateByCnd(sqlc.M(&OwWallet{}).Upset([]string{"password"}, "1111").Eq("id", 1110371778615574533)); err != nil {
		fmt.Println(err)
	}
	fmt.Println("cost: ", utils.Time()-l)
}

func TestMysqlDetele(t *testing.T) {
	db, err := new(sqld.MysqlManager).Get(sqld.Option{OpenTx: true})
	if err != nil {
		panic(err)
	}
	defer db.Close()
	vs := []interface{}{}
	for i := 0; i < 2000; i++ {
		wallet := OwWallet{
			Id: utils.GetSnowFlakeIntID(),
			//AppID:        map[string]interface{}{"test": 1},
			WalletID:     utils.GetSnowFlakeStrID(),
			PasswordType: 1,
			Password:     "123456",
			RootPath:     "m/44'/88'/0'",
			Alias:        "hello_qtum111333",
			IsTrust:      0,
			AuthKey:      "",
		}
		vs = append(vs, &wallet)
	}
	l := utils.Time()
	if err := db.Delete(vs...); err != nil {
		fmt.Println(err)
	}
	fmt.Println("cost: ", utils.Time()-l)
}

func TestMysqlFindById(t *testing.T) {
	db, err := new(sqld.MysqlManager).Get()
	if err != nil {
		panic(err)
	}
	defer db.Close()
	l := utils.Time()
	wallet := OwWallet{
		Id: 1109819683034365953,
	}
	if err := db.FindById(&wallet); err != nil {
		fmt.Println(err)
	}
	fmt.Println("cost: ", utils.Time()-l)
}

func TestMysqlFindOne(t *testing.T) {
	db, err := new(sqld.MysqlManager).Get(sqld.Option{OpenTx: false})
	if err != nil {
		panic(err)
	}
	defer db.Close()
	l := utils.Time()
	for i := 0; i < 1; i++ {

	}
	wallet := OwWallet{}
	if err := db.FindOne(sqlc.M().Fields("rootPath").Eq("id", 1109819683034365953), &wallet); err != nil {
		fmt.Println(err)
	}
	fmt.Println("cost: ", utils.Time()-l)
}

func TestMysqlFindList(t *testing.T) {
	db, err := new(sqld.MysqlManager).Get(sqld.Option{OpenTx: false})
	if err != nil {
		panic(err)
	}
	defer db.Close()
	l := utils.Time()
	result := []*OwWallet{}
	if err := db.FindList(sqlc.M().Eq("id", 1124853348080549889).Groupby("id").Orderby("id", sqlc.DESC_).Limit(1, 5), &result); err != nil {
		fmt.Println(err)
	}
	fmt.Println("cost: ", utils.Time()-l)
}

func TestMysqlFindListComplex(t *testing.T) {
	db, err := new(sqld.MysqlManager).Get(sqld.Option{OpenTx: false})
	if err != nil {
		panic(err)
	}
	defer db.Close()
	l := utils.Time()
	result := []*OwWallet{}
	if err := db.FindListComplex(sqlc.M(&OwWallet{}).Fields("count(a.id) as id").From("ow_wallet a").Orderby("a.id", sqlc.DESC_).Limit(1, 5), &result); err != nil {
		fmt.Println(err)
	}
	fmt.Println("cost: ", utils.Time()-l)
}

func TestMysqlFindOneComplex(t *testing.T) {
	db, err := new(sqld.MysqlManager).Get(sqld.Option{OpenTx: false})
	if err != nil {
		panic(err)
	}
	defer db.Close()
	l := utils.Time()
	result := OwWallet{}
	if err := db.FindOneComplex(sqlc.M(&OwWallet{}).Fields("count(a.id) as id").From("ow_wallet a").Orderby("a.id", sqlc.DESC_).Limit(1, 5), &result); err != nil {
		fmt.Println(err)
	}
	fmt.Println("cost: ", utils.Time()-l)
}

func TestMysqlCount(t *testing.T) {
	db, err := new(sqld.MysqlManager).Get(sqld.Option{OpenTx: false})
	if err != nil {
		panic(err)
	}
	defer db.Close()
	l := utils.Time()
	if c, err := db.Count(sqlc.M(&OwWallet{}).Orderby("id", sqlc.DESC_).Limit(1, 30)); err != nil {
		fmt.Println(err)
	} else {
		fmt.Println(c)
	}
	fmt.Println("cost: ", utils.Time()-l)
}

func TestMongoSave(t *testing.T) {
	db, err := new(sqld.MGOManager).Get(sqld.Option{OpenTx: true})
	if err != nil {
		panic(err)
	}
	defer db.Close()
	//l := utils.Time()
	o := OwWallet{
		AppID:    utils.GetSnowFlakeStrID(),
		WalletID: utils.GetSnowFlakeStrID(),
	}
	if err := db.Save(&o); err != nil {
		fmt.Println(err)
	}
}

func TestMongoUpdate(t *testing.T) {
	db, err := new(sqld.MGOManager).Get(sqld.Option{OpenTx: true})
	if err != nil {
		panic(err)
	}
	defer db.Close()
	//wallet := OwWallet{
	//	Id: 1110012978914131972,
	//	// AppID:        map[string]interface{}{"test": 1},
	//	WalletID:     "1110012978914131972",
	//	PasswordType: 1,
	//	Password:     "123456",
	//	RootPath:     "m/44'/88'/01'",
	//	Alias:        "hello_qtum",
	//	IsTrust:      0,
	//	AuthKey:      "",
	//}
	l := utils.Time()
	if err := db.UpdateByCnd(sqlc.M(&OwWallet{}).Or(sqlc.M().Eq("id", 1110012978914131972), sqlc.M().Eq("id", 1110012978914131973)).Upset([]string{"appID", "ctime"}, "test1test1", 1)); err != nil {
		fmt.Println(err)
	}
	//fmt.Println(wallet.Id)
	fmt.Println("cost: ", utils.Time()-l)
}

func TestMongoDelete(t *testing.T) {
	db, err := new(sqld.MGOManager).Get(sqld.Option{OpenTx: true, Timeout: 3000})
	if err != nil {
		panic(err)
	}
	defer db.Close()
	vs := []interface{}{}
	for i := 0; i < 1; i++ {
		wallet := OwWallet{
			Id: 1136806872108498948,
			// AppID:        map[string]interface{}{"test": 1},
			WalletID:     utils.GetSnowFlakeStrID(),
			PasswordType: 1,
			Password:     "123456",
			RootPath:     "m/44'/88'/0'",
			Alias:        "hello_qtum",
			IsTrust:      0,
			AuthKey:      "",
		}
		vs = append(vs, &wallet)
	}
	l := utils.Time()
	if err := db.Delete(vs...); err != nil {
		fmt.Println(err)
	}
	fmt.Println("cost: ", utils.Time()-l)
}

func TestMongoAgg(t *testing.T) {
	db, err := new(sqld.MGOManager).Get(sqld.Option{OpenTx: false})
	if err != nil {
		panic(err)
	}
	defer db.Close()
	db.FindOne(sqlc.M().Agg(sqlc.SUM_, "paidPrice").Groupby("shopId", "userId").Asc("shopId").Limit(1, 1), &OwWallet{})
	//db.FindOne(sqlc.M().Groupby("appID"), &OwWallet{})
}

func TestMongoCount(t *testing.T) {
	db, err := new(sqld.MGOManager).Get(sqld.Option{OpenTx: false})
	if err != nil {
		panic(err)
	}
	defer db.Close()
	l := utils.Time()
	if c, err := db.Count(sqlc.M(&OwWallet{}).Eq("_id", 1110012978914131972).Orderby("_id", sqlc.DESC_).Limit(1, 30)); err != nil {
		fmt.Println(err)
	} else {
		fmt.Println(c)
	}
	fmt.Println("cost: ", utils.Time()-l)
}

func TestMongoFindList(t *testing.T) {
	db, err := new(sqld.MGOManager).Get(sqld.Option{OpenTx: true})
	if err != nil {
		panic(err)
	}
	defer db.Close()
	l := utils.Time()
	o := []*OwWallet{}
	if err := db.FindList(sqlc.M().Eq("id", 1194915104647282690).Groupby("appID"), &o); err != nil {
		fmt.Println(err)
	}
	fmt.Println("cost: ", utils.Time()-l)
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
	x := utils.Time()
	fmt.Println(x)
	fmt.Println(utils.GetFmtDate(x))
}

func TestGA(t *testing.T) {
	// 生成种子
	seed := gauth.GenerateSeed()
	fmt.Println("种子: ", seed)
	// 通过种子生成密钥
	key, _ := gauth.GenerateSecretKey(seed)
	fmt.Println("密钥: ", key)
	// 通过密钥+时间生成验证码
	rs := gauth.GetNewCode(key, time.Now().Unix())
	fmt.Println("验证码: ", rs)
	fmt.Println("开始睡眠延迟中,请耐心等待...")
	time.Sleep(5 * time.Second)
	// 校验已有验证码
	fmt.Println("校验结果: ", gauth.ValidCode(key, rs))
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
		send, _ := utils.JsonMarshal(&data)
		conn.WriteMessage(websocket.TextMessage, send)
	}
}

func goLogin(conn *websocket.Conn) {
	time.Sleep(time.Second * 1)
	data := map[string]interface{}{"user": "zhangsan"}
	send, _ := utils.JsonMarshal(&data)
	conn.WriteMessage(websocket.TextMessage, send)
}

func goLogout(conn *websocket.Conn) {
	time.Sleep(time.Second * 1)
	data := map[string]interface{}{"token": token}
	send, _ := utils.JsonMarshal(&data)
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

func TestSorter(t *testing.T) {
	type Obj struct {
		Key string
		Val int64
	}
	d := []interface{}{
		&Obj{"e", 10},
		&Obj{"a", 2},
		&Obj{"d", 15},
		&Obj{"c", 8},
		&Obj{"f", 1},
		&Obj{"b", 12},
	}
	result := concurrent.NewSorter(d, func(o1, o2 interface{}) bool {
		a := o1.(*Obj)
		b := o2.(*Obj)
		return a.Val < b.Val // 判断值大小排序
	}).Sort()
	for _, v := range result {
		fmt.Println(v)
	}
}

func TestB(t *testing.T) {
	l := utils.Time()
	s := "mydata4vipday.720.datx"
	for i := 0; i < 1000000000; i++ {
		a := utils.Str2Bytes(s)
		_ = utils.Bytes2Str(a)
	}
	fmt.Println(utils.Time() - l)
}

func TestRGX1(t *testing.T) {
	//var aeskey = "x4kkptzFsUOVnuya"
	//fmt.Println("密钥: ", aeskey)
	//pass := "123456"
	//xpass := utils.AesEncrypt(pass, aeskey)
	//
	//fmt.Printf("加密后:%v\n", xpass)
	//
	//tpass := utils.AesDecrypt(xpass, string(aeskey))
	//fmt.Printf("解密后:%s\n", tpass)

	//s := "(?i)eval\\s*\\((.*?)\\)"
	//c := "<eval  ()"
	//fmt.Println(utils.ValidPattern(c, s))

	//s := "(?i)expression\\s*\\((.*?)\\)"
	//c := "<ExpressiOn  ()"
	//fmt.Println(utils.ValidPattern(c, s))

	//s := "(?i)scrip1t\\s*\\>(.*?)"
	//c := "<scRip1t  >"
	//fmt.Println(utils.ValidPattern(c, s))

	//s := "javascript:(.*?)|vbscript\\s*\\:(.*?)|view-source\\s*\\:(.*?)"
	//c := "vbscript:vbscript"
	//fmt.Println(utils.ValidPattern(c, s))

	//s := "\\{1}"
	//c := `\`
	//fmt.Println(utils.ValidPattern(c, s))
	fmt.Println(fmt.Sprintf("%x", `%`))
	fmt.Println(url.QueryEscape("%"))

}

type HtmlValidResult struct {
	NewData    string
	ContentLen int
}

var htmlstr = `
<section style="text-align: center; color: rgb(68, 198, 123); font-weight: 800; font-style: italic; text-decoration: line-through;">&nbsp;&nbsp;&lt;a&gt;斯蒂芬  撒旦法毒贩夫妇%3Csectioin&lt;/a&gt;%#\'":;.img src='test'/></section>
<h2 style="text-align: center; color: rgb(68, 198, 123); font-weight: 800; font-style: italic; text-decoration: line-through;">&nbsp;&nbsp;&lt;a&gt;斯蒂芬撒旦法毒贩夫妇%3Csectioin&lt;/a&gt;%#\'":;.img src='test'/></h2>
`

func TestHtml(t *testing.T) {
	//valid := goquery.ValidZxHtml(htmlstr)
	//fmt.Println(valid.ContentLen, valid.NewContent, valid.FailMsg)
	s := "//static.pgwjc.com/skin/images/1180654561279344640/1243153846100819968.jpg"
	fmt.Println(goquery.ValidImgURL(s, "//static.pgwjc.com/skin/images/"))
}

func TestRGX2(t *testing.T) {
	m := map[string]int{"test": 1}
	b, _ := utils.JsonMarshal(m)
	r := map[string]interface{}{}
	utils.JsonUnmarshal(b, &r)
	fmt.Println(len("1566722843972"))
	//x := "世界上最邪恶最专制的现代奴隶制国家--朝鲜"
	//key :=utils.Substr( utils.MD5("hgfedcba87654321"), 0, 16)
	//x1 := utils.AesEncrypt(x, key)
	//fmt.Println(x1)
	//x2 := utils.AesDecrypt(x1, key)
	//fmt.Print(string(x2))
	//start := utils.Time()
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
	//fmt.Println("cost: ", utils.Time()-start)
}

//func BenchmarkLoopsParallel(b *testing.B) {
//	i := float64(1)
//	// b.SetParallelism(1000)
//	b.N = 50000
//	b.ReportAllocs()
//	b.RunParallel(func(pb *testing.PB) { //并发
//		for pb.Next() {
//			s := 145647.454564
//			s = s + i
//			utils.Shift(s, 10, true)
//		}
//	})
//}
