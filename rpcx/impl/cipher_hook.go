package impl

import "github.com/godaddy-x/freego/utils/crypto"

// CipherHook 按客户端用户 ID 动态解析 ML-DSA Cipher（本端私钥 + 对端公钥）。
type CipherHook func(usr int64) (crypto.Cipher, error)
