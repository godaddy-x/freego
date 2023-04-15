package crypto

import (
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	ecc "github.com/godaddy-x/eccrypto"
	"github.com/godaddy-x/eccrypto/crypto"
	"github.com/godaddy-x/eccrypto/crypto/ecies"
)

type EccObj struct {
	privateKey       *ecies.PrivateKey
	publicKey        ecies.PublicKey
	PrivateKeyBase64 string
	PublicKeyBase64  string
}

func (self *EccObj) CreateS256ECC() error {
	prk, err := ecies.GenerateKey(rand.Reader, crypto.S256(), nil)
	if err != nil {
		return err
	}
	prkBs := prk.D.Bytes()
	pubBs := elliptic.Marshal(prk.Curve, prk.PublicKey.X, prk.PublicKey.Y)
	self.privateKey = prk
	self.publicKey = prk.PublicKey
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
	return ecc.Decrypt(self.privateKey, msg)
}

func (self *EccObj) Sign(msg []byte) ([]byte, error) {
	return nil, nil
}

func (self *EccObj) Verify(msg, sign []byte) error {
	return nil
}
