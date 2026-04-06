// Ed25519 / X25519（Curve25519）封装，依赖 github.com/godaddy-x/eccrypto。
//
// 能力划分：
//   - Ed25519Object：HTTP、WebSocket、gRPC rpcx 的「外层」身份与签名（rpcx：s=SHA256(规范串)，e=Ed25519(s)，仅 P=0 明文）。
//   - X25519Object：Plan2 匿名通道，临时密钥 + ecc.EncryptX25519 混合加密。
//   - X25519RPCObject：静态 X25519（ECDH 共享密钥、EncryptX25519 载荷），供自建协议或扩展；当前 rpcx 包 CommonWorker 未使用。
package crypto

import (
	"crypto/ecdh"
	"crypto/ed25519"
	"errors"
	"fmt"
	"sync"

	ecc "github.com/godaddy-x/eccrypto"
	"github.com/godaddy-x/freego/utils"
)

// X25519Object 匿名通道（Plan2）使用的临时 X25519 密钥对，基于标准库 crypto/ecdh（Curve25519）。
type X25519Object struct {
	PrivateKeyBase64 string
	PublicKeyBase64  string

	privateKey *ecdh.PrivateKey
	publicKey  *ecdh.PublicKey

	// 仅加密给对方时使用：ecc.EncryptX25519 需要接收方 X25519 公钥（32 字节）；解密用本对象私钥 + 密文内临时公钥即可。
	encryptPeerPub *ecdh.PublicKey
}

// CreateX25519 生成新的 X25519 密钥对，并填充 PublicKeyBase64。
func (self *X25519Object) CreateX25519() error {
	prk, err := ecc.CreateX25519()
	if err != nil {
		return err
	}

	self.privateKey = prk
	self.publicKey = prk.PublicKey()
	self.PublicKeyBase64 = utils.Base64Encode(self.publicKey.Bytes())
	return nil
}

// LoadX25519PrivateFromBase64 从 Base64 加载 X25519 私钥，并推导公钥与 PublicKeyBase64。
func (self *X25519Object) LoadX25519PrivateFromBase64(b64 string) error {
	prk, err := ecc.LoadX25519PrivateKeyFromBase64(b64)
	if err != nil {
		return err
	}
	self.privateKey = prk
	self.publicKey = prk.PublicKey()
	self.PublicKeyBase64 = utils.Base64Encode(self.publicKey.Bytes())
	return nil
}

// SetPeerPublicKeyForEncrypt 设置接收方 X25519 公钥；调用 Encrypt 前必须设置。
// 使用 ecc.EncryptX25519(nil, …) 路径，避免 eccrypto 在加密后清零传入的私钥导致本对象私钥损坏。
func (self *X25519Object) SetPeerPublicKeyForEncrypt(peer *ecdh.PublicKey) {
	self.encryptPeerPub = peer
}

// ******************************************************* X25519 Cipher（Cipher 接口）*******************************************************

func (self *X25519Object) GetPrivateKey() (interface{}, string) {
	return self.privateKey, self.PrivateKeyBase64
}

func (self *X25519Object) GetPublicKey() (interface{}, string) {
	return self.publicKey, self.PublicKeyBase64
}

func (self *X25519Object) Encrypt(msg, aad []byte) (string, error) {
	if self.encryptPeerPub == nil {
		return "", errors.New("X25519 Encrypt: set peer with SetPeerPublicKeyForEncrypt first")
	}
	peerBytes := ecc.GetX25519PublicKeyBytes(self.encryptPeerPub)
	if len(peerBytes) != 32 {
		return "", errors.New("X25519 Encrypt: invalid peer public key")
	}
	out, err := ecc.EncryptX25519(nil, peerBytes, msg, aad)
	if err != nil {
		return "", err
	}
	return utils.Base64Encode(out), nil
}

func (self *X25519Object) Decrypt(msg string, aad []byte) ([]byte, error) {
	bs := utils.Base64Decode(msg)
	if len(bs) == 0 {
		return nil, errors.New("base64 parse failed")
	}
	r, err := ecc.DecryptX25519(self.privateKey, bs, aad, nil)
	if err != nil {
		return nil, err
	}
	return r, nil
}

