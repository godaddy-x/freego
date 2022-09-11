# freego
High performance secure GRPC/ORM/NODE framework

#### 1. Create simple http demo
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
	my.GET("/pubkey", my.pubkey, &node.RouterConfig{Guest: true})
	my.StartServer(":8090")
}
```


#### 2. Create plugin filter
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
	ctx.AddStorage("httpLog", node.HttpLog{Method: ctx.Path, LogNo: utils.GetSnowFlakeStrID(), CreateAt: utils.Time()})
	if err := chain.DoFilter(chain, ctx, args...); err != nil {
		return err
	}
	v := ctx.GetStorage("httpLog")
	if v == nil {
		return utils.Error("httpLog is nil")
	}
	httpLog, _ := v.(node.HttpLog)
	httpLog.UpdateAt = utils.Time()
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

#### 3. Create JWT&RSA login 

```
func (self *MyWebNode) login(ctx *node.Context) error {
	subject := &jwt.Subject{}
	config := ctx.GetJwtConfig()
	token := subject.Create(utils.GetSnowFlakeStrID()).Dev("APP").Generate(config)
	secret := jwt.GetTokenSecret(token, config.TokenKey)
	return self.Json(ctx, map[string]interface{}{"token": token, "secret": secret})
}

// client request demo
func getServerPubkey() string {
	reqcli := fasthttp.AcquireRequest()
	reqcli.Header.SetMethod("GET")
	reqcli.SetRequestURI(domain + "/pubkey")
	defer fasthttp.ReleaseRequest(reqcli)
	respcli := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(respcli)
	if _, b, err := fasthttp.Get(nil, domain+"/pubkey"); err != nil {
		panic(err)
	} else {
		return utils.Bytes2Str(b)
	}
}

func ToPostBy(path string, req *node.JsonBody) {
	if len(srvPubkeyBase64) == 0 {
		srvPubkeyBase64 = getServerPubkey()
	}
	if req.Plan == 0 {
		d := utils.Base64URLEncode(req.Data.([]byte))
		req.Data = d
		output("Base64数据: ", req.Data)
	} else if req.Plan == 1 {
		d, err := utils.AesEncrypt(req.Data.([]byte), token_secret, utils.AddStr(req.Nonce, req.Time))
		if err != nil {
			panic(err)
		}
		req.Data = d
		output("AES加密数据: ", req.Data)
	} else if req.Plan == 2 {
		newRsa := &gorsa.RsaObj{}
		if err := newRsa.LoadRsaPemFileBase64(srvPubkeyBase64); err != nil {
			panic(err)
		}
		rsaData, err := newRsa.Encrypt(req.Data.([]byte))
		if err != nil {
			panic(err)
		}
		req.Data = rsaData
		output("RSA加密数据: ", req.Data)
	}
	secret := token_secret
	if req.Plan == 2 {
		secret = srvPubkeyBase64
		output("nonce secret:", pubkey)
	}
	req.Sign = utils.HMAC_SHA256(utils.AddStr(path, req.Data.(string), req.Nonce, req.Time, req.Plan), secret, true)
	bytesData, err := utils.JsonMarshal(req)
	if err != nil {
		panic(err)
	}
	output("请求示例: ")
	output(utils.Bytes2Str(bytesData))

	reqcli := fasthttp.AcquireRequest()
	reqcli.Header.SetContentType("application/json;charset=UTF-8")
	reqcli.Header.Set("Authorization", access_token)
	reqcli.Header.SetMethod("POST")
	reqcli.SetRequestURI(domain + path)
	reqcli.SetBody(bytesData)
	defer fasthttp.ReleaseRequest(reqcli)

	respcli := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(respcli)

	if err := fasthttp.DoTimeout(reqcli, respcli, 5*time.Second); err != nil {
		panic(err)
	}

	respBytes := respcli.Body()

	output("响应示例: ")
	output(utils.Bytes2Str(respBytes))
	respData := &node.JsonResp{}
	if err := utils.JsonUnmarshal(respBytes, &respData); err != nil {
		panic(err)
	}
	if respData.Code == 200 {
		key := token_secret
		if respData.Plan == 2 {
			key = pubkey
		}
		s := utils.HMAC_SHA256(utils.AddStr(path, respData.Data, respData.Nonce, respData.Time, respData.Plan), key, true)
		output("****************** Response Signature Verify:", s == respData.Sign, "******************")
		if respData.Plan == 0 {
			dec := utils.Base64URLDecode(respData.Data)
			output("Base64数据明文: ", string(dec))
		}
		if respData.Plan == 1 {
			dec, err := utils.AesDecrypt(respData.Data.(string), key, utils.AddStr(respData.Nonce, respData.Time))
			if err != nil {
				panic(err)
			}
			respData.Data = dec
			output("AES数据明文: ", respData.Data)
		}
		if respData.Plan == 2 {
			dec, err := utils.AesDecrypt(respData.Data.(string), pubkey, pubkey)
			if err != nil {
				panic(err)
			}
			output("LOGIN数据明文: ", dec)
		}
	}
}

