package crypto

import (
	"encoding/base64"
	"errors"
	"github.com/btcsuite/btcd/btcec/v2"
	"unsafe"
)

type EccObj struct {
	privateKey       *btcec.PrivateKey
	publicKey        *btcec.PublicKey
	PrivateKeyBase64 string
	PublicKeyBase64  string
}

func (self *EccObj) CreateS256ECC() error {
	prk, _, err := CreateECC()
	if err != nil {
		return err
	}
	prkBs := prk.Serialize()
	pubBs := prk.PubKey().SerializeUncompressed()
	self.privateKey = prk
	self.publicKey = prk.PubKey()
	self.PrivateKeyBase64 = base64.StdEncoding.EncodeToString(prkBs)
	self.PublicKeyBase64 = base64.StdEncoding.EncodeToString(pubBs)
	return nil
}

// ******************************************************* ECC Implement *******************************************************

func (self *EccObj) GetPrivateKey() (interface{}, string) {
	return self.privateKey, self.PrivateKeyBase64
}

func (self *EccObj) GetPublicKey() (interface{}, string) {
	return self.publicKey, self.PublicKeyBase64
}

func (self *EccObj) Encrypt(msg []byte) (string, error) {
	return "", nil
}

func (self *EccObj) Decrypt(msg string) (string, error) {
	bs, err := base64.StdEncoding.DecodeString(msg)
	if err != nil {
		return "", errors.New("base64 parse failed")
	}
	r, err := ECCDecrypt(self.privateKey, bs)
	if err != nil {
		return "", err
	}
	return *(*string)(unsafe.Pointer(&r)), nil
}

func (self *EccObj) Sign(msg []byte) ([]byte, error) {
	return nil, nil
}

func (self *EccObj) Verify(msg, sign []byte) error {
	return nil
}
