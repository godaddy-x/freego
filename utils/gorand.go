package utils

import (
	cr "crypto/rand"
	"encoding/hex"
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
	return RandStr2(16)
}

// ModRand 调用底层生成随机数,进行取模运算,性能提升10倍
func ModRand(n int) int {
	return int(fastrand() % uint32(n))
}

func RandStr2(n int) string {
	// 创建一个字节切片来存储随机数
	randomBytes := make([]byte, n) // 生成 32 字节（256 位）的随机数，适用于 HMAC-SHA256
	// 使用 crypto/rand 包中的 Read 函数获取密码学安全的随机数
	_, err := cr.Read(randomBytes)
	if err != nil {
		return ""
	}
	return hex.EncodeToString(randomBytes)
}
