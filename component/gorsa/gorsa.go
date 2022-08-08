package gorsa

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"os"
	"strings"
)

const rsa_bits = 2048

type RsaObj struct {
	prikey       *rsa.PrivateKey
	pubkey       *rsa.PublicKey
	PubkeyBase64 string
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

func (self *RsaObj) CreateRsaFileBase64(bits ...int) (string, string, error) {
	x := rsa_bits
	if len(bits) > 0 {
		x = bits[0]
	}
	// 生成私钥文件
	prikey, err := rsa.GenerateKey(rand.Reader, x)
	if err != nil {
		return "", "", err
	}
	block := pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(prikey),
	}
	prikeybase64 := base64.StdEncoding.EncodeToString(pem.EncodeToMemory(&block))

	// 生成公钥文件
	block1 := pem.Block{
		Type:  "RSA PUBLICK KEY",
		Bytes: x509.MarshalPKCS1PublicKey(&prikey.PublicKey),
	}
	bs := pem.EncodeToMemory(&block1)
	pubkeybase64 := base64.StdEncoding.EncodeToString(bs)
	self.prikey = prikey
	self.pubkey = &prikey.PublicKey
	self.PubkeyBase64 = pubkeybase64
	return prikeybase64, pubkeybase64, nil
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
	if block == nil {
		return errors.New("RSA block invalid")
	}
	prikey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return err
	}
	self.prikey = prikey
	self.pubkey = &prikey.PublicKey
	//self.LoadRsaPubkey(filePath)
	return nil
}

func (self *RsaObj) LoadRsaKeyFileBase64(fileBase64 string) error {
	dec, err := base64.StdEncoding.DecodeString(fileBase64)
	if err != nil {
		return err
	}
	block, _ := pem.Decode(dec)
	if block == nil {
		return errors.New("RSA block invalid")
	}
	prikey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return err
	}
	self.prikey = prikey
	self.pubkey = &prikey.PublicKey
	return nil
}

func (self *RsaObj) LoadRsaPemFileByte(fileByte []byte) error {
	block, _ := pem.Decode(fileByte)
	if block == nil {
		return errors.New("RSA block invalid")
	}
	pubkey, err := x509.ParsePKCS1PublicKey(block.Bytes)
	if err != nil {
		return err
	}
	self.pubkey = pubkey
	return nil
}

func (self *RsaObj) LoadRsaPemFileBase64(fileBase64 string) error {
	dec, err := base64.StdEncoding.DecodeString(fileBase64)
	if err != nil {
		return err
	}
	block, _ := pem.Decode(dec)
	if block == nil {
		return errors.New("RSA block invalid")
	}
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

func (self *RsaObj) Encrypt(msg []byte) ([]byte, error) {
	res, err := rsa.EncryptPKCS1v15(rand.Reader, self.pubkey, msg)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (self *RsaObj) Decrypt(msg []byte) ([]byte, error) {
	res, err := rsa.DecryptPKCS1v15(rand.Reader, self.prikey, msg)
	if err != nil {
		return nil, err
	}
	return res, nil
}

// support 1024/2048
func (self *RsaObj) EncryptPlanText(pubkey string) (string, error) {
	size := 200
	count := 0
	length := len(pubkey)
	if length%size == 0 {
		count = length / size
	} else {
		count = length/size + 1
	}
	result := ""
	index := 0
	for i := 1; i <= count; i++ {
		var part string
		if i == count {
			part = pubkey[index:]
		} else {
			part = pubkey[index : index+size]
		}
		res, err := rsa.EncryptPKCS1v15(rand.Reader, self.pubkey, []byte(part))
		if err != nil {
			return "", err
		}
		result = result + "." + base64.StdEncoding.EncodeToString(res)
		index += size
	}
	return result[1:], nil
}

// support 1024/2048
func (self *RsaObj) DecryptPlanText(msg string) ([]byte, error) {
	parts := strings.Split(msg, ".")
	if len(parts) > 6 {
		return nil, errors.New("invalid base64 data length")
	}
	result := ""
	for _, v := range parts {
		bs, err := base64.StdEncoding.DecodeString(v)
		if err != nil {
			return nil, err
		}
		res, err := rsa.DecryptPKCS1v15(rand.Reader, self.prikey, bs)
		if err != nil {
			return nil, err
		}
		result += string(res)
	}
	dec, err := base64.StdEncoding.DecodeString(result)
	if err != nil {
		return nil, err
	}
	return dec, nil
}

func (self *RsaObj) SignBySHA256(msg []byte) ([]byte, error) {
	h := sha256.New()
	h.Write(msg)
	hased := h.Sum(nil)
	res, err := rsa.SignPKCS1v15(rand.Reader, self.prikey, crypto.SHA256, hased)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (self *RsaObj) VerifyBySHA256(msg []byte, sign string) error {
	bs, err := base64.StdEncoding.DecodeString(sign)
	if err != nil {
		return err
	}
	if bs == nil || len(bs) == 0 {
		return errors.New("RSA sign invalid")
	}
	hased := sha256.Sum256(msg)
	if err := rsa.VerifyPKCS1v15(self.pubkey, crypto.SHA256, hased[:], bs); err != nil {
		return err
	}
	return nil
}
