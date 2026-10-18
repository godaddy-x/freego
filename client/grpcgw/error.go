package grpcgw

import (
	pb "github.com/godaddy-x/freego/protocol/grpcgw"
	"github.com/godaddy-x/freego/core/ex"
)

// gatewayResponseThrow 与 client/http 在 JsonResp.Code != 200 时的语义一致。
func gatewayResponseThrow(res *pb.GatewayResponse) error {
	if res == nil {
		return ex.Throw{Msg: "grpcgw: empty response"}
	}
	if res.C == 200 {
		return nil
	}
	if res.C > 0 {
		return ex.Throw{Code: int(res.C), Msg: res.M}
	}
	if res.M != "" {
		return ex.Throw{Msg: res.M}
	}
	return ex.Throw{Msg: "grpcgw: request failed"}
}
