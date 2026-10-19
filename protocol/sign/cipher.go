package sign

import (
	"net/http"

	"github.com/godaddy-x/freego/core/crypto"
	"github.com/godaddy-x/freego/core/ex"
)

// CipherHook 的 usr 是本端 PQC 证书编号（clientNo），不是登录用户 ID。
type CipherHook func(usr int64) (crypto.Cipher, error)

// CipherKeyLoader 的 usr 是本端 PQC 证书编号（clientNo），不是登录用户 ID。
type CipherKeyLoader func(usr int64) (serverPrkB64, clientPubB64 string, err error)

func CipherHookFromLoader(loader CipherKeyLoader) CipherHook {
	return func(usr int64) (crypto.Cipher, error) {
		prk, pub, err := loader(usr)
		if err != nil {
			return nil, err
		}
		return crypto.CreateMLDSA87WithBase64(prk, pub)
	}
}

func ResolvePQCipher(hook CipherHook, usr int64) (crypto.Cipher, error) {
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
