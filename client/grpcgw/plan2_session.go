package grpcgw

import (
	DIC "github.com/godaddy-x/freego/core/const"
	"github.com/godaddy-x/freego/core/crypto"
	utils "github.com/godaddy-x/freego/core/str"
	pb "github.com/godaddy-x/freego/protocol/grpcgw"
	fwsign "github.com/godaddy-x/freego/protocol/sign"
)

func plan2Unary(cli *Client, req *pb.GatewayRequest, macKey []byte, cipher crypto.Cipher, usr int64, timeoutSec int64) (*pb.GatewayResponse, error) {
	pb.SignRequest(req, macKey)
	if err := SignOuter(req, cipher); err != nil {
		return nil, err
	}
	sess := cli.CopySession()
	ctx, cancel := cli.callCtxFor(timeoutSec)
	defer cancel()
	res, err := cli.CallSession(ctx, req, sess)
	if err != nil {
		return nil, err
	}
	if err := verifyPlan2Response(req, res, macKey, cipher, usr); err != nil {
		return nil, err
	}
	return res, nil
}

func Plan2KeyExchange(cli *Client, usr int64, cipher crypto.Cipher, keyRoute string, boot *pb.Plan2PublicKey, timeoutSec int64) ([]byte, error) {
	body, err := pb.MarshalPlan2PublicKey(boot)
	if err != nil {
		return nil, err
	}
	req := &pb.GatewayRequest{
		R: keyRoute, D: body, N: pb.NewNonce(), T: utils.UnixSecond(), P: 2, U: usr,
	}
	res, err := plan2Unary(cli, req, nil, cipher, usr, timeoutSec)
	if err != nil {
		return nil, err
	}
	serverPK, err := pb.UnmarshalPlan2PublicKey(res.D)
	if err != nil {
		return nil, err
	}
	if err := pb.CheckPlan2PublicKey(nil, serverPK, cipher, usr, false); err != nil {
		return nil, err
	}
	if len(serverPK.Key) != crypto.MLKEM1024EncapsulationKeySize {
		return nil, utils.Error("server encapsulation key invalid")
	}
	shared, kemCt, err := crypto.EncapsulateToPeerBytes(serverPK.Key)
	if err != nil {
		return nil, err
	}
	defer DIC.ClearData(shared)
	sharedKey, err := fwsign.HKDFKeyBytes(shared, serverPK.Noc)
	if err != nil {
		return nil, err
	}
	authObj, err := pb.CreatePlan2PublicKey(serverPK.Key, kemCt, usr, cipher)
	if err != nil {
		return nil, err
	}
	authWire, err := pb.MarshalPlan2PublicKey(authObj)
	if err != nil {
		return nil, err
	}
	cli.SetPlan2Authorization(authWire)
	return sharedKey, nil
}

func Plan2Call(cli *Client, usr int64, cipher crypto.Cipher, route string, sharedKey []byte, plain []byte, timeoutSec int64) ([]byte, error) {
	req := &pb.GatewayRequest{
		R: route, N: pb.NewNonce(), T: utils.UnixSecond(), P: 2, U: usr,
	}
	aad := pb.AADRequest(req.R, req.N, req.T, req.P, req.U)
	d, err := utils.AesGCMEncryptBaseByteResult(plain, sharedKey[:32], aad)
	if err != nil {
		return nil, err
	}
	req.D = d
	res, err := plan2Unary(cli, req, sharedKey[:32], cipher, usr, timeoutSec)
	if err != nil {
		return nil, err
	}
	aadRes := pb.AADResponse(res.R, res.N, res.T, res.P, usr, res.C, res.M)
	return utils.AesGCMDecryptBaseByteResult(res.D, sharedKey[:32], aadRes)
}

func Plan2Login(cli *Client, usr int64, cipher crypto.Cipher, keyRoute, loginRoute string, loginPlain []byte, timeoutSec int64) ([]byte, error) {
	cli.SetClientNo(usr)
	boot, err := pb.CreatePlan2PublicKey(utils.GetRandomSecure(32), utils.GetRandomSecure(32), usr, cipher)
	if err != nil {
		return nil, err
	}
	sharedKey, err := Plan2KeyExchange(cli, usr, cipher, keyRoute, boot, timeoutSec)
	if err != nil {
		return nil, err
	}
	defer DIC.ClearData(sharedKey)
	return Plan2Call(cli, usr, cipher, loginRoute, sharedKey, loginPlain, timeoutSec)
}
