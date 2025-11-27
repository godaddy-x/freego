package impl

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"strconv"

	ecc "github.com/godaddy-x/eccrypto"
	"github.com/godaddy-x/freego/cache"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/utils/crypto"
	"github.com/godaddy-x/freego/utils/jwt"

	"github.com/godaddy-x/freego/rpcx/pb"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"google.golang.org/grpc/codes"
)

// ConfigProvider 配置提供者接口
type ConfigProvider interface {
	GetRSA() []crypto.Cipher
	GetLocalCache() cache.Cache
	GetRedisCache() cache.Cache
}

type CommonWorker struct {
	pb.UnimplementedCommonWorkerServer
	ConfigProvider ConfigProvider // 配置提供者接口
}

// GetRSA 获取RSA密钥列表
func (self *CommonWorker) GetRSA() []crypto.Cipher {
	if self.ConfigProvider != nil {
		return self.ConfigProvider.GetRSA()
	}
	return nil
}

// GetLocalCache 获取本地缓存
func (self *CommonWorker) GetLocalCache() cache.Cache {
	if self.ConfigProvider != nil {
		return self.ConfigProvider.GetLocalCache()
	}
	return nil
}

// GetRedisCache 获取Redis缓存
func (self *CommonWorker) GetRedisCache() cache.Cache {
	if self.ConfigProvider != nil {
		return self.ConfigProvider.GetRedisCache()
	}
	return nil
}

// Do 所有业务请求的统一入口
func (self *CommonWorker) Do(ctx context.Context, req *pb.CommonRequest) (*pb.CommonResponse, error) {

	// 1. 验证请求数据
	cipher, err := self.validRequest(req)
	if err != nil {
		return buildErrorResponse(req, codes.InvalidArgument, err.Error()), nil
	}

	// 3. 根据路由获取业务处理器
	handler := GetHandler(req.R)
	if handler == nil {
		return buildErrorResponse(req, codes.NotFound, fmt.Sprintf("route (r) not found: %s", req.R)), nil
	}

	// 4. 解包 Any 字段到业务请求
	bizReq := handler.RequestType() // 获取业务请求类型（如 &userpb.UserGetRequest{}）
	if err := UnpackAny(req.D, bizReq); err != nil {
		return buildErrorResponse(req, codes.InvalidArgument, fmt.Sprintf("unpack business request failed: %v", err)), nil
	}

	// 4. 调用业务处理器逻辑
	bizResp, err := handler.Handle(ctx, bizReq)
	if err != nil {
		return buildErrorResponse(req, codes.Internal, fmt.Sprintf("business handle failed: %v", err)), nil
	}

	// 5. 包装业务响应为 CommonResponse
	return self.buildSuccessResponse(cipher, req, bizResp)
}

// -------------------------- 验证数据 --------------------------

func (self *CommonWorker) validRequest(req *pb.CommonRequest) (crypto.Cipher, error) {
	// 1. 校验基础参数是否有效
	if len(req.R) == 0 {
		return nil, errors.New("request router is nil")
	}
	if req.D == nil {
		return nil, errors.New("request data is nil")
	}
	if req.T <= 0 {
		return nil, errors.New("request time must be > 0")
	}
	if utils.MathAbs(utils.UnixSecond()-req.T) > jwt.FIVE_MINUTES { // 判断绝对时间差超过5分钟
		return nil, errors.New("request time invalid")
	}
	if !utils.CheckLen(req.N, 8, 32) {
		return nil, errors.New("request nonce invalid")
	}
	if !utils.CheckInt64(req.P, 0, 1) {
		return nil, errors.New("request plan invalid")
	}
	if !utils.CheckRangeInt(len(req.S), 32, 64) {
		return nil, errors.New("request signature length invalid")
	}
	if !utils.CheckRangeInt(len(req.E), 64, 96) {
		return nil, errors.New("request ecdsa signature length invalid")
	}

	// 1.遍历rsa列表验证s
	var cipher crypto.Cipher
	for _, rsa := range self.GetRSA() {
		if err := rsa.Verify(req.S, req.E); err == nil {
			cipher = rsa // 校验成功后保存校验器
			break
		}
	}
	if cipher == nil {
		return nil, errors.New("request ecdsa signature check failed")
	}

	c := self.GetLocalCache()
	if c == nil {
		return nil, errors.New("cache object is nil")
	}

	sig, err := Signature(c, cipher, req.D.Value, req.N, req.T, req.P, req.R)
	if err != nil {
		return nil, err
	}

	if !bytes.Equal(sig, req.S) {
		return nil, errors.New("request body signature invalid")
	}

	return cipher, nil
}

