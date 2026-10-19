package grpcgw

import (
	"github.com/godaddy-x/freego/core/ex"
	"github.com/godaddy-x/freego/core/jwt"
	utils "github.com/godaddy-x/freego/core/str"
	pb "github.com/godaddy-x/freego/protocol/grpcgw"
)

func (c *Context) validatePlan01() error {
	req := c.req
	if err := c.validateWireEnvelope(); err != nil {
		return err
	}
	if !utils.CheckInt64(req.P, 0, 1) {
		return ex.Throw{Code: 400, Msg: "plan must be 0 or 1"}
	}
	if len(req.E) != 0 {
		return ex.Throw{Code: 400, Msg: "e must be empty for plan0/1"}
	}
	if c.cfg.AesRequest && req.P != 1 {
		return ex.Throw{Code: 400, Msg: "route requires encrypted request p=1"}
	}

	auth := c.authTokenBytes()
	if len(auth) == 0 || len(auth) > pb.MaxMetadataAuthorizationLen {
		return ex.Throw{Code: 401, Msg: "authorization metadata required"}
	}
	sub := &jwt.Subject{}
	if err := sub.Verify(auth, c.gw.jwtConfig.TokenKey); err != nil {
		return ex.Throw{Code: 401, Msg: "token invalid", Err: err}
	}
	c.subject = sub
	if sub.Payload == nil {
		return ex.Throw{Code: 401, Msg: "token invalid"}
	}
	// Unary / StreamBind：req.U = PQC 证书编号（clientNo），参与 MAC；Connect hub 键为 JWT sub（见 serveConnect）。
	secret := sub.GetTokenSecret(utils.Bytes2Str(auth), c.gw.jwtConfig.TokenKey)
	if len(secret) < 32 {
		return ex.Throw{Code: 401, Msg: "token secret too short"}
	}
	macKey := secret[:32]

	if err := c.verifyRequestHMAC(macKey); err != nil {
		return err
	}
	return c.openRequestBody(macKey)
}
