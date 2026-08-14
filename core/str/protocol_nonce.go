package utils

// ProtocolNonceBytes HTTP/WebSocket/RPCX 协议随机数（n / Nonce）固定长度：32 字节。
const ProtocolNonceBytes = 32

// RandProtocolNonce 生成协议用 Nonce：Base64(32 字节 CSPRNG)。
func RandProtocolNonce() string {
	return Base64Encode(GetRandomSecure(ProtocolNonceBytes))
}

// ValidProtocolNonce 校验 Nonce 为 Base64 编码且解码后恰好 32 字节。
func ValidProtocolNonce(nonce string) bool {
	if len(nonce) == 0 {
		return false
	}
	b := Base64Decode(nonce)
	return len(b) == ProtocolNonceBytes
}

// ValidProtocolNonceBytes 校验原始字节 Nonce（如 RPCX CommonRequest.n）。
func ValidProtocolNonceBytes(n []byte) bool {
	return len(n) == ProtocolNonceBytes
}
