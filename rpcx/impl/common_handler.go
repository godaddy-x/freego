package impl

import (
	"context"

	"google.golang.org/protobuf/proto"
)

var (
	handlerMap = map[string]BusinessHandler{} // 只在初始化注册，不能运行期添加handler
)

// GetHandler 根据路由获取对应的业务处理器
// router: 路由标识符，用于匹配具体的业务处理逻辑
// 返回: 如果找到对应的处理器则返回，否则返回nil
func GetHandler(router string) BusinessHandler {
	return handlerMap[router]
}

// SetHandler 注册业务处理器到指定的路由
// router: 路由标识符，用于唯一标识业务处理逻辑
// handler: 实现了BusinessHandler接口的业务处理器实例
func SetHandler(router string, handler BusinessHandler) {
	handlerMap[router] = handler
}

// GetAllHandlers 获取所有已注册的业务处理器
// 返回: 包含所有路由和对应处理器映射的map副本，防止外部修改原始数据
func GetAllHandlers() map[string]BusinessHandler {
	// 返回副本以避免外部修改
	result := make(map[string]BusinessHandler)
	for k, v := range handlerMap {
		result[k] = v
	}
	return result
}

// ClearAllHandlers 清空所有已注册的业务处理器
// 注意: 此方法主要用于测试场景，在生产环境中应谨慎使用
func ClearAllHandlers() {
	handlerMap = make(map[string]BusinessHandler)
}

// BusinessHandler 业务处理器接口，定义了处理RPC业务请求的标准协议
// 所有具体的业务处理器都必须实现此接口，以确保统一的请求处理流程
type BusinessHandler interface {
	// Handle 执行具体的业务逻辑处理
	// ctx: 上下文信息，包含请求的上下文数据，如超时控制、元数据等
	// req: 解包后的具体业务请求对象，由RequestType()方法定义的类型
	// 返回: 业务响应对象和可能的错误信息
	Handle(ctx context.Context, req proto.Message) (proto.Message, error)

	// RequestType 返回该处理器期望的业务请求类型
	// 用于protobuf Any类型的解包操作，确保类型安全
	// 例如：用户查询处理器应返回 &userpb.UserGetRequest{}
	RequestType() proto.Message
}
