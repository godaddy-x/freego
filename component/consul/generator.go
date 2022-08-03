package consul

import (
	"github.com/godaddy-x/freego/util"
)

type ReqObj struct {
	Node int64  // seed node
	Kind string // default snowflake
}

type ResObj struct {
	Value int64
}

type WorkId interface {
	Generate(req *ReqObj, res *ResObj) error
}

type SnowflakeWorkId struct {
}

func (self *SnowflakeWorkId) Generate(req *ReqObj, res *ResObj) error {
	if req == nil {
		return util.Error("request parameter invalid")
	}
	res.Value = util.GetSnowFlakeIntID(req.Node)
	return nil
}

func StartSnowflakeServe() {
	new(ConsulManager).InitConfig(ConsulConfig{Node: "dc/snowflake",})

	mgr, err := new(ConsulManager).Client()
	if err != nil {
		panic(err)
	}

	mgr.AddRPC(&CallInfo{
		Tags:  []string{"ID生成器服务"},
		Iface: &SnowflakeWorkId{},
	})

	mgr.StartListenAndServe()
}
