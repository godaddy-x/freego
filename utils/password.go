package utils

import (
	"bytes"
	"encoding/base64"
	"golang.org/x/crypto/scrypt"
)

// PasswordHash 根据byte生成抗攻击的派生密钥
func PasswordHash(password, salt []byte, N, R, P, keyLen int) ([]byte, error) {
	if N < 2 {
		N = 2
	}
	if R < 2 {
		R = 2
	}
	if P < 2 {
		P = 2
	}
	if keyLen < 32 {
		keyLen = 32
	}
	key, err := scrypt.Key(password, salt, N, R, P, keyLen)
	if err != nil {
		return nil, err
	}
	return key, nil
}

// PasswordVerify 应该在密钥使用[]byte方式进行base64结果的情况下使用
func PasswordVerify(password, salt, target string, N, R, P, keyLen int) bool {
	if len(target) == 0 {
		return false
	}
	if N < 2 {
		N = 2
	}
	if R < 2 {
		R = 2
	}
	if P < 2 {
		P = 2
	}
	if keyLen < 32 {
		keyLen = 32
	}
	passwordBs, err := base64.StdEncoding.DecodeString(password)
	if err != nil || len(passwordBs) == 0 {
		return false
	}
	saltBs, err := base64.StdEncoding.DecodeString(salt)
	if err != nil || len(saltBs) == 0 {
		return false
	}
	targetBs, err := base64.StdEncoding.DecodeString(target)
	if err != nil || len(targetBs) == 0 {
		return false
	}
	check, err := PasswordHash(passwordBs, saltBs, N, R, P, keyLen)
	if err != nil {
		return false
	}
	return bytes.Equal(check, targetBs)
}
