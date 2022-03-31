package util

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/hex"
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

func AesEncrypt(s, key1, key2 string) (string, error) {
	s1 := Str2Bytes(MD5(key1))
	s2 := Str2Bytes(Substr2(MD5(key2), 12, 28))
	block, err := aes.NewCipher(s1) //选择加密算法
	if err != nil {
		return "", err
	}
	plantText := Str2Bytes(s)
	plantText = PKCS7Padding(plantText, block.BlockSize())
	blockModel := cipher.NewCBCEncrypter(block, s2)
	ciphertext := make([]byte, len(plantText))
	blockModel.CryptBlocks(ciphertext, plantText)
	return hex.EncodeToString(ciphertext), nil
}

func AesDecrypt(s, key1, key2 string) (string, error) {
	s1 := Str2Bytes(MD5(key1))
	s2 := Str2Bytes(Substr2(MD5(key2), 12, 28))
	block, err := aes.NewCipher(s1) //选择加密算法
	if err != nil {
		return "", err
	}
	ciphertext, err := hex.DecodeString(s)
	if err != nil {
		return "", err
	}
	blockModel := cipher.NewCBCDecrypter(block, s2)
	plantText := make([]byte, len(ciphertext))
	blockModel.CryptBlocks(plantText, ciphertext)
	plantText = PKCS7UnPadding(plantText, block.BlockSize())
	return Bytes2Str(plantText), nil
}
