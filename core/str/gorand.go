package utils

import (
	"crypto/rand"
	"math/big"
	_ "unsafe"
)

const numbers = "0123456789"
const letters = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
const lettersSp = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ!@#$%^&*"

//go:linkname fastrand runtime.fastrand
func fastrand() uint32

func gorand(n int, characters string) string {
	randomString := make([]byte, n)
	charLen := big.NewInt(int64(len(characters)))

	for i := range randomString {
		randomIndex, err := rand.Int(rand.Reader, charLen)
		if err != nil {
			// 与 GetRandomSecure 一致：crypto/rand 失败视为系统级故障，禁止回退弱随机源。
			panic("crypto/rand failed: " + err.Error())
		}
		randomString[i] = characters[randomIndex.Int64()]
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
	return Base64Encode(GetAesIVSecure())
}

// ModRand 调用底层生成随机数,进行取模运算,性能提升10倍
func ModRand(n int) int {
	return int(fastrand() % uint32(n))
}
