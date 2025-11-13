package crypto

import (
	"crypto/ecdh"
	"errors"

	ecc "github.com/godaddy-x/eccrypto"
	"github.com/godaddy-x/freego/utils"
)

type EccObj struct {
	// 16字节string字段组
	PrivateKeyBase64 string
	PublicKeyBase64  string

	// 8字节指针字段组
	privateKey *ecdh.PrivateKey
	publicKey  *ecdh.PublicKey
}

func (self *EccObj) CreateS256ECC() error {
	prk, err := ecc.CreateECDH()
	if err != nil {
		return err
	}

	self.privateKey = prk
	self.publicKey = prk.PublicKey()
	self.PublicKeyBase64 = utils.Base64Encode(self.publicKey.Bytes())
	return nil
}

func (self *EccObj) LoadS256ECC(b64 string) error {
	prk, err := ecc.LoadECDHPrivateKeyFromBase64(b64)
	if err != nil {
		return err
	}
	self.privateKey = prk
	self.publicKey = prk.PublicKey()
	self.PublicKeyBase64 = utils.Base64Encode(self.publicKey.Bytes())
	return nil
}

// ******************************************************* ECC Implement *******************************************************

func (self *EccObj) GetPrivateKey() (interface{}, string) {
	return self.privateKey, self.PrivateKeyBase64
}

func (self *EccObj) GetPublicKey() (interface{}, string) {
	return self.publicKey, self.PublicKeyBase64
}

func (self *EccObj) Encrypt(msg, aad []byte) (string, error) {
	return "", nil
}

func (self *EccObj) Decrypt(msg string, aad []byte) ([]byte, error) {
	bs := utils.Base64Decode(msg)
	if len(bs) == 0 {
		return nil, errors.New("base64 parse failed")
	}
	r, err := ecc.Decrypt(self.privateKey, bs, aad)
	if err != nil {
		return nil, err
	}
	return r, nil
}

func (self *EccObj) Sign(msg []byte) ([]byte, error) {
	return nil, nil
}

func (self *EccObj) Verify(msg, sign []byte) error {
	return nil
}
