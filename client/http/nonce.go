package httpx

import (
	utils "github.com/godaddy-x/freego/core/str"
	"github.com/godaddy-x/freego/protocol/wire"
)

func assignProtocolNonce(body *wire.JsonBody) {
	if body == nil {
		return
	}
	body.Nonce = utils.RandProtocolNonce()
}
