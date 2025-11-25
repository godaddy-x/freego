package node

import (
	"sync"
)

// JsonBody对象池，用于降低GC压力
var jsonBodyPool = sync.Pool{
	New: func() interface{} {
		return &JsonBody{}
	},
}

// JsonResp对象池，用于降低GC压力
var jsonRespPool = sync.Pool{
	New: func() interface{} {
		return &JsonResp{}
	},
}

// GetJsonBody 从对象池获取JsonBody对象
func GetJsonBody() *JsonBody {
	return jsonBodyPool.Get().(*JsonBody)
}

// PutJsonBody 将JsonBody对象放回对象池
// 注意：放回前需要重置对象字段，避免数据污染
func PutJsonBody(body *JsonBody) {
	if body == nil {
		return
	}

	// 重置所有字段
	body.Data = ""
	body.Nonce = ""
	body.Sign = ""
	body.Valid = ""
	body.Router = ""
	body.Time = 0
	body.Plan = 0

	// 放回池中
	jsonBodyPool.Put(body)
}

// GetJsonResp 从对象池获取JsonResp对象
func GetJsonResp() *JsonResp {
	return jsonRespPool.Get().(*JsonResp)
}

// PutJsonResp 将JsonResp对象放回对象池
// 注意：放回前需要重置对象字段，避免数据污染
func PutJsonResp(resp *JsonResp) {
	if resp == nil {
		return
	}

	// 重置所有字段
	resp.Code = 0
	resp.Message = ""
	resp.Data = ""
	resp.Nonce = ""
	resp.Router = ""
	resp.Time = 0
	resp.Plan = 0
	resp.Valid = ""

	// 放回池中
	jsonRespPool.Put(resp)
}
