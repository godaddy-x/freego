package grpcgw

import (
	"testing"

	"github.com/godaddy-x/freego/core/ex"
	pb "github.com/godaddy-x/freego/protocol/grpcgw"
)

func TestGatewayResponseThrowMatchesHTTPSemantics(t *testing.T) {
	throw := ex.Catch(gatewayResponseThrow(&pb.GatewayResponse{C: int32(ex.BIZ), M: "test error"}))
	if throw.Code != ex.BIZ || throw.Msg != "test error" {
		t.Fatalf("biz code=%d msg=%q", throw.Code, throw.Msg)
	}
	throw = ex.Catch(gatewayResponseThrow(&pb.GatewayResponse{C: 401, M: "wrong password"}))
	if throw.Code != 401 || throw.Msg != "wrong password" {
		t.Fatalf("http code=%d msg=%q", throw.Code, throw.Msg)
	}
	if gatewayResponseThrow(&pb.GatewayResponse{C: 200, M: "ok"}) != nil {
		t.Fatal("200 must be nil")
	}
}
