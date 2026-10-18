package grpcgw

import (
	"github.com/godaddy-x/freego/core/ex"
	utils "github.com/godaddy-x/freego/core/str"
	pb "github.com/godaddy-x/freego/protocol/grpcgw"
	"google.golang.org/protobuf/proto"
)

func plan01Invoke(cli *Client, route string, plain []byte, plan int64, timeoutSec int64) ([]byte, error) {
	sess := cli.CopySession()
	if sess.User < 0 {
		return nil, ex.Throw{Msg: "client user id not set"}
	}
	macKey, err := sess.MACKey()
	if err != nil {
		return nil, err
	}
	req := &pb.GatewayRequest{
		R: route, N: pb.NewNonce(), T: utils.UnixSecond(), P: plan, U: sess.User,
	}
	if plan == 0 {
		if err := SealPlan0(req, plain, macKey); err != nil {
			return nil, err
		}
	} else {
		if err := SealPlan1(req, plain, macKey); err != nil {
			return nil, err
		}
	}
	ctx, cancel := cli.callCtxFor(timeoutSec)
	defer cancel()
	res, err := cli.CallSession(ctx, req, sess)
	if err != nil {
		return nil, err
	}
	return OpenPlan01Response(req, res, macKey)
}

func callPlanProto(cli *Client, route string, reqMsg, resMsg proto.Message, plan int64, timeoutSec int64) error {
	in, err := proto.Marshal(reqMsg)
	if err != nil {
		return err
	}
	out, err := plan01Invoke(cli, route, in, plan, timeoutSec)
	if err != nil {
		return err
	}
	return pb.UnmarshalProto(out, resMsg)
}

func CallPlan0Proto(cli *Client, route string, reqMsg, resMsg proto.Message, timeoutSec int64) error {
	return callPlanProto(cli, route, reqMsg, resMsg, 0, timeoutSec)
}

func CallPlan1Proto(cli *Client, route string, reqMsg, resMsg proto.Message, timeoutSec int64) error {
	return callPlanProto(cli, route, reqMsg, resMsg, 1, timeoutSec)
}

// InvokePlan0/1 明文业务 bytes（JSON 等），供 relay 等非 proto 调用方使用。
func InvokePlan0(cli *Client, route string, plain []byte, timeoutSec int64) ([]byte, error) {
	return plan01Invoke(cli, route, plain, 0, timeoutSec)
}

func InvokePlan1(cli *Client, route string, plain []byte, timeoutSec int64) ([]byte, error) {
	return plan01Invoke(cli, route, plain, 1, timeoutSec)
}
