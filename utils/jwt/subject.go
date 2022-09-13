package jwt

/**
 * @author shadow
 * @createby 2018.12.13
 */

import (
	"github.com/godaddy-x/freego/utils"
	"strings"
)

const (
	JWT          = "JWT"
	HS256        = "HS256"
	SHA256       = "SHA256"
	MD5          = "MD5"
	AES          = "AES"
	RSA          = "RSA"
	FIVE_MINUTES = int64(300)
	TWO_WEEK     = int64(1209600)
)

type Subject struct {
	Header  *Header
	Payload *Payload
}

type JwtConfig struct {
	TokenKey string
	TokenAlg string
	TokenTyp string
	TokenExp int64
}

type Header struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

type Payload struct {
	Sub string `json:"sub"` // 用户主体
	Aud string `json:"aud"` // 接收token主体
	Iss string `json:"iss"` // 签发token主体
	Iat int64  `json:"iat"` // 授权token时间1
	Exp int64  `json:"exp"` // 授权token过期时间
	Dev string `json:"dev"` // 设备类型,web/app
	Jti string `json:"jti"` // 唯一身份标识,主要用来作为一次性token,从而回避重放攻击
	Ext string `json:"ext"` // 扩展信息
}

func (self *Subject) AddHeader(config JwtConfig) *Subject {
	self.Header = &Header{Alg: config.TokenAlg, Typ: config.TokenTyp}
	return self
}

func (self *Subject) Create(sub string) *Subject {
	self.Payload = &Payload{
		Sub: sub,
		Exp: utils.TimeSecond() + TWO_WEEK,
		Jti: utils.MD5(utils.GetUUID(), true),
	}
	return self
}

// exp seconds
func (self *Subject) Expired(exp int64) *Subject {
	if exp > 0 {
		if self.Payload.Iat > 0 {
			self.Payload.Exp = self.Payload.Iat + exp
		} else {
			self.Payload.Exp = utils.TimeSecond() + exp
		}
	}
	return self
}

func (self *Subject) Dev(dev string) *Subject {
	if len(dev) > 0 {
		self.Payload.Dev = dev
	}
	return self
}

func (self *Subject) Iss(iss string) *Subject {
	if len(iss) > 0 {
		self.Payload.Iss = iss
	}
	return self
}

func (self *Subject) Aud(aud string) *Subject {
	if len(aud) > 0 {
		self.Payload.Aud = aud
	}
	return self
}

func (self *Subject) Generate(config JwtConfig) string {
	self.AddHeader(config)
	header, err := utils.ToJsonBase64(self.Header)
	if err != nil {
		return ""
	}
	payload, err := utils.ToJsonBase64(self.Payload)
	if err != nil {
		return ""
	}
	part1 := utils.AddStr(header, ".", payload)
	return part1 + "." + self.Signature(part1, config.TokenKey)
}

func (self *Subject) Signature(text, key string) string {
	return utils.HMAC_SHA256(text, utils.AddStr(utils.GetLocalSecretKey(), key), true)
}

func (self *Subject) GetTokenSecret(token, secret string) string {
	key := utils.GetLocalTokenSecretKey()
	key2 := utils.HMAC_SHA256(utils.AddStr(utils.SHA256(token, true), utils.MD5(utils.GetLocalSecretKey()), true), secret, true)
	return key2[0:15] + key[3:13] + key2[15:30] + key[10:20] + key2[30:]
}

func (self *Subject) Verify(token, key string, decode bool) error {
	if len(token) == 0 {
		return utils.Error("token is nil")
	}
	part := strings.Split(token, ".")
	if part == nil || len(part) != 3 {
		return utils.Error("token part length invalid")
	}
	part0 := part[0]
	part1 := part[1]
	part2 := part[2]
	if self.Signature(utils.AddStr(part0, ".", part1), key) != part2 {
		return utils.Error("token signature invalid")
	}
	b64 := utils.Base64URLDecode(part1)
	if b64 == nil || len(b64) == 0 {
		return utils.Error("token part base64 data decode failed")
	}
	if int64(utils.GetJsonInt(b64, "exp")) <= utils.TimeSecond() {
		return utils.Error("token expired or invalid")
	}
	if decode {
		//payload := &Payload{}
		//if err := utils.ParseJsonBase64(part1, payload); err != nil {
		//	return err
		//}
		//self.Payload = payload
		if decode {
			self.Payload = &Payload{
				Sub: utils.GetJsonString(b64, "sub"),
				Ext: utils.GetJsonString(b64, "ext"),
			}
		}
	}
	return nil
}

// 获取token的私钥
func GetTokenSecret(token, secret string) string {
	if len(token) == 0 {
		return ""
	}
	subject := &Subject{}
	return subject.GetTokenSecret(token, secret)
}
