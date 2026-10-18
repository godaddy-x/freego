package grpcgw

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
)

func HMACSHA256(key, message []byte) []byte {
	m := hmac.New(sha256.New, key)
	m.Write(message)
	return m.Sum(nil)
}

func SHA256Sum(message []byte) []byte {
	h := sha256.Sum256(message)
	return h[:]
}

func ConstantTimeEqual(a, b []byte) bool {
	return subtle.ConstantTimeCompare(a, b) == 1
}
