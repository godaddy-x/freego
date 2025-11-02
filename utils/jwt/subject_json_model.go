package jwt

// Subject 结构体 - 16字节 (2个指针，8字节对齐，无填充)
// 排列优化：指针字段连续排列
//
//easyjson:json
type Subject struct {
	Header  *Header  // 8字节 - 指针字段
	Payload *Payload // 8字节 - 指针字段
}

// Header 结构体 - 32字节 (2个string，8字节对齐，无填充)
// 排列优化：string字段连续排列
//
//easyjson:json
type Header struct {
	Alg string `json:"alg"` // 16字节 (8+8) - string字段
	Typ string `json:"typ"` // 16字节 (8+8) - string字段
}

// Payload 结构体 - 112字节 (8个字段，8字节对齐，无填充)
// 排列优化：string字段组在前(96字节)，int64字段组在后(16字节)
//
//easyjson:json
type Payload struct {
	// 16字节字段组 (6个string字段，96字节)
	Sub string `json:"sub"` // 用户主体 - 16字节 (8+8)
	Aud string `json:"aud"` // 接收token主体 - 16字节 (8+8)
	Iss string `json:"iss"` // 签发token主体 - 16字节 (8+8)
	Dev string `json:"dev"` // 设备类型,web/app - 16字节 (8+8)
	Jti string `json:"jti"` // 唯一身份标识,主要用来作为一次性token,从而回避重放攻击 - 16字节 (8+8)
	Ext string `json:"ext"` // 扩展信息 - 16字节 (8+8)

	// 8字节字段组 (2个int64字段，16字节)
	Iat int64 `json:"iat"` // 授权token时间 - 8字节
	Exp int64 `json:"exp"` // 授权token过期时间 - 8字节
}
