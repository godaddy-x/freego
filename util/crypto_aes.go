package util

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
)

func PKCS7Padding(ciphertext []byte, blockSize int) []byte {
	padding := blockSize - len(ciphertext)%blockSize
	padtext := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(ciphertext, padtext...)
}

func PKCS7UnPadding(plantText []byte, blockSize int) []byte {
	length := len(plantText)
	unpadding := int(plantText[length-1])
	return plantText[:(length - unpadding)]
}

func AesEncrypt(plantText []byte, key, iv string) (string, error) {
	block, err := aes.NewCipher(GetAesKey(key)) //选择加密算法
	if err != nil {
		return "", err
	}
	plantText = PKCS7Padding(plantText, block.BlockSize())
	blockModel := cipher.NewCBCEncrypter(block, GetAesIV(iv))
	ciphertext := make([]byte, len(plantText))
	blockModel.CryptBlocks(ciphertext, plantText)
	return Base64URLEncode(ciphertext), nil
}

func AesDecrypt(msg, key, iv string) (string, error) {
	block, err := aes.NewCipher(GetAesKey(key)) //选择加密算法
	if err != nil {
		return "", err
	}
	ciphertext := Base64URLDecode(msg)
	if ciphertext == nil || len(ciphertext) == 0 {
		return "", err
	}
	blockModel := cipher.NewCBCDecrypter(block, GetAesIV(iv))
	plantText := make([]byte, len(ciphertext))
	blockModel.CryptBlocks(plantText, ciphertext)
	plantText = PKCS7UnPadding(plantText, block.BlockSize())
	return Bytes2Str(plantText), nil
}

func GetAesKey(key string) []byte {
	return Str2Bytes(MD5(key))
}

func GetAesIV(iv string) []byte {
	return Str2Bytes(Substr(MD5(iv), 12, 28))
}
