package utils

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"errors"
)

const (
	keyLen16 = 16
	keyLen32 = 32
)

func PKCS7Padding(ciphertext []byte, blockSize int) []byte {
	padding := blockSize - len(ciphertext)%blockSize
	padtext := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(ciphertext, padtext...)
}

func PKCS7UnPadding(plantText []byte, blockSize int) []byte {
	if plantText == nil || len(plantText) == 0 {
		return nil
	}
	length := len(plantText)
	unpadding := int(plantText[length-1])
	if length-unpadding <= 0 {
		return nil
	}
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
	return Base64Encode(ciphertext), nil
}

func AesDecrypt(msg, key, iv string) ([]byte, error) {
	block, err := aes.NewCipher(GetAesKey(key)) //选择加密算法
	if err != nil {
		return nil, err
	}
	ciphertext := Base64Decode(msg)
	if ciphertext == nil || len(ciphertext) == 0 {
		return nil, err
	}
	blockModel := cipher.NewCBCDecrypter(block, GetAesIV(iv))
	plantText := make([]byte, len(ciphertext))
	blockModel.CryptBlocks(plantText, ciphertext)
	plantText = PKCS7UnPadding(plantText, block.BlockSize())
	if plantText == nil {
		return nil, errors.New("unPadding data failed")
	}
	return plantText, nil
}

func AesEncrypt2(plantText []byte, key string) string {
	block, err := aes.NewCipher(GetAesKey(key)) //选择加密算法
	if err != nil {
		return ""
	}
	iv := RandStr2(keyLen16)
	plantText = PKCS7Padding(plantText, block.BlockSize())
	blockModel := cipher.NewCBCEncrypter(block, GetAesIV(iv))
	ciphertext := make([]byte, len(plantText))
	blockModel.CryptBlocks(ciphertext, plantText)
	return Base64Encode(append(Str2Bytes(iv), ciphertext...))
}

func AesDecrypt2(msg, key string) ([]byte, error) {
	block, err := aes.NewCipher(GetAesKey(key)) //选择加密算法
	if err != nil {
		return nil, err
	}
	bs := Base64Decode(msg)
	if bs == nil || len(bs) <= keyLen32 {
		return nil, errors.New("msg bs invalid")
	}
	iv := Bytes2Str(bs[:keyLen32])
	ciphertext := bs[keyLen32:]
	blockModel := cipher.NewCBCDecrypter(block, GetAesIV(iv))
	plantText := make([]byte, len(ciphertext))
	blockModel.CryptBlocks(plantText, ciphertext)
	plantText = PKCS7UnPadding(plantText, block.BlockSize())
	if plantText == nil {
		return nil, errors.New("unPadding data failed")
	}
	return plantText, nil
}

func GetAesKey(key string) []byte {
	return Str2Bytes(MD5(key))
}

func GetAesIV(iv string) []byte {
	return Str2Bytes(Substr(MD5(iv), 0, 16))
}
