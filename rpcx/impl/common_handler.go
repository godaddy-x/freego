package impl

import (
	"context"

	"google.golang.org/protobuf/proto"
)

var handlerRegistry = make(map[string]HandlerEntry)

// 1. 定义请求对象构造函数：负责返回非 nil 的业务请求实例（核心）
// 替代原来的 RequestType()，编译期保证返回非 nil
type RequestConstructor func() proto.Message

// 2. 定义路由项：绑定处理器 + 构造函数
type HandlerEntry struct {
	Handler     BusinessHandler    // 原业务处理器
	Constructor RequestConstructor // 请求对象构造函数（new object 方法）
}

// GetHandler 根据路由获取对应的业务处理器
// router: 路由标识符，用于匹配具体的业务处理逻辑
// 返回: 如果找到对应的处理器则返回，否则返回nil
// GetHandler 获取：返回处理器 + 构造函数（确保非 nil）
func GetHandler(router string) (BusinessHandler, RequestConstructor) {
	entry, ok := handlerRegistry[router]
	if !ok {
		return nil, nil
	}
	// 兜底：构造函数不能为空（编译期可保证）
	if entry.Constructor == nil {
		return entry.Handler, func() proto.Message { return nil } // 空消息兜底
	}
	return entry.Handler, entry.Constructor
}

// SetHandler 注册：同时传入处理器 + 构造函数
func SetHandler(router string, handler BusinessHandler, constructor RequestConstructor) {
	handlerRegistry[router] = HandlerEntry{
		Handler:     handler,
		Constructor: constructor,
	}
}

// GetAllHandlers 获取所有已注册的业务处理器
// 返回: 包含所有路由和对应处理器映射的map副本，防止外部修改原始数据
func GetAllHandlers() map[string]HandlerEntry {
	// 返回副本以避免外部修改
	result := make(map[string]HandlerEntry)
	for k, v := range handlerRegistry {
		result[k] = v
	}
	return result
}

// ClearAllHandlers 清空所有已注册的业务处理器
// 注意: 此方法主要用于测试场景，在生产环境中应谨慎使用
func ClearAllHandlers() {
	handlerRegistry = make(map[string]HandlerEntry)
}

// BusinessHandler 业务处理器接口，定义了处理RPC业务请求的标准协议
// 所有具体的业务处理器都必须实现此接口，以确保统一的请求处理流程
type BusinessHandler interface {
	// Handle 执行具体的业务逻辑处理
	// ctx: 上下文信息，包含请求的上下文数据，如超时控制、元数据等
	// req: 解包后的具体业务请求对象，由RequestType()方法定义的类型
	// 返回: 业务响应对象和可能的错误信息
	Handle(ctx context.Context, req proto.Message) (proto.Message, error)
}
