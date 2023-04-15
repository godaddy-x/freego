package crypto

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"errors"
	"github.com/btcsuite/btcd/btcec/v2"
	"io"
)

const (
	iLen   = 16
	mLen   = 32
	pLen   = 65
	sLen   = 16
	minLen = 113
	secLen = 81
)

func loadPublicKey(b []byte) (*btcec.PublicKey, error) {
	if len(b) != pLen {
		return nil, errors.New("publicKey invalid")
	}
	pub, err := btcec.ParsePubKey(b)
	if err != nil {
		return nil, errors.New("bad publicKey")
	}
	return pub, nil
}

func LoadBase64PublicKey(b string) (*btcec.PublicKey, error) {
	bs, err := base64.StdEncoding.DecodeString(b)
	if err != nil {
		return nil, err
	}
	return loadPublicKey(bs)
}

func loadPrivateKey(b []byte) (*btcec.PrivateKey, error) {
	prk, _ := btcec.PrivKeyFromBytes(b)
	return prk, nil
}

func LoadBase64PrivateKey(b string) (*btcec.PrivateKey, error) {
	bs, err := base64.StdEncoding.DecodeString(b)
	if err != nil {
		return nil, err
	}
	return loadPrivateKey(bs)
}

func hmac256(key, msg []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(msg)
	return h.Sum(nil)
}

func concat(iv, pub, text []byte) []byte {
	ct := make([]byte, len(iv)+len(pub)+len(text))
	copy(ct, iv)
	copy(ct[len(iv):], pub)
	copy(ct[len(iv)+len(pub):], text)
	return ct
}

func concatResult(pub, iv, mac, text []byte) []byte {
	ct := make([]byte, len(pub)+len(iv)+len(mac)+len(text))
	copy(ct, pub)
	copy(ct[len(pub):], iv)
	copy(ct[len(pub)+len(iv):], mac)
	copy(ct[len(pub)+len(iv)+len(mac):], text)
	return ct
}

func randomBytes(l int) ([]byte, error) {
	bs := make([]byte, l)
	_, err := io.ReadFull(rand.Reader, bs)
	if err != nil {
		return nil, err
	}
	return bs, nil
}

func pkcs7Padding(ciphertext []byte, blockSize int) []byte {
	padding := blockSize - len(ciphertext)%blockSize
	padtext := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(ciphertext, padtext...)
}

func pkcs7UnPadding(plantText []byte) []byte {
	length := len(plantText)
	unpadding := int(plantText[length-1])
	return plantText[:(length - unpadding)]
}

func aes256CbcDecrypt(iv, key, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	plaintext := make([]byte, len(ciphertext))
	mode := cipher.NewCBCDecrypter(block, iv)
	mode.CryptBlocks(plaintext, ciphertext)
	plaintext = pkcs7UnPadding(plaintext)
	return plaintext, nil
}

func aes256CbcEncrypt(iv, key, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	// 对明文进行 ZeroPadding 填充
	padded := pkcs7Padding(plaintext, block.BlockSize())
	ciphertext := make([]byte, len(padded))
	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(ciphertext, padded)
	return ciphertext, nil
}

func CreateECC() (*btcec.PrivateKey, *btcec.PublicKey, error) {
	prk, err := btcec.NewPrivateKey()
	if err != nil {
		return nil, nil, err
	}
	return prk, prk.PubKey(), nil
}

func ECCEncrypt(publicTo, message []byte) ([]byte, error) {
	if len(publicTo) != pLen {
		return nil, errors.New("bad public key")
	}
	pub, err := loadPublicKey(publicTo)
	if err != nil {
		return nil, errors.New("public key invalid")
	}
	// temp private key
	prv, err := btcec.NewPrivateKey()
	if err != nil {
		return nil, err
	}
	sharedKey := btcec.GenerateSharedSecret(prv, pub)
	if sharedKey == nil || len(sharedKey) == 0 {
		return nil, errors.New("shared failed")
	}

	sharedKeyHash := sha512.Sum512(sharedKey)
	macKey := sharedKeyHash[mLen:]
	encryptionKey := sharedKeyHash[0:mLen]

	iv, err := randomBytes(sLen)
	if err != nil {
		return nil, errors.New("random iv failed")
	}

	ciphertext, err := aes256CbcEncrypt(iv, encryptionKey, message)
	if err != nil {
		return nil, errors.New("encrypt failed")
	}

	ephemPublicKey := prv.PubKey().SerializeUncompressed()
	hashData := concat(iv, ephemPublicKey, ciphertext)
	realMac := hmac256(macKey, hashData)

	return concatResult(ephemPublicKey, iv, realMac, ciphertext), nil
}

func ECCDecrypt(privateKey *btcec.PrivateKey, bs []byte) ([]byte, error) {
	if len(bs) <= minLen {
		return nil, errors.New("message invalid")
	}
	ephemPublicKey := bs[0:pLen]
	clientPublicKey, err := loadPublicKey(ephemPublicKey)
	if err != nil {
		return nil, errors.New("bad public key")
	}

	sharedKey := btcec.GenerateSharedSecret(privateKey, clientPublicKey)
	if sharedKey == nil || len(sharedKey) == 0 {
		return nil, errors.New("shared failed")
	}

	sharedKeyHash := sha512.Sum512(sharedKey)
	macKey := sharedKeyHash[mLen:]
	encryptionKey := sharedKeyHash[0:mLen]

	iv := bs[pLen:secLen]
	mac := bs[secLen:minLen]
	ciphertext := bs[minLen:]

	hashData := concat(iv, ephemPublicKey, ciphertext)

	realMac := hmac256(macKey, hashData)

	if !bytes.Equal(mac, realMac) {
		return nil, errors.New("mac invalid")
	}

	plaintext, err := aes256CbcDecrypt(iv, encryptionKey, ciphertext)
	if err != nil {
		return nil, errors.New("decrypt failed")
	}
	return plaintext, nil
}
