package utils

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"errors"
	"fmt"
)

const (
	keyLen16 = 16
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

func AesEncrypt2(text, keyB64 string) string {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("Recovered from panic:", r)
		}
	}()
	key := Base64Decode(keyB64)
	if len(key) < 32 {
		return ""
	} else {
		key = key[:32]
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return ""
	}
	plantText := Str2Bytes(text)
	iv := RandomBytes(keyLen16)
	plantText = PKCS7Padding(plantText, block.BlockSize())
	blockModel := cipher.NewCBCEncrypter(block, iv)
	ciphertext := make([]byte, len(plantText))
	blockModel.CryptBlocks(ciphertext, plantText)
	return Base64Encode(append(iv, ciphertext...))
}

func AesDecrypt2(text, keyB64 string) string {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("Recovered from panic:", r)
		}
	}()
	key := Base64Decode(keyB64)
	if len(key) < 32 {
		return ""
	} else {
		key = key[:32]
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return ""
	}
	bs := Base64Decode(text)
	if bs == nil || len(bs) <= keyLen16 {
		return ""
	}
	iv := bs[0:keyLen16]
	ciphertext := bs[keyLen16:]
	blockModel := cipher.NewCBCDecrypter(block, iv)
	plantText := make([]byte, len(ciphertext))
	blockModel.CryptBlocks(plantText, ciphertext)
	plantText = PKCS7UnPadding(plantText, block.BlockSize())
	if plantText == nil {
		return ""
	}
	return Bytes2Str(plantText)
}

func GetAesKey(key string) []byte {
	return Str2Bytes(MD5(key))
}

func GetAesIV(iv string) []byte {
	return Str2Bytes(Substr(MD5(iv), 0, 16))
}
