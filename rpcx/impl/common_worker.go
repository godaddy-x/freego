package impl

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"

	DIC "github.com/godaddy-x/freego/common"

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
	GetCipher() map[int64]crypto.Cipher
	GetLocalCache() cache.Cache
	GetRedisCache() cache.Cache
}

type CommonWorker struct {
	pb.UnimplementedCommonWorkerServer
	ConfigProvider ConfigProvider // 配置提供者接口
}

// GetCipher 获取RSA/ECDSA密钥对列表，用于数字签名验证
// 返回: 配置的RSA密钥列表，如果ConfigProvider未设置则返回nil
func (self *CommonWorker) GetCipher() map[int64]crypto.Cipher {
	if self.ConfigProvider != nil {
		return self.ConfigProvider.GetCipher()
	}
	return nil
}

// GetLocalCache 获取本地缓存实例，用于存储共享密钥等临时数据
// 返回: 本地缓存实例，如果ConfigProvider未设置则返回nil
func (self *CommonWorker) GetLocalCache() cache.Cache {
	if self.ConfigProvider != nil {
		return self.ConfigProvider.GetLocalCache()
	}
	return nil
}

// GetRedisCache 获取Redis分布式缓存实例，用于跨服务数据共享
// 返回: Redis缓存实例，如果ConfigProvider未设置则返回nil
func (self *CommonWorker) GetRedisCache() cache.Cache {
	if self.ConfigProvider != nil {
		return self.ConfigProvider.GetRedisCache()
	}
	return nil
}

// Do 统一的RPC业务请求处理入口，实现完整的请求生命周期
// ctx: 请求上下文，包含超时、元数据等信息
// req: 通用请求对象，包含加密数据、签名、路由等信息
// 返回: 通用响应对象和可能的处理错误
func (self *CommonWorker) Do(ctx context.Context, req *pb.CommonRequest) (*pb.CommonResponse, error) {

	// 1. 验证请求数据
	cipher, key, err := self.validRequest(req)
	if err != nil {
		return buildErrorResponse(req, codes.InvalidArgument, err.Error()), nil
	}
	defer DIC.ClearData(key)

	// 3. 根据路由获取业务处理器  获取处理器 + 构造函数（核心改动）
	handler, constructor := GetHandler(req.R)
	if handler == nil {
		return buildErrorResponse(req, codes.NotFound, fmt.Sprintf("route (r) not found: %s", req.R)), nil
	}
	if constructor == nil {
		return buildErrorResponse(req, codes.NotFound, fmt.Sprintf("route (r) constructor not found: %s", req.R)), nil
	}

	bizReq := constructor() // 获取业务请求类型（如 &userpb.UserGetRequest{}）
	if bizReq == nil {
		return buildErrorResponse(req, codes.NotFound, fmt.Sprintf("route (r) constructor returns nil request object: %s", req.R)), nil
	}
	// 4. 解包 Any 字段到业务请求
	if req.P == 1 {
		enc := &pb.Encrypt{}
		if err := UnpackAny(req.D, enc); err != nil {
			return buildErrorResponse(req, codes.InvalidArgument, fmt.Sprintf("unpack encrypt failed: %v", err)), nil
		}
		dec, err := utils.AesGCMDecryptBaseByteResult(enc.D, key, enc.N)
		if err != nil {
			return buildErrorResponse(req, codes.InvalidArgument, fmt.Sprintf("unpack decrypt failed: %v", err)), nil
		}
		if err := proto.Unmarshal(dec, bizReq); err != nil {
			return buildErrorResponse(req, codes.InvalidArgument, fmt.Sprintf("unpack business request failed: %v", err)), nil
		}
	} else {
		if err := UnpackAny(req.D, bizReq); err != nil {
			return buildErrorResponse(req, codes.InvalidArgument, fmt.Sprintf("unpack business request failed: %v", err)), nil
		}
	}

	// 4. 调用业务处理器逻辑
	bizResp, err := handler(ctx, bizReq)
	if err != nil {
		return buildErrorResponse(req, codes.Internal, fmt.Sprintf("business handle failed: %v", err)), nil
	}

	// 5. 包装业务响应为 CommonResponse
	return self.buildSuccessResponse(cipher, key, req, bizResp)
}

// -------------------------- 验证数据 --------------------------

