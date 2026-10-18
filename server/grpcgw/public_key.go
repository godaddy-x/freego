package grpcgw

import (
	ecc "github.com/godaddy-x/eccrypto"
	"github.com/godaddy-x/freego/core/ex"
	utils "github.com/godaddy-x/freego/core/str"
	pb "github.com/godaddy-x/freego/protocol/grpcgw"
	fwsign "github.com/godaddy-x/freego/protocol/sign"
	"github.com/godaddy-x/freego/protocol/wire"
)

func (c *Context) CreatePublicKey() error {
	if len(c.plain) > pb.MaxPlan2PublicKeyWireLen() {
		return ex.Throw{Code: 400, Msg: "request data too long"}
	}
	in, err := pb.UnmarshalPlan2PublicKey(c.plain)
	if err != nil {
		return err
	}
	if in.Usr != c.req.U {
		return ex.Throw{Code: 400, Msg: "request usr mismatch"}
	}
	cipher, err := fwsign.ResolvePQCipher(c.gw.cipherHook, in.Usr)
	if err != nil {
		return err
	}
	cache := c.gw.cacheObject()
	if err := pb.CheckPlan2PublicKey(cache, in, cipher, c.req.U, false); err != nil {
		return err
	}
	dk, err := ecc.CreateMLKEM1024()
	if err != nil {
		return ex.Throw{Code: 400, Msg: "ML-KEM key generation failed", Err: err}
	}
	ek := ecc.GetMLKEM1024EncapsulationKeyBytes(dk.EncapsulationKey())
	out, err := pb.CreatePlan2PublicKey(ek, utils.GetRandomSecure(32), in.Usr, cipher)
	if err != nil {
		return err
	}
	if err := cache.Put(
		pb.Plan2PublicKeyCacheKey(out),
		&wire.PrivateKey{Key: ecc.MLKEM1024DecapsulationKeyToBase64(dk), Noc: utils.Base64Encode(out.Noc)},
		180,
	); err != nil {
		return ex.Throw{Code: 400, Msg: "prk cache setting error", Err: err}
	}
	c.plain, err = pb.MarshalPlan2PublicKey(out)
	return err
}
