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
	AES    = "AES"
	RSA    = "RSA"

	FIVE_MINUTES = int64(300000);
	QUARTER_HOUR = int64(900000);
	HALF_HOUR    = int64(1800000);
	ONE_DAY      = int64(86400000);
	TWO_WEEK     = int64(1209600000);
)

type Subject struct {
	Payload *Payload
}

type SecretKey struct {
	ApiSecretKey string
	JwtSecretKey string
	SecretKeyAlg string
}

type Payload struct {
	Sub int64             `json:"sub"` // 用户主体
	Aud string            `json:"aud"` // 接收token主体
	Iss string            `json:"iss"` // 签发token主体
	Iat int64             `json:"iat"` // 授权token时间1
	Exp int64             `json:"exp"` // 授权token过期时间
	Dev string            `json:"dev"` // 设备类型,web/app
	Jti string            `json:"jti"` // 唯一身份标识,主要用来作为一次性token,从而回避重放攻击
	Nsr string            `json:"nsr"` // 随机种子
	Ext map[string]string `json:"ext"` // 扩展信息
}

func (self *Subject) Create(sub int64) (*Subject) {
	nsr := util.Substr(util.MD5(util.GetSnowFlakeStrID()), 5, 21)
	iat := util.Time()
	self.Payload = &Payload{
		Sub: sub,
		Iat: iat,
		Exp: iat + TWO_WEEK,
		Jti: util.HMAC256(util.GetSnowFlakeStrID(), nsr),
		Nsr: nsr,
		Ext: map[string]string{},
	}
	return self
}

func (self *Subject) Expired(exp int64) (*Subject) {
	if exp > 0 {
		self.Payload.Exp = self.Payload.Iat + exp
	}
	return self
}

func (self *Subject) Dev(dev string) (*Subject) {
	if len(dev) > 0 {
		self.Payload.Dev = dev
	}
	return self
}

func (self *Subject) Iss(iss string) (*Subject) {
	if len(iss) > 0 {
		self.Payload.Iss = iss
	}
	return self
}

func (self *Subject) Aud(aud string) (*Subject) {
	if len(aud) > 0 {
		self.Payload.Aud = aud
	}
	return self
}

func (self *Subject) Extinfo(key, value string) (*Subject) {
	if len(key) > 0 && len(value) > 0 {
		self.Payload.Ext[key] = value
	}
	return self
}

func (self *Subject) Generate(key string) (string) {
	result, err := util.ToJsonBase64(self.Payload)
	if err != nil {
		return ""
	}
	return result + "." + self.Signature(result, key)
}

func (self *Subject) Signature(text, key string) (string) {
	return util.HMAC256(text, key)
}

func (self *Subject) GetTokenSecret(token string) (string) {
	s := util.SHA256(token)
	return util.SHA256(util.Substr(s, 15, 19), util.Substr(s, 12, 19), util.Substr(s, 11, 27))
}

func (self *Subject) Verify(token, key string) (error) {
	if len(token) == 0 {
		return util.Error("token is nil")
	}
	part := strings.Split(token, ".")
	if part == nil || len(part) != 2 {
		return util.Error("token part invalid")
	}
	part0 := part[0]
	part1 := part[1]
	if len(part1) != 64 || self.Signature(part0, key) != part1 {
		return util.Error("token sign invalid")
	}
	payload := &Payload{}
	if err := util.ParseJsonBase64(part0, payload); err != nil {
		return err
	}
	if payload.Exp <= util.Time() {
		return util.Error("token expired")
	}
	self.Payload = payload
	return nil
}

func (self *Subject) GetTokenRole() []int64 {
	ext := self.Payload.Ext
	if ext == nil || len(ext) == 0 {
		return make([]int64, 0)
	}
	val, ok := ext["rol"]
	if !ok {
		return make([]int64, 0)
	}
	spl := strings.Split(val, ",")
	role := make([]int64, 0, len(spl))
	for _, v := range spl {
		if len(v) > 0 {
			x, err := util.StrToInt64(v)
			if err != nil {
				continue
			}
			role = append(role, x)
		}
	}
	return role
}

// 获取token的私钥
func GetTokenSecret(token string) string {
	subject := &Subject{}
	return subject.GetTokenSecret(token)
}
