package jwt

/**
 * @author shadow
 * @createby 2018.12.13
 */

import (
	"crypto/sha512"
	"strings"

	"github.com/godaddy-x/freego/cache"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/zlog"
	"golang.org/x/crypto/pbkdf2"
)

var (
	localSubjectCache = cache.NewLocalCache(120, 10)
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
	Header     *Header
	Payload    *Payload
	tokenBytes []byte
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

type SimplePayload struct {
	Sub string // 用户主体
	Exp int64  // 授权token过期时间
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
	return self.GetTokenSecretEnhanced(token, secret)
}

func (self *Subject) Verify(token, key string, decode bool) error {
	if len(token) == 0 {
		return utils.Error("token is nil")
	}

	// 1. 获取本地密钥
	localKey := utils.GetLocalTokenSecretKey()

	// 2. 从token中提取sub（JWT中已包含sub，不同用户的sub不同，token也不同）
	//    token本身就包含了sub信息，所以使用token即可区分不同用户
	password := utils.AddStr("TokenVerify", token, localKey, key)

	// 3. 计算缓存键：基于password的SHA256
	//    不同token（包含不同sub）产生不同的缓存键
	cacheKey := utils.SHA256(password)

	// 2. 尝试从缓存获取Payload指针
	if value, b, err := localSubjectCache.Get(cacheKey, nil); err == nil && b && value != nil {
		simple := value.(*SimplePayload)
		// 分布式系统时间同步缓冲区：提前300秒判断过期，避免时间同步误差
		if simple.Exp <= utils.UnixSecond()-300 {
			// 失效的token主动清退
			_ = localSubjectCache.Del(cacheKey)
			return utils.Error("token expired")
		}
		if self.Payload == nil {
			self.Payload = &Payload{}
		}
		self.Payload.Sub = simple.Sub
		self.Payload.Exp = simple.Exp
		return nil
	}

	part := strings.Split(token, ".")
	if part == nil || len(part) != 3 {
		return utils.Error("token part length invalid")
	}
	part0 := part[0]
	part1 := part[1]
	part2 := part[2]

	// 性能优化1: 先检查过期时间（计算量小），避免过期token的昂贵签名验证
	decodeb64 := utils.Base64Decode(part1)
	if len(decodeb64) == 0 {
		return utils.Error("token part base64 data decode failed")
	}
	exp := utils.GetJsonInt64(decodeb64, "exp")
	// 分布式系统时间同步缓冲区：提前300秒判断过期，避免时间同步误差
	if exp <= utils.UnixSecond()-300 {
		return utils.Error("token expired or invalid")
	}

	// 性能优化2: 预计算header.payload，避免在Signature内重复拼接
	headerPayload := utils.AddStr(part0, ".", part1)
	if self.Signature(headerPayload, key) != part2 {
		return utils.Error("token signature invalid")
	}

	if self.Payload == nil {
		self.Payload = &Payload{}
	}
	//self.payloadBytes = b64
	self.Payload.Exp = exp
	self.Payload.Sub = self.getStringValue("sub", decodeb64)

	// 9. 缓存结果，1小时有效期（平衡性能和内存占用）
	if err := localSubjectCache.Put(cacheKey, &SimplePayload{Sub: self.Payload.Sub, Exp: self.Payload.Exp}, 3600); err != nil {
		zlog.Error("put localSubjectCache token verify failed", 0, zlog.AddError(err))
	}

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
	//if b == nil && len(self.payloadBytes) == 0 {
	//	return
	//}
	//self.payloadBytes = nil
	//if b == nil {
	//	return
	//}
	//self.payloadBytes = b
}

func (self *Subject) GetRawBytes() []byte {
	if len(self.tokenBytes) == 0 {
		return []byte{}
	}
	return self.tokenBytes
}

func (self *Subject) GetSub(b []byte) string {
	if self.Payload == nil {
		self.Payload = &Payload{}
	}
	if len(self.Payload.Sub) == 0 {
		self.Payload.Sub = self.getStringValue("sub", b)
	}
	return self.Payload.Sub
}

func (self *Subject) GetIss(b []byte) string {
	return self.getStringValue("iss", b)
}

func (self *Subject) GetAud(b []byte) string {
	return self.getStringValue("aud", b)
}

func (self *Subject) GetIat(b []byte) int64 {
	return self.getInt64Value("iat", b)
}

func (self *Subject) GetExp(b []byte) int64 {
	return self.getInt64Value("exp", b)
}

func (self *Subject) GetDev(b []byte) string {
	return self.getStringValue("dev", b)
}

func (self *Subject) GetJti(b []byte) string {
	return self.getStringValue("jti", b)
}

func (self *Subject) GetExt(b []byte) string {
	return self.getStringValue("ext", b)
}

func (self *Subject) getStringValue(k string, payload []byte) string {
	if len(payload) == 0 {
		return ""
	}
	return utils.GetJsonString(payload, k)
}

func (self *Subject) getInt64Value(k string, payload []byte) int64 {
	if len(payload) == 0 {
		return 0
	}
	return utils.GetJsonInt64(payload, k)
}

// 获取token的私钥
func GetTokenSecret(token, secret string) string {
	if len(token) == 0 {
		return ""
	}
	subject := &Subject{}
	return subject.GetTokenSecret(token, secret)
}

// GetTokenSecretEnhanced 获取token的私钥（增强版）
// 使用标准PBKDF2密钥派生，确保安全性和性能
func (self *Subject) GetTokenSecretEnhanced(token, secret string) string {

	// 1. 获取本地密钥
	localKey := utils.GetLocalTokenSecretKey()

	// 2. 从token中提取sub（JWT中已包含sub，不同用户的sub不同，token也不同）
	//    token本身就包含了sub信息，所以使用token即可区分不同用户
	password := utils.AddStr("GetTokenSecretEnhanced", token, localKey, secret)

	// 3. 计算缓存键：基于password的SHA256
	//    不同token（包含不同sub）产生不同的缓存键
	cacheKey := utils.SHA256(password)

	// 4. 尝试从缓存获取结果
	if value, err := localSubjectCache.GetString(cacheKey); err == nil && len(value) > 0 {
		return value
	}

	// 5. 计算盐值：使用不同顺序的参数组合（顺序：token, secret, localKey）
	//    与password顺序不同，确保缓存键和盐值不同，提升安全性
	//    即使缓存键泄露，攻击者也无法直接获得盐值
	salt := utils.SHA256(utils.AddStr(token, secret, localKey))

	// 6. 使用标准PBKDF2密钥派生（HMAC-SHA512，10,000次迭代）
	//    输出64字节密钥（SHA-512）
	derivedKey := pbkdf2.Key(utils.Str2Bytes(password), utils.Str2Bytes(salt), 10000, 64, sha512.New)

	// 7. HMAC-SHA512增强
	localKeyBytes := utils.Str2Bytes(localKey)
	enhancedHashBytes := utils.HMAC_SHA512_BASE(derivedKey, localKeyBytes)

	// 8. 转换为Base64字符串
	result := utils.Base64Encode(enhancedHashBytes)

	// 9. 缓存结果，1小时有效期（平衡性能和内存占用）
	if err := localSubjectCache.Put(cacheKey, result, 3600); err != nil {
		zlog.Error("put localSubjectCache token secret failed", 0, zlog.AddError(err))
	}
	return result
}
