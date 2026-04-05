package crypto

import (
	"bytes"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/ed25519"
	"errors"
	"fmt"

	ecc "github.com/godaddy-x/eccrypto"
	"github.com/godaddy-x/freego/utils"
)

type EcdhObject struct {
	// 16字节string字段组
	PrivateKeyBase64 string
	PublicKeyBase64  string

	// 8字节指针字段组
	privateKey *ecdh.PrivateKey
	publicKey  *ecdh.PublicKey
}

func (self *EcdhObject) CreateECDH() error {
	prk, err := ecc.CreateECDH()
	if err != nil {
		return err
	}

	self.privateKey = prk
	self.publicKey = prk.PublicKey()
	self.PublicKeyBase64 = utils.Base64Encode(self.publicKey.Bytes())
	return nil
}

func (self *EcdhObject) LoadS256ECC(b64 string) error {
	prk, err := ecc.LoadECDHPrivateKeyFromBase64(b64)
	if err != nil {
		return err
	}
	self.privateKey = prk
	self.publicKey = prk.PublicKey()
	self.PublicKeyBase64 = utils.Base64Encode(self.publicKey.Bytes())
	return nil
}

// ******************************************************* ECDH X25519 Implement *******************************************************

type EcdhX25519Object struct {
	PrivateKeyBase64 string
	PublicKeyBase64  string

	privateKey *ecdh.PrivateKey
	publicKey  *ecdh.PublicKey
}

func (self *EcdhX25519Object) CreateX25519() error {
	prk, err := ecc.CreateX25519()
	if err != nil {
		return err
	}
	self.privateKey = prk
	self.publicKey = prk.PublicKey()
	self.PublicKeyBase64 = utils.Base64Encode(self.publicKey.Bytes())
	return nil
}

func (self *EcdhX25519Object) LoadX25519(b64 string) error {
	prk, err := ecc.LoadX25519PrivateKeyFromBase64(b64)
	if err != nil {
		return err
	}
	self.privateKey = prk
	self.publicKey = prk.PublicKey()
	self.PublicKeyBase64 = utils.Base64Encode(self.publicKey.Bytes())
	return nil
}

// ******************************************************* ECC Implement *******************************************************

func (self *EcdhObject) GetPrivateKey() (interface{}, string) {
	return self.privateKey, self.PrivateKeyBase64
}

func (self *EcdhObject) GetPublicKey() (interface{}, string) {
	return self.publicKey, self.PublicKeyBase64
}

func (self *EcdhObject) Encrypt(msg, aad []byte) (string, error) {
	return "", nil
}

func (self *EcdhObject) Decrypt(msg string, aad []byte) ([]byte, error) {
	bs := utils.Base64Decode(msg)
	if len(bs) == 0 {
		return nil, errors.New("base64 parse failed")
	}
	r, err := ecc.Decrypt(self.privateKey, bs, aad, nil)
	if err != nil {
		return nil, err
	}
	return r, nil
}

func (self *EcdhObject) Sign(msg []byte) ([]byte, error) {
	return nil, nil
}

func (self *EcdhObject) Verify(msg, sign []byte) error {
	return nil
}

// ******************************************************* X25519 ECC Implement *******************************************************

func (self *EcdhX25519Object) GetPrivateKey() (interface{}, string) {
	return self.privateKey, self.PrivateKeyBase64
}

func (self *EcdhX25519Object) GetPublicKey() (interface{}, string) {
	return self.publicKey, self.PublicKeyBase64
}

func (self *EcdhX25519Object) Encrypt(msg, aad []byte) (string, error) {
	return "", nil
}

func (self *EcdhX25519Object) Decrypt(msg string, aad []byte) ([]byte, error) {
	bs := utils.Base64Decode(msg)
	if len(bs) == 0 {
		return nil, errors.New("base64 parse failed")
	}
	r, err := ecc.DecryptX25519(self.privateKey, bs, aad, nil)
	if err != nil {
		return nil, err
	}
	return r, nil
}

func (self *EcdhX25519Object) Sign(msg []byte) ([]byte, error) {
	return nil, nil
}

func (self *EcdhX25519Object) Verify(msg, sign []byte) error {
	return nil
}

// ******************************************************* ECDSA Implement *******************************************************

type EcdsaObject struct {
	// 16字节string字段组
	PrivateKeyBase64 string
	PublicKeyBase64  string

	// 8字节指针字段组
	privateKey *ecdsa.PrivateKey
	publicKey  *ecdsa.PublicKey
}

func PrintECDSABase64() {
	o := &EcdsaObject{}
	_ = o.CreateS256ECDSA()
	fmt.Println("私钥：", o.PrivateKeyBase64)
	fmt.Println("公钥：", o.PublicKeyBase64)
}

func (self *EcdsaObject) CreateS256ECDSA() error {
	prk, err := ecc.CreateECDSA()
	if err != nil {
		return err
	}

	self.privateKey = prk
	self.publicKey = &prk.PublicKey
	pubBs, err := ecc.GetECDSAPublicKeyBytes(self.publicKey)
	if err != nil {
		return err
	}
	self.PublicKeyBase64 = utils.Base64Encode(pubBs)
	prkBs, err := ecc.GetECDSAPrivateKeyBytes(prk)
	if err != nil {
		return err
	}
	self.PrivateKeyBase64 = utils.Base64Encode(prkBs)
	return nil
}

func CreateS256ECDSAWithBase64(prkB64, pubB64 string) (*EcdsaObject, error) {
	prk, err := ecc.LoadECDSAPrivateKeyFromBase64(prkB64)
	if err != nil {
		return nil, err
	}
	pub, err := ecc.LoadECDSAPublicKeyFromBase64(pubB64)
	if err != nil {
		return nil, err
	}
	object := &EcdsaObject{}
	object.privateKey = prk
	object.publicKey = pub
	return object, nil
}

