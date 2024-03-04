package utils

import (
	"math/rand"
	_ "unsafe"
)

const numbers = "0123456789"
const letters = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
const lettersSp = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ!@#$%^&*"

//go:linkname fastrand runtime.fastrand
func fastrand() uint32

func gorand(n int, characters string) string {
	randomString := make([]byte, n)
	for i := range randomString {
		randomString[i] = characters[rand.Intn(len(characters))]
	}
	return Bytes2Str(randomString)
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
