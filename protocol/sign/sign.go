package sign

import (
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/sha256"
	"io"
	"net/http"
	"strconv"

	DIC "github.com/godaddy-x/freego/core/const"
	"github.com/godaddy-x/freego/core/crypto"
	"github.com/godaddy-x/freego/core/ex"
	"github.com/godaddy-x/freego/core/jwt"
	utils "github.com/godaddy-x/freego/core/str"
	cache "github.com/godaddy-x/freego/infra/cache/contract"
	"github.com/godaddy-x/freego/protocol/wire"
)

const SharedInfo = "freego-ecdh-aes-gcm"

func HKDFKey(shared []byte, nonce string) ([]byte, error) {
	return hkdf.Key(sha256.New, shared, utils.Base64Decode(nonce), SharedInfo, 32)
}

func appendBodyMessageTo(dst []byte, path, data, nonce string, t, plan, usr int64) []byte {
	const int64MaxLen = 20
	sep := DIC.SEP
	est := len(path) + len(data) + len(nonce) + len(sep)*5 + int64MaxLen*3
	if cap(dst)-len(dst) < est {
		nd := make([]byte, len(dst), len(dst)+est)
		copy(nd, dst)
		dst = nd
	}
	dst = append(dst, path...)
	dst = append(dst, sep...)
	dst = append(dst, data...)
	dst = append(dst, sep...)
	dst = append(dst, nonce...)
	dst = append(dst, sep...)
	dst = strconv.AppendInt(dst, t, 10)
	dst = append(dst, sep...)
	dst = strconv.AppendInt(dst, plan, 10)
	dst = append(dst, sep...)
	dst = strconv.AppendInt(dst, usr, 10)
	return dst
}

func AppendBodyMessage(path, data, nonce string, time, plan, usr int64) []byte {
	return appendBodyMessageTo(nil, path, data, nonce, time, plan, usr)
}

func DigestBodyMessage(path, data, nonce string, time, plan, usr int64) []byte {
	return utils.SHA256_BASE(AppendBodyMessage(path, data, nonce, time, plan, usr))
}

func SignAndDigestBodyMessage(path, data, nonce string, time, plan, usr int64, key []byte) ([]byte, []byte) {
	msg := AppendBodyMessage(path, data, nonce, time, plan, usr)
	return utils.HMAC_SHA256_BASE(msg, key), utils.SHA256_BASE(msg)
}

func SignBodyMessage(path, data, nonce string, time, plan, usr int64, key []byte) []byte {
	h := hmac.New(sha256.New, key)
	sep := DIC.SEP
	_, _ = io.WriteString(h, path)
	_, _ = io.WriteString(h, sep)
	_, _ = io.WriteString(h, data)
	_, _ = io.WriteString(h, sep)
	_, _ = io.WriteString(h, nonce)
	_, _ = io.WriteString(h, sep)
	var ibuf [20]byte
	b := strconv.AppendInt(ibuf[:0], time, 10)
	_, _ = h.Write(b)
	_, _ = io.WriteString(h, sep)
	b = strconv.AppendInt(ibuf[:0], plan, 10)
	_, _ = h.Write(b)
	_, _ = io.WriteString(h, sep)
	b = strconv.AppendInt(ibuf[:0], usr, 10)
	_, _ = h.Write(b)
	return h.Sum(nil)
}

func CreatePublicKey(key, tag string, usr int64, cipher crypto.Cipher) (*wire.PublicKey, error) {
	requestObject := &wire.PublicKey{}
	requestObject.Key = key
	requestObject.Tag = tag
	requestObject.Noc = utils.Base64Encode(utils.GetRandomSecure(32))
	requestObject.Exp = utils.UnixSecond()
	requestObject.Usr = usr
	sig, err := cipher.Sign(utils.Str2Bytes(utils.AddStr(requestObject.Key, DIC.SEP, requestObject.Tag, DIC.SEP, requestObject.Noc, DIC.SEP, requestObject.Exp, DIC.SEP, requestObject.Usr)))
	if err != nil {
		return nil, ex.Throw{Msg: "outer sign message error: " + err.Error()}
	}
	requestObject.Sig = utils.Base64Encode(sig)
	return requestObject, nil
}

func CheckPublicKey(c cache.Cache, requestObject *wire.PublicKey, cipher crypto.Cipher) error {
	if cipher == nil {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request cipher invalid"}
	}
	if len(requestObject.Key) < 32 {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request key invalid"}
	}
	if len(requestObject.Tag) < 32 {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request tag invalid"}
	}
	if !crypto.CheckOuterSignatureB64Valid(requestObject.Sig) {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request sig invalid"}
	}
	if len(requestObject.Noc) < 32 {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request noc invalid"}
	}
	if requestObject.Usr < 0 {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request usr invalid"}
	}
	if utils.MathAbs(utils.UnixSecond()-requestObject.Exp) > jwt.FIVE_MINUTES {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request exp invalid"}
	}
	key := utils.FNV1a64(requestObject.Noc)
	if c != nil {
		if ok, err := c.Exists(key); err != nil {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "noc check failed", Err: err}
		} else if ok {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "request noc duplicated"}
		}
	}
	if err := cipher.Verify(utils.Str2Bytes(utils.AddStr(requestObject.Key, DIC.SEP, requestObject.Tag, DIC.SEP, requestObject.Noc, DIC.SEP, requestObject.Exp, DIC.SEP, requestObject.Usr)), utils.Base64Decode(requestObject.Sig)); err != nil {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request verify sig invalid"}
	}
	if c != nil {
		if err := c.Put(key, 1, 300); err != nil {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "request noc cache error", Err: err}
		}
	}
	return nil
}
