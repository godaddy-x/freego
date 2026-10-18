package grpcgw

import (
	"errors"

	"github.com/godaddy-x/freego/core/crypto"
	"github.com/godaddy-x/freego/core/jwt"
	utils "github.com/godaddy-x/freego/core/str"
	pb "github.com/godaddy-x/freego/protocol/grpcgw"
)

var (
	errBadMAC         = errors.New("grpcgw: response signature invalid")
	errResponseBind   = errors.New("grpcgw: response bind mismatch")
	errResponseOuter  = errors.New("grpcgw: response outer signature invalid")
	errResponseTime   = errors.New("grpcgw: response time window exceeded")
)

// SealPlan0 将业务 protobuf 原始字节写入 req.D（p=0，不做 AES）。
func SealPlan0(req *pb.GatewayRequest, plain, macKey []byte) error {
	req.P = 0
	req.D = plain
	pb.SignRequest(req, macKey)
	return nil
}

// SealPlan1 将 plain AES-GCM 加密后写入 req.D（p=1）并设置 HMAC s。
func SealPlan1(req *pb.GatewayRequest, plain, macKey []byte) error {
	req.P = 1
	aad := pb.AADRequest(req.R, req.N, req.T, req.P, req.U)
	d, err := utils.AesGCMEncryptBaseByteResult(plain, macKey, aad)
	if err != nil {
		return err
	}
	req.D = d
	pb.SignRequest(req, macKey)
	return nil
}

func verifyInboundResponse(req *pb.GatewayRequest, res *pb.GatewayResponse, u int64, macKey []byte) error {
	if req == nil || res == nil || res.R != req.R || len(req.N) == 0 || !pb.ConstantTimeEqual(res.N, req.N) {
		return errResponseBind
	}
	canonical := pb.CanonicalResponse(res.R, res.D, res.N, res.T, res.P, u, res.C, res.M)
	if len(res.S) != 32 || !pb.ConstantTimeEqual(pb.HMACSHA256(macKey, canonical), res.S) {
		return errBadMAC
	}
	return verifyResponseTime(res.T)
}

// OpenPlan01Response 校验响应绑定 (r,n)、HMAC；p=0 返回原始 d，p=1 解密 d。
func OpenPlan01Response(req *pb.GatewayRequest, res *pb.GatewayResponse, macKey []byte) ([]byte, error) {
	var u int64
	if req != nil {
		u = req.U
	}
	if err := verifyInboundResponse(req, res, u, macKey); err != nil {
		return nil, err
	}
	if res.C != 200 {
		return nil, gatewayResponseThrow(res)
	}
	if res.P == 0 {
		return res.D, nil
	}
	aad := pb.AADResponse(res.R, res.N, res.T, res.P, req.U, res.C, res.M)
	return utils.AesGCMDecryptBaseByteResult(res.D, macKey, aad)
}

func verifyResponseTime(t int64) error {
	if t <= 0 || utils.MathAbs(utils.UnixSecond()-t) > jwt.FIVE_MINUTES {
		return errResponseTime
	}
	return nil
}

func verifyPlan2Response(req *pb.GatewayRequest, res *pb.GatewayResponse, macKey []byte, cipher crypto.Cipher, usr int64) error {
	if err := verifyInboundResponse(req, res, usr, macKey); err != nil {
		return err
	}
	canonical := pb.CanonicalResponse(res.R, res.D, res.N, res.T, res.P, usr, res.C, res.M)
	if !crypto.CheckRPCXSignatureValid(res.E) {
		return errResponseOuter
	}
	if err := cipher.Verify(pb.SHA256Sum(canonical), res.E); err != nil {
		return errResponseOuter
	}
	return gatewayResponseThrow(res)
}

// SignOuter sets req.E = ML-DSA(SHA256(canonical)); call after D and S are final.
func SignOuter(req *pb.GatewayRequest, cipher crypto.Cipher) error {
	sig, err := cipher.Sign(pb.SHA256Sum(pb.CanonicalRequest(req.R, req.D, req.N, req.T, req.P, req.U)))
	if err != nil {
		return err
	}
	req.E = sig
	return nil
}
