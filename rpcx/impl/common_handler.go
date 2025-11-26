package impl

import (
	"context"

	"google.golang.org/protobuf/proto"
)

var (
	handlerMap = map[string]BusinessHandler{} // 只在初始化注册，不能运行期添加handler
)

// GetHandler 如果没有找到则返回nil
func GetHandler(router string) BusinessHandler {
	return handlerMap[router]
}

// SetHandler 添加handler
func SetHandler(router string, handler BusinessHandler) {
	handlerMap[router] = handler
}

// GetAllHandlers 获取所有已注册的处理器
func GetAllHandlers() map[string]BusinessHandler {
	// 返回副本以避免外部修改
	result := make(map[string]BusinessHandler)
	for k, v := range handlerMap {
		result[k] = v
	}
	return result
}

// ClearAllHandlers 清空所有处理器（测试用）
func ClearAllHandlers() {
	handlerMap = make(map[string]BusinessHandler)
}

// BusinessHandler 业务处理器接口：所有业务都必须实现此接口
type BusinessHandler interface {
	// Handle 处理业务逻辑
	// req: 解包后的具体业务请求（如 *userpb.UserGetRequest）
	// 返回：具体业务响应（如 *userpb.UserGetResponse）、错误
	Handle(ctx context.Context, req proto.Message) (proto.Message, error)
	// RequestType 返回该处理器对应的业务请求类型（用于 Any 解包）
	// 例如：return &userpb.UserGetRequest{}
	RequestType() proto.Message
}
