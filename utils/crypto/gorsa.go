package crypto

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

	"github.com/godaddy-x/freego/utils"
)

const bits = 2048

type Cipher interface {
	GetPrivateKey() (interface{}, string)
	GetPublicKey() (interface{}, string)
	Encrypt(msg []byte) (string, error)
	Decrypt(msg string) ([]byte, error)
	Sign(msg []byte) ([]byte, error)
	Verify(msg, sign []byte) error
}

type RsaObj struct {
	// 16字节string字段组
	PrivateKeyBase64 string
	PublicKeyBase64  string

	// 8字节指针字段组
	privateKey *rsa.PrivateKey
	publicKey  *rsa.PublicKey
}

func (self *RsaObj) CreateRsaFile(keyfile, pemfile string) error {
	// 生成私钥文件
	prikey, err := rsa.GenerateKey(rand.Reader, bits)
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
	if err := file.Close(); err != nil {
		return err
	}
	// 将publicKey转换为PKIX, ASN.1 DER格式
	derPkix, err := x509.MarshalPKIXPublicKey(&prikey.PublicKey)
	if err != nil {
		return err
	}
	// 设置PEM编码结构
	block1 := pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: derPkix,
	}
	file, err = os.Create(pemfile)
	if err != nil {
		return err
	}
	if err := pem.Encode(file, &block1); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return nil
}

func (self *RsaObj) CreateRsaPemFile(pemfile string) error {
	block1, _ := pem.Decode([]byte(self.PublicKeyBase64))
	if block1 == nil {
		return errors.New("RSA block invalid")
	}
	file, err := os.Create(pemfile)
	if err != nil {
		return err
	}
	if err := pem.Encode(file, block1); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return nil
}

func (self *RsaObj) CreateRsa2048() error {
	return self.CreateRsaFileBase64()
}

func (self *RsaObj) CreateRsa1024() error {
	return self.CreateRsaFileBase64(1024)
}

func (self *RsaObj) CreateRsaFileBase64(b ...int) error {
	x := bits
	if len(b) > 0 {
		x = b[0]
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
	privateKeyBase64 := base64.URLEncoding.EncodeToString(pem.EncodeToMemory(&block))
	// 将publicKey转换为PKIX, ASN.1 DER格式
	derPkix, err := x509.MarshalPKIXPublicKey(&prikey.PublicKey)
	if err != nil {
		return err
	}
	// 设置PEM编码结构
	block1 := &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: derPkix,
	}
	self.privateKey = prikey
	self.publicKey = &prikey.PublicKey
	self.PublicKeyBase64 = string(pem.EncodeToMemory(block1))
	self.PrivateKeyBase64 = privateKeyBase64
	return nil
}

func (self *RsaObj) LoadRsaFile(filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	stat, _ := file.Stat()
	content := make([]byte, stat.Size())
	if _, err := file.Read(content); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	block, _ := pem.Decode(content)
	if block == nil {
		return errors.New("RSA block invalid")
	}
	prikey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return err
	}
	// 将publicKey转换为PKIX, ASN.1 DER格式
	derPkix, err := x509.MarshalPKIXPublicKey(&prikey.PublicKey)
	if err != nil {
		return err
	}
	// 设置PEM编码结构
	block1 := pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: derPkix,
	}
	self.PublicKeyBase64 = string(pem.EncodeToMemory(&block1))
	self.PrivateKeyBase64 = base64.URLEncoding.EncodeToString(pem.EncodeToMemory(block))
	self.privateKey = prikey
	self.publicKey = &prikey.PublicKey
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
	// 将publicKey转换为PKIX, ASN.1 DER格式
	derPkix, err := x509.MarshalPKIXPublicKey(&prikey.PublicKey)
	if err != nil {
		return err
	}
	// 设置PEM编码结构
	block1 := pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: derPkix,
	}
	self.PublicKeyBase64 = string(pem.EncodeToMemory(&block1))
	self.PrivateKeyBase64 = fileBase64
	self.privateKey = prikey
	self.publicKey = &prikey.PublicKey
	return nil
}

func (self *RsaObj) LoadRsaPemFile(filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	stat, _ := file.Stat()
	content := make([]byte, stat.Size())
	if _, err := file.Read(content); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	block, _ := pem.Decode(content)
	if block == nil {
		return errors.New("RSA block invalid")
	}
	pki, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return err
	}
	pub, b := pki.(*rsa.PublicKey)
	if !b {
		return errors.New("RSA pub key invalid")
	}
	self.PublicKeyBase64 = string(pem.EncodeToMemory(block))
	self.publicKey = pub
	return nil
}

func (self *RsaObj) LoadRsaPemFileBase64(fileBase64 string) error {
	block, _ := pem.Decode([]byte(fileBase64))
	if block == nil {
		return errors.New("RSA block invalid")
	}
	pki, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return err
	}
	pub, b := pki.(*rsa.PublicKey)
	if !b {
		return errors.New("RSA pub key invalid")
	}
	self.publicKey = pub
	self.PublicKeyBase64 = fileBase64
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

// ******************************************************* RSA Implement *******************************************************

func (self *RsaObj) GetPrivateKey() (interface{}, string) {
	return self.privateKey, self.PrivateKeyBase64
}

func (self *RsaObj) GetPublicKey() (interface{}, string) {
	return self.publicKey, self.PublicKeyBase64
}

func (self *RsaObj) Encrypt(msg []byte) (string, error) {
	partLen := self.publicKey.N.BitLen()/8 - 11
	chunks := split(msg, partLen)
	buffer := bytes.NewBufferString("")
	for _, chunk := range chunks {
		bytes, err := rsa.EncryptPKCS1v15(rand.Reader, self.publicKey, chunk)
		if err != nil {
			return "", err
		}
		buffer.Write(bytes)
	}
	return utils.Base64Encode(buffer.Bytes()), nil
}

func (self *RsaObj) Decrypt(msg string) ([]byte, error) {
	partLen := self.publicKey.N.BitLen() / 8
	raw := utils.Base64Decode(msg)
	if len(raw) == 0 {
		return nil, errors.New("msg base64 decode failed")
	}
	chunks := split(raw, partLen)
	buffer := bytes.NewBufferString("")
	for _, chunk := range chunks {
		decrypted, err := rsa.DecryptPKCS1v15(rand.Reader, self.privateKey, chunk)
		if err != nil {
			return nil, err
		}
		buffer.Write(decrypted)
	}
	return buffer.Bytes(), nil
}

func (self *RsaObj) Sign(msg []byte) ([]byte, error) {
	h := sha256.New()
	h.Write(msg)
	has := h.Sum(nil)
	res, err := rsa.SignPKCS1v15(rand.Reader, self.privateKey, crypto.SHA256, has)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (self *RsaObj) Verify(msg, sign []byte) error {
	if msg == nil || len(msg) == 0 {
		return errors.New("RSA msg invalid")
	}
	if sign == nil || len(sign) == 0 {
		return errors.New("RSA sign invalid")
	}
	has := sha256.Sum256(msg)
	if err := rsa.VerifyPKCS1v15(self.publicKey, crypto.SHA256, has[:], sign); err != nil {
		return err
	}
	return nil
}
