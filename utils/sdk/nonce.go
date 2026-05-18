package sdk

import (
	"github.com/godaddy-x/freego/node"
	"github.com/godaddy-x/freego/utils"
)

// assignProtocolNonce 为 JsonBody 设置协议要求的 32 字节随机 Nonce（Base64 编码字符串）。
func assignProtocolNonce(body *node.JsonBody) {
	if body == nil {
		return
	}
	body.Nonce = utils.RandProtocolNonce()
}

// ValidProtocolNonce 供测试与外部校验：Nonce 解码后须为 32 字节。
func ValidProtocolNonce(nonce string) bool {
	return utils.ValidProtocolNonce(nonce)
}
