package node

import (
	"sync"

	"github.com/godaddy-x/freego/utils/crypto"
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

// MessageHandler对象池
var messageHandlerPool = sync.Pool{
	New: func() interface{} {
		return &MessageHandler{}
	},
}

// GetMessageHandler 从池中获取一个MessageHandler对象
func GetMessageHandler(rsa []crypto.Cipher, handle Handle) *MessageHandler {
	mh := messageHandlerPool.Get().(*MessageHandler)
	mh.rsa = rsa       // 设置RSA密钥
	mh.handle = handle // 设置路由处理器
	return mh
}

// PutMessageHandler 将MessageHandler对象放回池中，并重置其字段
func PutMessageHandler(mh *MessageHandler) {
	if mh == nil {
		return
	}
	// 重置所有字段，避免数据污染
	// 注意：rsa字段不需要重置，因为它是共享的只读数据
	// handle字段会在下次获取时重新设置
	mh.handle = nil
	messageHandlerPool.Put(mh)
}
