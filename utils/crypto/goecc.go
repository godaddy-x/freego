package crypto

import (
	"crypto/ecdsa"
	"encoding/base64"
	"errors"
	"github.com/godaddy-x/eccrypto"
	"github.com/godaddy-x/freego/utils"
	"unsafe"
)

type EccObj struct {
	privateKey       *ecdsa.PrivateKey
	publicKey        *ecdsa.PublicKey
	PrivateKeyBase64 string
	PublicKeyBase64  string
}

func NewEccObject() *EccObj {
	obj := &EccObj{}
	obj.CreateS256ECC()
	return obj
}

func LoadEccObject(b64 string) *EccObj {
	prk, err := ecc.LoadBase64PrivateKey(b64)
	if err != nil {
		return nil
	}
	_, pub, _ := ecc.GetObjectBase64(nil, &prk.PublicKey)
	return &EccObj{privateKey: prk, publicKey: &prk.PublicKey, PublicKeyBase64: pub}
}

func (self *EccObj) CreateS256ECC() error {
	prk, err := ecc.CreateECDSA()
	if err != nil {
		return err
	}
	_, pubBs, err := ecc.GetObjectBytes(nil, &prk.PublicKey)
	self.privateKey = prk
	self.publicKey = &prk.PublicKey
	//self.PrivateKeyBase64 = base64.StdEncoding.EncodeToString(prkBs)
	self.PublicKeyBase64 = base64.StdEncoding.EncodeToString(pubBs)
	return nil
}

func (self *EccObj) LoadS256ECC(b64 string) error {
	prk, err := ecc.LoadBase64PrivateKey(b64)
	if err != nil {
		return err
	}
	_, pubBs, err := ecc.GetObjectBytes(nil, &prk.PublicKey)
	self.privateKey = prk
	self.publicKey = &prk.PublicKey
	//self.PrivateKeyBase64 = base64.StdEncoding.EncodeToString(prkBs)
	self.PublicKeyBase64 = base64.StdEncoding.EncodeToString(pubBs)
	return nil
}

func (self *EccObj) GenSharedKey(b64 string) (string, error) {
	pub, _, err := ecc.LoadBase64PublicKey(b64)
	if err != nil {
		return "", err
	}
	key, err := ecc.GenSharedKey(self.privateKey, pub)
	if err != nil {
		return "", err
	}
	return utils.SHA512(utils.Bytes2Str(key)), nil
}

// ******************************************************* ECC Implement *******************************************************

func (self *EccObj) GetPrivateKey() (interface{}, string) {
	return self.privateKey, self.PrivateKeyBase64
}

func (self *EccObj) GetPublicKey() (interface{}, string) {
	return self.publicKey, self.PublicKeyBase64
}

func (self *EccObj) Encrypt(publicTo, msg []byte) (string, error) {
	b, err := ecc.Encrypt(nil, publicTo, msg)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), err
}

func (self *EccObj) Decrypt(msg string) (string, error) {
	bs, err := base64.StdEncoding.DecodeString(msg)
	if err != nil {
		return "", errors.New("base64 parse failed")
	}
	r, err := ecc.Decrypt(self.privateKey, bs)
	if err != nil {
		return "", err
	}
	return *(*string)(unsafe.Pointer(&r)), nil
}

func (self *EccObj) Sign(msg []byte) ([]byte, error) {
	return ecc.Sign(self.privateKey, msg)
}

func (self *EccObj) Verify(msg, sign []byte) error {
	b := ecc.Verify(self.publicKey, msg, sign)
	if !b {
		return errors.New("verify failed")
	}
	return nil
}
