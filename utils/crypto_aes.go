package utils

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"time"
)

func PKCS7Padding(ciphertext []byte, blockSize int) []byte {
	padding := blockSize - len(ciphertext)%blockSize
	padtext := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(ciphertext, padtext...)
}

func PKCS7UnPadding(plantText []byte, blockSize int) []byte {
	if len(plantText) == 0 {
		return nil
	}
	length := len(plantText)
	unpadding := int(plantText[length-1])
	if length-unpadding <= 0 {
		return nil
	}
	return plantText[:(length - unpadding)]
}

// AesEncrypt 不安全的AES加密（已废弃，仅用于兼容性）
// Deprecated: 请使用 AesCBCEncrypt 替代，使用更安全的密钥和IV生成
//func AesEncrypt(plantText []byte, key, iv string) (string, error) {
//	block, err := aes.NewCipher(GetAesKey(key)) //选择加密算法
//	if err != nil {
//		return "", err
//	}
//	plantText = PKCS7Padding(plantText, block.BlockSize())
//	blockModel := cipher.NewCBCEncrypter(block, GetAesIV(iv))
//	ciphertext := make([]byte, len(plantText))
//	blockModel.CryptBlocks(ciphertext, plantText)
//	return Base64Encode(ciphertext), nil
//}

// AesDecrypt 不安全的AES解密（已废弃，仅用于兼容性）
// Deprecated: 请使用 AesCBCDecrypt 替代，使用更安全的密钥和IV生成
//func AesDecrypt(msg, key, iv string) ([]byte, error) {
//	block, err := aes.NewCipher(GetAesKey(key)) //选择加密算法
//	if err != nil {
//		return nil, err
//	}
//	ciphertext := Base64Decode(msg)
//	if len(ciphertext) == 0 {
//		return nil, err
//	}
//	blockModel := cipher.NewCBCDecrypter(block, GetAesIV(iv))
//	plantText := make([]byte, len(ciphertext))
//	blockModel.CryptBlocks(plantText, ciphertext)
//	plantText = PKCS7UnPadding(plantText, block.BlockSize())
//	if plantText == nil {
//		return nil, errors.New("unPadding data failed")
//	}
//	return plantText, nil
//}

// GetAesKey 不安全的MD5密钥生成（已废弃，仅用于兼容性）
// Deprecated: 请使用 GetAesKeySecure 替代，MD5已被认为不安全
//func GetAesKey(key string) []byte {
//	return Str2Bytes(MD5(key))
//}

// GetAesKeySecure 安全的AES密钥生成（推荐使用）
func GetAesKeySecure(key string) []byte {
	// 输入验证
	if len(key) == 0 {
		return nil
	}

	// 使用SHA-512生成64字节密钥，然后截取32字节（AES-256）
	hash := SHA512(key)
	hashBytes := Str2Bytes(hash)
	return hashBytes[:32]
}

// GetAesIV 不安全的MD5 IV生成（已废弃，仅用于兼容性）
// Deprecated: 请使用 GetAesIVSecure 替代，MD5已被认为不安全
func GetAesIV(iv string) []byte {
	return Str2Bytes(Substr(MD5(iv), 0, 16))
}

// GetAesIVSecure 使用加密安全的随机数生成器生成IV（推荐）
func GetAesIVSecure() []byte {
	return GetRandomSecure(aes.BlockSize)
}

// GetRandomSecure 使用加密安全的随机数生成器生成指定字节数组（推荐）
func GetRandomSecure(l int) []byte {
	randomIV := make([]byte, l)
	if _, err := io.ReadFull(rand.Reader, randomIV); err != nil {
		// 记录错误日志，便于调试
		fmt.Printf("crypto/rand failed: %v, using fallback", err)
		return GetAesIVFallback(l)
	}
	return randomIV
}

// GetAesIVFallback 备用IV生成方法（当crypto/rand失败时使用）
func GetAesIVFallback(l int) []byte {
	// 修复：使用单次时间戳调用
	now := time.Now().UnixNano()
	timeBytes := make([]byte, 8)
	timeBytes[0] = byte(now >> 56)
	timeBytes[1] = byte(now >> 48)
	timeBytes[2] = byte(now >> 40)
	timeBytes[3] = byte(now >> 32)
	timeBytes[4] = byte(now >> 24)
	timeBytes[5] = byte(now >> 16)
	timeBytes[6] = byte(now >> 8)
	timeBytes[7] = byte(now)

	// 修复：使用参数 l 而不是硬编码 aes.BlockSize
	iv := make([]byte, l)

	// 复制时间戳到前8字节（如果长度足够）
	if l >= 8 {
		copy(iv[:8], timeBytes)
	} else {
		copy(iv, timeBytes[:l])
		return iv
	}

	// 修复：使用正确的循环边界
	for i := 8; i < l; i++ {
		iv[i] = byte(ModRand(256))
	}

	return iv
}

// AesCBCEncryptBase 标准AES-CBC加密，返回IV+密文的Base64编码
func AesCBCEncryptBase(plaintext, key []byte) (string, error) {
	// 生成随机IV
	iv := GetAesIVSecure()
	return AesCBCEncryptWithIV(plaintext, key, iv)
}

// AesCBCEncrypt 字符串版本的便捷方法
func AesCBCEncrypt(plaintext []byte, key string) (string, error) {
	return AesCBCEncryptBase(plaintext, GetAesKeySecure(key))
}

// AesCBCDecrypt 字符串版本的便捷方法
func AesCBCDecrypt(encryptedData string, key string) ([]byte, error) {
	return AesCBCDecryptBase(encryptedData, GetAesKeySecure(key))
}

// AesCBCEncryptWithIV 使用指定IV的AES-CBC加密
func AesCBCEncryptWithIV(plaintext, key, iv []byte) (string, error) {
	// 创建AES加密器
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	// PKCS7填充
	plaintext = PKCS7Padding(plaintext, block.BlockSize())

	// 创建CBC加密器
	encrypter := cipher.NewCBCEncrypter(block, iv)

	// 加密
	ciphertext := make([]byte, len(plaintext))
	encrypter.CryptBlocks(ciphertext, plaintext)

	// 拼接IV+密文
	result := append(iv, ciphertext...)

	// 返回Base64编码
	return Base64Encode(result), nil
}

// AesCBCDecryptBase 使用指定IV的AES-CBC解密
func AesCBCDecryptBase(encryptedData string, key []byte) ([]byte, error) {
	// 解码Base64
	data := Base64Decode(encryptedData)
	if data == nil {
		return nil, errors.New("base64 decode failed")
	}

	// 检查数据长度
	if len(data) < aes.BlockSize {
		return nil, errors.New("encrypted data too short")
	}

	// 分离IV和密文
	ivBytes := data[:aes.BlockSize]
	ciphertext := data[aes.BlockSize:]

	// 创建AES解密器
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	// 创建CBC解密器
	decrypter := cipher.NewCBCDecrypter(block, ivBytes)

	// 解密
	plaintext := make([]byte, len(ciphertext))
	decrypter.CryptBlocks(plaintext, ciphertext)

	// 去除PKCS7填充
	plaintext = PKCS7UnPadding(plaintext, block.BlockSize())
	if plaintext == nil {
		return nil, errors.New("unpadding failed")
	}

	return plaintext, nil
}
