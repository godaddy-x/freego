package crypto

import (
	"errors"

	"crypto/mlkem"

	ecc "github.com/godaddy-x/eccrypto"
	utils "github.com/godaddy-x/freego/core/str"
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

// EncapsulateToPeerBytes 向服务端封装密钥（1568B 封装公钥 raw bytes）。
func EncapsulateToPeerBytes(serverEncapKey []byte) (sharedKey, kemCt []byte, err error) {
	ek, err := ecc.LoadMLKEM1024EncapsulationKey(serverEncapKey)
	if err != nil {
		return nil, nil, err
	}
	return ecc.EncapsulateMLKEM1024(ek)
}

// EncapsulateToPeer 向服务端封装密钥（Plan2 HTTP：封装公钥 Base64）。
func EncapsulateToPeer(serverEncapKeyB64 string) (sharedKey, kemCtB64 string, err error) {
	sk, ct, err := EncapsulateToPeerBytes(utils.Base64Decode(serverEncapKeyB64))
	if err != nil {
		return "", "", err
	}
	return utils.Base64Encode(sk), utils.Base64Encode(ct), nil
}

// DecapsulatePeerCiphertextBytes dkB64 为解封装私钥 Base64；kemCt 为 1568B KEM 密文 raw bytes。
func DecapsulatePeerCiphertextBytes(dkB64 string, kemCt []byte) ([]byte, error) {
	dk, err := ecc.LoadMLKEM1024DecapsulationKeyFromBase64(dkB64)
	if err != nil {
		return nil, err
	}
	if len(kemCt) != MLKEM1024CiphertextSize {
		return nil, errors.New("invalid ML-KEM ciphertext length")
	}
	return ecc.DecapsulateMLKEM1024(dk, kemCt)
}

// DecapsulatePeerCiphertext HTTP：KEM 密文 Base64。
func DecapsulatePeerCiphertext(dkB64, kemCtB64 string) ([]byte, error) {
	return DecapsulatePeerCiphertextBytes(dkB64, utils.Base64Decode(kemCtB64))
}
