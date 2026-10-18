package grpcgw

import (
	"github.com/godaddy-x/freego/core/ex"
	utils "github.com/godaddy-x/freego/core/str"
)

// validateGuestInbound: Plan0 + broadcast_key HMAC (GRPC_GATEWAY_PROTOCOL.md §3 mac_key table).
func (c *Context) validateGuestInbound() error {
	req := c.req
	if err := c.validateWireEnvelope(); err != nil {
		return err
	}
	if req.P != 0 {
		return ex.Throw{Code: 400, Msg: "guest route requires p=0"}
	}
	if req.U != 0 {
		return ex.Throw{Code: 400, Msg: "guest route requires u=0"}
	}
	if len(req.E) != 0 {
		return ex.Throw{Code: 400, Msg: "e must be empty for p=0"}
	}
	macKey := utils.Str2Bytes(c.gw.broadcastKey)
	if err := c.verifyRequestHMAC(macKey); err != nil {
		return err
	}
	c.plain = req.D
	return nil
}
