package impl

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/godaddy-x/freego/cache"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/utils/crypto"

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
	// 1. 校验路由字段（r 不能为空）
	if req.R == "" {
		return buildErrorResponse(req, codes.InvalidArgument, "route (r) is empty"), nil
	}

	// 2. 根据路由获取业务处理器
	handler := GetHandler(req.R)
	if handler == nil {
		return buildErrorResponse(req, codes.NotFound, fmt.Sprintf("route (r) not found: %s", req.R)), nil
	}

	// 3. 解包 Any 字段到业务请求
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
	return self.buildSuccessResponse(req, bizResp)
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
func (self *CommonWorker) buildSuccessResponse(req *pb.CommonRequest, bizResp proto.Message) (*pb.CommonResponse, error) {
	// 包装业务响应到 Any
	anyResp, err := PackAny(bizResp)
	if err != nil {
		return buildErrorResponse(req, codes.Internal, fmt.Sprintf("pack business response failed: %v", err)), nil
	}

	// 构建响应（复用请求的 r、p 字段，保持一致性）
	commonResp := &pb.CommonResponse{
		D: anyResp,
		N: utils.GetUUID(true),
		T: utils.UnixSecond(),
		R: req.R,
		P: req.P,
		C: 200, // 200 = 成功
		M: "",
	}

	return commonResp, nil
}

// buildErrorResponse 构建错误响应
func buildErrorResponse(req *pb.CommonRequest, code codes.Code, msg string) *pb.CommonResponse {
	respNonce := "uuid-" + time.Now().Format("20060102150405") + "-" + req.N[:8]
	respTimestamp := time.Now().Unix()

	// 错误响应的 s 和 e 字段可简化生成（或按规则生成，客户端按需验证）
	return &pb.CommonResponse{
		D: nil,
		N: respNonce,
		S: "",
		T: respTimestamp,
		E: "",
		R: req.R,
		P: req.P,
		C: int64(code), // 响应代码 = gRPC 错误码
		M: msg,         // 业务消息 = 错误描述
	}
}
