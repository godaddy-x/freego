package util

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
)

func padding(src []byte, blocksize int) []byte {
	padnum := blocksize - len(src)%blocksize
	pad := bytes.Repeat([]byte{byte(padnum)}, padnum)
	return append(src, pad...)
}

func unpadding(src []byte) []byte {
	defer func() {
		if r := recover(); r != nil {
			// log.Debug("Aes解密失败", 0, log.String("src", string(src)))
		}
	}()
	n := len(src)
	unpadnum := int(src[n-1])
	return src[:n-unpadnum]
}

func AesEncrypt(s string, k string) string {
	if len(s) == 0 || len(k) != 16 {
		return ""
	}
	key := Str2Bytes(k)
	src := Str2Bytes(s)
	block, _ := aes.NewCipher(key)
	src = padding(src, block.BlockSize())
	blockmode := cipher.NewCBCEncrypter(block, key)
	blockmode.CryptBlocks(src, src)
	return Base64URLEncode(src)
}

func AesDecrypt(s string, k string) string {
	if len(s) == 0 || len(k) != 16 {
		return ""
	}
	key := Str2Bytes(k)
	src := Base64URLDecode(s)
	block, _ := aes.NewCipher(key)
	blockmode := cipher.NewCBCDecrypter(block, key)
	blockmode.CryptBlocks(src, src)
	src = unpadding(src)
	return Bytes2Str(src)
}
