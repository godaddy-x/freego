package crypto

import (
	"crypto/ecdh"
	"crypto/ed25519"
	"errors"
	"fmt"

	ecc "github.com/godaddy-x/eccrypto"
	"github.com/godaddy-x/freego/utils"
)

// X25519Object 匿名通道（Plan2）使用的临时 X25519 密钥对，基于标准库 crypto/ecdh（Curve25519）。
type X25519Object struct {
	PrivateKeyBase64 string
	PublicKeyBase64  string

	privateKey *ecdh.PrivateKey
	publicKey  *ecdh.PublicKey
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

// ******************************************************* X25519 Cipher（Cipher 接口）*******************************************************

func (self *X25519Object) GetPrivateKey() (interface{}, string) {
	return self.privateKey, self.PrivateKeyBase64
}

func (self *X25519Object) GetPublicKey() (interface{}, string) {
	return self.publicKey, self.PublicKeyBase64
}

func (self *X25519Object) Encrypt(msg, aad []byte) (string, error) {
	return "", nil
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

// ******************************************************* Ed25519 Implement *******************************************************

type Ed25519Object struct {
	PrivateKeyBase64 string
	PublicKeyBase64  string

	privateKey ed25519.PrivateKey
	publicKey  ed25519.PublicKey

	// RPCX 专用：与 eccrypto X25519 ECDH 一致（本端 X25519 私钥 + 对端 X25519 公钥），与 Ed25519 签名密钥独立。
	rpcX25519Prk     *ecdh.PrivateKey
	rpcPeerX25519Pub *ecdh.PublicKey
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

// CreateEd25519WithBase64 按「本端私钥 + 对端公钥」加载身份，用于双向外层签名（Sign 用私钥，Verify 用对端公钥）。
//
// HTTP、WebSocket、RPCX 是彼此独立的服务，各自单独配置，不要求三处共用同一份配置文件。
// 镜像关系：服务端配置为（服务端私钥, 客户端公钥），客户端配置为（客户端私钥, 服务端公钥）。
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

// CreateEd25519WithBase64ForRPC 仅用于 RPCX：在 CreateEd25519WithBase64 同一套「本端 Ed25519 私钥 + 对端 Ed25519 公钥」之上，
// 再绑定一对 X25519（本端 X25519 私钥 + 对端 X25519 公钥），与 eccrypto GenSharedKeyX25519 一致，供 gRPC 共享密钥。
// 服务端示例：(serverEdPrk, clientEdPub, serverXPrk, clientXPub)；客户端示例：(clientEdPrk, serverEdPub, clientXPrk, serverXPub)。
// HTTP/WebSocket 只用 CreateEd25519WithBase64，不需要 X25519 静态密钥对。
func CreateEd25519WithBase64ForRPC(edPrkB64, peerEdPubB64, selfX25519PrkB64, peerX25519PubB64 string) (*Ed25519Object, error) {
	o, err := CreateEd25519WithBase64(edPrkB64, peerEdPubB64)
	if err != nil {
		return nil, err
	}
	xprk, err := ecc.LoadX25519PrivateKeyFromBase64(selfX25519PrkB64)
	if err != nil {
		return nil, err
	}
	xpub, err := ecc.LoadX25519PublicKeyFromBase64(peerX25519PubB64)
	if err != nil {
		return nil, err
	}
	o.rpcX25519Prk = xprk
	o.rpcPeerX25519Pub = xpub
	return o, nil
}

// RPCXSharedSecret 使用本端 X25519 私钥与对端 X25519 公钥做 ECDH，返回 32 字节共享秘密。
func (o *Ed25519Object) RPCXSharedSecret() ([]byte, error) {
	if o.rpcX25519Prk == nil || o.rpcPeerX25519Pub == nil {
		return nil, errors.New("RPC X25519 not configured: use CreateEd25519WithBase64ForRPC")
	}
	return ecc.GenSharedKeyX25519(o.rpcX25519Prk, o.rpcPeerX25519Pub)
}

// RPCXCacheKeyBytes 返回对端 X25519 公钥 32 字节，用于本地缓存 RPC 共享密钥的索引；未配置 RPC X25519 时返回 nil。
func (o *Ed25519Object) RPCXCacheKeyBytes() []byte {
	if o.rpcPeerX25519Pub == nil {
		return nil
	}
	return ecc.GetX25519PublicKeyBytes(o.rpcPeerX25519Pub)
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