// validRequest 执行完整的请求验证流程，包括参数校验、双重签名验证和重放攻击防护
// req: 待验证的通用请求对象
// 返回: 验证通过的密钥对、共享密钥和可能的验证错误
func (self *CommonWorker) validRequest(req *pb.CommonRequest) (crypto.Cipher, []byte, error) {
	// 1. 校验基础参数是否有效
	if len(req.R) == 0 {
		return nil, nil, errors.New("request router is nil")
	}
	if req.D == nil {
		return nil, nil, errors.New("request data is nil")
	}
	if req.T <= 0 {
		return nil, nil, errors.New("request time must be > 0")
	}
	if utils.MathAbs(utils.UnixSecond()-req.T) > jwt.FIVE_MINUTES { // 判断绝对时间差超过5分钟
		return nil, nil, errors.New("request time invalid")
	}
	if !utils.CheckLen(req.N, 8, 32) {
		return nil, nil, errors.New("request nonce invalid")
	}
	if !utils.CheckInt64(req.P, 0, 1) {
		return nil, nil, errors.New("request plan invalid")
	}
	if !utils.CheckRangeInt(len(req.S), 32, 64) {
		return nil, nil, errors.New("request signature length invalid")
	}
	if !utils.CheckRangeInt(len(req.E), 64, 96) {
		return nil, nil, errors.New("request ecdsa signature length invalid")
	}

	// 2. 先验证ECDSA签名（双重签名验证的第一层）
	cipher, exists := self.GetCipher()[req.U]
	if !exists || cipher == nil {
		return nil, nil, errors.New("request ecdsa not found")
	}
	if err := cipher.Verify(req.S, req.E); err != nil {
		return nil, nil, errors.New("request ecdsa signature check failed")
	}

	// 3. 获取共享密钥
	c := self.GetLocalCache()
	if c == nil {
		return nil, nil, errors.New("cache object is nil")
	}
	key, err := GetSharedKey(c, cipher)
	if err != nil {
		return nil, nil, err
	}

	// 4. 验证HMAC签名（双重签名验证的第二层）
	sig, err := Signature(key, req.D.Value, req.N, req.T, req.P, req.R, req.U)
	if err != nil {
		return nil, nil, err
	}

	if !bytes.Equal(sig, req.S) {
		return nil, nil, errors.New("request body signature invalid")
	}

	// 5. 只有在签名验证通过后，才检查重放攻击
	validKey := utils.FNV1a64Base(req.S)
	exists, err = c.Exists(validKey)
	if err == nil && exists {
		return nil, nil, errors.New("request signature already used (replay attack detected)")
	}
	// 缓存签名，过期时间设置为10分钟
	_ = c.Put(validKey, true, 600)

	return cipher, key, nil
}

// -------------------------- 响应构建工具 --------------------------

// UnpackAny 将protobuf Any类型的数据解包到指定的目标消息类型
// anyData: 包含任意类型数据的Any包装器
// target: 目标消息对象，用于接收解包后的数据
// 返回: 解包过程中的错误信息
func UnpackAny(anyData *anypb.Any, target proto.Message) error {
	if anyData == nil || target == nil {
		return errors.New("any data or target is nil")
	}
	return anyData.UnmarshalTo(target)
}

// PackAny 将protobuf消息包装为Any类型，便于传输和类型擦除
// data: 需要包装的protobuf消息对象
// 返回: 包装后的Any对象和可能的错误信息
func PackAny(data proto.Message) (*anypb.Any, error) {
	return anypb.New(data)
}

