package grpcgw

import (
	utils "github.com/godaddy-x/freego/core/str"
)

// SignRequest sets req.S = HMAC-SHA256(macKey, CanonicalRequest(...)).
func SignRequest(req *GatewayRequest, macKey []byte) {
	req.S = HMACSHA256(macKey, CanonicalRequest(req.R, req.D, req.N, req.T, req.P, req.U))
}

// NewNonce returns 32 random bytes for GatewayRequest.n（GRPC_GATEWAY_PROTOCOL §3，与 Plan2 noc 长度一致）。
func NewNonce() []byte {
	return utils.GetRandomSecure(32)
}