func (self *X25519Object) Sign(msg []byte) ([]byte, error) {
	return nil, nil
}

func (self *X25519Object) Verify(msg, sign []byte) error {
	return nil
}

// ******************************************************* X25519 静态 ECDH + 载荷封装（非 rpcx 默认路径）*******************************************************

// X25519RPCObject 本端 X25519 私钥 + 对端 X25519 公钥：可 ECDH 出共享密钥（RPCXSharedSecret），
// 并可对业务载荷做 ecc.EncryptX25519 / DecryptX25519。
// 注意：当前 github.com/godaddy-x/freego/rpcx 的 CommonWorker 使用 Ed25519Object + SHA256(s)+e，不注册本类型。
type X25519RPCObject struct {
	PrivateKeyBase64 string
	PublicKeyBase64  string
	PeerPublicKeyB64 string

	privateKey    *ecdh.PrivateKey
	peerPublicKey *ecdh.PublicKey

	mu           sync.Mutex
	cachedShared []byte
}

func (o *X25519RPCObject) sharedSecretLocked() ([]byte, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.cachedShared != nil {
		return o.cachedShared, nil
	}
	sk, err := ecc.GenSharedKeyX25519(o.privateKey, o.peerPublicKey)
	if err != nil {
		return nil, err
	}
	o.cachedShared = sk
	return o.cachedShared, nil
}

// RPCXSharedSecret 返回共享秘密的副本（调用方可安全 ClearData，不影响对象内缓存）。
func (o *X25519RPCObject) RPCXSharedSecret() ([]byte, error) {
	sk, err := o.sharedSecretLocked()
	if err != nil {
		return nil, err
	}
	out := make([]byte, len(sk))
	copy(out, sk)
	return out, nil
}

// RPCXCacheKeyBytes 对端 X25519 公钥 32 字节，用于本地缓存索引。
func (o *X25519RPCObject) RPCXCacheKeyBytes() []byte {
	return ecc.GetX25519PublicKeyBytes(o.peerPublicKey)
}

// RPCXEncryptPayload ecc.EncryptX25519(nil, 对端公钥, …)，与 RPCXDecryptPayload 配对。
func (o *X25519RPCObject) RPCXEncryptPayload(plaintext, additionalData []byte) ([]byte, error) {
	pub := ecc.GetX25519PublicKeyBytes(o.peerPublicKey)
	if len(pub) != 32 {
		return nil, errors.New("invalid peer X25519 public key")
	}
	return ecc.EncryptX25519(nil, pub, plaintext, additionalData)
}

// RPCXDecryptPayload 解密 RPCXEncryptPayload 密文（本端私钥）。
func (o *X25519RPCObject) RPCXDecryptPayload(ciphertext, additionalData []byte) ([]byte, error) {
	return ecc.DecryptX25519(o.privateKey, ciphertext, additionalData, nil)
}

func (o *X25519RPCObject) GetPrivateKey() (interface{}, string) {
	return o.privateKey, o.PrivateKeyBase64
}

func (o *X25519RPCObject) GetPublicKey() (interface{}, string) {
	return o.privateKey.PublicKey(), o.PublicKeyBase64
}

func (o *X25519RPCObject) Encrypt(msg, aad []byte) (string, error) {
	out, err := o.RPCXEncryptPayload(msg, aad)
	if err != nil {
		return "", err
	}
	return utils.Base64Encode(out), nil
}

func (o *X25519RPCObject) Decrypt(msg string, aad []byte) ([]byte, error) {
	bs := utils.Base64Decode(msg)
	if len(bs) == 0 {
		return nil, errors.New("base64 parse failed")
	}
	return o.RPCXDecryptPayload(bs, aad)
}

func (o *X25519RPCObject) Sign(msg []byte) ([]byte, error) {
	return nil, errors.New("X25519RPCObject: 不提供签名，请使用 Ed25519Object")
}

func (o *X25519RPCObject) Verify(msg, sign []byte) error {
	if len(sign) == 0 {
		return nil
	}
	return errors.New("X25519RPCObject: 不提供验签，请使用 Ed25519Object")
}

