package crypto

import (
	"crypto/ecdsa"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/godaddy-x/eccrypto"
	"github.com/godaddy-x/freego/utils"
	"io/ioutil"
	"os"
)

var (
	defaultKeyB64 = utils.SHA512(utils.GetLocalSecretKey())
)

type EccObject struct {
	privateKey       *ecdsa.PrivateKey
	publicKey        *ecdsa.PublicKey
	PrivateKeyBase64 string
	PublicKeyBase64  string
}

func NewEccObject() *EccObject {
	obj := &EccObject{}
	obj.CreateS256ECC()
	return obj
}

func CreateEccKeystore(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		file, err := os.Create(path)
		if err != nil {
			return errors.New("create file fail: " + err.Error())
		}
		defer file.Close()
		str := `{
    "PrivateKey": "%s",
    "PublicKey": "%s"
}`
		eccObject := NewEccObject()
		prk, pub, _ := ecc.GetObjectBase64(eccObject.privateKey, eccObject.publicKey)
		if _, err := file.WriteString(fmt.Sprintf(str, utils.AesEncrypt2(prk, defaultKeyB64), pub)); err != nil {
			return errors.New("write file fail: " + err.Error())
		}
		return nil
	}
	return nil
}

func LoadEccKeystore(path string) (*EccObject, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, err
	}
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, errors.New("read file fail: " + err.Error())
	}
	privateKey := utils.GetJsonString(data, "PrivateKey")
	if len(privateKey) == 0 {
		return nil, errors.New("privateKey not found")
	}
	prk := utils.AesDecrypt2(privateKey, defaultKeyB64)
	if len(prk) == 0 {
		return nil, errors.New("privateKey decode invalid")
	}
	return LoadEccObject(prk), nil
}

func LoadEccObject(b64 string) *EccObject {
	prk, err := ecc.LoadBase64PrivateKey(b64)
	if err != nil {
		return nil
	}
	_, pub, _ := ecc.GetObjectBase64(nil, &prk.PublicKey)
	return &EccObject{privateKey: prk, publicKey: &prk.PublicKey, PublicKeyBase64: pub}
}

func LoadEccObjectByHex(h string) *EccObject {
	prk, err := ecc.LoadHexPrivateKey(h)
	if err != nil {
		return nil
	}
	_, pub, _ := ecc.GetObjectBase64(nil, &prk.PublicKey)
	return &EccObject{privateKey: prk, publicKey: &prk.PublicKey, PublicKeyBase64: pub}
}

func (self *EccObject) CreateS256ECC() error {
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

func (self *EccObject) LoadS256ECC(b64 string) error {
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

func (self *EccObject) GenSharedKey(b64 string) (string, error) {
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

func (self *EccObject) GetPrivateKey() (interface{}, string) {
	return self.privateKey, self.PrivateKeyBase64
}

func (self *EccObject) GetPublicKey() (interface{}, string) {
	return self.publicKey, self.PublicKeyBase64
}

func (self *EccObject) Encrypt(privateKey interface{}, publicTo, msg []byte) (string, error) {
	var prk *ecdsa.PrivateKey
	if privateKey != nil {
		prk, _ = privateKey.(*ecdsa.PrivateKey)
	}
	bs, err := ecc.Encrypt(prk, publicTo, msg)
	if err != nil {
		return "", err
	}
	return utils.Base64Encode(bs), err
}

func (self *EccObject) Decrypt(msg string) (string, error) {
	if len(msg) == 0 {
		return "", errors.New("msg is nil")
	}
	r, err := ecc.Decrypt(self.privateKey, utils.Base64Decode(msg))
	if err != nil {
		return "", err
	}
	return utils.Bytes2Str(r), nil
}

func (self *EccObject) Sign(msg string) (string, error) {
	r, err := ecc.Sign(self.privateKey, utils.Str2Bytes(msg))
	if err != nil {
		return "", err
	}
	return utils.Base64Encode(r), nil
}

func (self *EccObject) Verify(msg, sign string) error {
	signBs := utils.Base64Decode(sign)
	if len(signBs) == 0 {
		return errors.New("sign invalid")
	}
	b := ecc.Verify(self.publicKey, utils.Str2Bytes(msg), signBs)
	if !b {
		return errors.New("verify failed")
	}
	return nil
}
