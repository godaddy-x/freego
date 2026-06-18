package rpcx

import (
	"errors"

	"github.com/godaddy-x/freego/rpcx/impl"
	"github.com/godaddy-x/freego/utils/crypto"
)

// CipherHook 按客户端用户 ID 动态解析 ML-DSA Cipher（本端私钥 + 对端公钥）。
type CipherHook = impl.CipherHook

// CipherKeyLoader 从外部源加载本端 ML-DSA 私钥与对端 ML-DSA 公钥（Base64）。
type CipherKeyLoader func(usr int64) (prkB64, peerPubB64 string, err error)

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

var (
	errCipherHookNotConfigured = errors.New("cipher hook not configured")
	errCipherHookReturnedNil   = errors.New("cipher hook returned nil")
)

func resolveCipher(hook CipherHook, usr int64) (crypto.Cipher, error) {
	if hook == nil {
		return nil, errCipherHookNotConfigured
	}
	cipher, err := hook(usr)
	if err != nil {
		return nil, err
	}
	if cipher == nil {
		return nil, errCipherHookReturnedNil
	}
	return cipher, nil
}
