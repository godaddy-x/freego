package grpcgw

import (
	"encoding/binary"
)

const (
	DomainRequest  = "GRPCGWv1"
	DomainResponse = "GRPCGWv2"
	DomainAADReq    = "GRPCGWa1"
	DomainAADRes    = "GRPCGWa2"
	DomainPlan2Sign = "GRPCGWpk1" // Plan2PublicKey ML-DSA 签名字符串域
	TransportVersion            = "grpcgw-v1"
	MetadataTransportVersionKey   = "x-grpcgw-transport-version"
	MetadataDeviceKey             = "x-grpcgw-device"
	MetadataAuthorizationKey      = "authorization"
	// Plan2 会话凭证：metadata 值为 proto.Marshal(Plan2PublicKey) 原始字节（grpc -bin 头由库做 HTTP/2 编码）。
	MetadataPlan2AuthorizationKey = "x-grpcgw-plan2-authorization-bin"
	PushCode                    = int32(300)
	StreamBindRoute             = "StreamBind"
)

func appendU32BE(b []byte, x uint32) []byte {
	var buf [4]byte
	binary.BigEndian.PutUint32(buf[:], x)
	return append(b, buf[:]...)
}

func appendI32BE(b []byte, x int32) []byte {
	return appendU32BE(b, uint32(x))
}

func appendI64BE(b []byte, x int64) []byte {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], uint64(x))
	return append(b, buf[:]...)
}

// CanonicalRequest builds the MAC input for GatewayRequest (excludes s, e).
func CanonicalRequest(r string, d, n []byte, t, p, u int64) []byte {
	rb := []byte(r)
	b := make([]byte, 0, 8+4+len(rb)+4+len(d)+4+len(n)+24)
	b = append(b, DomainRequest...)
	b = appendU32BE(b, uint32(len(rb)))
	b = append(b, rb...)
	b = appendU32BE(b, uint32(len(d)))
	b = append(b, d...)
	b = appendU32BE(b, uint32(len(n)))
	b = append(b, n...)
	b = appendI64BE(b, t)
	b = appendI64BE(b, p)
	b = appendI64BE(b, u)
	return b
}

// CanonicalResponse builds the MAC input for GatewayResponse; u comes from request context.
func CanonicalResponse(r string, d, n []byte, t, p, u int64, c int32, m string) []byte {
	rb := []byte(r)
	mb := []byte(m)
	b := make([]byte, 0, 8+4+len(rb)+4+len(d)+4+len(n)+32+4+len(mb))
	b = append(b, DomainResponse...)
	b = appendU32BE(b, uint32(len(rb)))
	b = append(b, rb...)
	b = appendU32BE(b, uint32(len(d)))
	b = append(b, d...)
	b = appendU32BE(b, uint32(len(n)))
	b = append(b, n...)
	b = appendI64BE(b, t)
	b = appendI64BE(b, p)
	b = appendI64BE(b, u)
	b = appendI32BE(b, c)
	b = appendU32BE(b, uint32(len(mb)))
	b = append(b, mb...)
	return b
}

// AADRequest is AES-GCM additional data for Plan1 (no ciphertext d).
func AADRequest(r string, n []byte, t, p, u int64) []byte {
	rb := []byte(r)
	b := make([]byte, 0, 8+4+len(rb)+4+len(n)+24)
	b = append(b, DomainAADReq...)
	b = appendU32BE(b, uint32(len(rb)))
	b = append(b, rb...)
	b = appendU32BE(b, uint32(len(n)))
	b = append(b, n...)
	b = appendI64BE(b, t)
	b = appendI64BE(b, p)
	b = appendI64BE(b, u)
	return b
}

// AADResponse is AES-GCM additional data for Plan1 response.
func AADResponse(r string, n []byte, t, p, u int64, c int32, m string) []byte {
	rb := []byte(r)
	mb := []byte(m)
	b := make([]byte, 0, 8+4+len(rb)+4+len(n)+32+4+len(mb))
	b = append(b, DomainAADRes...)
	b = appendU32BE(b, uint32(len(rb)))
	b = append(b, rb...)
	b = appendU32BE(b, uint32(len(n)))
	b = append(b, n...)
	b = appendI64BE(b, t)
	b = appendI64BE(b, p)
	b = appendI64BE(b, u)
	b = appendI32BE(b, c)
	b = appendU32BE(b, uint32(len(mb)))
	b = append(b, mb...)
	return b
}
