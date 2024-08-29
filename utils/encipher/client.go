package encipher

type Client interface {
	// NextId 获取雪花ID下一个数值
	NextId() (int64, error)

	// PublicKey 获取加密器公钥
	PublicKey() (string, error)

	// Signature 签名数据
	Signature(input string) (string, error)

	// TokenSignature 授权令牌签名数据
	TokenSignature(token, input string) (string, error)

	// SignatureVerify 签名数据验签
	SignatureVerify(input, target string) (bool, error)

	// TokenSignatureVerify 授权令牌签名数据验签
	TokenSignatureVerify(token, input, target string) (bool, error)

	// Config 读取加密器指定配置
	Config(input string) (string, error)

	// AesEncrypt AES对称加密数据
	AesEncrypt(input string) (string, error)

	// AesDecrypt AES对称解密数据
	AesDecrypt(input string) (string, error)

	// EccEncrypt ECC对称加密数据
	EccEncrypt(input, publicTo string) (string, error)

	// EccDecrypt ECC对称解密数据
	EccDecrypt(input string) (string, error)

	// TokenEncrypt 授权令牌加密数据
	TokenEncrypt(token, input string) (string, error)

	// TokenDecrypt 授权令牌解密数据
	TokenDecrypt(token, input string) (string, error)

	// TokenCreate 授权令牌创建
	TokenCreate(input string) (interface{}, error)

	// TokenVerify 授权令牌验证
	TokenVerify(input string) (string, error)
}
