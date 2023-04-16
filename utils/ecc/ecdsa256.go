package ecc

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"errors"
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

var (
	defaultCurve = elliptic.P256()
)

func hmac256(key, msg []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(msg)
	return h.Sum(nil)
}

func hash512(msg []byte) []byte {
	h := sha512.New()
	h.Write(msg)
	r := h.Sum(nil)
	h.Reset()
	return r
}

func concat(iv, pub, text []byte) []byte {
	ct := make([]byte, len(iv)+len(pub)+len(text))
	copy(ct, iv)
	copy(ct[len(iv):], pub)
	copy(ct[len(iv)+len(pub):], text)
	return ct
}

func concatKDF(pub, iv, mac, text []byte) []byte {
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

func aes256CtrEncrypt(iv, key, plaintext []byte) (ct []byte, err error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	stream := cipher.NewCTR(block, iv)
	dst := make([]byte, len(plaintext))
	stream.XORKeyStream(dst, plaintext)
	return dst, nil
}

func aes256CtrDecrypt(iv, key, ciphertext []byte) (m []byte, err error) {
	return aes256CtrEncrypt(iv, key, ciphertext)
}

func CreateECDSA() (*ecdsa.PrivateKey, error) {
	prk, err := ecdsa.GenerateKey(defaultCurve, rand.Reader)
	if err != nil {
		return nil, err
	}
	return prk, nil
}

func loadPrivateKey(h string) (*ecdsa.PrivateKey, error) {
	b, err := hex.DecodeString(h)
	if err != nil {
		return nil, errors.New("bad private key")
	}
	prk, err := x509.ParseECPrivateKey(b)
	if err != nil {
		return nil, errors.New("parse private key failed")
	}
	return prk, nil
}

func LoadPublicKey(h []byte) (*ecdsa.PublicKey, error) {
	if len(h) != pLen {
		return nil, errors.New("publicKey invalid")
	}
	x, y := elliptic.Unmarshal(defaultCurve, h)
	if x == nil || y == nil {
		return nil, errors.New("bad point format")
	}
	return &ecdsa.PublicKey{Curve: defaultCurve, X: x, Y: y}, nil
}

func LoadBase64PublicKey(b64 string) (*ecdsa.PublicKey, []byte, error) {
	b, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, nil, err
	}
	pub, err := LoadPublicKey(b)
	if err != nil {
		return nil, nil, err
	}
	pubBs := elliptic.Marshal(defaultCurve, pub.X, pub.Y)
	return pub, pubBs, nil
}

func GetKeyBytes(prk *ecdsa.PrivateKey, pub *ecdsa.PublicKey) ([]byte, []byte, error) {
	var err error
	var prkBs, pubBs []byte
	if prk != nil {
		prkBs, err = x509.MarshalECPrivateKey(prk)
		if err != nil {
			return nil, nil, err
		}
	}
	if pub != nil {
		pubBs = elliptic.Marshal(defaultCurve, pub.X, pub.Y)
	}
	return prkBs, pubBs, nil
}

func Encrypt(publicTo, message []byte) ([]byte, error) {
	if len(publicTo) != pLen {
		return nil, errors.New("bad public key")
	}
	pub, err := LoadPublicKey(publicTo)
	if err != nil {
		return nil, errors.New("public key invalid")
	}
	// temp private key
	prk, err := CreateECDSA()
	if err != nil {
		return nil, err
	}

	sharedKey, _ := defaultCurve.ScalarMult(pub.X, pub.Y, prk.D.Bytes())
	if sharedKey == nil || len(sharedKey.Bytes()) == 0 {
		return nil, errors.New("shared failed")
	}

	sharedKeyHash := hash512(sharedKey.Bytes())
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

	_, ephemPublicKey, err := GetKeyBytes(nil, &prk.PublicKey)
	if err != nil {
		return nil, errors.New("temp public key invalid")
	}
	hashData := concat(iv, ephemPublicKey, ciphertext)
	realMac := hmac256(macKey, hashData)

	return concatKDF(ephemPublicKey, iv, realMac, ciphertext), nil
}

func Decrypt(privateKey *ecdsa.PrivateKey, msg []byte) ([]byte, error) {
	if len(msg) <= minLen {
		return nil, errors.New("bad msg data")
	}

	ephemPublicKey := msg[0:pLen]
	pub, err := LoadPublicKey(ephemPublicKey)
	if err != nil {
		return nil, errors.New("bad public key")
	}

	sharedKey, _ := defaultCurve.ScalarMult(pub.X, pub.Y, privateKey.D.Bytes())
	if sharedKey == nil || len(sharedKey.Bytes()) == 0 {
		return nil, errors.New("shared failed")
	}

	sharedKeyHash := hash512(sharedKey.Bytes())

	macKey := sharedKeyHash[mLen:]
	encryptionKey := sharedKeyHash[0:mLen]

	iv := msg[pLen:secLen]
	mac := msg[secLen:minLen]
	ciphertext := msg[minLen:]

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
