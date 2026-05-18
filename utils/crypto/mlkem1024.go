package crypto

import (
	"errors"

	ecc "github.com/godaddy-x/eccrypto"
	"crypto/mlkem"
	"github.com/godaddy-x/freego/utils"
)

// MLKEM1024Object Plan2 / 匿名通道：对端封装公钥 + 本端解封装私钥，Encrypt/Decrypt 走 ecc.EncryptMLKEM1024。
type MLKEM1024Object struct {
	PrivateKeyBase64 string
	PublicKeyBase64  string

	decapsKey *mlkem.DecapsulationKey1024
	encapPeer []byte // 接收方 1568 字节封装公钥（加密时使用）
}

func (self *MLKEM1024Object) CreateMLKEM1024() error {
	dk, err := ecc.CreateMLKEM1024()
	if err != nil {
		return err
	}
	self.decapsKey = dk
	self.PrivateKeyBase64 = ecc.MLKEM1024DecapsulationKeyToBase64(dk)
	self.PublicKeyBase64 = utils.Base64Encode(ecc.GetMLKEM1024EncapsulationKeyBytes(dk.EncapsulationKey()))
	return nil
}

func (self *MLKEM1024Object) LoadMLKEM1024DecapsulationFromBase64(b64 string) error {
	dk, err := ecc.LoadMLKEM1024DecapsulationKeyFromBase64(b64)
	if err != nil {
		return err
	}
	self.decapsKey = dk
	self.PrivateKeyBase64 = b64
	self.PublicKeyBase64 = utils.Base64Encode(ecc.GetMLKEM1024EncapsulationKeyBytes(dk.EncapsulationKey()))
	return nil
}

// SetPeerEncapsulationKeyForEncrypt 设置接收方 ML-KEM 封装公钥（1568 字节）；Encrypt 前必须调用。
func (self *MLKEM1024Object) SetPeerEncapsulationKeyForEncrypt(peerEncapKey []byte) {
	if len(peerEncapKey) == MLKEM1024EncapsulationKeySize {
		self.encapPeer = append([]byte(nil), peerEncapKey...)
	} else {
		self.encapPeer = nil
	}
}

func (self *MLKEM1024Object) GetPrivateKey() (interface{}, string) {
	return self.decapsKey, self.PrivateKeyBase64
}

func (self *MLKEM1024Object) GetPublicKey() (interface{}, string) {
	if self.decapsKey == nil {
		return nil, self.PublicKeyBase64
	}
	return self.decapsKey.EncapsulationKey(), self.PublicKeyBase64
}

func (self *MLKEM1024Object) Encrypt(msg, aad []byte) (string, error) {
	if len(self.encapPeer) != MLKEM1024EncapsulationKeySize {
		return "", errors.New("ML-KEM Encrypt: set peer encapsulation key with SetPeerEncapsulationKeyForEncrypt first")
	}
	out, err := ecc.EncryptMLKEM1024(self.encapPeer, msg, aad)
	if err != nil {
		return "", err
	}
	return utils.Base64Encode(out), nil
}

func (self *MLKEM1024Object) Decrypt(msg string, aad []byte) ([]byte, error) {
	if self.decapsKey == nil {
		return nil, errors.New("ML-KEM decapsulation key not initialized")
	}
	bs := utils.Base64Decode(msg)
	if len(bs) == 0 {
		return nil, errors.New("base64 parse failed")
	}
	return ecc.DecryptMLKEM1024(self.decapsKey, bs, aad, nil)
}

func (self *MLKEM1024Object) Sign(msg []byte) ([]byte, error) {
	return nil, nil
}

func (self *MLKEM1024Object) Verify(msg, sign []byte) error {
	return nil
}

// EncapsulateToPeer 向服务端封装密钥（Plan2 /key 之后客户端调用）：返回共享秘密与 KEM 密文。
func EncapsulateToPeer(serverEncapKeyB64 string) (sharedKey, kemCtB64 string, err error) {
	ek, err := ecc.LoadMLKEM1024EncapsulationKeyFromBase64(serverEncapKeyB64)
	if err != nil {
		return "", "", err
	}
	sk, ct, err := ecc.EncapsulateMLKEM1024(ek)
	if err != nil {
		return "", "", err
	}
	return utils.Base64Encode(sk), utils.Base64Encode(ct), nil
}

// DecapsulatePeerCiphertext 服务端用缓存的解封装私钥从客户端 KEM 密文恢复共享秘密。
func DecapsulatePeerCiphertext(dkB64, kemCtB64 string) ([]byte, error) {
	dk, err := ecc.LoadMLKEM1024DecapsulationKeyFromBase64(dkB64)
	if err != nil {
		return nil, err
	}
	ct := utils.Base64Decode(kemCtB64)
	if len(ct) != MLKEM1024CiphertextSize {
		return nil, errors.New("invalid ML-KEM ciphertext length")
	}
	return ecc.DecapsulateMLKEM1024(dk, ct)
}
