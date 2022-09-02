# freego
High performance secure GRPC/ORM framework

#### Create simple demo
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


#### Create plugin filter
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
```

#### Benchmark test webapi /pubkey
```
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



