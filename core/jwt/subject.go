package jwt

/**
 * @author shadow
 * @createby 2018.12.13
 */

import (
	"crypto/hmac"
	"crypto/sha256"
	"io"
	"strings"

	DIC "github.com/godaddy-x/freego/core/const"
	utils "github.com/godaddy-x/freego/core/str"
)

const (
	JWT                = "JWT"
	HS256              = "HS256"
	FIVE_MINUTES       = int64(300)
	TWO_WEEK           = int64(1209600)
	SubjectTokenSecret = "Subject-Token-Secret-V1"
	SubjectTokenVerify = "Subject-Token-Verify-V1"
	SubjectKDFSecret   = "Subject-KDF-Secret-V1"
	SubjectKDFVerify   = "Subject-KDF-Verify-V1"
)

// JwtConfig 结构体 - 56字节 (4个字段，8字节对齐，无填充)
// 排列优化：int64放在最后，利用string的16字节对齐
type JwtConfig struct {
	TokenKey string // 16字节 (8+8)
	TokenAlg string // 16字节 (8+8)
	TokenTyp string // 16字节 (8+8)
	TokenExp int64  // 8字节
}

// SimplePayload 结构体 - 24字节 (string+int64，8字节对齐，无填充)
// 排列优化：int64放在string后面，利用8字节对齐
type SimplePayload struct {
	Sub string // 用户主体 - 16字节 (8+8)
	Exp int64  // 授权token过期时间 - 8字节
}

func (self *Subject) AddHeader(config JwtConfig) *Subject {
	self.Header = &Header{Alg: config.TokenAlg, Typ: config.TokenTyp}
	return self
}

func (self *Subject) Create(sub string) *Subject {
	self.Payload = &Payload{
		Sub: sub,
		Exp: utils.UnixSecond() + TWO_WEEK,
		Jti: utils.GetUUID(true),
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
	if config.TokenAlg == "" {
		config.TokenAlg = HS256
	}
	if config.TokenTyp == "" {
		config.TokenTyp = JWT
	}
	// 当前仅实现 HMAC-SHA256 签名；拒绝在 header 中声明其它算法
	if config.TokenAlg != HS256 {
		return ""
	}
	self.AddHeader(config)
	headerBs, err := utils.JsonMarshal(self.Header)
	if err != nil {
		return ""
	}
	headerB64 := utils.Base64EncodeWithPool(headerBs)
	if config.TokenExp > 0 {
		if self.Payload.Iat > 0 {
			self.Payload.Exp = self.Payload.Iat + config.TokenExp
		} else {
			self.Payload.Exp = utils.UnixSecond() + config.TokenExp
		}
	}
	payloadBs, err := utils.JsonMarshal(self.Payload)
	if err != nil {
		return ""
	}
	payloadB64 := utils.Base64EncodeWithPool(payloadBs)
	part1 := utils.AddStr(headerB64, ".", payloadB64)
	return part1 + "." + self.Signature(part1, config.TokenKey)
}

func (self *Subject) Signature(text, key string) string {
	return utils.Base64Encode(self.GetTokenSecretExtract(text, key, SubjectKDFVerify, SubjectTokenVerify))
}

func (self *Subject) Verify(auth []byte, key string) error {
	if len(auth) == 0 {
		return utils.Error("token is nil")
	}

	token := utils.Bytes2Str(auth)

	part := strings.Split(token, ".")
	if part == nil || len(part) != 3 {
		return utils.Error("token part length invalid")
	}
	part0 := part[0]
	part1 := part[1]
	part2 := part[2]

	// 1) 先验签（恒定时间），再解析 payload，避免未认证数据进入反序列化路径
	headerPayload := utils.AddStr(part0, ".", part1)
	expected := self.GetTokenSecretExtract(headerPayload, key, SubjectKDFVerify, SubjectTokenVerify)
	if !utils.CompareBase64Sign(expected, part2) {
		return utils.Error("token signature invalid")
	}

	// 2) 可选校验 header.alg（签名已覆盖 header，此处拒绝非预期算法声明）
	headerRaw := utils.Base64DecodeWithPool(part0)
	if len(headerRaw) == 0 {
		return utils.Error("token header base64 data decode failed")
	}
	if self.Header == nil {
		self.Header = &Header{}
	}
	if err := utils.JsonUnmarshal(headerRaw, self.Header); err != nil {
		return utils.Error("token header parse failed")
	}
	if self.Header.Alg != HS256 {
		return utils.Error("token alg invalid")
	}

	// 3) 解析 payload 并校验过期（提前 15 秒，兼容时钟漂移）
	decodeB64 := utils.Base64DecodeWithPool(part1)
	if len(decodeB64) == 0 {
		return utils.Error("token part base64 data decode failed")
	}
	if self.Payload == nil {
		self.Payload = &Payload{}
	}
	if err := utils.JsonUnmarshal(decodeB64, self.Payload); err != nil {
		return utils.Error("token part parse failed")
	}
	if self.Payload.Exp <= utils.UnixSecond()-15 {
		return utils.Error("token expired or invalid")
	}

	return nil
}

func (self *Subject) CheckReady() bool {
	if self.Payload == nil || len(self.Payload.Sub) == 0 {
		return false
	}
	return true
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

// GetTokenSecret 最简单高效的派生密钥，因为token已经有足够的熵值
func (self *Subject) GetTokenSecret(token, secret string) []byte {
	return self.GetTokenSecretExtract(token, secret, SubjectKDFSecret, SubjectTokenSecret)
}

// GetTokenSecretExtract 最简单高效的派生密钥，因为token已经有足够的熵值
func (self *Subject) GetTokenSecretExtract(token, secret, kdfType, msgType string) []byte {
	h := hmac.New(sha256.New, utils.Str2Bytes(secret))
	_, _ = io.WriteString(h, msgType)
	_, _ = io.WriteString(h, DIC.SEP)
	localKey := utils.GetLocalDynamicSecretKey()
	if len(localKey) > 0 {
		_, _ = io.WriteString(h, localKey)
		_, _ = io.WriteString(h, DIC.SEP)
	}
	_, _ = io.WriteString(h, token)
	return h.Sum(nil)
}