// -------------------------- 响应构建工具 --------------------------

// UnpackAny 从 Any 解包到目标类型
func UnpackAny(anyData *anypb.Any, target proto.Message) error {
	if anyData == nil || target == nil {
		return errors.New("any data or target is nil")
	}
	return anyData.UnmarshalTo(target)
}

// PackAny 将业务数据包装为 Any
func PackAny(data proto.Message) (*anypb.Any, error) {
	return anypb.New(data)
}

// buildSuccessResponse 构建成功响应
func (self *CommonWorker) buildSuccessResponse(cipher crypto.Cipher, req *pb.CommonRequest, bizResp proto.Message) (*pb.CommonResponse, error) {
	c := self.GetLocalCache()
	if c == nil {
		return nil, errors.New("cache object is nil")
	}
	// 包装业务响应到 Any
	anyResp, err := PackAny(bizResp)
	if err != nil {
		return buildErrorResponse(req, codes.Internal, fmt.Sprintf("pack business response failed: %v", err)), nil
	}

	// 构建响应（复用请求的 r、p 字段，保持一致性）
	res := &pb.CommonResponse{
		D: anyResp,
		N: utils.GetRandomSecure(32),
		T: utils.UnixSecond(),
		R: req.R,
		P: req.P,
		C: 200, // 200 = 成功
		M: "",
	}

	sig, err := Signature(c, cipher, res.D.Value, res.N, res.T, res.P, res.R)
	if err != nil {
		return nil, err
	}

	res.S = sig
	res.E, err = cipher.Sign(res.S)
	if err != nil {
		return buildErrorResponse(req, codes.Internal, fmt.Sprintf("pack business response sign failed: %v", err)), nil
	}
	return res, nil
}

func Signature(c cache.Cache, cipher crypto.Cipher, d, n []byte, t, p int64, r string) ([]byte, error) {
	// 获取协商密钥
	prk, _ := cipher.GetPrivateKey()
	pub, _ := cipher.GetPublicKey()
	pubBs, err := ecc.GetECDSAPublicKeyBytes(pub.(*ecdsa.PublicKey))
	if err != nil {
		return nil, errors.New("pub bytes invalid")
	}
	cacheKey := utils.FNV1a64Base(pubBs)
	sharedKey, err := c.GetBytes(cacheKey)
	if err != nil {
		return nil, errors.New("load cache pub bytes error")
	}
	if len(sharedKey) == 0 { // 如果没有找到缓存则重新处理协商
		sharedKey, err = ecc.GenSharedKeyECDSA(prk.(*ecdsa.PrivateKey), pub.(*ecdsa.PublicKey))
		if err != nil {
			return nil, errors.New("cipher shared key error：" + err.Error())
		}
		_ = c.Put(cacheKey, sharedKey)
	}

	// 3.拼接数据载体进行验证S（优化：预分配内存，避免多次append）
	tBytes := []byte(strconv.FormatInt(t, 10))
	pBytes := []byte(strconv.FormatInt(p, 10))
	rBytes := utils.Str2Bytes(r)

	// 预计算总长度并一次性分配
	totalLen := len(d) + len(n) + len(tBytes) + len(pBytes) + len(rBytes)
	body := make([]byte, totalLen)

	// 使用copy依次填充，避免内存重新分配
	offset := 0
	offset += copy(body[offset:], d)
	offset += copy(body[offset:], n)
	offset += copy(body[offset:], tBytes)
	offset += copy(body[offset:], pBytes)
	copy(body[offset:], rBytes)

	return utils.HMAC_SHA256_BASE(body, sharedKey), nil
}

// buildErrorResponse 构建错误响应
func buildErrorResponse(req *pb.CommonRequest, code codes.Code, msg string) *pb.CommonResponse {
	// 错误响应的 s 和 e 字段可简化生成（或按规则生成，客户端按需验证）
	return &pb.CommonResponse{
		D: nil,
		N: utils.GetRandomSecure(32),
		S: nil,
		T: utils.UnixSecond(),
		E: nil,
		R: req.R,
		P: req.P,
		C: int64(code), // 响应代码 = gRPC 错误码
		M: msg,         // 业务消息 = 错误描述
	}
}
