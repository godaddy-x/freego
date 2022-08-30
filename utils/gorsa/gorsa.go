package gorsa

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"os"
)

const rsa_bits = 2048

type RsaObj struct {
	prikey       *rsa.PrivateKey
	pubkey       *rsa.PublicKey
	PubkeyBase64 string
	PrikeyBase64 string
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

func (self *RsaObj) CreateRsa2048() error {
	return self.CreateRsaFileBase64()
}

func (self *RsaObj) CreateRsa1024() error {
	return self.CreateRsaFileBase64(1024)
}

func (self *RsaObj) CreateRsaFileBase64(bits ...int) error {
	x := rsa_bits
	if len(bits) > 0 {
		x = bits[0]
	}
	// 生成私钥文件
	prikey, err := rsa.GenerateKey(rand.Reader, x)
	if err != nil {
		return err
	}
	block := pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(prikey),
	}
	prikeybase64 := base64.URLEncoding.EncodeToString(pem.EncodeToMemory(&block))

	// 生成公钥文件
	block1 := pem.Block{
		Type:  "RSA PUBLICK KEY",
		Bytes: x509.MarshalPKCS1PublicKey(&prikey.PublicKey),
	}
	pubkeybase64 := base64.URLEncoding.EncodeToString(pem.EncodeToMemory(&block1))
	self.prikey = prikey
	self.pubkey = &prikey.PublicKey
	self.PubkeyBase64 = pubkeybase64
	self.PrikeyBase64 = prikeybase64
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
	if block == nil {
		return errors.New("RSA block invalid")
	}
	prikey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return err
	}
	block1 := pem.Block{
		Type:  "RSA PUBLICK KEY",
		Bytes: x509.MarshalPKCS1PublicKey(&prikey.PublicKey),
	}
	pubkeybase64 := base64.URLEncoding.EncodeToString(pem.EncodeToMemory(&block1))
	self.prikey = prikey
	self.pubkey = &prikey.PublicKey
	self.PubkeyBase64 = pubkeybase64
	return nil
}

func (self *RsaObj) LoadRsaKeyFileBase64(fileBase64 string) error {
	dec, err := base64.URLEncoding.DecodeString(fileBase64)
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
	dec, err := base64.URLEncoding.DecodeString(fileBase64)
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
	self.PubkeyBase64 = fileBase64
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

func (self *RsaObj) Encrypt(msg []byte) (string, error) {
	partLen := self.pubkey.N.BitLen()/8 - 11
	chunks := split(msg, partLen)
	buffer := bytes.NewBufferString("")
	for _, chunk := range chunks {
		bytes, err := rsa.EncryptPKCS1v15(rand.Reader, self.pubkey, chunk)
		if err != nil {
			return "", err
		}
		buffer.Write(bytes)
	}
	return base64.URLEncoding.EncodeToString(buffer.Bytes()), nil
}

func (self *RsaObj) Decrypt(msg string) (string, error) {
	partLen := self.pubkey.N.BitLen() / 8
	raw, err := base64.URLEncoding.DecodeString(msg)
	if err != nil {
		return "", errors.New("msg base64 decode failed")
	}
	chunks := split(raw, partLen)
	buffer := bytes.NewBufferString("")
	for _, chunk := range chunks {
		decrypted, err := rsa.DecryptPKCS1v15(rand.Reader, self.prikey, chunk)
		if err != nil {
			return "", err
		}
		buffer.Write(decrypted)
	}
	return buffer.String(), err
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
	bs, err := base64.URLEncoding.DecodeString(sign)
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

func split(buf []byte, lim int) [][]byte {
	var chunk []byte
	chunks := make([][]byte, 0, len(buf)/lim+1)
	for len(buf) >= lim {
		chunk, buf = buf[:lim], buf[lim:]
		chunks = append(chunks, chunk)
	}
	if len(buf) > 0 {
		chunks = append(chunks, buf[:])
	}
	return chunks
}