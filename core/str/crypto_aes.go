package utils

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
)

// GetAesKeySecure 由字符串材料派生 AES-256 密钥（SHA-512 截断 32 字节）。
// 协议会话密钥请直接使用 32 字节随机材料，勿依赖弱口令派生。
func GetAesKeySecure(key string) []byte {
	if len(key) == 0 {
		return nil
	}
	hash := SHA512(key)
	hashBytes := Str2Bytes(hash)
	return hashBytes[:32]
}

// GetRandomSecure 从 crypto/rand 读取 l 字节。失败时 panic：视为系统级故障，
// 继续用弱随机源（尤其作 AES-GCM nonce）会破坏安全假设。
func GetRandomSecure(l int) []byte {
	randomIV := make([]byte, l)
	if _, err := io.ReadFull(rand.Reader, randomIV); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return randomIV
}

// ==================== AES-GCM 认证加密 ====================

// AesGCMEncrypt AES-GCM 加密（带认证）
func AesGCMEncrypt(plaintext []byte, key string) (string, error) {
	return AesGCMEncryptBase(plaintext, GetAesKeySecure(key), nil)
}

// AesGCMEncryptWithAAD AES-GCM 加密（带附加认证数据）
func AesGCMEncryptWithAAD(plaintext []byte, key, additionalData string) (string, error) {
	return AesGCMEncryptBase(plaintext, GetAesKeySecure(key), Str2Bytes(additionalData))
}

// AesGCMEncryptBase AES-GCM 加密基础方法
// 返回格式：Base64(Nonce + Ciphertext + AuthTag)
func AesGCMEncryptBase(plaintext, key, additionalData []byte) (string, error) {
	result, err := AesGCMEncryptBaseByteResult(plaintext, key, additionalData)
	if err != nil {
		return "", err
	}
	return Base64Encode(result), nil
}

// AesGCMEncryptBaseByteResult AES-GCM 加密基础方法
// 返回格式：Byte(Nonce + Ciphertext + AuthTag)
func AesGCMEncryptBaseByteResult(plaintext, key, additionalData []byte) ([]byte, error) {
	if len(key) != 32 {
		return nil, errors.New("key must be 32 bytes for AES-256")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := GetRandomSecure(gcm.NonceSize())
	ciphertext := gcm.Seal(nil, nonce, plaintext, additionalData)
	result := append(nonce, ciphertext...)
	return result, nil
}

// AesGCMDecrypt AES-GCM 解密（带认证验证）
func AesGCMDecrypt(encryptedData string, key string) ([]byte, error) {
	return AesGCMDecryptBase(encryptedData, GetAesKeySecure(key), nil)
}

// AesGCMDecryptWithAAD AES-GCM 解密（带附加认证数据验证）
func AesGCMDecryptWithAAD(encryptedData string, key, additionalData string) ([]byte, error) {
	return AesGCMDecryptBase(encryptedData, GetAesKeySecure(key), Str2Bytes(additionalData))
}

// AesGCMDecryptBase AES-GCM 解密基础方法
func AesGCMDecryptBase(encryptedData string, key, additionalData []byte) ([]byte, error) {
	if len(key) != 32 {
		return nil, errors.New("key must be 32 bytes for AES-256")
	}

	data := Base64Decode(encryptedData)
	if data == nil {
		return nil, errors.New("base64 decode failed")
	}

	plaintext, err := AesGCMDecryptBaseByteResult(data, key, additionalData)
	if err != nil {
		return nil, err
	}
	return plaintext, nil
}

// AesGCMDecryptBaseByteResult AES-GCM 解密基础方法
func AesGCMDecryptBaseByteResult(encryptedData, key, additionalData []byte) ([]byte, error) {
	if len(key) != 32 {
		return nil, errors.New("key must be 32 bytes for AES-256")
	}

	data := encryptedData
	if data == nil {
		return nil, errors.New("base64 decode failed")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, errors.New("encrypted data too short")
	}

	nonce := data[:nonceSize]
	ciphertext := data[nonceSize:]

	plaintext, err := gcm.Open(nil, nonce, ciphertext, additionalData)
	if err != nil {
		return nil, fmt.Errorf("authentication failed - data may be tampered: %w", err)
	}

	return plaintext, nil
}
