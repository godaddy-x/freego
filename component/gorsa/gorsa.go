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

type RsaObj struct {
	prikey *rsa.PrivateKey
	pubkey *rsa.PublicKey
}

func CreateRsaFile(filePath string) error {
	// 生成私钥文件
	prikey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}
	block := pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(prikey),
	}
	file, err := os.Create(filePath + ".key")
	if err != nil {
		return err
	}
	if err := pem.Encode(file, &block); err != nil {
		return err
	}
	file.Close()

	// 生成公钥文件
	pubkey := prikey.PublicKey
	bolck1 := pem.Block{
		Type:  "RSA PUBLICK KEY",
		Bytes: x509.MarshalPKCS1PublicKey(&pubkey),
	}
	file, err = os.Create(filePath + ".pem")
	if err != nil {
		return err
	}
	pem.Encode(file, &bolck1)
	file.Close()
	return nil
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

func (self *RsaObj) Decrypt(msg string) (string, error) {
	bs, err := hex.DecodeString(msg)
	if err != nil {
		return "", err
	}
	res, err := rsa.DecryptPKCS1v15(rand.Reader, self.prikey, bs)
	if err != nil {
		return "", err
	}
	return string(res), nil
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
