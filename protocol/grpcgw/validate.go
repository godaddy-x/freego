package grpcgw

import (
	"errors"
	"unicode/utf8"
)

// MaxWireBlob default upper bound for GatewayRequest.d (align httpsvr.MAX_BODY_LEN).
const DefaultMaxWireBlob = 200000

// MaxMetadataAuthorizationLen JWT authorization metadata 上限（字节）。
const MaxMetadataAuthorizationLen = 8192

// MaxMetadataDeviceLen Connect device metadata 上限（字节）。
const MaxMetadataDeviceLen = 256

// ValidateRequestFields enforces GRPC_GATEWAY_PROTOCOL.md §3 constraints before MAC/AES work.
func ValidateRequestFields(r string, d, n []byte, t, p, u int64, maxWireBlob int) error {
	if maxWireBlob <= 0 {
		maxWireBlob = DefaultMaxWireBlob
	}
	if r == "" {
		return errors.New("r is empty")
	}
	if len(r) > 256 {
		return errors.New("r too long")
	}
	if !isASCII(r) {
		return errors.New("r must be ASCII")
	}
	if len(d) == 0 && r != StreamBindRoute {
		return errors.New("d is empty")
	}
	if len(d) > maxWireBlob {
		return errors.New("d too long")
	}
	if len(n) < 32 || len(n) > 64 {
		return errors.New("n length invalid")
	}
	if t <= 0 {
		return errors.New("t invalid")
	}
	if p != 0 && p != 1 && p != 2 {
		return errors.New("p invalid")
	}
	if u < 0 {
		return errors.New("u invalid")
	}
	return nil
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}
