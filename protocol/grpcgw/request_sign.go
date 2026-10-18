package grpcgw

import (
	utils "github.com/godaddy-x/freego/core/str"
)

// SignRequest sets req.S = HMAC-SHA256(macKey, CanonicalRequest(...)).
func SignRequest(req *GatewayRequest, macKey []byte) {
	req.S = HMACSHA256(macKey, CanonicalRequest(req.R, req.D, req.N, req.T, req.P, req.U))
}

// NewNonce returns 16 random bytes for GatewayRequest.n (wire bytes, not base64).
func NewNonce() []byte {
	return utils.GetRandomSecure(16)
}
