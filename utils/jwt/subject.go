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
	Header       *Header
	Payload      *Payload
	tokenBytes   []byte
	payloadBytes []byte
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
	Iat int64  `json:"iat"` // 授权token时间
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
		Exp: utils.UnixSecond() + TWO_WEEK,
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
			self.Payload.Exp = utils.UnixSecond() + exp
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
	if config.TokenExp > 0 {
		if self.Payload.Iat > 0 {
			self.Payload.Exp = self.Payload.Iat + config.TokenExp
		} else {
			self.Payload.Exp = utils.UnixSecond() + config.TokenExp
		}
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
	return utils.HMAC_SHA512(utils.AddStr(token, utils.GetLocalTokenSecretKey(), utils.GetLocalSecretKey()), secret, true)
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
	b64 := utils.Base64Decode(part1)
	if b64 == nil || len(b64) == 0 {
		return utils.Error("token part base64 data decode failed")
	}
	if int64(utils.GetJsonInt(b64, "exp")) <= utils.UnixSecond() {
		return utils.Error("token expired or invalid")
	}
	if !decode {
		return nil
	}
	if self.Payload == nil {
		self.Payload = &Payload{}
	}
	self.payloadBytes = b64
	self.Payload.Sub = self.getStringValue("sub")
	return nil
}

func (self *Subject) CheckReady() bool {
	if self.Payload == nil || len(self.Payload.Sub) == 0 {
		return false
	}
	return true
}

func (self *Subject) ResetTokenBytes(b []byte) {
	if b == nil && len(self.tokenBytes) == 0 {
		return
	}
	self.tokenBytes = nil
	if b == nil {
		return
	}
	self.tokenBytes = b
}

func (self *Subject) ResetPayloadBytes(b []byte) {
	if b == nil && len(self.payloadBytes) == 0 {
		return
	}
	self.payloadBytes = nil
	if b == nil {
		return
	}
	self.payloadBytes = b
}

func (self *Subject) GetRawBytes() []byte {
	if len(self.tokenBytes) == 0 {
		return []byte{}
	}
	return self.tokenBytes
}

func (self *Subject) GetSub() string {
	if self.Payload == nil {
		self.Payload = &Payload{}
	}
	if len(self.Payload.Sub) == 0 {
		self.Payload.Sub = self.getStringValue("sub")
	}
	return self.Payload.Sub
}

func (self *Subject) GetIss() string {
	return self.getStringValue("iss")
}

func (self *Subject) GetAud() string {
	return self.getStringValue("aud")
}

func (self *Subject) GetIat() int64 {
	return self.getInt64Value("iat")
}

func (self *Subject) GetExp() int64 {
	return self.getInt64Value("exp")
}

func (self *Subject) GetDev() string {
	return self.getStringValue("dev")
}

func (self *Subject) GetJti() string {
	return self.getStringValue("jti")
}

func (self *Subject) GetExt() string {
	return self.getStringValue("ext")
}

func (self *Subject) getStringValue(k string) string {
	if len(self.payloadBytes) == 0 {
		return ""
	}
	return utils.GetJsonString(self.payloadBytes, k)
}

func (self *Subject) getInt64Value(k string) int64 {
	if len(self.payloadBytes) == 0 {
		return 0
	}
	return utils.GetJsonInt64(self.payloadBytes, k)
}

// 获取token的私钥
func GetTokenSecret(token, secret string) string {
	if len(token) == 0 {
		return ""
	}
	subject := &Subject{}
	return subject.GetTokenSecret(token, secret)
}