// ******************************************************* Ed25519 Implement *******************************************************

// Ed25519Object 双向身份：Sign 用本端私钥，Verify 用对端公钥（CreateEd25519WithBase64 镜像配置时 PublicKeyBase64 字段存对端公钥）。
type Ed25519Object struct {
	PrivateKeyBase64 string
	PublicKeyBase64  string

	privateKey ed25519.PrivateKey
	publicKey  ed25519.PublicKey
}

// PrintEd25519Base64 本地快速打印一对 Base64 Ed25519 密钥（调试用）
func PrintEd25519Base64() {
	o := &Ed25519Object{}
	_ = o.CreateEd25519()
	fmt.Println("私钥：", o.PrivateKeyBase64)
	fmt.Println("公钥：", o.PublicKeyBase64)
}

func (self *Ed25519Object) CreateEd25519() error {
	prk, err := ecc.CreateEd25519()
	if err != nil {
		return err
	}
	self.privateKey = prk
	self.publicKey = prk.Public().(ed25519.PublicKey)
	pubBs, err := ecc.GetEd25519PublicKeyBytes(self.publicKey)
	if err != nil {
		return err
	}
	self.PublicKeyBase64 = utils.Base64Encode(pubBs)
	prkBs, err := ecc.GetEd25519PrivateKeyBytes(prk)
	if err != nil {
		return err
	}
	self.PrivateKeyBase64 = utils.Base64Encode(prkBs)
	return nil
}

// CreateEd25519WithBase64 按「本端私钥 + 对端公钥」加载身份（Sign/Verify）。
//
// 用途：HTTP、WebSocket 外层签名；gRPC rpcx.AddCipher 同样注册本类型（与 X25519 / X25519RPCObject 独立）。
// 镜像：服务端（服务端私钥, 客户端公钥），客户端（客户端私钥, 服务端公钥）。
func CreateEd25519WithBase64(prkB64, peerPubB64 string) (*Ed25519Object, error) {
	prk, err := ecc.LoadEd25519PrivateKeyFromBase64(prkB64)
	if err != nil {
		return nil, err
	}
	pub, err := ecc.LoadEd25519PublicKeyFromBase64(peerPubB64)
	if err != nil {
		return nil, err
	}
	return &Ed25519Object{
		privateKey:       prk,
		publicKey:        pub,
		PrivateKeyBase64: prkB64,
		PublicKeyBase64:  peerPubB64,
	}, nil
}

func (self *Ed25519Object) LoadEd25519(b64 string) error {
	prk, err := ecc.LoadEd25519PrivateKeyFromBase64(b64)
	if err != nil {
		return err
	}
	self.privateKey = prk
	self.publicKey = prk.Public().(ed25519.PublicKey)
	pubBs, err := ecc.GetEd25519PublicKeyBytes(self.publicKey)
	if err != nil {
		return err
	}
	self.PublicKeyBase64 = utils.Base64Encode(pubBs)
	self.PrivateKeyBase64 = b64
	return nil
}

// ******************************************************* Ed25519 Cipher Interface Implement *******************************************************

func (self *Ed25519Object) GetPrivateKey() (interface{}, string) {
	return self.privateKey, self.PrivateKeyBase64
}

func (self *Ed25519Object) GetPublicKey() (interface{}, string) {
	return self.publicKey, self.PublicKeyBase64
}

func (self *Ed25519Object) Encrypt(msg, aad []byte) (string, error) {
	return "", errors.New("Ed25519 does not support encryption")
}

func (self *Ed25519Object) Decrypt(msg string, aad []byte) ([]byte, error) {
	return nil, errors.New("Ed25519 does not support decryption")
}

func (self *Ed25519Object) Sign(msg []byte) ([]byte, error) {
	if len(self.privateKey) == 0 {
		return nil, errors.New("Ed25519 private key not initialized")
	}
	return ecc.SignEd25519(self.privateKey, msg)
}

func (self *Ed25519Object) Verify(msg, sign []byte) error {
	if len(self.publicKey) == 0 {
		return errors.New("Ed25519 public key not initialized")
	}
	return ecc.VerifyEd25519(self.publicKey, msg, sign)
}
