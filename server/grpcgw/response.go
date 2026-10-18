package grpcgw

import (
	DIC "github.com/godaddy-x/freego/core/const"
	"github.com/godaddy-x/freego/core/ex"
	utils "github.com/godaddy-x/freego/core/str"
	"github.com/godaddy-x/freego/infra/zlog"
	pb "github.com/godaddy-x/freego/protocol/grpcgw"
)

// signedError 入站失败尚无会话密钥时，用 broadcast_key 签 s，避免裸错误包被伪造。
func (g *GrpcGateway) signedError(route string, code int32, msg string, u int64) *pb.GatewayResponse {
	return g.signedErrorForReq(nil, route, code, msg, u)
}

func (g *GrpcGateway) signedErrorForReq(req *pb.GatewayRequest, route string, code int32, msg string, u int64) *pb.GatewayResponse {
	n := pb.NewNonce()
	if req != nil {
		if r := req.R; route == "" {
			route = r
		}
		if len(req.N) > 0 {
			n = req.N
		}
		if u == 0 && req.U != 0 {
			u = req.U
		}
	}
	t := utils.UnixSecond()
	const p int64 = 0
	canonical := pb.CanonicalResponse(route, nil, n, t, p, u, code, msg)
	return &pb.GatewayResponse{
		R: route,
		C: code,
		M: msg,
		T: t,
		N: n,
		P: p,
		S: pb.HMACSHA256(utils.Str2Bytes(g.broadcastKey), canonical),
	}
}

func clientSafeMessage(code int32) string {
	switch {
	case code == 401:
		return "unauthorized"
	case code == 403:
		return "forbidden"
	case code == 404:
		return "not found"
	case code == 429:
		return "too many requests"
	case code >= 500:
		return "internal error"
	default:
		return "bad request"
	}
}

func (c *Context) renderError(err error) (*pb.GatewayResponse, error) {
	out := ex.Normalize(ex.Catch(err))
	if c.gw.errorHandle != nil {
		if hookErr := c.gw.errorHandle(c, out); hookErr != nil {
			zlog.Error("grpcgw error handle failed", 0, zlog.String("route", c.req.R), zlog.AddError(hookErr))
		}
	}
	code := int32(out.Code)
	msg := out.Msg
	if msg == "" {
		if code == 0 {
			code = 400
		}
		msg = clientSafeMessage(code)
	}
	if out.Msg != "" || out.Err != nil {
		zlog.Info("grpcgw handler error", 0, zlog.String("route", c.req.R), zlog.Int("code", int(code)), zlog.String("err", out.Msg))
	}
	return c.buildSignedResponse(code, msg, nil)
}

func (c *Context) renderSuccess() (*pb.GatewayResponse, error) {
	return c.buildSignedResponse(200, "ok", c.plain)
}

func (c *Context) buildSignedResponse(code int32, msg string, plain []byte) (*pb.GatewayResponse, error) {
	req := c.req
	n := req.N
	if len(n) == 0 {
		n = pb.NewNonce()
	}
	t := utils.UnixSecond()
	p := req.P
	u := req.U

	var wireD []byte
	var macKey []byte
	var needOuter bool

	if c.cfg.KeyRoute {
		p = 2
		needOuter = true
		macKey = nil
		wireD = plain
	} else if c.cfg.Guest && !c.cfg.UsePlan2 {
		p = 0
		macKey = utils.Str2Bytes(c.gw.broadcastKey)
		wireD = plain
	} else if c.cfg.UsePlan2 {
		p = 2
		needOuter = true
		sk := c.getStorage(storageSharedKey)
		if sk == nil {
			return nil, ex.Throw{Code: 500, Msg: "shared key missing"}
		}
		key, ok := sk.([]byte)
		if !ok || len(key) < 32 {
			return nil, ex.Throw{Code: 500, Msg: "shared key invalid"}
		}
		defer DIC.ClearData(key)
		aad := pb.AADResponse(req.R, n, t, p, u, code, msg)
		if len(plain) > 0 {
			enc, err := utils.AesGCMEncryptBaseByteResult(plain, key[:32], aad)
			if err != nil {
				return nil, err
			}
			wireD = enc
		}
		macKey = key[:32]
	} else {
		auth := c.authTokenBytes()
		secret := c.subject.GetTokenSecret(utils.Bytes2Str(auth), c.gw.jwtConfig.TokenKey)
		macKey = secret[:32]
		if p == 1 || c.cfg.AesResponse {
			p = 1
			aad := pb.AADResponse(req.R, n, t, p, u, code, msg)
			enc, err := utils.AesGCMEncryptBaseByteResult(plain, macKey, aad)
			if err != nil {
				return nil, err
			}
			wireD = enc
		} else {
			wireD = plain
		}
	}

	canonical := pb.CanonicalResponse(req.R, wireD, n, t, p, u, code, msg)
	s := pb.HMACSHA256(macKey, canonical)

	resp := &pb.GatewayResponse{
		R: req.R,
		C: code,
		M: msg,
		D: wireD,
		N: n,
		S: s,
		T: t,
		P: p,
	}

	if needOuter {
		cipher, err := c.plan2Cipher()
		if err != nil {
			return nil, err
		}
		digest := pb.SHA256Sum(canonical)
		sig, err := cipher.Sign(digest)
		if err != nil {
			return nil, err
		}
		resp.E = sig
	}
	return resp, nil
}
