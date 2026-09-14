package crypto

// Cipher 非对称加解密 / 签名抽象。当前生产实现为 ML-DSA-87（见 mldsa87.go）。
type Cipher interface {
	GetPrivateKey() (interface{}, string)
	GetPublicKey() (interface{}, string)
	Encrypt(msg, aad []byte) (string, error)
	Decrypt(msg string, aad []byte) ([]byte, error)
	Sign(msg []byte) ([]byte, error)
	Verify(msg, sign []byte) error
}