// buildSuccessResponse 构建业务处理成功的标准响应，包含数据加密和签名
// cipher: 用于签名的密钥对
// key: 用于数据加密的共享密钥
// req: 原始请求对象，用于保持请求-响应一致性
// bizResp: 业务处理成功后的响应数据
// 返回: 构建完成的通用响应对象和可能的错误
func (self *CommonWorker) buildSuccessResponse(cipher crypto.Cipher, key []byte, req *pb.CommonRequest, bizResp proto.Message) (*pb.CommonResponse, error) {
	// 注意：key由外层传入，避免重复获取
	// key的清理由外层Do方法负责

	// 包装业务响应到 Any
	anyResp, err := PackAny(bizResp)
	if err != nil {
		return buildErrorResponse(req, codes.Internal, fmt.Sprintf("pack business response failed: %v", err)), nil
	}

	// 如果请求是加密的，响应也应该加密
	if req.P == 1 {
		aad := utils.GetRandomSecure(32)
		encrypted, err := utils.AesGCMEncryptBaseByteResult(anyResp.Value, key, aad)
		if err != nil {
			return buildErrorResponse(req, codes.Internal, fmt.Sprintf("encrypt response failed: %v", err)), nil
		}
		anyResp, err = PackAny(&pb.Encrypt{D: encrypted, N: aad})
		if err != nil {
			return buildErrorResponse(req, codes.Internal, fmt.Sprintf("pack encrypted response failed: %v", err)), nil
		}
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

	sig, err := Signature(key, res.D.Value, res.N, res.T, res.P, res.R, req.U)
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

// GetSharedKey 获取ECDH密钥协商生成的共享密钥，并通过缓存优化性能
// c: 缓存实例，用于存储加密后的共享密钥
// cipher: 包含ECDSA密钥对的加密器，用于密钥协商
// 返回: 协商后的共享密钥和可能的错误信息
func GetSharedKey(c cache.Cache, cipher crypto.Cipher) ([]byte, error) {
	// 获取协商密钥
	prk, _ := cipher.GetPrivateKey()
	pub, _ := cipher.GetPublicKey()
	prkBs, err := ecc.GetECDSAPrivateKeyBytes(prk.(*ecdsa.PrivateKey))
	if err != nil {
		return nil, errors.New("prk bytes invalid")
	}
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
			return nil, errors.New("cipher shared key error")
		}
		res, err := utils.AesGCMEncryptBaseByteResult(sharedKey, utils.HMAC_SHA256_BASE(utils.SHA256_BASE(prkBs), utils.GetLocalDynamicSecretKey()), pubBs)
		if err != nil {
			return nil, errors.New("encrypt shared key error")
		}
		_ = c.Put(cacheKey, res)
		return sharedKey, nil
	} else {
		res, err := utils.AesGCMDecryptBaseByteResult(sharedKey, utils.HMAC_SHA256_BASE(utils.SHA256_BASE(prkBs), utils.GetLocalDynamicSecretKey()), pubBs)
		if err != nil {
			return nil, errors.New("decrypt shared key error")
		}
		return res, nil
	}
}

// Signature 使用HMAC-SHA256算法生成请求/响应的数字签名，确保数据完整性
// key: HMAC密钥，用于签名计算
// d: 数据负载，通常是请求体的二进制数据
// n: 随机数nonce，防止重放攻击
// t: 时间戳，确保请求时效性
// p: 加密标识，0表示明文，1表示密文
// r: 路由标识符，指定业务处理逻辑
// u: 客户端ID，指定Cipher
// 返回: 计算后的签名字节数组
func Signature(key, d, n []byte, t, p int64, r string, u int64) ([]byte, error) {
	// 核心安全理论：数据体d必须Base64编码，其他字段在客户端/服务端使用过程中不存在分隔符|
	// 这样可以防止构造绕过签名的碰撞攻击
	// 格式: r|base64(d)|base64(n)|t|p|u
	signMessage := utils.AddStr(r, DIC.SEP, utils.Base64Encode(d), DIC.SEP,
		utils.Base64Encode(n), DIC.SEP, t, DIC.SEP, p, DIC.SEP, u)
	return utils.HMAC_SHA256_BASE(utils.Str2Bytes(signMessage), key), nil
}

// buildErrorResponse 构建标准化的错误响应，不包含数据和签名以提高性能
// req: 原始请求对象，用于填充响应中的路由和加密标识
// code: gRPC错误码，表示错误类型
// msg: 错误描述信息，提供详细的错误原因
// 返回: 构建完成的错误响应对象
func buildErrorResponse(req *pb.CommonRequest, code codes.Code, msg string) *pb.CommonResponse {
	// 错误响应不包含数据，也不包含签名（客户端会跳过验证）
	// 这样可以避免在错误情况下还需要计算签名，提高性能
	return &pb.CommonResponse{
		D: nil,
		N: utils.GetRandomSecure(32),
		S: nil, // 错误响应不签名，客户端会跳过验证
		T: utils.UnixSecond(),
		E: nil, // 错误响应不签名，客户端会跳过验证
		R: req.R,
		P: req.P,
		C: int64(code), // 响应代码 = gRPC 错误码
		M: msg,         // 业务消息 = 错误描述
	}
}
