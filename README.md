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
type NewPostFilter struct{}

func (self *NewPostFilter) DoFilter(chain node.Filter, ctx *node.Context, handle node.PostHandle, args ...interface{}) error {
	ctx.AddStorage("httpLog", node.HttpLog{Method: ctx.Path, LogNo: utils.GetSnowFlakeStrID(), CreateAt: utils.Time()})
	if err := chain.DoFilter(chain, ctx, handle, args...); err != nil {
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

