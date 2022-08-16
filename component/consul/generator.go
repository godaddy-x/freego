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

func (self *ConsulManager) AddSnowflakeService() {
	self.AddRPC(&CallInfo{
		Tags:          []string{"ID Generator"},
		ClassInstance: &SnowflakeWorkId{},
	})
}

func GetWorkID() (int64, error) {
	mgr, err := new(ConsulManager).Client()
	if err != nil {
		return 0, err
	}
	req := &ReqObj{}
	res := &ResObj{}
	call := &CallInfo{
		Service:  "SnowflakeWorkId",
		Method:   "Generate",
		Request:  req,
		Response: res,
	}
	if err := mgr.CallRPC(call); err != nil {
		return 0, err
	}
	return res.Value, nil
}
