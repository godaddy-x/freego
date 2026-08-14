// Package impl 实现 gRPC CommonWorker：P=0 明文；s=SHA256(规范字段)；e=ML-DSA.Sign(SHA256(规范字段))；Cipher 为 *crypto.MLDSA87Object。
package impl

import (
	"context"
	"errors"
	"fmt"

	fgocrypto "github.com/godaddy-x/freego/core/crypto"
	"github.com/godaddy-x/freego/core/jwt"
	utils "github.com/godaddy-x/freego/core/str"
	cache "github.com/godaddy-x/freego/infra/cache/contract"

	pb "github.com/godaddy-x/freego/protocol/rpcpb"
	"google.golang.org/protobuf/proto"

	"google.golang.org/grpc/codes"
)

// ConfigProvider 配置提供者接口
type ConfigProvider interface {
	GetCipherHook() CipherHook
	GetLocalCache() cache.Cache
	GetRedisCache() cache.Cache
}

type CommonWorker struct {
	pb.UnimplementedCommonWorkerServer
	ConfigProvider ConfigProvider // 配置提供者接口
}

func (self *CommonWorker) resolveCipher(usr int64) (fgocrypto.Cipher, error) {
	if self.ConfigProvider == nil {
		return nil, errors.New("config provider is nil")
	}
	hook := self.ConfigProvider.GetCipherHook()
	if hook == nil {
		return nil, errors.New("cipher hook not configured")
	}
	cipher, err := hook(usr)
	if err != nil {
		return nil, err
	}
	if cipher == nil {
		return nil, errors.New("cipher hook returned nil")
	}
	return cipher, nil
}

func (self *CommonWorker) GetLocalCache() cache.Cache {
	if self.ConfigProvider != nil {
		return self.ConfigProvider.GetLocalCache()
	}
	return nil
}

func (self *CommonWorker) GetRedisCache() cache.Cache {
	if self.ConfigProvider != nil {
		return self.ConfigProvider.GetRedisCache()
	}
	return nil
}

func (self *CommonWorker) Do(ctx context.Context, req *pb.CommonRequest) (*pb.CommonResponse, error) {

	cipher, err := self.validRequest(req)
	if err != nil {
		return buildErrorResponse(req, codes.InvalidArgument, err.Error()), nil
	}

	handler, constructor := GetHandler(req.R)
	if handler == nil {
		return buildErrorResponse(req, codes.NotFound, fmt.Sprintf("route (r) not found: %s", req.R)), nil
	}
	if constructor == nil {
		return buildErrorResponse(req, codes.NotFound, fmt.Sprintf("route (r) constructor not found: %s", req.R)), nil
	}

	bizReq := constructor()
	if bizReq == nil {
		return buildErrorResponse(req, codes.NotFound, fmt.Sprintf("route (r) constructor returns nil request object: %s", req.R)), nil
	}

	if err := pb.UnpackAny(req.D, bizReq); err != nil {
		return buildErrorResponse(req, codes.InvalidArgument, fmt.Sprintf("unpack business request failed: %v", err)), nil
	}

	bizResp, err := handler(ctx, bizReq)
	if err != nil {
		return buildErrorResponse(req, codes.Internal, fmt.Sprintf("business handle failed: %v", err)), nil
	}

	return self.buildSuccessResponse(cipher, req, bizResp)
}

// rpcxMLDSACipher RPCX 当前仅支持明文 P=0，Cipher 须为 *crypto.MLDSA87Object（CreateMLDSA87WithBase64）。
func rpcxMLDSACipher(c fgocrypto.Cipher) (*fgocrypto.MLDSA87Object, error) {
	ed, ok := c.(*fgocrypto.MLDSA87Object)
	if !ok || ed == nil {
		return nil, errors.New("RPCX: cipher must be *crypto.MLDSA87Object (CreateMLDSA87WithBase64)")
	}
	return ed, nil
}

func (self *CommonWorker) validRequest(req *pb.CommonRequest) (fgocrypto.Cipher, error) {
	if len(req.R) == 0 {
		return nil, errors.New("request router is nil")
	}
	if req.D == nil {
		return nil, errors.New("request data is nil")
	}
	if req.T <= 0 {
		return nil, errors.New("request time must be > 0")
	}
	if utils.MathAbs(utils.UnixSecond()-req.T) > jwt.FIVE_MINUTES {
		return nil, errors.New("request time invalid")
	}
	if !utils.CheckLen(req.N, 16, 64) {
		return nil, errors.New("request nonce invalid")
	}
	if req.P != 0 {
		return nil, errors.New("RPCX only supports plaintext P=0")
	}
	if len(req.S) != 32 {
		return nil, errors.New("request s must be 32-byte SHA256 digest")
	}
	if !fgocrypto.CheckRPCXSignatureValid(req.E) {
		return nil, errors.New("request e must be ML-DSA-87 signature")
	}

	cipher, err := self.resolveCipher(req.U)
	if err != nil {
		return nil, errors.New("request cipher not found: " + err.Error())
	}

	ml, err := rpcxMLDSACipher(cipher)
	if err != nil {
		return nil, err
	}

	sWant := pb.RPCXRequestDigestS(req.D.Value, req.N, req.T, req.P, req.R, req.U)
	if !utils.CompareSign(sWant, req.S) {
		return nil, errors.New("request digest s invalid")
	}
	if err := ml.Verify(req.S, req.E); err != nil {
		return nil, errors.New("request ML-DSA signature invalid")
	}

	c := self.GetLocalCache()
	if c == nil {
		return nil, errors.New("cache object is nil")
	}
	validKey := utils.FNV1a64Base(req.S)
	exists2, err := c.Exists(validKey)
	if err == nil && exists2 {
		return nil, errors.New("request signature already used (replay attack detected)")
	}
	_ = c.Put(validKey, true, 600)

	return cipher, nil
}

func (self *CommonWorker) buildSuccessResponse(cipher fgocrypto.Cipher, req *pb.CommonRequest, bizResp proto.Message) (*pb.CommonResponse, error) {
	ml, err := rpcxMLDSACipher(cipher)
	if err != nil {
		return nil, err
	}

	anyResp, err := pb.PackAny(bizResp)
	if err != nil {
		return buildErrorResponse(req, codes.Internal, fmt.Sprintf("pack business response failed: %v", err)), nil
	}

	res := &pb.CommonResponse{
		D: anyResp,
		N: utils.GetRandomSecure(32),
		T: utils.UnixSecond(),
		R: req.R,
		P: 0,
		C: 200,
		M: "",
	}

	s := pb.RPCXResponseDigestS(res.D.Value, res.N, res.T, res.P, res.R, req.U, res.C, res.M)
	eBytes, err := ml.Sign(s)
	if err != nil {
		return nil, err
	}
	res.S = s
	res.E = eBytes
	return res, nil
}

func buildErrorResponse(req *pb.CommonRequest, code codes.Code, msg string) *pb.CommonResponse {
	return &pb.CommonResponse{
		D: nil,
		N: utils.GetRandomSecure(32),
		S: nil,
		T: utils.UnixSecond(),
		E: nil,
		R: req.R,
		P: 0,
		C: int64(code),
		M: msg,
	}
}
