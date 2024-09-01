package encipher

import (
	"github.com/godaddy-x/freego/utils/crypto"
	"github.com/godaddy-x/freego/utils/jwt"
)

type Param struct {
	EncryptKey      []byte
	SignKey         []byte
	SignDepth       int
	EcdsaPrivateKey []byte
	EcdsaPublicKey  string
	EccObject       crypto.Cipher
	JwtConfig       jwt.JwtConfig
}

type Server interface {
	// LoadConfig 读取配置
	LoadConfig(path string) (Param, error)

	// ReadConfig 读取加密配置
	ReadConfig(key []byte) []byte

	// Signature 数据签名
	Signature(input []byte) []byte

	// VerifySignature 数据签名验证
	VerifySignature(input, sign []byte) bool

	// TokenSignature JWT令牌数据签名
	TokenSignature(token, input []byte) []byte

	// TokenVerifySignature JWT令牌数据签名验证
	TokenVerifySignature(token, input, sign []byte) bool

	// AesEncrypt AES数据加密
	AesEncrypt(input []byte) ([]byte, error)

	// AesDecrypt AES数据解密
	AesDecrypt(input []byte) ([]byte, error)

	// EccEncrypt 私钥和客户端公钥协商加密
	EccEncrypt(input, publicTo []byte) ([]byte, error)

	// EccDecrypt 私钥和客户端公钥协商解密
	EccDecrypt(input []byte) ([]byte, error)

	// TokenEncrypt JWT令牌加密数据
	TokenEncrypt(token, input []byte) ([]byte, error)

	// TokenDecrypt JWT令牌解密数据
	TokenDecrypt(token, input []byte) ([]byte, error)

	// TokenCreate JWT令牌生成
	TokenCreate(input []byte) ([]byte, error)

	// TokenVerify JWT令牌校验
	TokenVerify(input []byte) ([]byte, error)
}
