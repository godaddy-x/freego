package utils

import (
	"math/rand"
	"time"
	"unsafe"
	_ "unsafe"
)

const numbers = "0123456789"
const letters = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
const lettersSp = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ!@#$%^&*"

var src = rand.NewSource(time.Now().UnixNano())

const (
	// 6 bits to represent a letter index
	letterIdBits = 6
	// All 1-bits as many as letterIdBits
	letterIdMask = 1<<letterIdBits - 1
	letterIdMax  = 63 / letterIdBits
)

//go:linkname fastrand runtime.fastrand
func fastrand() uint32

func gorand(n int, letters string) string {
	b := make([]byte, n)
	// A rand.Int63() generates 63 random bits, enough for letterIdMax letters!
	for i, cache, remain := n-1, src.Int63(), letterIdMax; i >= 0; {
		if remain == 0 {
			cache, remain = src.Int63(), letterIdMax
		}
		if idx := int(cache & letterIdMask); idx < len(letters) {
			b[i] = letters[idx]
			i--
		}
		cache >>= letterIdBits
		remain--
	}
	return *(*string)(unsafe.Pointer(&b))
}

func RandStr(n int, b ...bool) string {
	if len(b) > 0 {
		return gorand(n, lettersSp)
	}
	return gorand(n, letters)
}

func RandInt(n int) string {
	return gorand(n, numbers)
}

func RandNonce() string {
	return RandStr(8)
}

// ModRand 调用底层生成随机数,进行取模运算,性能提升10倍
func ModRand(n int) int {
	return int(fastrand() % uint32(n))
}