// go test
func TestRSALogin(t *testing.T) {
	data, _ := utils.JsonMarshal(map[string]string{"username": "1234567890123456", "password": "1234567890123456", "pubkey": pubkey})
	path := "/login"
	req := &node.JsonBody{
		Data:  data,
		Time:  utils.TimeSecond(),
		Nonce: utils.RandNonce(),
		Plan:  int64(2),
	}
	ToPostBy(path, req)
}

// Benchmark test 
goos: darwin
goarch: amd64
cpu: Intel(R) Core(TM) i7-6820HQ CPU @ 2.70GHz
BenchmarkRSALogin
BenchmarkRSALogin-8         2408            524652 ns/op            1018 B/op         15 allocs/op
BenchmarkRSALogin-8         2397            495642 ns/op            1016 B/op         15 allocs/op
BenchmarkRSALogin-8         2090            498629 ns/op            1011 B/op         15 allocs/op
BenchmarkRSALogin-8         2053            500789 ns/op            1015 B/op         15 allocs/op
BenchmarkRSALogin-8         2094            499364 ns/op            1017 B/op         15 allocs/op
BenchmarkRSALogin-8         2409            495365 ns/op            1016 B/op         15 allocs/op
BenchmarkRSALogin-8         2028            505224 ns/op            1017 B/op         15 allocs/op
BenchmarkRSALogin-8         2079            501502 ns/op            1017 B/op         15 allocs/op
BenchmarkRSALogin-8         2150            496770 ns/op            1013 B/op         15 allocs/op
BenchmarkRSALogin-8         2020            500296 ns/op            1019 B/op         15 allocs/op
```

#### 4. Create simple orm demo

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
        sqld.NewHooK(func() interface{} { return &OwWallet{} }, func() interface{} { return &[]*OwWallet{} }),
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
	client := &grpcx.GRPCManager{}
	client.CreateJwtConfig(APPKEY)
	client.CreateAppConfigCall(func(appid string) (grpcx.AppConfig, error) {
		if appid == APPKEY {
			return grpcx.AppConfig{Appid: APPID, Appkey: APPKEY}, nil
		}
		return grpcx.AppConfig{}, utils.Error("appid invalid")
	})
	client.CreateRateLimiterCall(func(method string) (rate.Option, error) {
		return rate.Option{}, nil
	})
	client.CreateServerTLS(grpcx.TlsConfig{
		UseMTLS:   true,
		CACrtFile: "./consul/grpcx/cert/ca.crt",
		KeyFile:   "./consul/grpcx/cert/server.key",
		CrtFile:   "./consul/grpcx/cert/server.crt",
	})
	client.CreateClientTLS(grpcx.TlsConfig{
		UseMTLS:   true,
		CACrtFile: "./consul/grpcx/cert/ca.crt",
		KeyFile:   "./consul/grpcx/cert/client.key",
		CrtFile:   "./consul/grpcx/cert/client.crt",
		HostName:  "localhost",
	})
	client.CreateAuthorizeTLS("./consul/grpcx/cert/server.key")
}

// grpc server
func TestConsulxGRPCServer(t *testing.T) {
	objects := []*grpcx.GRPC{
		{
			Address: "localhost",
			Service: "PubWorker",
			Tags:    []string{"ID Generator"},
			AddRPC:  func(server *grpc.Server) { pb.RegisterPubWorkerServer(server, &impl.PubWorker{}) },
		},
	}
	grpcx.RunServer("", true, objects...)
}

// grpc client
func TestConsulxGRPCClient(t *testing.T) {
	grpcx.RunClient(grpcx.ClientConfig{Appid: APPID, Timeout: 30, Addrs: []string{addr}})
	conn, err := grpcx.NewClientConn(grpcx.GRPC{Service: "PubWorker", Cache: 30})
	if err != nil {
		panic(err)
	}
	defer conn.Close()
	res, err := pb.NewPubWorkerClient(conn.Value()).GenerateId(conn.Context(), &pb.GenerateIdReq{})
	if err != nil {
		panic(err)
	}
	fmt.Println("call rpc:", res)
}
```