func (self *EcdsaObject) LoadS256ECDSA(b64 string) error {
	prk, err := ecc.LoadECDSAPrivateKeyFromBase64(b64)
	if err != nil {
		return err
	}
	self.privateKey = prk
	self.publicKey = &prk.PublicKey
	pubBs, err := ecc.GetECDSAPublicKeyBytes(self.publicKey)
	if err != nil {
		return err
	}
	self.PublicKeyBase64 = utils.Base64Encode(pubBs)
	self.PrivateKeyBase64 = b64 // 直接使用传入的base64字符串
	return nil
}

// ******************************************************* ECDSA Cipher Interface Implement *******************************************************

func (self *EcdsaObject) GetPrivateKey() (interface{}, string) {
	return self.privateKey, self.PrivateKeyBase64
}

func (self *EcdsaObject) GetPublicKey() (interface{}, string) {
	return self.publicKey, self.PublicKeyBase64
}

func (self *EcdsaObject) Encrypt(msg, aad []byte) (string, error) {
	// ECDSA不支持加密操作
	return "", errors.New("ECDSA does not support encryption")
}

func (self *EcdsaObject) Decrypt(msg string, aad []byte) ([]byte, error) {
	// ECDSA不支持解密操作
	return nil, errors.New("ECDSA does not support decryption")
}

func (self *EcdsaObject) Sign(msg []byte) ([]byte, error) {
	if self.privateKey == nil {
		return nil, errors.New("ECDSA private key not initialized")
	}
	return ecc.SignECDSA(self.privateKey, msg)
}

func (self *EcdsaObject) Verify(msg, sign []byte) error {
	if self.publicKey == nil {
		return errors.New("ECDSA public key not initialized")
	}
	return ecc.VerifyECDSA(self.publicKey, msg, sign)
}

// ******************************************************* Ed25519 Implement *******************************************************

type Ed25519Object struct {
	PrivateKeyBase64 string
	PublicKeyBase64  string

	privateKey ed25519.PrivateKey
	publicKey  ed25519.PublicKey
}

// PrintEd25519Base64 与 PrintECDSABase64 相同用途：本地快速打印一对 Base64 密钥
func PrintEd25519Base64() {
	o := &Ed25519Object{}
	_ = o.CreateEd25519()
	fmt.Println("私钥：", o.PrivateKeyBase64)
	fmt.Println("公钥：", o.PublicKeyBase64)
}

func (self *Ed25519Object) CreateEd25519() error {
	prk, err := ecc.CreateEd25519()
	if err != nil {
		return err
	}
	self.privateKey = prk
	self.publicKey = prk.Public().(ed25519.PublicKey)
	pubBs, err := ecc.GetEd25519PublicKeyBytes(self.publicKey)
	if err != nil {
		return err
	}
	self.PublicKeyBase64 = utils.Base64Encode(pubBs)
	prkBs, err := ecc.GetEd25519PrivateKeyBytes(prk)
	if err != nil {
		return err
	}
	self.PrivateKeyBase64 = utils.Base64Encode(prkBs)
	return nil
}

func CreateEd25519WithBase64(prkB64, pubB64 string) (*Ed25519Object, error) {
	prk, err := ecc.LoadEd25519PrivateKeyFromBase64(prkB64)
	if err != nil {
		return nil, err
	}
	pub, err := ecc.LoadEd25519PublicKeyFromBase64(pubB64)
	if err != nil {
		return nil, err
	}
	derived := prk.Public().(ed25519.PublicKey)
	if !bytes.Equal(derived, pub) {
		return nil, errors.New("Ed25519 public key does not match private key")
	}
	return &Ed25519Object{
		privateKey:       prk,
		publicKey:        pub,
		PrivateKeyBase64: prkB64,
		PublicKeyBase64:  pubB64,
	}, nil
}

func (self *Ed25519Object) LoadEd25519(b64 string) error {
	prk, err := ecc.LoadEd25519PrivateKeyFromBase64(b64)
	if err != nil {
		return err
	}
	self.privateKey = prk
	self.publicKey = prk.Public().(ed25519.PublicKey)
	pubBs, err := ecc.GetEd25519PublicKeyBytes(self.publicKey)
	if err != nil {
		return err
	}
	self.PublicKeyBase64 = utils.Base64Encode(pubBs)
	self.PrivateKeyBase64 = b64
	return nil
}

// ******************************************************* Ed25519 Cipher Interface Implement *******************************************************

func (self *Ed25519Object) GetPrivateKey() (interface{}, string) {
	return self.privateKey, self.PrivateKeyBase64
}

func (self *Ed25519Object) GetPublicKey() (interface{}, string) {
	return self.publicKey, self.PublicKeyBase64
}

func (self *Ed25519Object) Encrypt(msg, aad []byte) (string, error) {
	return "", errors.New("Ed25519 does not support encryption")
}

func (self *Ed25519Object) Decrypt(msg string, aad []byte) ([]byte, error) {
	return nil, errors.New("Ed25519 does not support decryption")
}

func (self *Ed25519Object) Sign(msg []byte) ([]byte, error) {
	if len(self.privateKey) == 0 {
		return nil, errors.New("Ed25519 private key not initialized")
	}
	return ecc.SignEd25519(self.privateKey, msg)
}

func (self *Ed25519Object) Verify(msg, sign []byte) error {
	if len(self.publicKey) == 0 {
		return errors.New("Ed25519 public key not initialized")
	}
	return ecc.VerifyEd25519(self.publicKey, msg, sign)
}
