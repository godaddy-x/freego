package node

//easyjson:json
type PublicKey struct {
	Key string `json:"key"` // 服务端临时公钥
	Tag string `json:"tag"` // 客户端临时公钥
	Noc string `json:"noc"` // 随机数
	Sig string `json:"sig"` // 数据签名
	Exp int64  `json:"exp"` // 有效时间，单位秒
}

//easyjson:json
type PrivateKey struct {
	Key string `json:"key"` // 私钥Base64编码格式
	Noc string `json:"noc"` // 随机数
}

// JsonBody 结构体 - 64字节 (5个字段，8字节对齐，无填充)
// 排列优化：string字段组在前(48字节)，int64字段组在后(16字节)
//
//easyjson:json
type JsonBody struct {
	// 16字节字段组 (3个string字段，48字节)
	Data   string `json:"d"` // 16字节 (8+8) - string字段
	Nonce  string `json:"n"` // 16字节 (8+8) - string字段
	Sign   string `json:"s"` // 16字节 (8+8) - string字段
	Valid  string `json:"e"` // 16字节 (8+8) - string字段
	Router string `json:"r"` // 16字节 (8+8) - string字段

	// 8字节字段组 (2个int64字段，16字节)
	Time int64 `json:"t"` // 8字节 - int64字段
	Plan int64 `json:"p"` // 0.默认(登录状态) 1.AES(登录状态) 2.RSA/ECC模式(匿名状态) 3.独立验签模式(匿名状态) - 8字节
}

// JsonResp 结构体 - 96字节 (7个字段，8字节对齐，无填充)
// 排列优化：string字段组在前(64字节)，int和int64字段组在后(32字节)
//
//easyjson:json
type JsonResp struct {
	// 16字节字段组 (4个string字段，64字节)
	Message string `json:"m"` // 16字节 (8+8) - string字段
	Data    string `json:"d"` // 16字节 (8+8) - string字段
	Nonce   string `json:"n"` // 16字节 (8+8) - string字段
	Sign    string `json:"s"` // 16字节 (8+8) - string字段
	Valid   string `json:"e"` // 16字节 (8+8) - string字段
	Router  string `json:"r"` // 16字节 (8+8) - 路由字段，用于消息订阅

	// 8字节字段组 (3个字段：1个int+2个int64，24字节)
	Code int   `json:"c"` // 8字节 - int字段
	Time int64 `json:"t"` // 8字节 - int64字段
	Plan int64 `json:"p"` // 8字节 - int64字段
}
