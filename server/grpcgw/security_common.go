package grpcgw

import (
	"encoding/hex"

	"github.com/godaddy-x/freego/core/crypto"
	"github.com/godaddy-x/freego/core/ex"
	"github.com/godaddy-x/freego/core/jwt"
	utils "github.com/godaddy-x/freego/core/str"
	cache "github.com/godaddy-x/freego/infra/cache/contract"
	pb "github.com/godaddy-x/freego/protocol/grpcgw"
	"github.com/godaddy-x/freego/protocol/wire"
)

func (g *GrpcGateway) validReplayAttack(sign []byte) error {
	if len(sign) != 32 {
		return ex.Throw{Code: 400, Msg: "replay check: signature length invalid"}
	}
	key := utils.AddStr("grpcgw:replay:", hex.EncodeToString(sign))
	c := g.cacheObject()
	b, err := c.Exists(key)
	if err != nil {
		return ex.Throw{Code: 400, Msg: "cache operation failed", Err: err}
	}
	if b {
		return ex.Throw{Code: 400, Msg: "replay attack detected"}
	}
	if err := c.Put(key, 1, int(jwt.FIVE_MINUTES)); err != nil {
		return ex.Throw{Code: 400, Msg: "cache replay record failed", Err: err}
	}
	return nil
}

func (c *Context) validateTimeWindow(t int64) error {
	if utils.MathAbs(utils.UnixSecond()-t) > jwt.FIVE_MINUTES {
		return ex.Throw{Code: 400, Msg: "time window exceeded"}
	}
	return nil
}

func (c *Context) verifyRequestHMAC(macKey []byte) error {
	req := c.req
	if len(req.S) != 32 {
		return ex.Throw{Code: 400, Msg: "signature must be 32 bytes"}
	}
	canonical := pb.CanonicalRequest(req.R, req.D, req.N, req.T, req.P, req.U)
	if !pb.ConstantTimeEqual(pb.HMACSHA256(macKey, canonical), req.S) {
		return ex.Throw{Code: 400, Msg: "signature invalid"}
	}
	return c.gw.validReplayAttack(req.S)
}

func (c *Context) verifyOuterSign(cipher crypto.Cipher, digest, sig []byte) error {
	if cipher == nil {
		return ex.Throw{Code: 400, Msg: "cipher invalid"}
	}
	if !crypto.CheckRPCXSignatureValid(sig) {
		return ex.Throw{Code: 400, Msg: "outer signature length invalid"}
	}
	if err := cipher.Verify(digest, sig); err != nil {
		return ex.Throw{Code: 400, Msg: "outer signature invalid", Err: err}
	}
	return nil
}

func plan2PrivateKeyFromCache(c cache.Cache, cacheKey string) (*wire.PrivateKey, error) {
	if c.Mode() == cache.LOCAL {
		v, b, err := c.Get(cacheKey, nil)
		if err != nil || !b {
			return nil, ex.Throw{Code: 400, Msg: "prk read error", Err: err}
		}
		return v.(*wire.PrivateKey), nil
	}
	prk := &wire.PrivateKey{}
	if _, b, err := c.Get(cacheKey, prk); err != nil || !b {
		return nil, ex.Throw{Code: 400, Msg: "prk read error", Err: err}
	}
	return prk, nil
}

func (c *Context) validateWireEnvelope() error {
	if err := pb.ValidateRequestFields(c.req.R, c.req.D, c.req.N, c.req.T, c.req.P, c.req.U, pb.DefaultMaxWireBlob); err != nil {
		return ex.Throw{Code: 400, Msg: err.Error()}
	}
	return c.validateTimeWindow(c.req.T)
}

// openRequestBody：p=0 取明文 d；p=1/2 用 macKey 解密 AES-GCM。
func (c *Context) openRequestBody(macKey []byte) error {
	req := c.req
	if req.P == 0 {
		c.plain = req.D
		return nil
	}
	aad := pb.AADRequest(req.R, req.N, req.T, req.P, req.U)
	plain, err := utils.AesGCMDecryptBaseByteResult(req.D, macKey, aad)
	if err != nil {
		return ex.Throw{Code: 400, Msg: "decrypt failed", Err: err}
	}
	c.plain = plain
	return nil
}
