package grpcgw

import (
	DIC "github.com/godaddy-x/freego/core/const"
	"github.com/godaddy-x/freego/core/crypto"
	"github.com/godaddy-x/freego/core/ex"
	utils "github.com/godaddy-x/freego/core/str"
	pb "github.com/godaddy-x/freego/protocol/grpcgw"
	fwsign "github.com/godaddy-x/freego/protocol/sign"
)

func (c *Context) validatePlan2() error {
	req := c.req
	if err := c.validateWireEnvelope(); err != nil {
		return err
	}
	if req.P != 2 {
		return ex.Throw{Code: 400, Msg: "plan must be 2"}
	}

	cipher, err := fwsign.ResolvePQCipher(c.gw.cipherHook, req.U)
	if err != nil {
		return err
	}
	canonical := pb.CanonicalRequest(req.R, req.D, req.N, req.T, req.P, req.U)
	digest := pb.SHA256Sum(canonical)
	if err := c.verifyOuterSign(cipher, digest, req.E); err != nil {
		return err
	}
	c.setStorage(storageCipher, cipher)

	// KeyRoute：尚无 shared_key。d 为明文（客户端公钥材料），s 用空 HMAC key。
	if c.cfg.KeyRoute {
		if err := c.verifyRequestHMAC(nil); err != nil {
			return err
		}
		c.plain = req.D
		return nil
	}

	sharedKey, err := c.negotiatePlan2SharedKey()
	if err != nil {
		return err
	}
	c.setStorage(storageSharedKey, DIC.CopyData(sharedKey))
	defer DIC.ClearData(sharedKey)

	macKey := sharedKey[:32]
	if err := c.verifyRequestHMAC(macKey); err != nil {
		return err
	}
	return c.openRequestBody(macKey)
}

func (c *Context) negotiatePlan2SharedKey() ([]byte, error) {
	wire := c.plan2AuthorizationBytes()
	if len(wire) == 0 || len(wire) > pb.MaxPlan2PublicKeyWireLen() {
		return nil, ex.Throw{Code: 400, Msg: "plan2 authorization invalid"}
	}
	public, err := pb.UnmarshalPlan2PublicKey(wire)
	if err != nil {
		return nil, err
	}

	cache := c.gw.cacheObject()
	cipher, err := c.plan2Cipher()
	if err != nil {
		return nil, err
	}
	if err := pb.CheckPlan2PublicKey(cache, public, cipher, c.req.U, true); err != nil {
		return nil, err
	}

	prkKey := pb.Plan2PublicKeyCacheKey(public)
	prkObject, err := plan2PrivateKeyFromCache(cache, prkKey)
	if err != nil {
		return nil, err
	}
	defer cache.Del(prkKey)
	if len(prkObject.Key) == 0 {
		return nil, ex.Throw{Code: 400, Msg: "prk read is nil"}
	}
	shared, err := crypto.DecapsulatePeerCiphertextBytes(prkObject.Key, public.Tag)
	if err != nil || len(shared) == 0 {
		return nil, ex.Throw{Code: 400, Msg: "ML-KEM shared key error", Err: err}
	}
	defer DIC.ClearData(shared)
	noc := utils.Base64Decode(prkObject.Noc)
	if len(noc) == 0 {
		return nil, ex.Throw{Code: 400, Msg: "prk noc invalid"}
	}
	return fwsign.HKDFKeyBytes(shared, noc)
}
