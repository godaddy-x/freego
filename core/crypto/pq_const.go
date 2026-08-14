package crypto

import (
	"encoding/base64"

	fmldsa "filippo.io/mldsa"
)

const (
	// MLDSA87SignatureSize ML-DSA-87 裸签名长度（字节）。
	MLDSA87SignatureSize = fmldsa.MLDSA87SignatureSize
	// MLDSA87PublicKeySize ML-DSA-87 公钥编码长度（字节）。
	MLDSA87PublicKeySize = fmldsa.MLDSA87PublicKeySize
	// MLDSA87PrivateKeySeedSize ML-DSA-87 私钥种子长度（字节）。
	MLDSA87PrivateKeySeedSize = fmldsa.PrivateKeySize

	// MLKEM1024EncapsulationKeySize ML-KEM-1024 封装公钥长度（字节）。
	MLKEM1024EncapsulationKeySize = 1568
	// MLKEM1024CiphertextSize ML-KEM-1024 KEM 密文长度（字节）。
	MLKEM1024CiphertextSize = 1568
	// MLKEM1024DecapsulationKeySize ML-KEM-1024 解封装私钥种子长度（字节）。
	MLKEM1024DecapsulationKeySize = 64
)

// MLDSA87SignatureB64Len 标准 Base64 编码后的 ML-DSA-87 签名长度（无换行）。
func MLDSA87SignatureB64Len() int {
	return base64.StdEncoding.EncodedLen(MLDSA87SignatureSize)
}

// MLKEM1024EncapsulationKeyB64Len 封装公钥 Base64 长度。
func MLKEM1024EncapsulationKeyB64Len() int {
	return base64.StdEncoding.EncodedLen(MLKEM1024EncapsulationKeySize)
}

// MLKEM1024CiphertextB64Len KEM 密文 Base64 长度。
func MLKEM1024CiphertextB64Len() int {
	return base64.StdEncoding.EncodedLen(MLKEM1024CiphertextSize)
}

// MaxPublicKeyJSONLen Plan2 PublicKey JSON 最大长度（key + tag + sig + noc 等）。
func MaxPublicKeyJSONLen() int {
	const overhead = 512
	return overhead +
		MLKEM1024EncapsulationKeyB64Len() +
		MLKEM1024CiphertextB64Len() +
		MLDSA87SignatureB64Len()
}

// MaxPlan2AuthorizationB64Len Authorization 头中 base64(PublicKey JSON) 的最大长度。
func MaxPlan2AuthorizationB64Len() int {
	return base64.StdEncoding.EncodedLen(MaxPublicKeyJSONLen())
}

// MinFasthttpReadBufferSize fasthttp 单连接读缓冲下限（默认 4096 无法容纳 Plan2 Authorization）。
// 含 Authorization 与其它常规请求头余量。
func MinFasthttpReadBufferSize() int {
	const extraHeaders = 2048
	n := MaxPlan2AuthorizationB64Len() + extraHeaders
	if n < 8192 {
		return 8192
	}
	return n
}

// CheckRPCXSignatureValid 校验 RPCX protobuf 字段 e 的 ML-DSA-87 裸签名长度。
func CheckRPCXSignatureValid(sig []byte) bool {
	return len(sig) == MLDSA87SignatureSize
}

// CheckOuterSignatureB64Valid 校验 JsonBody.Valid / JsonResp.Valid 外层签名 Base64 长度（ML-DSA-87）。
func CheckOuterSignatureB64Valid(b64 string) bool {
	n := len(b64)
	want := MLDSA87SignatureB64Len()
	// 允许 ±4 字符容差（padding / 无 padding 差异）
	return n >= want-4 && n <= want+4
}
