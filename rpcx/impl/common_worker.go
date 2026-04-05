package impl

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	DIC "github.com/godaddy-x/freego/common"

	"github.com/godaddy-x/freego/cache"
	"github.com/godaddy-x/freego/utils"
	fgocrypto "github.com/godaddy-x/freego/utils/crypto"
	"github.com/godaddy-x/freego/utils/jwt"

	"github.com/godaddy-x/freego/rpcx/pb"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"google.golang.org/grpc/codes"
)

// ConfigProvider 配置提供者接口
type ConfigProvider interface {
	GetCipher() map[int64]fgocrypto.Cipher
	GetLocalCache() cache.Cache
	GetRedisCache() cache.Cache
}

type CommonWorker struct {
	pb.UnimplementedCommonWorkerServer
	ConfigProvider ConfigProvider // 配置提供者接口
}

func (self *CommonWorker) GetCipher() map[int64]fgocrypto.Cipher {
	if self.ConfigProvider != nil {
		return self.ConfigProvider.GetCipher()
	}
	return nil
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

	if err := UnpackAny(req.D, bizReq); err != nil {
		return buildErrorResponse(req, codes.InvalidArgument, fmt.Sprintf("unpack business request failed: %v", err)), nil
	}

	bizResp, err := handler(ctx, bizReq)
	if err != nil {
		return buildErrorResponse(req, codes.Internal, fmt.Sprintf("business handle failed: %v", err)), nil
	}

	return self.buildSuccessResponse(cipher, req, bizResp)
}

// rpcxEd25519Cipher RPCX 当前仅支持明文 P=0，Cipher 须为 *crypto.Ed25519Object（CreateEd25519WithBase64）。
func rpcxEd25519Cipher(c fgocrypto.Cipher) (*fgocrypto.Ed25519Object, error) {
	ed, ok := c.(*fgocrypto.Ed25519Object)
	if !ok || ed == nil {
		return nil, errors.New("RPCX: cipher must be *crypto.Ed25519Object (CreateEd25519WithBase64)")
	}
	return ed, nil
}

// RPCXRequestDigestS 请求体摘要：SHA256( r|base64(d)|base64(n)|t|p|u )，与 s 字段 32 字节对应。
func RPCXRequestDigestS(d, n []byte, t, p int64, r string, u int64) []byte {
	msg := utils.AddStr(r, DIC.SEP, utils.Base64Encode(d), DIC.SEP,
		utils.Base64Encode(n), DIC.SEP, t, DIC.SEP, p, DIC.SEP, u)
	return utils.SHA256_BASE(utils.Str2Bytes(msg))
}

// RPCXResponseDigestS 响应体摘要：SHA256( r|base64(d)|base64(n)|t|p|u|c|m )；签名 s 后再 e=Ed25519(s)。
func RPCXResponseDigestS(d, n []byte, t, p int64, r string, u, c int64, m string) []byte {
	msg := utils.AddStr(r, DIC.SEP, utils.Base64Encode(d), DIC.SEP,
		utils.Base64Encode(n), DIC.SEP, t, DIC.SEP, p, DIC.SEP, u, DIC.SEP, c, DIC.SEP, m)
	return utils.SHA256_BASE(utils.Str2Bytes(msg))
}

// validRequest：仅 P=0；s 为 32 字节 SHA256，e 为 64 字节 Ed25519(s)；校验摘要与对方对 s 的签名。
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
		return nil, errors.New("RPCX 暂仅支持明文 P=0")
	}
	if len(req.S) != 32 {
		return nil, errors.New("request s must be 32-byte SHA256 digest")
	}
	if len(req.E) != 64 {
		return nil, errors.New("request e must be 64-byte Ed25519 signature")
	}

	cipher, exists := self.GetCipher()[req.U]
	if !exists || cipher == nil {
		return nil, errors.New("request cipher not found")
	}

	ed, err := rpcxEd25519Cipher(cipher)
	if err != nil {
		return nil, err
	}

	sWant := RPCXRequestDigestS(req.D.Value, req.N, req.T, req.P, req.R, req.U)
	if !bytes.Equal(sWant, req.S) {
		return nil, errors.New("request digest s invalid")
	}
	if err := ed.Verify(req.S, req.E); err != nil {
		return nil, errors.New("request ed25519 signature invalid")
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

func UnpackAny(anyData *anypb.Any, target proto.Message) error {
	if anyData == nil || target == nil {
		return errors.New("any data or target is nil")
	}
	return anyData.UnmarshalTo(target)
}

func PackAny(data proto.Message) (*anypb.Any, error) {
	return anypb.New(data)
}

func (self *CommonWorker) buildSuccessResponse(cipher fgocrypto.Cipher, req *pb.CommonRequest, bizResp proto.Message) (*pb.CommonResponse, error) {
	ed, err := rpcxEd25519Cipher(cipher)
	if err != nil {
		return nil, err
	}

	anyResp, err := PackAny(bizResp)
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

	s := RPCXResponseDigestS(res.D.Value, res.N, res.T, res.P, res.R, req.U, res.C, res.M)
	eBytes, err := ed.Sign(s)
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
