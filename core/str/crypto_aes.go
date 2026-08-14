package utils

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
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
// func GetAesIV(iv string) []byte {
// 	return Str2Bytes(Substr(MD5(iv), 0, 16))
// }

// GetAesIVSecure 使用加密安全的随机数生成器生成IV（推荐）
func GetAesIVSecure() []byte {
	return GetRandomSecure(aes.BlockSize)
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

// ==================== AES-GCM 认证加密（推荐用于金融级应用）====================

// AesGCMEncrypt AES-GCM 加密（带认证）- 推荐用于金融/银行级应用
// 相比 CBC 的优势：
// 1. 内置完整性保护（GMAC 认证标签）
// 2. 防止密文篡改攻击
// 3. 无需 PKCS7 Padding（支持任意长度）
// 4. 并行加密（性能提升 2-5 倍）
// 5. TLS 1.3 强制使用
// 6. NIST/PCI DSS 优先推荐
func AesGCMEncrypt(plaintext []byte, key string) (string, error) {
	return AesGCMEncryptBase(plaintext, GetAesKeySecure(key), nil)
}

// AesGCMEncryptWithAAD AES-GCM 加密（带附加认证数据）
// additionalData: 不加密但需要认证的数据（如请求头、用户ID等）
// 使用场景：确保关联数据（如交易ID、用户ID）未被篡改
func AesGCMEncryptWithAAD(plaintext []byte, key, additionalData string) (string, error) {
	return AesGCMEncryptBase(plaintext, GetAesKeySecure(key), Str2Bytes(additionalData))
}

// AesGCMEncryptBase AES-GCM 加密基础方法
// 返回格式：Base64(Nonce + Ciphertext + AuthTag)
// Nonce: 12 字节（GCM 标准）
// AuthTag: 16 字节（128-bit 认证标签）
func AesGCMEncryptBase(plaintext, key, additionalData []byte) (string, error) {
	result, err := AesGCMEncryptBaseByteResult(plaintext, key, additionalData)
	if err != nil {
		return "", err
	}
	return Base64Encode(result), nil
}

// AesGCMEncryptBaseByteResult AES-GCM 加密基础方法
// 返回格式：Byte(Nonce + Ciphertext + AuthTag)
// Nonce: 12 字节（GCM 标准）
// AuthTag: 16 字节（128-bit 认证标签）
func AesGCMEncryptBaseByteResult(plaintext, key, additionalData []byte) ([]byte, error) {
	// 1. 输入验证
	if len(key) != 32 {
		return nil, errors.New("key must be 32 bytes for AES-256")
	}

	// 2. 创建 AES 加密器
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	// 3. 创建 GCM 模式（AEAD: Authenticated Encryption with Associated Data）
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// 4. 生成 Nonce（GCM 标准：12 字节）
	// 注意：Nonce 必须唯一，否则会破坏 GCM 安全性
	nonce := GetRandomSecure(gcm.NonceSize())

	// 5. 加密并生成认证标签（单步操作）
	// Seal 会自动：
	//   - 加密 plaintext
	//   - 对 ciphertext + additionalData 生成 GMAC 认证标签
	//   - 返回：ciphertext + authTag
	ciphertext := gcm.Seal(nil, nonce, plaintext, additionalData)

	// 6. 拼接：Nonce + Ciphertext + AuthTag
	result := append(nonce, ciphertext...)

	// 7. Base64 编码
	return result, nil
}

// AesGCMDecrypt AES-GCM 解密（带认证验证）
func AesGCMDecrypt(encryptedData string, key string) ([]byte, error) {
	return AesGCMDecryptBase(encryptedData, GetAesKeySecure(key), nil)
}

// AesGCMDecryptWithAAD AES-GCM 解密（带附加认证数据验证）
// additionalData: 必须与加密时使用的 AAD 完全一致
func AesGCMDecryptWithAAD(encryptedData string, key, additionalData string) ([]byte, error) {
	return AesGCMDecryptBase(encryptedData, GetAesKeySecure(key), Str2Bytes(additionalData))
}

// AesGCMDecryptBase AES-GCM 解密基础方法
// 会自动验证：
// 1. 认证标签（AuthTag）- 确保密文未被篡改
// 2. 附加认证数据（AAD）- 确保关联数据未被篡改
// 任何一项验证失败都会返回错误，拒绝解密
func AesGCMDecryptBase(encryptedData string, key, additionalData []byte) ([]byte, error) {
	// 1. 输入验证
	if len(key) != 32 {
		return nil, errors.New("key must be 32 bytes for AES-256")
	}

	// 2. 解码 Base64
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
// 会自动验证：
// 1. 认证标签（AuthTag）- 确保密文未被篡改
// 2. 附加认证数据（AAD）- 确保关联数据未被篡改
// 任何一项验证失败都会返回错误，拒绝解密
func AesGCMDecryptBaseByteResult(encryptedData, key, additionalData []byte) ([]byte, error) {
	// 1. 输入验证
	if len(key) != 32 {
		return nil, errors.New("key must be 32 bytes for AES-256")
	}

	// 2. 解码 Base64
	data := encryptedData
	if data == nil {
		return nil, errors.New("base64 decode failed")
	}

	// 3. 创建 AES 加密器
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	// 4. 创建 GCM 模式
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// 5. 检查数据长度（Nonce + Ciphertext + AuthTag）
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, errors.New("encrypted data too short")
	}

	// 6. 分离 Nonce 和 Ciphertext+AuthTag
	nonce := data[:nonceSize]
	ciphertext := data[nonceSize:]

	// 7. 解密并验证认证标签（单步操作）
	// Open 会自动：
	//   - 验证 GMAC 认证标签（防篡改）
	//   - 验证 additionalData（如果提供）
	//   - 解密 ciphertext
	// 任何验证失败都会返回 error
	plaintext, err := gcm.Open(nil, nonce, ciphertext, additionalData)
	if err != nil {
		// 🚨 这个错误非常重要！表示数据被篡改或密钥错误
		return nil, fmt.Errorf("authentication failed - data may be tampered: %w", err)
	}

	return plaintext, nil
}
