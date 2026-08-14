package jwt

import (
	"crypto/hkdf"
	"crypto/sha512"
)

// DerivedKeySize HKDF 输出长度（JWT 签名段、GetTokenSecret、Plan2 HKDFKey 等均 32 字节）。
const DerivedKeySize = 32

// DeriveKeySHA512 统一密钥派生：HKDF-SHA512(IKM, salt, info) → 32 字节（RFC 5869，Hash=SHA-512）。
func DeriveKeySHA512(ikm, salt []byte, info string) ([]byte, error) {
	return hkdf.Key(sha512.New, ikm, salt, info, DerivedKeySize)
}
