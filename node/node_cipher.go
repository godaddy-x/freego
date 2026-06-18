package node

import (
	"net/http"

	"github.com/godaddy-x/freego/ex"
	"github.com/godaddy-x/freego/utils/crypto"
)

// CipherHook 按客户端用户 ID 动态解析 Plan2 ML-DSA Cipher（服务端私钥 + 该客户端公钥）。
type CipherHook func(usr int64) (crypto.Cipher, error)

// CipherKeyLoader 从外部源加载服务端 ML-DSA 私钥与指定客户端的 ML-DSA 公钥（Base64）。
type CipherKeyLoader func(usr int64) (serverPrkB64, clientPubB64 string, err error)

// CipherHookFromLoader 将 CipherKeyLoader 包装为 CipherHook。
func CipherHookFromLoader(loader CipherKeyLoader) CipherHook {
	return func(usr int64) (crypto.Cipher, error) {
		prk, pub, err := loader(usr)
		if err != nil {
			return nil, err
		}
		return crypto.CreateMLDSA87WithBase64(prk, pub)
	}
}

func resolvePQCipher(hook CipherHook, usr int64) (crypto.Cipher, error) {
	if hook == nil {
		return nil, ex.Throw{Code: http.StatusBadRequest, Msg: "plan2 cipher hook not configured"}
	}
	cipher, err := hook(usr)
	if err != nil {
		return nil, err
	}
	if cipher == nil {
		return nil, ex.Throw{Code: http.StatusBadRequest, Msg: "plan2 cipher hook returned nil"}
	}
	return cipher, nil
}

func (self *Context) getPQCipher(usr int64) (crypto.Cipher, error) {
	return resolvePQCipher(self.cipherHook, usr)
}

func (self *WsServer) getPQCipher(usr int64) (crypto.Cipher, error) {
	return resolvePQCipher(self.cipherHook, usr)
}

func (mh *MessageHandler) getPQCipher(usr int64) (crypto.Cipher, error) {
	return resolvePQCipher(mh.cipherHook, usr)
}
