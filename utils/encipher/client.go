package encipher

type Client interface {
	// CheckReady 检测状态
	CheckReady() error

	// NextId 获取雪花ID下一个数值
	NextId() (int64, error)

	// PublicKey 获取加密器公钥
	PublicKey() (string, error)

	// Signature 签名数据
	Signature(input string) (string, error)

	// TokenSignature 授权令牌签名数据
	TokenSignature(token, input string) (string, error)

	// VerifySignature 签名数据验签
	VerifySignature(input, sign string) (bool, error)

	// TokenVerifySignature 授权令牌签名数据验签
	TokenVerifySignature(token, input, sign string) (bool, error)

	// ReadConfig 读取加密器指定配置
	ReadConfig(input string) (string, error)

	// AesEncrypt AES对称加密数据
	AesEncrypt(input string) (string, error)

	// AesDecrypt AES对称解密数据
	AesDecrypt(input string) (string, error)

	// EccEncrypt ECC对称加密数据
	EccEncrypt(input, publicKey string) (string, error)

	// EccDecrypt ECC对称解密数据
	EccDecrypt(input string) (string, error)

	// EccSignature ECC签名数据
	EccSignature(input string) (string, error)

	// EccVerifySignature ECC签名数据验签
	EccVerifySignature(input, sign string) (bool, error)

	// TokenEncrypt 授权令牌加密数据
	TokenEncrypt(token, input string) (string, error)

	// TokenDecrypt 授权令牌解密数据
	TokenDecrypt(token, input string) (string, error)

	// TokenCreate 授权令牌创建
	TokenCreate(input, dev string) (interface{}, error)

	// TokenVerify 授权令牌验证
	TokenVerify(input string) (string, error)
}
