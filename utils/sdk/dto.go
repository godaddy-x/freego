package sdk

// AuthToken 认证令牌结构体
// 包含JWT token、动态secret和过期时间
//
//easyjson:json
type AuthToken struct {
	Token   string `json:"token"`   // JWT认证令牌
	Secret  string `json:"secret"`  // 动态生成的AES密钥(Base64编码)
	Expired int64  `json:"expired"` // 令牌过期时间戳(Unix秒)
}
