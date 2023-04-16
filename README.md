# freego
High performance secure GRPC/ORM/NODE framework

#### 1. Create simple HTTP/NODE demo
```
type MyWebNode struct {
	node.HttpNode
}

func (self *MyWebNode) pubkey(ctx *node.Context) error {
	return self.Text(ctx, "hello world!!!")
}

func NewHTTP() *MyWebNode {
	var my = &MyWebNode{}
	my.AddJwtConfig(jwt.JwtConfig{
		TokenTyp: jwt.JWT,
		TokenAlg: jwt.HS256,
		TokenKey: "123456" + utils.CreateLocalSecretKey(12, 45, 23, 60, 58, 30),
		TokenExp: jwt.TWO_WEEK,
	})
	my.AddCacheAware(func(ds ...string) (cache.Cache, error) {
		return local, nil
	})
	return my
}

func main()  {
	my := NewHTTP()
	my.EnableECC(true) // use ecc, default rsa
	my.GET("/pubkey", my.pubkey, &node.RouterConfig{Guest: true})
	my.StartServer(":8090")
}
```


#### 2. Create plugin filter chain
#### You can implement any pre and post operations, and configure `MatchPattern` parameter to apply the specified method

```
// default filters
var filterMap = map[string]*FilterObject{
	GatewayRateLimiterFilterName: {Name: GatewayRateLimiterFilterName, Order: -100, Filter: &GatewayRateLimiterFilter{}},
	ParameterFilterName:          {Name: ParameterFilterName, Order: -90, Filter: &ParameterFilter{}},
	SessionFilterName:            {Name: SessionFilterName, Order: -80, Filter: &SessionFilter{}},
	UserRateLimiterFilterName:    {Name: UserRateLimiterFilterName, Order: -70, Filter: &UserRateLimiterFilter{}},
	RoleFilterName:               {Name: RoleFilterName, Order: -60, Filter: &RoleFilter{}},
	ReplayFilterName:             {Name: ReplayFilterName, Order: -50, Filter: &ReplayFilter{}},
	PostHandleFilterName:         {Name: PostHandleFilterName, Order: math.MaxInt, Filter: &PostHandleFilter{}},
	RenderHandleFilterName:       {Name: RenderHandleFilterName, Order: math.MinInt, Filter: &RenderHandleFilter{}},
}

type NewPostFilter struct{}

func (self *NewPostFilter) DoFilter(chain node.Filter, ctx *node.Context, args ...interface{}) error {
	ctx.AddStorage("httpLog", node.HttpLog{Method: ctx.Path, LogNo: utils.GetSnowFlakeStrID(), CreateAt: utils.UnixMilli()})
	if err := chain.DoFilter(chain, ctx, args...); err != nil {
		return err
	}
	v := ctx.GetStorage("httpLog")
	if v == nil {
		return utils.Error("httpLog is nil")
	}
	httpLog, _ := v.(node.HttpLog)
	httpLog.UpdateAt = utils.UnixMilli()
	httpLog.CostMill = httpLog.UpdateAt - httpLog.CreateAt
	return nil
}

func NewHTTP() *MyWebNode {
	var my = &MyWebNode{}
	my.AddJwtConfig(jwt.JwtConfig{
		TokenTyp: jwt.JWT,
		TokenAlg: jwt.HS256,
		TokenKey: "123456" + utils.CreateLocalSecretKey(12, 45, 23, 60, 58, 30),
		TokenExp: jwt.TWO_WEEK,
	})
	my.AddCacheAware(func(ds ...string) (cache.Cache, error) {
		return local, nil
	})
	my.AddFilter(&node.FilterObject{Name: "NewPostFilter", Order: 100, Filter: &NewPostFilter{}})
	return my
}

// Benchmark test 
goos: darwin
goarch: amd64
cpu: Intel(R) Core(TM) i7-6820HQ CPU @ 2.70GHz
BenchmarkPubkey
BenchmarkPubkey-8          14770             80893 ns/op             515 B/op          1 allocs/op
BenchmarkPubkey-8          14908             79999 ns/op             515 B/op          1 allocs/op
BenchmarkPubkey-8          15063             79425 ns/op             514 B/op          1 allocs/op
BenchmarkPubkey-8          15031             82307 ns/op             514 B/op          1 allocs/op
BenchmarkPubkey-8          14925             80306 ns/op             515 B/op          1 allocs/op
BenchmarkPubkey-8          15015             79758 ns/op             515 B/op          1 allocs/op
BenchmarkPubkey-8          15133             79156 ns/op             515 B/op          1 allocs/op
BenchmarkPubkey-8          15070             83157 ns/op             515 B/op          1 allocs/op
BenchmarkPubkey-8          14887             79710 ns/op             517 B/op          1 allocs/op
BenchmarkPubkey-8          15061             79666 ns/op             516 B/op          1 allocs/op
```

