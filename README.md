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

### 6. Benchmark Encipher test

```
var (
	encipherClient = sdk.NewEncipherClient("http://localhost:4141")
)

const (
	msg = "35JA1BaJJpzbLMcD07a!mJNF%#dKi5JSjyR&zCGQ%Ap*g02Qgf!fhvzYpI0@rPBCCGWzD5of7wCU$e7O&B0aLID5oMHDjS4b%PyOA5ycDQb759Pr&WRgJdIBC7ButDp#9Do%UKqV0r6KJca*FTd3Jao*W1mcv$*q&5aEj7IVSlA4q0aAZr1L#y7p1zHO1km^m#WUByo$9d^rj1NFNt2gwJ11T#5!iyrR#GVd0#C9G^^ws8N0!k$vKxl!QP!QeuYBRDXfTSgL#b^70ckWK17baD3RuXL9zE6ZxO7EKE48whMRaSCrLC9!^K5EzcDJo8j27USzF*$YursdoAalqvA89lRLRd2&Uz2bZHs7s&!bSnHHbSJZYQhlAAjpLLJvGdTLs8Oz3#^^MzgGo3cY%K7!bd%N%VkJk0mpPBRsFHcaSpJATn4tfC0&z&st^zO3G1QBE93*Wqj292!6wOJhi^W@mUVnuNlfeO3TTgn5MdDZFNZRVHNohN*HXbo9s5&SRH*3X7gvb6sVrs7widS0%UAg88i%Q^v9yRBRo12PgO5pTFS9%s0OdhDqDHhDAaY&l*rpMoZU!dDNEX@Bp$GLz^v%!c#EAuqLx!#PA5rTMxnKqBoLfohBCp&SEgH9spbTHoytz!T5pKWAgT!Wt&D@PQU5lV6jMWU*C*iRu4%vhB#*Z9C#1KUGYkl^8CEKT7OTih6BcnKyG#YH0JndHGuhAGH0Y*1@dvVaAla8jKCh85zw2@n^xf!$66COp2EQndGn@or4mjbkkBQtiyW#JmRgxRImh3Tugf3zK1p6bcHJOY6R^sta8mTyxX7H2Au#qLmmlFD@Jd9Yrmf*ONPglXEK*0RSA%Zz3vKK490t1dw8kPp&Y$g$MvCK*$5c^BjFu#RRhn^ne97kw9MELO%Ho0G2X2qmSM*q1yCpyu1H1Y0Bp^y%0z3ZWseJvyBd!p$qEQy%$6Z9jc3DA6iK*q5TQJSjT8zTZX^58##M@XT$frKg$aDH!Gdh$2jEHdO73kpI%zkr*oAoh#pbC@X*Q^#uLtx5XHIVxukhN*sWgva^SC^J2rhxpt0C3B&DDGJ!grRTbKrCe16cWagif681ttOA&zj1*UXEiW*0lq@x2KiS^Hus9Icmx*A8cCliaYpKVMeJGhfrdS^pzm#aBa&6UcA3kjHNL0ie*qr4FgIDCMzk*vr0jPdVmL9PB!FTzzKEJ5V0Z5$aT!jrsLIR@rFhb5XccG0wQqwA&Q!N2xVj50C0L&#eRj4Q*J0uuuSo*CnN#tJoYpOe$^jUTr#STmXVqu8HLoJz6v%A@%KkXDQCv7%L$T^sZ7H2Xyzyxec$zDGhYiQL9J0*aIq5#MQwxv2bRGAEVJx9Nf2cf7Nma1cvb#j9m@ol$AtlCj^NpDZAs*L8Ivm#@bN2%VSxsW*QHAkilY#RSz$aPIj3KB^vb9Z$veN1A0IdI#9pdGSiskm5#Mmb4vONhczLog3Na9pD^D&BBHVH6lyyqnFvVqCQ$eJabdZwK!ldL5sZHzO1LIeyc2ROkzDr5a3kykje*r35F8VYefVr&^lKCYFGhlec3CFC$JjBlNTUXhNzBx!W!#L#gh#f#j&e#qc02nW8Lsi07am85!o0CZ10uq$!V9P^6ph7p&0*jn@sewwQxgzbRX7fbsgDq!atTGctJ0PNvFdDpxKt2xPjCLU*qE5jrKjGT#1piu#GNpUvlq5KbkF!ic6x0esCIMNXK&#XSvuc%s5@8^tRbRP4MkB9#oYgsT20vNE4t7HGG%wFePC0tcwFfALZW!Kjp70C4ph@cpzxcP*438lJL1@qWG@1mA^IbeWMHkBPY!3KXvC*R%AZdVjbsSjIZy1NaIZ0ku6AVf5E4I3C1L4!*2RZQb^M^zkqcI7yHuy8Q924NQbsc8DrbYvuR6GD4s8cKcgC7fg4h5#ajoF@m5s1LQPBMYd@7eT7a*#m&b5!2^*!!ZUgyAjU1$qHYl6ru!bCPV*Jexk2C&%6AT&x#qYKTAr2dur0#@GlR%x!cJSLIwAK6Gw3R7$Fm28HMObxx1VXUwzO!#T4ObaS\n--- PASS: TestSignature (0.00s)"
)

func BenchmarkEncSignature(b *testing.B) {
	b.StopTimer()
	b.StartTimer()
	for i := 0; i < b.N; i++ { //use b.N for looping
		_, err := encipherClient.Signature(msg)
		if err != nil {
			panic(err)
		}
	}
}

func BenchmarkEncNextId(b *testing.B) {
	b.StopTimer()
	b.StartTimer()
	for i := 0; i < b.N; i++ { //use b.N for looping
		_, err := encipherClient.NextId()
		if err != nil {
			panic(err)
		}
	}
}

func BenchmarkEncPublicKey(b *testing.B) {
	b.StopTimer()
	b.StartTimer()
	for i := 0; i < b.N; i++ { //use b.N for looping
		_, err := encipherClient.PublicKey()
		if err != nil {
			panic(err)
		}
	}
}

func BenchmarkEncVerify(b *testing.B) {
	b.StopTimer()
	b.StartTimer()
	for i := 0; i < b.N; i++ { //use b.N for looping
		_, err := encipherClient.SignatureVerify(msg, "a0aed385a895109ca3e82d6ba0fcbaecc717fb8ca47ede204409e7428408be377901d5cccc7e8ddf5bf195c39dfaf0e4")
		if err != nil {
			panic(err)
		}
	}
}

func BenchmarkEncEncrypt(b *testing.B) {
	b.StopTimer()
	b.StartTimer()
	for i := 0; i < b.N; i++ { //use b.N for looping
		_, err := encipherClient.Encrypt(msg)
		if err != nil {
			panic(err)
		}
	}
}

func BenchmarkEncDecrypt(b *testing.B) {
	b.StopTimer()
	b.StartTimer()
	for i := 0; i < b.N; i++ { //use b.N for looping
		_, err := encipherClient.Decrypt("RGFGQVhqYzBYNEVkWjJ6d0tSNnFBeFZOexYFJpG7J6Hw+QJKMsxNGerIJRs8R70rh2DbnlPm7CBcCZyPOoPW63JnlU0bHTu5T9Qx3SZFxPeRLH4rRZtw1ukHcTpSTL8ViDIcP4dah0289+rQL7yUISWbiHLMku5KmcWgAyqOaUOP99axHIPJgEqUXn4MYxBT1GSXoRym6RPNh2ax9mz5zLI3B93MZztC6oX6m+Lx4kG8K7cME5bz/vWFTofl6IUYFyWtqbi9M3heJPUh9jusE/J/KcwpSA4at0V9ItH+tN8bsMavmKA0zHasgU9dmrVyn2crEWoR8lU64PFfbkHLDemTviG6+rt3KhZ5UdU0zvECV2bRWcrprAE1UGwhX4PZJvJnyf3p3tpHnY1dMIKtZtfz457bLBVDYRdF6ulQ+dht5qKe9EsIt3dk6c22SJ1P5dCZIfPAwxA3lNP1OKI3C07aatoiNvlrPYoXKFRg1nLkZRKCm1D5BZUWI9qsSVMRXT/T9zoZaBDVjNlBDlW955MuqoE1luIjzr6TIxLx6aD5uR/wdWy+YRhgwgQfSqAAGII49f5wd2iFCBRysc/p1RfLtldtMDgFE4+dFjn5LfUBkXidwbc7RhKoiRsaNFcTt+YkgCoSaqJMAypeVelV8w+y8NkPBNL0qhM2OyUtC/aC567Nxi6zCO/m3PgYUWr3luGq8x+hRWwSmQydrsjCjw0OhbVIpZnMWXKFndGCfbA/Pns6QeNGATc131M4D6XirbLPnt0u0YS50ndRNLLBaRQRaTtNdkm7STK40flWGHEyoh8pUdHVcLXhbvXXtFXMegRTDwjmtCSi6ZIgbqQiWAyqT/R7gAXS29AW66r96Sz6RWpEFBpbUrON6ZLssD0bRIc8Kqa+ZGpOZgZTaWCUQZxKRcW96ePlCPAbGwm3BKNtB7KX5DF5GqEt/iRsN/9n2Ft5105Ff1RwFnRmyGYcuyDu4CHGIMfk/2B8p7c4QQJK0AfYoSo9NYSH5Rbz/GWLhf8SiUEzzX2ScqnquCOng+Tvj47N7YMZVxlSkXxmflp1x/KtU0on3DnDHQMETdrRijwuRDdGDllG0z/mLKLZL7RnoSi0yH7MU4M5lDOFjqbemZRJhF1puCHk91nvuGFWfV3vB9HYfu4/UUwhfMsyjDwG5+iH7Y9cGUJILI/osWpUW1+tQ+yFu37yCbVRgiP36RzIFpVipFVnCQWPRA/vZHkTWLCB3RUHB3vp0xp6+GNvIok5dz8KjG/h3hgGKYoJYwR0lFHG04u9rJRf/58uS3LKn+c3j7MRTdM+y5kdYi+GoAbhuAIq4a9IyyAvGF0MlwouMXDOFXCB0ajeXOCXdVpSzdpqUAlM/TCPtIktJ8RYbxb865ihSyOBAyC9k1gCFyH2YF5B0h2xs52wMVP1XERUitDtdXCYIFMTdt5bu7lIYsVXUzTZs/JDbB6dXGpzoqSuGkahxzRlidTQc1qPDYp8jD4FY3lE2yllAHYGJBaaAaf2qPDH2rbveVjYjv4okhoJaZ6+mBOOjgUSi41d5ikypzcBI57iAc2buySwr7UrdekhJtR+xQCM05PfIY5i/BCbap5BStEgFsPR33NppJPgd4G3iD0bjM8DX1at7uKIia6LTvgz4BT+PgI1pwJC/lcHSvKmc+iHwzc29zebn5v4NvvgWZedYIG4+SeriYhb60coiBkUne3dJoVqw6GG2GPF2LQY//pl4DhTwIyBw65TfsQL1tpgB5WozWxFFUGfDF244lH5wZzf0McA6XCJpieYqn29eY8tG55mJk4tNO88VaAu08BFT3QBmHt7cP6f5H0QFa8YtuKdFFRSeEgrwi1uDLPe+Wd24yel0qg68Unwx5VZXYPUc8Etbz6VqqpBSq6lR2UD8HkLbamUll4TFv/w1YmatNoOckV8FBpR8RVKHT3O/km9OL20j+2V2Ju2nxKd41K2oL4pjhPvem6ybn3kw/Xnm61v80vSibHfq/G9sK4DeoVMxZJhzxwSTCew3oQhl3wlY9QNdgHBhj+dKMvsybmx+TAGpvqH2C8Dyl5kFvkgvBITbLH8/+HLHjdHqZdWO1YOsW84jf58K3wYXaAmOjUOU8nGJAIQc8pbo8SnPdrociXYwPnH7GkEoiziSvDsqi68bq3dVXlZqy2H3ZZ/lMTzP+vcUvxYXm7BpDKnM0tvCPqIT+Iz59w5LkHIVeXpzQDacxqmcjJUvET+W/svYE/7nUM0qGsRHwsVaSm5zmHMsoxbJj+pqDk0wcfrCGiKpzJhiSv7BtSb3KOT6E+L7MerObhSH46Fi2rUUhp1WRhFvwdOaNhSJehPn16a0X5Og2EeS0cW3X5NB8lnJRSbcuElozkJb4gQ5I4KN6CPTCe8wm1nn9qJg3ySJPbVKYKNca9J73kGnCu7oevI+yBbpYMVZzI9U2s/td4f+wyXtU1WyFoRumsmQQ1iF5/W6wkBm88bRgcKQV5gbFSRAwL7JbedX80kVox8D8KEhuY2mYpbBN1Ai/S8oIE92dUvH148weYNgxFFkZ1gIcRDSOzqG20bNbAZRcI0cr+41mxJkW22JVoayg2mzv+//+Dw9qZ8ryusPYnpDcVlsYefmP9ld97aqj7UuOytGPrclfAehwO5ifjGwO0LToKjWnoFeNH+wC+zSijeEEwtoqfK9yb/aiBANvcaFU3lGNtgGLJOL/eTtV4XJu9l1GWHXjc166vHKoHEUzmzppR+GP5ZKDyAUkep8sf8J2SLErFywRdQWTqH50wwVjaaZBnAnDuoPAPTfMaJLAwOgJQ4384qzPhyVbDOjAw=")
		if err != nil {
			panic(err)
		}
	}
}

goos: windows                            
goarch: amd64                            
cpu: 12th Gen Intel(R) Core(TM) i5-12400F
BenchmarkEncSignature-12           15465             73376 ns/op           15596 B/op         29 allocs/op
BenchmarkEncNextId-12              56492             20899 ns/op             113 B/op          3 allocs/op
BenchmarkEncPublicKey-12           57313             20857 ns/op             209 B/op          3 allocs/op
BenchmarkEncVerify-12              16543             72787 ns/op           15592 B/op         29 allocs/op
BenchmarkEncEncrypt-12             17452             68853 ns/op           31002 B/op         46 allocs/op
BenchmarkEncDecrypt-12             17481             70527 ns/op           29987 B/op         44 allocs/op
```