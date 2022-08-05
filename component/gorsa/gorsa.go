package gorsa

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"os"
)

const rsa_bits = 2048

type RsaObj struct {
	prikey    *rsa.PrivateKey
	pubkey    *rsa.PublicKey
	PubkeyHex string
}

func (self *RsaObj) CreateRsaFile(keyfile, pemfile string) error {
	// 生成私钥文件
	prikey, err := rsa.GenerateKey(rand.Reader, rsa_bits)
	if err != nil {
		return err
	}
	block := pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(prikey),
	}
	file, err := os.Create(keyfile)
	if err != nil {
		return err
	}
	if err := pem.Encode(file, &block); err != nil {
		return err
	}
	file.Close()

	// 生成公钥文件
	block1 := pem.Block{
		Type:  "RSA PUBLICK KEY",
		Bytes: x509.MarshalPKCS1PublicKey(&prikey.PublicKey),
	}
	file, err = os.Create(pemfile)
	if err != nil {
		return err
	}
	pem.Encode(file, &block1)
	file.Close()
	return nil
}

func (self *RsaObj) CreateRsaFileHex() (string, string, error) {
	// 生成私钥文件
	prikey, err := rsa.GenerateKey(rand.Reader, rsa_bits)
	if err != nil {
		return "", "", err
	}
	block := pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(prikey),
	}
	prikeyhex := hex.EncodeToString(pem.EncodeToMemory(&block))

	// 生成公钥文件
	block1 := pem.Block{
		Type:  "RSA PUBLICK KEY",
		Bytes: x509.MarshalPKCS1PublicKey(&prikey.PublicKey),
	}
	pubkeyhex := hex.EncodeToString(pem.EncodeToMemory(&block1))
	self.prikey = prikey
	self.pubkey = &prikey.PublicKey
	self.PubkeyHex = pubkeyhex
	return prikeyhex, pubkeyhex, nil
}

func (self *RsaObj) LoadRsaFile(filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	stat, _ := file.Stat()
	content := make([]byte, stat.Size())
	file.Read(content)
	file.Close()
	block, _ := pem.Decode(content)
	prikey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return err
	}
	self.prikey = prikey
	self.pubkey = &prikey.PublicKey
	//self.LoadRsaPubkey(filePath)
	return nil
}

func (self *RsaObj) LoadRsaKeyFileHex(fileHex string) error {
	dec, err := hex.DecodeString(fileHex)
	if err != nil {
		return err
	}
	block, _ := pem.Decode(dec)
	prikey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return err
	}
	self.prikey = prikey
	self.pubkey = &prikey.PublicKey
	return nil
}

func (self *RsaObj) LoadRsaPemFileHex(fileHex string) error {
	dec, err := hex.DecodeString(fileHex)
	if err != nil {
		return err
	}
	block, _ := pem.Decode(dec)
	pubkey, err := x509.ParsePKCS1PublicKey(block.Bytes)
	if err != nil {
		return err
	}
	self.pubkey = pubkey
	return nil
}

func (self *RsaObj) LoadRsaPubkey(filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	stat, _ := file.Stat()
	content := make([]byte, stat.Size())
	file.Read(content)
	file.Close()
	block, _ := pem.Decode(content)
	pubkey, err := x509.ParsePKCS1PublicKey(block.Bytes)
	if err != nil {
		return err
	}
	self.pubkey = pubkey
	return nil
}

func (self *RsaObj) Encrypt(msg string) (string, error) {
	res, err := rsa.EncryptPKCS1v15(rand.Reader, self.pubkey, []byte(msg))
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(res), nil
}

func (self *RsaObj) Decrypt(msg string) ([]byte, error) {
	bs, err := hex.DecodeString(msg)
	if err != nil {
		return nil, err
	}
	res, err := rsa.DecryptPKCS1v15(rand.Reader, self.prikey, bs)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (self *RsaObj) SignBySHA256(msg string) (string, error) {
	h := sha256.New()
	h.Write([]byte(msg))
	hased := h.Sum(nil)
	res, err := rsa.SignPKCS1v15(rand.Reader, self.prikey, crypto.SHA256, hased)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(res), nil
}

func (self *RsaObj) VerifyBySHA256(msg, sig string) error {
	bs, err := hex.DecodeString(sig)
	if err != nil {
		return err
	}
	hased := sha256.Sum256([]byte(msg))
	if err := rsa.VerifyPKCS1v15(self.pubkey, crypto.SHA256, hased[:], bs); err != nil {
		return err
	}
	return nil
}

func EncryptByRsaPubkey(pubkey, msg string) (string, error) {
	rsaObj := RsaObj{}
	if err := rsaObj.LoadRsaPemFileHex(pubkey); err != nil {
		return "", err
	}
	return rsaObj.Encrypt(msg)
}