#### 3. Create JWT&ECC login demo

``` see @http_test.go
// Benchmark test 
goos: windows
goarch: amd64
pkg: github.com/godaddy-x/freego
cpu: 12th Gen Intel(R) Core(TM) i5-12400F
BenchmarkRSALogin
BenchmarkRSALogin-12                4058            293012 ns/op
PASS
```

#### 4. Create simple ORM demo

```
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
    sqld.ModelDriver(
	&OwWallet{},
    )
    initMongoDB()
    initMysqlDB()
}

func TestMysqlUpdates(t *testing.T) {
    // db, err := sqld.NewMongo(sqld.Option{OpenTx: true})
	db, err := sqld.NewMysql(sqld.Option{OpenTx: true})
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
	if err := db.Update(&o1, &o2); err != nil {
		panic(err)
	}
}

func TestMysqlFind(t *testing.T) {
    // db, err := sqld.NewMongo()
	db, err := sqld.NewMysql()
	if err != nil {
		panic(err)
	}
	defer db.Close()
	sql := sqlc.M().Fields("rootPath").Eq("id", 1).Between("index", 10, 50).Gte("index", 30).Like("name", "test").Or(sqlc.M().Eq("id", 12), sqlc.M().Eq("id", 13))
	wallet := OwWallet{}
	if err := db.FindOne(sql, &wallet); err != nil {
		panic(err)
	}
}
```

#### 5. Create simple Consul&GRPC demo

```
func init() {
	client := &rpcx.GRPCManager{}
	client.CreateJwtConfig(APPKEY)
	client.CreateAppConfigCall(func(appid string) (rpcx.AppConfig, error) {
		if appid == APPKEY {
			return rpcx.AppConfig{Appid: APPID, Appkey: APPKEY}, nil
		}
		return rpcx.AppConfig{}, utils.Error("appid invalid")
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

// grpc server
func TestConsulxGRPCServer(t *testing.T) {
	objects := []*rpcx.GRPC{
		{
			Address: "localhost",
			Service: "PubWorker",
			Tags:    []string{"ID Generator"},
			AddRPC:  func(server *grpc.Server) { pb.RegisterPubWorkerServer(server, &impl.PubWorker{}) },
		},
	}
	rpcx.RunServer("", true, objects...)
}

// grpc client
func TestConsulxGRPCClient(t *testing.T) {
	rpcx.RunClient(APPID)
	conn, err := rpcx.NewClientConn(rpcx.GRPC{Service: "PubWorker", Cache: 30})
	if err != nil {
		panic(err)
	}
	defer conn.Close()
	res, err := pb.NewPubWorkerClient(conn.Value()).GenerateId(conn.Context(), &pb.GenerateIdReq{})
	if err != nil {
		panic(err)
	}
	fmt.Println("call result: ", res)
}

// grpc client benchmark test
func BenchmarkGRPCClient(b *testing.B) {
	rpcx.RunClient(APPID)
	b.StopTimer()
	b.StartTimer()
	for i := 0; i < b.N; i++ { //use b.N for looping
		conn, err := rpcx.NewClientConn(rpcx.GRPC{Service: "PubWorker", Cache: 30})
		if err != nil {
			return
		}
		_, err = pb.NewPubWorkerClient(conn.Value()).GenerateId(conn.Context(), &pb.GenerateIdReq{})
		if err != nil {
			return
		}
		conn.Close()
	}
}

BenchmarkGRPCClient-40              11070            212487 ns/op
```

