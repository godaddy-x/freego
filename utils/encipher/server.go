package encipher

import (
	"github.com/godaddy-x/freego/utils/crypto"
	"github.com/godaddy-x/freego/utils/jwt"
)

type Param struct {
	EncryptKey      string
	SignKey         string
	SignDepth       int
	EcdsaPrivateKey string
	EcdsaPublicKey  string
	EccObject       crypto.Cipher
	JwtConfig       jwt.JwtConfig
}

type Server interface {
	// LoadConfig 读取配置
	LoadConfig(path string) (Param, error)

	// ReadConfig 读取加密配置
	ReadConfig(key string) string

	// Signature 数据签名
	Signature(input string) string

	// TokenSignature JWT令牌数据签名
	TokenSignature(token, input string) string

	// VerifySignature 数据签名验证
	VerifySignature(input, sign string) bool

	// TokenVerifySignature JWT令牌数据签名验证
	TokenVerifySignature(token, input, sign string) bool

	// AesEncrypt AES数据加密
	AesEncrypt(input string) (string, error)

	// AesDecrypt AES数据解密
	AesDecrypt(input string) (string, error)

	// EccEncrypt 私钥和客户端公钥协商加密
	EccEncrypt(input, publicTo string) (string, error)

	// EccDecrypt 私钥和客户端公钥协商解密
	EccDecrypt(input string) (string, error)

	// TokenEncrypt JWT令牌加密数据
	TokenEncrypt(token, input string) (string, error)

	// TokenDecrypt JWT令牌解密数据
	TokenDecrypt(token, input string) (string, error)

	// TokenCreate JWT令牌生成
	TokenCreate(input string) (string, error)

	// TokenVerify JWT令牌校验
	TokenVerify(input string) (string, error)
}
