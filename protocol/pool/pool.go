package pool

import (
	"sync"

	"github.com/godaddy-x/freego/protocol/wire"
)

var jsonBodyPool = sync.Pool{
	New: func() interface{} {
		return &wire.JsonBody{}
	},
}

var jsonRespPool = sync.Pool{
	New: func() interface{} {
		return &wire.JsonResp{}
	},
}

func GetJsonBody() *wire.JsonBody {
	return jsonBodyPool.Get().(*wire.JsonBody)
}

func PutJsonBody(body *wire.JsonBody) {
	if body == nil {
		return
	}
	body.Data = ""
	body.Nonce = ""
	body.Sign = ""
	body.Valid = ""
	body.Router = ""
	body.Time = 0
	body.Plan = 0
	body.User = 0
	jsonBodyPool.Put(body)
}

func GetJsonResp() *wire.JsonResp {
	return jsonRespPool.Get().(*wire.JsonResp)
}

func PutJsonResp(resp *wire.JsonResp) {
	if resp == nil {
		return
	}
	resp.Code = 0
	resp.Message = ""
	resp.Data = ""
	resp.Nonce = ""
	resp.Router = ""
	resp.Time = 0
	resp.Plan = 0
	resp.Valid = ""
	jsonRespPool.Put(resp)
}
