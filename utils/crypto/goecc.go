package crypto

import (
	"crypto/ecdsa"
	"errors"
	"github.com/godaddy-x/eccrypto"
	"github.com/godaddy-x/freego/utils"
)

type EccObj struct {
	privateKey       *ecdsa.PrivateKey
	publicKey        *ecdsa.PublicKey
	PrivateKeyBase64 string
	PublicKeyBase64  string
}

func (self *EccObj) CreateS256ECC() error {
	prk, err := ecc.CreateECDSA()
	if err != nil {
		return err
	}
	_, pubBs, err := ecc.GetObjectBytes(nil, &prk.PublicKey)
	self.privateKey = prk
	self.publicKey = &prk.PublicKey
	self.PublicKeyBase64 = utils.Base64Encode(pubBs)
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
	self.PublicKeyBase64 = utils.Base64Encode(pubBs)
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

func (self *EccObj) Decrypt(msg string) ([]byte, error) {
	bs := utils.Base64Decode(msg)
	if len(bs) == 0 {
		return nil, errors.New("base64 parse failed")
	}
	r, err := ecc.Decrypt(self.privateKey, bs)
	if err != nil {
		return nil, err
	}
	return r, nil
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
