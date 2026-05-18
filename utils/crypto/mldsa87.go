package crypto

import (
	"errors"
	"fmt"

	ecc "github.com/godaddy-x/eccrypto"
	fmldsa "filippo.io/mldsa"
	"github.com/godaddy-x/freego/utils"
)

// MLDSA87Object 双向身份：Sign 用本端 ML-DSA-87 私钥，Verify 用对端公钥。
type MLDSA87Object struct {
	PrivateKeyBase64 string
	PublicKeyBase64  string

	privateKey *fmldsa.PrivateKey
	publicKey  *fmldsa.PublicKey
}

// PrintMLDSA87Base64 本地快速打印一对 Base64 ML-DSA-87 密钥（调试用）。
func PrintMLDSA87Base64() {
	o := &MLDSA87Object{}
	_ = o.CreateMLDSA87()
	fmt.Println("私钥：", o.PrivateKeyBase64)
	fmt.Println("公钥：", o.PublicKeyBase64)
}

func (self *MLDSA87Object) CreateMLDSA87() error {
	sk, err := ecc.CreateMLDSA87()
	if err != nil {
		return err
	}
	pk, err := ecc.DeriveMLDSA87PublicKey(sk)
	if err != nil {
		return err
	}
	prkB64, err := ecc.MLDSA87PrivateKeyToBase64(sk)
	if err != nil {
		return err
	}
	pubB64, err := ecc.MLDSA87PublicKeyToBase64(pk)
	if err != nil {
		return err
	}
	self.privateKey = sk
	self.publicKey = pk
	self.PrivateKeyBase64 = prkB64
	self.PublicKeyBase64 = pubB64
	return nil
}

// CreateMLDSA87WithBase64 按「本端私钥 + 对端公钥」加载身份（Sign/Verify）。
func CreateMLDSA87WithBase64(prkB64, peerPubB64 string) (*MLDSA87Object, error) {
	sk, err := ecc.LoadMLDSA87PrivateKeyFromBase64(prkB64)
	if err != nil {
		return nil, err
	}
	pk, err := ecc.LoadMLDSA87PublicKeyFromBase64(peerPubB64)
	if err != nil {
		return nil, err
	}
	return &MLDSA87Object{
		privateKey:       sk,
		publicKey:        pk,
		PrivateKeyBase64: prkB64,
		PublicKeyBase64:  peerPubB64,
	}, nil
}

func (self *MLDSA87Object) LoadMLDSA87FromBase64(b64 string) error {
	sk, err := ecc.LoadMLDSA87PrivateKeyFromBase64(b64)
	if err != nil {
		return err
	}
	pk, err := ecc.DeriveMLDSA87PublicKey(sk)
	if err != nil {
		return err
	}
	pubB64, err := ecc.MLDSA87PublicKeyToBase64(pk)
	if err != nil {
		return err
	}
	self.privateKey = sk
	self.publicKey = pk
	self.PrivateKeyBase64 = b64
	self.PublicKeyBase64 = pubB64
	return nil
}

func (self *MLDSA87Object) GetPrivateKey() (interface{}, string) {
	return self.privateKey, self.PrivateKeyBase64
}

func (self *MLDSA87Object) GetPublicKey() (interface{}, string) {
	return self.publicKey, self.PublicKeyBase64
}

func (self *MLDSA87Object) Encrypt(msg, aad []byte) (string, error) {
	return "", errors.New("ML-DSA-87 does not support encryption")
}

func (self *MLDSA87Object) Decrypt(msg string, aad []byte) ([]byte, error) {
	return nil, errors.New("ML-DSA-87 does not support decryption")
}

func (self *MLDSA87Object) Sign(msg []byte) ([]byte, error) {
	if self.privateKey == nil {
		return nil, errors.New("ML-DSA-87 private key not initialized")
	}
	return ecc.SignMLDSA87(self.privateKey, msg)
}

func (self *MLDSA87Object) Verify(msg, sign []byte) error {
	if self.publicKey == nil {
		return errors.New("ML-DSA-87 public key not initialized")
	}
	return ecc.VerifyMLDSA87(self.publicKey, msg, sign)
}

// CreateMLDSA87WithSeed 从 32 字节种子确定性加载私钥并派生对端公钥（测试/主种子派生场景）。
func CreateMLDSA87WithSeed(seed []byte, peerPubB64 string) (*MLDSA87Object, error) {
	sk, err := ecc.LoadMLDSA87PrivateKey(seed)
	if err != nil {
		return nil, err
	}
	prkB64, err := ecc.MLDSA87PrivateKeyToBase64(sk)
	if err != nil {
		return nil, err
	}
	return CreateMLDSA87WithBase64(prkB64, peerPubB64)
}

// PeerMLDSA87PublicKeyB64FromSeed 由种子派生本端密钥对并返回本端公钥 Base64。
func PeerMLDSA87PublicKeyB64FromSeed(seed []byte) (string, error) {
	sk, err := ecc.LoadMLDSA87PrivateKey(seed)
	if err != nil {
		return "", err
	}
	pk, err := ecc.DeriveMLDSA87PublicKey(sk)
	if err != nil {
		return "", err
	}
	return ecc.MLDSA87PublicKeyToBase64(pk)
}

// LabelSeedSHA256 将标签转为 32 字节种子（测试夹具用）。
func LabelSeedSHA256(label string) []byte {
	return utils.SHA256_BASE(utils.Str2Bytes(label))
}
