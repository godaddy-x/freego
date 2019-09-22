package jwt

/**
 * @author shadow
 * @createby 2018.12.13
 */

import (
	"github.com/godaddy-x/freego/util"
	"strings"
)

const (
	JWT    = "JWT"
	SHA256 = "SHA256"
	MD5    = "MD5"

	FIVE_MINUTES = int64(300000);
	QUARTER_HOUR = int64(900000);
	HALF_HOUR    = int64(1800000);
	ONE_DAY      = int64(86400000);
	TWO_WEEK     = int64(1209600000);
)

type Subject struct {
	Payload *Payload
}

type SubjectChecker struct {
	Subject      *Subject
	Content      string
	Signature    string
	SignatureKey string
}

type SecretKey struct {
	ApiSecretKey string
	JwtSecretKey string
	SecretKeyAlg string
}

type Authorization struct {
	AccessToken  string `json:"accessToken"`  // 授权Token
	Signature    string `json:"signature"`    // Token签名
	AccessKey    string `json:"accessKey"`    // 授权签名密钥
	SignatureKey string `json:"signatureKey"` // Token签名密钥
	ExpireDate   int64  `json:"expireDate"`   // 授权签名过期时间
}

type Payload struct {
	Sub string            `json:"sub"` // 用户主体
	Aud string            `json:"aud"` // 接收token主体
	Iss string            `json:"iss"` // 签发token主体
	Iat int64             `json:"iat"` // 授权token时间1
	Exp int64             `json:"exp"` // 授权token过期时间
	Nbf int64             `json:"nbf"` // 定义在什么时间之前,该token都是不可用的
	Jti string            `json:"jti"` // 唯一身份标识,主要用来作为一次性token,从而回避重放攻击
	Ext map[string]string `json:"ext"` // 扩展信息
}

func (self *Subject) GetAuthorization(key *SecretKey) (*Authorization, error) {
	jwt_secret_key := key.JwtSecretKey
	api_secret_key := key.ApiSecretKey
	if len(jwt_secret_key) == 0 {
		return nil, util.Error("secret key is nil")
	}
	if self.Payload == nil {
		return nil, util.Error("payload is nil")
	}
	var content, signature, access_token, signature_key, access_key string
	self.Payload.Jti = util.MD5(util.GetSnowFlakeStrID(), util.GetRandStr(6, true), self.Payload.Sub)
	if msg, err := util.JsonMarshal(self.Payload); err != nil {
		return nil, err
	} else if content = util.Base64Encode(msg); len(content) == 0 {
		return nil, util.Error("content is nil")
	}
	signature_key = util.MD5(jwt_secret_key, content, ".")                 // 生成数据签名密钥
	signature = util.MD5(jwt_secret_key, signature_key, ".", content, ".") // 通过密钥获得数据签名
	access_token = util.AddStr(content, ".", signature)
	access_key = util.GetApiAccessKeyByMD5(access_token, api_secret_key)
	return &Authorization{
		AccessToken:  access_token,
		Signature:    signature,
		AccessKey:    access_key,
		SignatureKey: signature_key,
		ExpireDate:   self.Payload.Exp,
	}, nil
}

func (self *Subject) GetSubjectChecker(access_token string) (*SubjectChecker, error) {
	if len(access_token) == 0 {
		return nil, util.Error("access token is nil")
	}
	token_str := strings.Split(access_token, ".")
	if len(token_str) != 2 {
		return nil, util.Error("access token invalid")
	}
	var content, signature string
	for i, v := range token_str {
		if i == 0 {
			content = v
		} else {
			signature = v
		}
	}
	if len(signature) < 32 {
		return nil, util.Error("signature invalid")
	}
	if b := util.Base64Decode(content); b == nil {
		return nil, util.Error("content invalid")
	} else if err := util.JsonUnmarshal(b, self.Payload); err != nil {
		return nil, util.Error("content error")
	}
	if self.Payload == nil {
		return nil, util.Error("payload is nil")
	}
	return &SubjectChecker{
		Subject:   self,
		Content:   content,
		Signature: signature,
	}, nil
}

func (self *Subject) GetRole() []string {
	ext := self.Payload.Ext
	if ext == nil || len(ext) == 0 {
		return make([]string, 0)
	}
	val, ok := ext["rol"]
	if !ok {
		return make([]string, 0)
	}
	spl := strings.Split(val, ",")
	role := make([]string, 0, len(spl))
	for _, v := range spl {
		if len(v) > 0 {
			role = append(role, v)
		}
	}
	return role
}

func (self *Subject) SetRole(roleList string) {
	if self.Payload.Ext == nil {
		self.Payload.Ext = map[string]string{"rol": roleList}
	} else {
		self.Payload.Ext["rol"] = roleList
	}
}

func (self *SubjectChecker) Authentication(signature_key, jwt_secret_key string) error {
	content := self.Content
	signature := self.Signature
	if len(signature_key) < 32 {
		return util.Error("signature invalid")
	}
	if signature_key != util.MD5(jwt_secret_key, content, ".") {
		return util.Error("signature_key invalid")
	} else if signature != util.MD5(jwt_secret_key, signature_key, ".", content, ".") {
		return util.Error("signature error")
	}
	self.SignatureKey = signature_key
	return nil
}
