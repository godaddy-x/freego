package httpsvr

import (
	"github.com/godaddy-x/freego/core/crypto"
	fwsign "github.com/godaddy-x/freego/protocol/sign"
)

func (self *Context) getPQCipher(usr int64) (crypto.Cipher, error) {
	return fwsign.ResolvePQCipher(self.cipherHook, usr)
}
